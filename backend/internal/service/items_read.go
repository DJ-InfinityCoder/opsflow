package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"opsflow/backend/internal/authz"
	"opsflow/backend/internal/model"
	"opsflow/backend/internal/repo"
	"opsflow/backend/internal/statemachine"
)

type ItemListInput struct {
	View        string
	TeamID      string
	Status      string
	Priority    int16
	PrioritySet bool
	AssigneeID  string
	Query       string
	Cursor      string
	Limit       int
	LimitSet    bool
}

type ItemListResult struct {
	Items      []model.WorkItem `json:"items"`
	NextCursor string           `json:"next_cursor,omitempty"`
}

type EventPageResult struct {
	Events     []model.ItemEvent `json:"events"`
	NextCursor string            `json:"next_cursor,omitempty"`
}

func (s *ItemService) Get(ctx context.Context, user model.User, itemID string) (model.WorkItem, error) {
	if !isUUID(itemID) {
		return model.WorkItem{}, ErrNotFound
	}
	items := repo.NewItemRepository(s.pool)
	item, err := items.Get(ctx, itemID)
	if errors.Is(err, repo.ErrItemNotFound) {
		return model.WorkItem{}, ErrNotFound
	}
	if err != nil {
		return model.WorkItem{}, err
	}
	authorizer := authz.NewAuthorizer(authz.NewPostgresMembershipStore(s.pool))
	if err := authorizer.Authorize(ctx, user, authz.Action{Name: authz.ActionItemView}, authz.Item{TeamID: item.TeamID, CreatedBy: item.CreatedBy}); err != nil {
		return model.WorkItem{}, err
	}
	if err := decorateItem(ctx, authorizer, user, &item, time.Now().UTC()); err != nil {
		return model.WorkItem{}, err
	}
	if item.Status == statemachine.StatePendingApproval {
		if pending, _ := items.GetPendingApproval(ctx, item.ID); pending != nil {
			item.PendingApproval = pending
		}
	}
	return item, nil
}

func (s *ItemService) List(ctx context.Context, user model.User, input ItemListInput) (ItemListResult, error) {
	if input.View == "" {
		input.View = "all"
	}
	if !input.LimitSet {
		input.Limit = 50
	}
	if input.Limit < 1 {
		return ItemListResult{}, NewAppError(KindValidation, "limit must be positive", nil)
	}
	if input.Limit > 100 {
		input.Limit = 100
	}
	if input.PrioritySet && (input.Priority < 1 || input.Priority > 4) {
		return ItemListResult{}, NewAppError(KindValidation, "priority must be between 1 and 4", nil)
	}
	switch input.View {
	case "all", "assigned_to_me", "team_unassigned", "urgent", "waiting_approval":
	default:
		return ItemListResult{}, NewAppError(KindValidation, "view is invalid", nil)
	}
	if input.Status != "" && !validStatus(input.Status) {
		return ItemListResult{}, NewAppError(KindValidation, "status is invalid", nil)
	}
	if input.TeamID != "" && !isUUID(input.TeamID) {
		return ItemListResult{}, NewAppError(KindValidation, "team must be a UUID", nil)
	}
	if input.Query = strings.TrimSpace(input.Query); len(input.Query) > 300 {
		return ItemListResult{}, NewAppError(KindValidation, "q must be at most 300 characters", nil)
	}
	if input.AssigneeID == "me" {
		input.AssigneeID = user.ID
	} else if input.AssigneeID != "" && !isUUID(input.AssigneeID) {
		return ItemListResult{}, NewAppError(KindValidation, "assignee must be a UUID or me", nil)
	}

	var cursor *repo.ItemCursor
	if input.Cursor != "" {
		var err error
		cursor, err = repo.DecodeItemCursor(input.Cursor)
		if err != nil {
			return ItemListResult{}, NewAppError(KindValidation, "cursor is invalid", nil)
		}
	}
	authorizer := authz.NewAuthorizer(authz.NewPostgresMembershipStore(s.pool))
	teamIDs, err := authorizer.ListAccessibleTeamIDs(ctx, user)
	if err != nil {
		return ItemListResult{}, fmt.Errorf("load accessible teams for item list: %w", err)
	}
	page, err := repo.NewItemRepository(s.pool).List(ctx, repo.ItemListFilter{
		TeamIDs: teamIDs, UserID: user.ID, View: input.View, TeamID: input.TeamID,
		Status: input.Status, Priority: input.Priority, AssigneeID: input.AssigneeID,
		Query: input.Query, Cursor: cursor, Limit: input.Limit,
	})
	if err != nil {
		if strings.Contains(err.Error(), "invalid item view") {
			return ItemListResult{}, NewAppError(KindValidation, "view is invalid", nil)
		}
		return ItemListResult{}, err
	}
	result := ItemListResult{Items: page.Items}
	if page.HasMore {
		result.NextCursor = page.NextCursor
	}
	return result, nil
}

func (s *ItemService) Counts(ctx context.Context, user model.User) (repo.ViewCounts, error) {
	authorizer := authz.NewAuthorizer(authz.NewPostgresMembershipStore(s.pool))
	teamIDs, err := authorizer.ListAccessibleTeamIDs(ctx, user)
	if err != nil {
		return repo.ViewCounts{}, fmt.Errorf("load accessible teams for view counts: %w", err)
	}
	return repo.NewItemRepository(s.pool).Counts(ctx, teamIDs, user.ID)
}

func (s *ItemService) Events(ctx context.Context, user model.User, itemID, encodedCursor string, limit int) (EventPageResult, error) {
	item, err := s.Get(ctx, user, itemID)
	if err != nil {
		return EventPageResult{}, err
	}
	if limit == 0 {
		limit = 50
	}
	if limit < 1 {
		return EventPageResult{}, NewAppError(KindValidation, "limit must be positive", nil)
	}
	if limit > 100 {
		limit = 100
	}
	var beforeID int64
	if encodedCursor != "" {
		beforeID, err = repo.DecodeEventCursor(encodedCursor)
		if err != nil {
			return EventPageResult{}, NewAppError(KindValidation, "event cursor is invalid", nil)
		}
	}
	events, hasMore, err := repo.NewItemRepository(s.pool).ListEvents(ctx, item.ID, beforeID, limit)
	if err != nil {
		return EventPageResult{}, err
	}
	result := EventPageResult{Events: events}
	if hasMore && len(events) > 0 {
		result.NextCursor = repo.EncodeEventCursor(events[len(events)-1].ID)
	}
	return result, nil
}

func (s *ItemService) TeamFieldSchemas(ctx context.Context, user model.User, teamID string) ([]model.TeamFieldSchema, error) {
	return withTeamMember(s, ctx, user, teamID, func(items *repo.ItemRepository) ([]model.TeamFieldSchema, error) {
		return items.ListFieldSchemas(ctx, teamID)
	})
}

func (s *ItemService) TeamMembers(ctx context.Context, user model.User, teamID string) ([]repo.TeamMember, error) {
	return withTeamMember(s, ctx, user, teamID, func(items *repo.ItemRepository) ([]repo.TeamMember, error) {
		return items.ListTeamMembers(ctx, teamID)
	})
}

func withTeamMember[T any](s *ItemService, ctx context.Context, user model.User, teamID string, query func(*repo.ItemRepository) (T, error)) (T, error) {
	if !isUUID(teamID) {
		var zero T
		return zero, ErrNotFound
	}
	items := repo.NewItemRepository(s.pool)
	authorizer := authz.NewAuthorizer(authz.NewPostgresMembershipStore(s.pool))
	if err := authorizer.Authorize(ctx, user, authz.Action{Name: authz.ActionItemView}, authz.Item{TeamID: teamID}); err != nil {
		var zero T
		return zero, err
	}
	exists, err := items.TeamExists(ctx, teamID)
	if err != nil {
		var zero T
		return zero, err
	}
	if !exists {
		var zero T
		return zero, ErrNotFound
	}
	return query(items)
}

func decorateItem(ctx context.Context, authorizer *authz.Authorizer, user model.User, item *model.WorkItem, now time.Time) error {
	item.SLAState = "ok"
	if item.DueAt != nil {
		remaining := item.DueAt.Sub(now)
		switch {
		case remaining <= 0:
			item.SLAState = "breached"
		case remaining <= slaForPriority(item.Priority)/4:
			item.SLAState = "warning"
		}
	}
	item.AllowedStates = make([]string, 0)
	for _, target := range statemachine.Allowed(item.Status) {
		err := authorizer.Authorize(ctx, user, authz.Action{Name: authz.ActionItemTransition, TargetState: target}, authz.Item{TeamID: item.TeamID, CreatedBy: item.CreatedBy})
		if err == nil {
			item.AllowedStates = append(item.AllowedStates, target)
			continue
		}
		if errors.Is(err, ErrForbidden) {
			continue
		}
		return err
	}
	return nil
}

func validStatus(status string) bool {
	switch status {
	case "new", "triaged", "in_progress", "pending_approval", "resolved", "closed":
		return true
	default:
		return false
	}
}
