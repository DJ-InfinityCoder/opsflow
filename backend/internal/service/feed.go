package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"opsflow/backend/internal/authz"
	"opsflow/backend/internal/db"
	"opsflow/backend/internal/model"
	"opsflow/backend/internal/repo"
)

type FeedService struct {
	pool *pgxpool.Pool
}

func NewFeedService(pool *pgxpool.Pool) *FeedService {
	return &FeedService{pool: pool}
}

type FeedPage struct {
	Events     []model.FeedEvent `json:"events"`
	NextCursor string            `json:"next_cursor,omitempty"`
}

type feedCursor struct {
	CreatedAt time.Time `json:"c"`
	ID        int64     `json:"i"`
}

func (s *FeedService) GetFeed(ctx context.Context, user model.User, teamID string, rawCursor string, limit int) (FeedPage, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	var cursorTime *time.Time
	var cursorID *int64
	if rawCursor != "" {
		data, err := base64.RawURLEncoding.DecodeString(rawCursor)
		if err == nil {
			var c feedCursor
			if json.Unmarshal(data, &c) == nil && !c.CreatedAt.IsZero() && c.ID > 0 {
				cursorTime = &c.CreatedAt
				cursorID = &c.ID
			}
		}
	}

	var page FeedPage
	err := db.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		authorizer := authz.NewAuthorizer(authz.NewPostgresMembershipStore(tx))
		var teamIDs []string

		if teamID != "" {
			if !isUUID(teamID) {
				return ErrNotFound
			}
			if err := authorizer.Authorize(ctx, user, authz.Action{Name: authz.ActionItemView}, authz.Item{TeamID: teamID}); err != nil {
				return err
			}
			teamIDs = []string{teamID}
		} else {
			accessible, err := authorizer.ListAccessibleTeamIDs(ctx, user)
			if err != nil {
				return err
			}
			teamIDs = accessible
		}

		if len(teamIDs) == 0 {
			page.Events = []model.FeedEvent{}
			return nil
		}

		feedRepo := repo.NewFeedRepository(tx)
		events, err := feedRepo.ListFeed(ctx, teamIDs, cursorTime, cursorID, limit+1)
		if err != nil {
			return err
		}

		if len(events) > limit {
			last := events[limit-1]
			c := feedCursor{CreatedAt: last.CreatedAt, ID: last.ID}
			encoded, _ := json.Marshal(c)
			page.NextCursor = base64.RawURLEncoding.EncodeToString(encoded)
			events = events[:limit]
		}
		page.Events = events
		return nil
	})
	if err != nil {
		return FeedPage{}, err
	}
	if page.Events == nil {
		page.Events = []model.FeedEvent{}
	}
	return page, nil
}
