package repo

import (
	"context"
	"fmt"
	"strings"
	"time"

	"opsflow/backend/internal/model"
)

type FeedRepository struct {
	db DBTX
}

func NewFeedRepository(db DBTX) *FeedRepository {
	return &FeedRepository{db: db}
}

func (r *FeedRepository) ListFeed(ctx context.Context, teamIDs []string, cursorTime *time.Time, cursorID *int64, limit int) ([]model.FeedEvent, error) {
	if len(teamIDs) == 0 {
		return []model.FeedEvent{}, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	var sb strings.Builder
	args := []any{teamIDs}
	argIdx := 2

	sb.WriteString(`
		SELECT ie.id, ie.item_id::text, wi.title, wi.team_id::text,
		       ie.actor_id::text, coalesce(u.name, 'System'),
		       ie.type, ie.field, ie.reason, ie.created_at
		FROM item_events ie
		JOIN work_items wi ON wi.id = ie.item_id
		LEFT JOIN users u ON u.id = ie.actor_id
		WHERE wi.team_id = ANY($1::uuid[])
	`)

	if cursorTime != nil && cursorID != nil {
		sb.WriteString(fmt.Sprintf(` AND (ie.created_at, ie.id) < ($%d, $%d)`, argIdx, argIdx+1))
		args = append(args, *cursorTime, *cursorID)
		argIdx += 2
	}

	sb.WriteString(fmt.Sprintf(` ORDER BY ie.created_at DESC, ie.id DESC LIMIT $%d`, argIdx))
	args = append(args, limit)

	rows, err := r.db.Query(ctx, sb.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("list team feed: %w", err)
	}
	defer rows.Close()

	var events []model.FeedEvent
	for rows.Next() {
		var e model.FeedEvent
		if err := rows.Scan(&e.ID, &e.ItemID, &e.ItemTitle, &e.TeamID, &e.ActorID, &e.ActorName, &e.Type, &e.Field, &e.Reason, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan feed event: %w", err)
		}
		events = append(events, e)
	}
	return events, rows.Err()
}
