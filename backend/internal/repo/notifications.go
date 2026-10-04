package repo

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"opsflow/backend/internal/model"
)

type NotificationRepository struct {
	db DBTX
}

func NewNotificationRepository(db DBTX) *NotificationRepository {
	return &NotificationRepository{db: db}
}

func (r *NotificationRepository) Insert(ctx context.Context, userID, itemID, notifType string, sourceEventID *int64, dedupeKey *string) (model.Notification, bool, error) {
	var notif model.Notification
	var query string
	var args []any

	if dedupeKey != nil && *dedupeKey != "" {
		query = `
			INSERT INTO notifications (user_id, item_id, type, source_event_id, dedupe_key)
			VALUES ($1::uuid, $2::uuid, $3, $4, $5)
			ON CONFLICT (dedupe_key) DO NOTHING
			RETURNING id::text, user_id::text, item_id::text, type, source_event_id, dedupe_key, read_at, created_at
		`
		args = []any{userID, itemID, notifType, sourceEventID, *dedupeKey}
	} else if sourceEventID != nil {
		query = `
			INSERT INTO notifications (user_id, item_id, type, source_event_id)
			VALUES ($1::uuid, $2::uuid, $3, $4)
			ON CONFLICT (user_id, source_event_id) DO NOTHING
			RETURNING id::text, user_id::text, item_id::text, type, source_event_id, dedupe_key, read_at, created_at
		`
		args = []any{userID, itemID, notifType, sourceEventID}
	} else {
		query = `
			INSERT INTO notifications (user_id, item_id, type)
			VALUES ($1::uuid, $2::uuid, $3)
			RETURNING id::text, user_id::text, item_id::text, type, source_event_id, dedupe_key, read_at, created_at
		`
		args = []any{userID, itemID, notifType}
	}

	err := r.db.QueryRow(ctx, query, args...).Scan(
		&notif.ID, &notif.UserID, &notif.ItemID, &notif.Type, &notif.SourceEventID, &notif.DedupeKey, &notif.ReadAt, &notif.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Notification{}, false, nil // skipped due to ON CONFLICT DO NOTHING
	}
	if err != nil {
		return model.Notification{}, false, fmt.Errorf("insert notification: %w", err)
	}
	return notif, true, nil
}

func (r *NotificationRepository) List(ctx context.Context, userID string, unreadOnly bool, cursorTime *time.Time, cursorID *string, limit int) ([]model.Notification, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	var sb strings.Builder
	args := []any{userID}
	argIdx := 2

	sb.WriteString(`
		SELECT id::text, user_id::text, item_id::text, type, source_event_id, dedupe_key, read_at, created_at
		FROM notifications
		WHERE user_id = $1::uuid
	`)

	if unreadOnly {
		sb.WriteString(` AND read_at IS NULL`)
	}

	if cursorTime != nil && cursorID != nil {
		sb.WriteString(fmt.Sprintf(` AND (created_at, id) < ($%d, $%d::uuid)`, argIdx, argIdx+1))
		args = append(args, *cursorTime, *cursorID)
		argIdx += 2
	}

	sb.WriteString(fmt.Sprintf(` ORDER BY created_at DESC, id DESC LIMIT $%d`, argIdx))
	args = append(args, limit)

	rows, err := r.db.Query(ctx, sb.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("list notifications: %w", err)
	}
	defer rows.Close()

	var notifs []model.Notification
	for rows.Next() {
		var n model.Notification
		if err := rows.Scan(&n.ID, &n.UserID, &n.ItemID, &n.Type, &n.SourceEventID, &n.DedupeKey, &n.ReadAt, &n.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan notification: %w", err)
		}
		notifs = append(notifs, n)
	}
	return notifs, rows.Err()
}

func (r *NotificationRepository) CountUnread(ctx context.Context, userID string) (int, error) {
	var count int
	if err := r.db.QueryRow(ctx, `
		SELECT count(*)
		FROM notifications
		WHERE user_id = $1::uuid AND read_at IS NULL
	`, userID).Scan(&count); err != nil {
		return 0, fmt.Errorf("count unread notifications: %w", err)
	}
	return count, nil
}

func (r *NotificationRepository) MarkRead(ctx context.Context, userID string, ids []string) (int64, error) {
	if len(ids) == 0 {
		tag, err := r.db.Exec(ctx, `
			UPDATE notifications
			SET read_at = now()
			WHERE user_id = $1::uuid AND read_at IS NULL
		`, userID)
		if err != nil {
			return 0, fmt.Errorf("mark all notifications read: %w", err)
		}
		return tag.RowsAffected(), nil
	}

	tag, err := r.db.Exec(ctx, `
		UPDATE notifications
		SET read_at = now()
		WHERE user_id = $1::uuid AND id = ANY($2::uuid[]) AND read_at IS NULL
	`, userID, ids)
	if err != nil {
		return 0, fmt.Errorf("mark notifications read: %w", err)
	}
	return tag.RowsAffected(), nil
}
