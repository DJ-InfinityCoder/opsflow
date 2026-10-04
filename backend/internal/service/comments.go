package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"opsflow/backend/internal/authz"
	"opsflow/backend/internal/db"
	"opsflow/backend/internal/model"
	"opsflow/backend/internal/repo"
)

var mentionRegex = regexp.MustCompile(`(?:^|\s)@([a-zA-Z0-9_\-\.]+)`)

type CommentService struct {
	pool *pgxpool.Pool
}

func NewCommentService(pool *pgxpool.Pool) *CommentService {
	return &CommentService{pool: pool}
}

func (s *CommentService) Create(ctx context.Context, user model.User, itemID, body string) (model.Comment, error) {
	if !isUUID(itemID) {
		return model.Comment{}, ErrNotFound
	}
	body = strings.TrimSpace(body)
	if body == "" {
		return model.Comment{}, NewAppError(KindValidation, "comment body is required", nil)
	}
	if len(body) > 10000 {
		return model.Comment{}, NewAppError(KindValidation, "comment body must be at most 10000 characters", nil)
	}

	var created model.Comment
	err := db.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		items := repo.NewItemRepository(tx)
		comments := repo.NewCommentRepository(tx)
		authorizer := authz.NewAuthorizer(authz.NewPostgresMembershipStore(tx))

		item, err := items.Get(ctx, itemID)
		if errors.Is(err, repo.ErrItemNotFound) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}

		authzItem := authz.Item{TeamID: item.TeamID, CreatedBy: item.CreatedBy}
		if err := authorizer.Authorize(ctx, user, authz.Action{Name: authz.ActionCommentCreate}, authzItem); err != nil {
			return err
		}

		handles := ExtractMentions(body)
		mentions := make([]string, 0, len(handles))
		seen := make(map[string]bool)

		for _, handle := range handles {
			member, isMember, err := comments.FindTeamMemberByUsername(ctx, item.TeamID, handle)
			if err != nil {
				return err
			}
			if !isMember {
				exists, err := comments.UserExistsByHandle(ctx, handle)
				if err != nil {
					return err
				}
				if exists {
					return NewAppError(KindValidation, fmt.Sprintf("mentioned user @%s is not a member of this team", handle), nil)
				}
				return NewAppError(KindValidation, fmt.Sprintf("mentioned user @%s does not exist", handle), nil)
			}
			if !seen[member.ID] {
				seen[member.ID] = true
				mentions = append(mentions, member.ID)
			}
		}

		c, err := comments.Insert(ctx, item.ID, user.ID, body, mentions)
		if err != nil {
			return err
		}
		created = c

		commentJSON, err := json.Marshal(map[string]any{
			"comment_id": c.ID,
			"body":       c.Body,
			"mentions":   c.Mentions,
		})
		if err != nil {
			return fmt.Errorf("encode comment event value: %w", err)
		}

		eventID, err := items.InsertEventWithID(ctx, model.ItemEvent{
			ItemID:   item.ID,
			ActorID:  user.ID,
			Type:     "commented",
			NewValue: commentJSON,
		})
		if err != nil {
			return err
		}

		// Enqueue notify_mention for each mentioned team member
		for _, mentionedUserID := range mentions {
			mentionPayload, err := json.Marshal(map[string]any{
				"item_id":    item.ID,
				"team_id":    item.TeamID,
				"user_id":    mentionedUserID,
				"actor_id":   user.ID,
				"comment_id": c.ID,
				"event_id":   eventID,
			})
			if err != nil {
				return fmt.Errorf("encode mention outbox payload: %w", err)
			}
			dedupeKey := fmt.Sprintf("notify_mention:%d:%s", eventID, mentionedUserID)
			if err := items.InsertOutbox(ctx, "notify_mention", dedupeKey, mentionPayload); err != nil {
				return err
			}
		}

		commentPayload, err := json.Marshal(map[string]any{
			"item_id":    item.ID,
			"team_id":    item.TeamID,
			"comment_id": c.ID,
			"event_id":   eventID,
			"version":    item.Version,
		})
		if err != nil {
			return fmt.Errorf("encode comment outbox payload: %w", err)
		}
		if err := items.InsertOutbox(ctx, "item.commented", fmt.Sprintf("item.commented:%s:%s", item.ID, c.ID), commentPayload); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return model.Comment{}, err
	}
	return created, nil
}

func (s *CommentService) List(ctx context.Context, user model.User, itemID string) ([]model.Comment, error) {
	if !isUUID(itemID) {
		return nil, ErrNotFound
	}
	var comments []model.Comment
	err := db.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		items := repo.NewItemRepository(tx)
		authorizer := authz.NewAuthorizer(authz.NewPostgresMembershipStore(tx))

		item, err := items.Get(ctx, itemID)
		if errors.Is(err, repo.ErrItemNotFound) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}

		authzItem := authz.Item{TeamID: item.TeamID, CreatedBy: item.CreatedBy}
		if err := authorizer.Authorize(ctx, user, authz.Action{Name: authz.ActionItemView}, authzItem); err != nil {
			return err
		}

		commentRepo := repo.NewCommentRepository(tx)
		c, err := commentRepo.ListByItem(ctx, itemID)
		if err != nil {
			return err
		}
		comments = c
		return nil
	})
	if err != nil {
		return nil, err
	}
	return comments, nil
}

func ExtractMentions(text string) []string {
	matches := mentionRegex.FindAllStringSubmatch(text, -1)
	seen := make(map[string]bool)
	var handles []string
	for _, m := range matches {
		if len(m) > 1 {
			h := strings.TrimRight(m[1], ".,!?:;")
			h = strings.ToLower(strings.TrimSpace(h))
			if h != "" && !seen[h] {
				seen[h] = true
				handles = append(handles, h)
			}
		}
	}
	return handles
}
