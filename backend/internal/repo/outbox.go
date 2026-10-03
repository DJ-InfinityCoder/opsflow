package repo

import (
	"context"
	"fmt"
	"time"

	"opsflow/backend/internal/model"
)

type OutboxRepository struct {
	db DBTX
}

func NewOutboxRepository(db DBTX) *OutboxRepository {
	return &OutboxRepository{db: db}
}

func (r *OutboxRepository) ListJobs(ctx context.Context, status string, cursorID int64, limit int) ([]model.OutboxJob, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	query := `
		SELECT id, type, dedupe_key, payload, status, attempts, max_attempts, run_at, locked_at, last_error, created_at
		FROM outbox_jobs
		WHERE status = $1
	`
	args := []any{status}
	if cursorID > 0 {
		query += ` AND id > $2 ORDER BY id ASC LIMIT $3`
		args = append(args, cursorID, limit)
	} else {
		query += ` ORDER BY id ASC LIMIT $2`
		args = append(args, limit)
	}

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list outbox jobs: %w", err)
	}
	defer rows.Close()

	var jobs []model.OutboxJob
	for rows.Next() {
		var j model.OutboxJob
		if err := rows.Scan(&j.ID, &j.Type, &j.DedupeKey, &j.Payload, &j.Status, &j.Attempts, &j.MaxAttempts, &j.RunAt, &j.LockedAt, &j.LastError, &j.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan outbox job: %w", err)
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

func (r *OutboxRepository) ReclaimStaleJobs(ctx context.Context, olderThan time.Duration) (int64, error) {
	seconds := int(olderThan.Seconds())
	if seconds <= 0 {
		seconds = 300 // 5 minutes
	}
	tag, err := r.db.Exec(ctx, `
		UPDATE outbox_jobs
		SET status = 'pending', locked_at = NULL
		WHERE status = 'processing' AND locked_at < now() - ($1 || ' seconds')::interval
	`, fmt.Sprintf("%d", seconds))
	if err != nil {
		return 0, fmt.Errorf("reclaim stale jobs: %w", err)
	}
	return tag.RowsAffected(), nil
}
