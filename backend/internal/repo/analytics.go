package repo

import (
	"context"
	"fmt"

	"opsflow/backend/internal/model"
)

type AnalyticsRepository struct {
	db DBTX
}

func NewAnalyticsRepository(db DBTX) *AnalyticsRepository {
	return &AnalyticsRepository{db: db}
}

func (r *AnalyticsRepository) GetTeamSummary(ctx context.Context, teamID string) (model.AnalyticsSummary, error) {
	summary := model.AnalyticsSummary{
		TeamID:       teamID,
		ByStatus:     map[string]int{"new": 0, "triaged": 0, "in_progress": 0, "pending_approval": 0, "resolved": 0, "closed": 0},
		ByPriority:   map[int]int{1: 0, 2: 0, 3: 0, 4: 0},
		AgingBuckets: map[string]int{"under_1_day": 0, "1_to_3_days": 0, "3_to_7_days": 0, "over_7_days": 0},
	}

	// 1. Total and status counts
	rows, err := r.db.Query(ctx, `
		SELECT status, count(*)
		FROM work_items
		WHERE team_id = $1::uuid
		GROUP BY status
	`, teamID)
	if err != nil {
		return model.AnalyticsSummary{}, fmt.Errorf("count items by status: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return model.AnalyticsSummary{}, fmt.Errorf("scan status count: %w", err)
		}
		summary.ByStatus[status] = count
		summary.TotalItems += count
		if status != "resolved" && status != "closed" {
			summary.OpenItems += count
		}
	}
	if err := rows.Err(); err != nil {
		return model.AnalyticsSummary{}, err
	}

	// 2. Priority counts for open items
	pRows, err := r.db.Query(ctx, `
		SELECT priority, count(*)
		FROM work_items
		WHERE team_id = $1::uuid AND status NOT IN ('resolved', 'closed')
		GROUP BY priority
	`, teamID)
	if err != nil {
		return model.AnalyticsSummary{}, fmt.Errorf("count items by priority: %w", err)
	}
	defer pRows.Close()

	for pRows.Next() {
		var priority int
		var count int
		if err := pRows.Scan(&priority, &count); err != nil {
			return model.AnalyticsSummary{}, fmt.Errorf("scan priority count: %w", err)
		}
		summary.ByPriority[priority] = count
	}
	if err := pRows.Err(); err != nil {
		return model.AnalyticsSummary{}, err
	}

	// 3. Age buckets for open work items
	var underOneDay, oneToThreeDays, threeToSevenDays, overSevenDays int
	err = r.db.QueryRow(ctx, `
		SELECT
			count(*) FILTER (WHERE created_at >= now() - interval '1 day'),
			count(*) FILTER (WHERE created_at < now() - interval '1 day' AND created_at >= now() - interval '3 days'),
			count(*) FILTER (WHERE created_at < now() - interval '3 days' AND created_at >= now() - interval '7 days'),
			count(*) FILTER (WHERE created_at < now() - interval '7 days')
		FROM work_items
		WHERE team_id = $1::uuid AND status NOT IN ('resolved', 'closed')
	`, teamID).Scan(
		&underOneDay,
		&oneToThreeDays,
		&threeToSevenDays,
		&overSevenDays,
	)
	if err != nil {
		return model.AnalyticsSummary{}, fmt.Errorf("count open items by age: %w", err)
	}
	summary.AgingBuckets["under_1_day"] = underOneDay
	summary.AgingBuckets["1_to_3_days"] = oneToThreeDays
	summary.AgingBuckets["3_to_7_days"] = threeToSevenDays
	summary.AgingBuckets["over_7_days"] = overSevenDays

	// 4. SLA breached and warning counts
	err = r.db.QueryRow(ctx, `
		SELECT
			count(*) FILTER (WHERE due_at < now()),
			count(*) FILTER (WHERE due_at >= now() AND due_at <= now() + interval '4 hours')
		FROM work_items
		WHERE team_id = $1::uuid AND status NOT IN ('resolved', 'closed') AND due_at IS NOT NULL
	`, teamID).Scan(&summary.SLABreachedCount, &summary.SLAWarningCount)
	if err != nil {
		return model.AnalyticsSummary{}, fmt.Errorf("count SLA breached/warning: %w", err)
	}

	// 5. Mean Time to Resolution (MTTR) in seconds
	var avgSeconds *float64
	err = r.db.QueryRow(ctx, `
		SELECT EXTRACT(EPOCH FROM avg(resolved_at - created_at))
		FROM work_items
		WHERE team_id = $1::uuid AND resolved_at IS NOT NULL
	`, teamID).Scan(&avgSeconds)
	if err != nil {
		return model.AnalyticsSummary{}, fmt.Errorf("calc MTTR: %w", err)
	}
	if avgSeconds != nil {
		summary.MTTRSeconds = *avgSeconds
	}

	return summary, nil
}

func (r *AnalyticsRepository) ListTeamSLABreaches(ctx context.Context, teamIDs []string) ([]model.AnalyticsTeamMetric, error) {
	if len(teamIDs) == 0 {
		return []model.AnalyticsTeamMetric{}, nil
	}

	rows, err := r.db.Query(ctx, `
		SELECT t.id::text, t.name,
			count(w.id) FILTER (
				WHERE w.due_at < now() AND w.status NOT IN ('resolved', 'closed')
			)::int
		FROM teams t
		LEFT JOIN work_items w ON w.team_id = t.id AND w.due_at IS NOT NULL
		WHERE t.id::text = ANY($1::text[])
		GROUP BY t.id, t.name
		ORDER BY t.name
	`, teamIDs)
	if err != nil {
		return nil, fmt.Errorf("list breached SLA counts by team: %w", err)
	}
	defer rows.Close()

	teams := make([]model.AnalyticsTeamMetric, 0, len(teamIDs))
	for rows.Next() {
		var team model.AnalyticsTeamMetric
		if err := rows.Scan(&team.TeamID, &team.TeamName, &team.SLABreachedCount); err != nil {
			return nil, fmt.Errorf("scan team SLA breach count: %w", err)
		}
		teams = append(teams, team)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate team SLA breach counts: %w", err)
	}
	return teams, nil
}
