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
		TeamID:     teamID,
		ByStatus:   map[string]int{"new": 0, "triaged": 0, "in_progress": 0, "pending_approval": 0, "resolved": 0, "closed": 0},
		ByPriority: map[int]int{1: 0, 2: 0, 3: 0, 4: 0},
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

	// 3. SLA breached and warning counts
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

	// 4. Mean Time to Resolution (MTTR) in seconds
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
