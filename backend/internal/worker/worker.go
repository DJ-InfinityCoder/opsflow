package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"opsflow/backend/internal/db"
	"opsflow/backend/internal/repo"
)

const (
	JobTypeIdempotencyCleanup = "idempotency_cleanup"
	JobTypeNotifyAssignment   = "notify_assignment"
	JobTypeNotifyMention      = "notify_mention"
	JobTypeNotifyP1Created    = "notify_p1_created"
	JobTypeSLAScan            = "sla_scan"
)

type Job struct {
	ID          int64
	Type        string
	DedupeKey   string
	Payload     []byte
	Attempts    int
	MaxAttempts int
}

func Run(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger) error {
	if err := pool.Ping(ctx); err != nil {
		return err
	}
	logger.InfoContext(ctx, "worker process started")

	// Tickers
	slaTicker := time.NewTicker(1 * time.Minute)
	defer slaTicker.Stop()

	reclaimTicker := time.NewTicker(1 * time.Minute)
	defer reclaimTicker.Stop()

	// Enqueue initial SLA scan immediately
	_ = EnqueueSLAScan(ctx, pool)

	for {
		select {
		case <-ctx.Done():
			logger.InfoContext(ctx, "worker shutting down")
			return nil
		case <-slaTicker.C:
			if err := EnqueueSLAScan(ctx, pool); err != nil {
				logger.ErrorContext(ctx, "failed to enqueue sla scan", "error", err)
			}
		case <-reclaimTicker.C:
			outboxRepo := repo.NewOutboxRepository(pool)
			if reclaimed, err := outboxRepo.ReclaimStaleJobs(ctx, 5*time.Minute); err != nil {
				logger.ErrorContext(ctx, "failed to reclaim stale jobs", "error", err)
			} else if reclaimed > 0 {
				logger.InfoContext(ctx, "reclaimed stale outbox jobs", "count", reclaimed)
			}
		default:
			processed, err := ProcessNextJob(ctx, pool, logger)
			if err != nil {
				logger.ErrorContext(ctx, "error processing outbox job", "error", err)
			}
			if !processed {
				// No jobs ready; wait short interval + jitter without holding idle connections
				jitterMs := rand.Intn(200)
				sleepDuration := time.Duration(300+jitterMs) * time.Millisecond
				select {
				case <-ctx.Done():
					return nil
				case <-time.After(sleepDuration):
				}
			}
		}
	}
}

// ProcessNextJob claims and executes the next pending outbox job using FOR UPDATE SKIP LOCKED.
func ProcessNextJob(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger) (bool, error) {
	var job Job
	var found bool

	err := db.WithTx(ctx, pool, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
			SELECT id, type, dedupe_key, payload, attempts, max_attempts
			FROM outbox_jobs
			WHERE status = 'pending' AND run_at <= now()
			ORDER BY run_at ASC, id ASC
			LIMIT 1
			FOR UPDATE SKIP LOCKED
		`)
		if err := row.Scan(&job.ID, &job.Type, &job.DedupeKey, &job.Payload, &job.Attempts, &job.MaxAttempts); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil
			}
			return err
		}
		found = true

		_, err := tx.Exec(ctx, `
			UPDATE outbox_jobs
			SET status = 'processing', locked_at = now(), attempts = attempts + 1
			WHERE id = $1
		`, job.ID)
		if err == nil {
			job.Attempts++
		}
		return err
	})
	if err != nil || !found {
		return found, err
	}

	processErr := ExecuteJob(ctx, pool, job, logger)

	return true, db.WithTx(ctx, pool, func(tx pgx.Tx) error {
		if processErr == nil {
			_, err := tx.Exec(ctx, `
				UPDATE outbox_jobs
				SET status = 'completed', locked_at = NULL
				WHERE id = $1
			`, job.ID)
			return err
		}

		if job.Attempts >= job.MaxAttempts {
			_, err := tx.Exec(ctx, `
				UPDATE outbox_jobs
				SET status = 'failed', last_error = $2, locked_at = NULL
				WHERE id = $1
			`, job.ID, processErr.Error())
			return err
		}

		baseBackoff := 1 << job.Attempts
		jitter := rand.Intn(baseBackoff/2 + 1)
		backoffSeconds := baseBackoff + jitter

		_, err := tx.Exec(ctx, `
			UPDATE outbox_jobs
			SET status = 'pending',
			    last_error = $2,
			    locked_at = NULL,
			    run_at = now() + ($3 || ' seconds')::interval
			WHERE id = $1
		`, job.ID, processErr.Error(), fmt.Sprintf("%d", backoffSeconds))
		return err
	})
}

func ExecuteJob(ctx context.Context, pool *pgxpool.Pool, job Job, logger *slog.Logger) error {
	switch job.Type {
	case JobTypeIdempotencyCleanup:
		deleted, err := repo.DeleteExpiredIdempotencyKeys(ctx, pool, 24*time.Hour)
		if err != nil {
			return fmt.Errorf("cleanup idempotency keys: %w", err)
		}
		logger.InfoContext(ctx, "idempotency cleanup completed", "deleted_keys", deleted)
		return nil

	case JobTypeNotifyAssignment:
		var payload struct {
			ItemID     string `json:"item_id"`
			TeamID     string `json:"team_id"`
			AssigneeID string `json:"assignee_id"`
			ActorID    string `json:"actor_id"`
			EventID    int64  `json:"event_id"`
		}
		if err := json.Unmarshal(job.Payload, &payload); err != nil {
			return fmt.Errorf("unmarshal notify_assignment payload: %w", err)
		}
		if payload.AssigneeID == "" || payload.ItemID == "" {
			return nil
		}
		notifRepo := repo.NewNotificationRepository(pool)
		_, _, err := notifRepo.Insert(ctx, payload.AssigneeID, payload.ItemID, "assignment", &payload.EventID, nil)
		if err != nil {
			return fmt.Errorf("insert assignment notification: %w", err)
		}
		logger.InfoContext(ctx, "assignment notification delivered", "assignee_id", payload.AssigneeID, "item_id", payload.ItemID)
		return nil

	case JobTypeNotifyMention:
		var payload struct {
			ItemID    string `json:"item_id"`
			TeamID    string `json:"team_id"`
			UserID    string `json:"user_id"`
			ActorID   string `json:"actor_id"`
			CommentID string `json:"comment_id"`
			EventID   int64  `json:"event_id"`
		}
		if err := json.Unmarshal(job.Payload, &payload); err != nil {
			return fmt.Errorf("unmarshal notify_mention payload: %w", err)
		}
		if payload.UserID == "" || payload.ItemID == "" {
			return nil
		}
		notifRepo := repo.NewNotificationRepository(pool)
		_, _, err := notifRepo.Insert(ctx, payload.UserID, payload.ItemID, "mention", &payload.EventID, nil)
		if err != nil {
			return fmt.Errorf("insert mention notification: %w", err)
		}
		logger.InfoContext(ctx, "mention notification delivered", "user_id", payload.UserID, "item_id", payload.ItemID)
		return nil

	case JobTypeNotifyP1Created:
		var payload struct {
			ItemID   string `json:"item_id"`
			TeamID   string `json:"team_id"`
			Title    string `json:"title"`
			Priority int16  `json:"priority"`
		}
		if err := json.Unmarshal(job.Payload, &payload); err != nil {
			return fmt.Errorf("unmarshal notify_p1_created payload: %w", err)
		}
		// Stub webhook call logged
		logger.InfoContext(ctx, "P1 webhook sent", "item_id", payload.ItemID, "title", payload.Title, "priority", payload.Priority)

		// Notify team leads
		rows, err := pool.Query(ctx, `SELECT user_id::text FROM team_members WHERE team_id = $1::uuid AND role = 'lead'`, payload.TeamID)
		if err != nil {
			return fmt.Errorf("query team leads for p1 alert: %w", err)
		}
		defer rows.Close()

		var leadIDs []string
		for rows.Next() {
			var leadID string
			if err := rows.Scan(&leadID); err != nil {
				return err
			}
			leadIDs = append(leadIDs, leadID)
		}
		rows.Close()

		notifRepo := repo.NewNotificationRepository(pool)
		for _, leadID := range leadIDs {
			dedupeKey := fmt.Sprintf("p1:%s:%s", payload.ItemID, leadID)
			_, _, _ = notifRepo.Insert(ctx, leadID, payload.ItemID, "p1_alert", nil, &dedupeKey)
		}
		return nil

	case JobTypeSLAScan:
		return ExecuteSLAScan(ctx, pool, logger)

	default:
		logger.InfoContext(ctx, "processed generic outbox job", "type", job.Type, "id", job.ID)
		return nil
	}
}

func ExecuteSLAScan(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger) error {
	now := time.Now().UTC()
	rows, err := pool.Query(ctx, `
		SELECT id::text, team_id::text, title, status, priority, assignee_id::text, due_at
		FROM work_items
		WHERE status NOT IN ('resolved', 'closed') AND due_at IS NOT NULL
	`)
	if err != nil {
		return fmt.Errorf("scan open items for sla: %w", err)
	}
	defer rows.Close()

	type slaItem struct {
		ID         string
		TeamID     string
		Title      string
		Status     string
		Priority   int16
		AssigneeID *string
		DueAt      time.Time
	}

	var items []slaItem
	for rows.Next() {
		var it slaItem
		if err := rows.Scan(&it.ID, &it.TeamID, &it.Title, &it.Status, &it.Priority, &it.AssigneeID, &it.DueAt); err != nil {
			return fmt.Errorf("scan sla item: %w", err)
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	notifRepo := repo.NewNotificationRepository(pool)

	for _, it := range items {
		var level string
		if now.After(it.DueAt) {
			level = "breached"
		} else if it.DueAt.Sub(now) <= 15*time.Minute {
			level = "warning"
		} else {
			continue
		}

		dedupeKey := fmt.Sprintf("sla:%s:%s", it.ID, level)
		notifType := fmt.Sprintf("sla_%s", level)

		// Determine recipients: assignee if present, otherwise team leads
		var recipients []string
		if it.AssigneeID != nil && *it.AssigneeID != "" {
			recipients = []string{*it.AssigneeID}
		} else {
			leadRows, err := pool.Query(ctx, `SELECT user_id::text FROM team_members WHERE team_id = $1::uuid AND role = 'lead'`, it.TeamID)
			if err == nil {
				for leadRows.Next() {
					var leadID string
					if err := leadRows.Scan(&leadID); err == nil {
						recipients = append(recipients, leadID)
					}
				}
				leadRows.Close()
			}
		}

		for _, userID := range recipients {
			userDedupe := fmt.Sprintf("%s:%s", dedupeKey, userID)
			_, _, _ = notifRepo.Insert(ctx, userID, it.ID, notifType, nil, &userDedupe)
		}
	}

	logger.InfoContext(ctx, "sla scan completed", "scanned_items", len(items))
	return nil
}

// EnqueueSLAScan enqueues an sla_scan outbox job with dedupe by minute.
func EnqueueSLAScan(ctx context.Context, db repo.DBTX) error {
	dedupeKey := fmt.Sprintf("sla_scan:%s", time.Now().UTC().Format("2006-01-02T15:04"))
	_, err := db.Exec(ctx, `
		INSERT INTO outbox_jobs (type, dedupe_key, payload, run_at)
		VALUES ('sla_scan', $1, '{}'::jsonb, now())
		ON CONFLICT (dedupe_key) DO NOTHING
	`, dedupeKey)
	return err
}

// EnqueueIdempotencyCleanup inserts an outbox job of type idempotency_cleanup.
func EnqueueIdempotencyCleanup(ctx context.Context, db repo.DBTX) error {
	dedupeKey := fmt.Sprintf("idempotency_cleanup:%s", time.Now().UTC().Format("2006-01-02-15"))
	_, err := db.Exec(ctx, `
		INSERT INTO outbox_jobs (type, dedupe_key, payload, run_at)
		VALUES ('idempotency_cleanup', $1, '{}'::jsonb, now())
		ON CONFLICT (dedupe_key) DO NOTHING
	`, dedupeKey)
	return err
}
