package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"opsflow/backend/internal/db"
	"opsflow/backend/internal/model"
	"opsflow/backend/internal/repo"
)

type NotificationService struct {
	pool *pgxpool.Pool
}

func NewNotificationService(pool *pgxpool.Pool) *NotificationService {
	return &NotificationService{pool: pool}
}

type NotificationPage struct {
	Notifications []model.Notification `json:"notifications"`
	NextCursor    string               `json:"next_cursor,omitempty"`
}

type notifCursor struct {
	CreatedAt time.Time `json:"c"`
	ID        string    `json:"i"`
}

func (s *NotificationService) List(ctx context.Context, user model.User, unreadOnly bool, rawCursor string, limit int) (NotificationPage, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	var cursorTime *time.Time
	var cursorID *string
	if rawCursor != "" {
		data, err := base64.RawURLEncoding.DecodeString(rawCursor)
		if err == nil {
			var c notifCursor
			if json.Unmarshal(data, &c) == nil && !c.CreatedAt.IsZero() && c.ID != "" {
				cursorTime = &c.CreatedAt
				cursorID = &c.ID
			}
		}
	}

	var page NotificationPage
	err := db.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		r := repo.NewNotificationRepository(tx)
		// Fetch limit + 1 to determine if next page exists
		items, err := r.List(ctx, user.ID, unreadOnly, cursorTime, cursorID, limit+1)
		if err != nil {
			return err
		}

		if len(items) > limit {
			last := items[limit-1]
			c := notifCursor{CreatedAt: last.CreatedAt, ID: last.ID}
			encoded, _ := json.Marshal(c)
			page.NextCursor = base64.RawURLEncoding.EncodeToString(encoded)
			items = items[:limit]
		}
		page.Notifications = items
		return nil
	})
	if err != nil {
		return NotificationPage{}, err
	}
	if page.Notifications == nil {
		page.Notifications = []model.Notification{}
	}
	return page, nil
}

func (s *NotificationService) MarkRead(ctx context.Context, user model.User, ids []string) (int64, error) {
	for _, id := range ids {
		if !isUUID(id) {
			return 0, NewAppError(KindValidation, "notification id must be a UUID", nil)
		}
	}

	var updated int64
	err := db.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		r := repo.NewNotificationRepository(tx)
		count, err := r.MarkRead(ctx, user.ID, ids)
		if err != nil {
			return err
		}
		updated = count
		return nil
	})
	if err != nil {
		return 0, err
	}
	return updated, nil
}
