package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"opsflow/backend/internal/authz"
	"opsflow/backend/internal/db"
	"opsflow/backend/internal/model"
	"opsflow/backend/internal/repo"
	"opsflow/backend/internal/statemachine"
)

type ItemService struct {
	pool *pgxpool.Pool
}

type CreateItemInput struct {
	TeamID       string                     `json:"team_id"`
	Title        string                     `json:"title"`
	Description  string                     `json:"description"`
	Priority     int16                      `json:"priority"`
	CustomFields map[string]json.RawMessage `json:"custom_fields"`
}

type MutationResponse struct {
	StatusCode int
	Body       json.RawMessage
	Replayed   bool
}

func NewItemService(pool *pgxpool.Pool) *ItemService {
	return &ItemService{pool: pool}
}

func (s *ItemService) Create(ctx context.Context, user model.User, input CreateItemInput, idempotencyKey, requestHash string) (MutationResponse, error) {
	input.TeamID = strings.TrimSpace(input.TeamID)
	input.Title = strings.TrimSpace(input.Title)
	if !isUUID(input.TeamID) {
		return MutationResponse{}, NewAppError(KindValidation, "team_id must be a UUID", nil)
	}
	if input.Title == "" || len(input.Title) > 500 {
		return MutationResponse{}, NewAppError(KindValidation, "title is required and must be at most 500 characters", nil)
	}
	if len(input.Description) > 30000 {
		return MutationResponse{}, NewAppError(KindValidation, "description must be at most 30000 characters", nil)
	}
	if slaForPriority(input.Priority) == 0 {
		return MutationResponse{}, NewAppError(KindValidation, "priority must be between 1 and 4", nil)
	}
	if err := validateIdempotencyKey(idempotencyKey, requestHash); err != nil {
		return MutationResponse{}, err
	}

	var resp MutationResponse
	err := db.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		if replayed, err := s.checkIdempotency(ctx, tx, user.ID, idempotencyKey, "POST:/items", requestHash); err != nil {
			return err
		} else if replayed != nil {
			resp = *replayed
			return nil
		}

		items := repo.NewItemRepository(tx)
		authorizer := authz.NewAuthorizer(authz.NewPostgresMembershipStore(tx))
		if err := authorizer.Authorize(ctx, user, authz.Action{Name: authz.ActionItemCreate}, authz.Item{TeamID: input.TeamID}); err != nil {
			return err
		}
		teamExists, err := items.TeamExists(ctx, input.TeamID)
		if err != nil {
			return err
		}
		if !teamExists {
			return ErrNotFound
		}

		schemas, err := items.ListFieldSchemas(ctx, input.TeamID)
		if err != nil {
			return err
		}
		customFields, err := validateCustomFields(schemas, input.CustomFields)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		item, err := items.Insert(ctx, model.WorkItem{
			TeamID:       input.TeamID,
			Title:        input.Title,
			Description:  input.Description,
			Status:       "new",
			Priority:     input.Priority,
			CreatedBy:    user.ID,
			CustomFields: customFields,
			DueAt:        timePointer(now.Add(slaForPriority(input.Priority))),
		})
		if err != nil {
			return err
		}
		if err := items.InsertEvent(ctx, model.ItemEvent{ItemID: item.ID, ActorID: user.ID, Type: "created"}); err != nil {
			return err
		}
		if err := insertItemOutbox(ctx, items, "item.created", item); err != nil {
			return err
		}
		if item.Priority == 1 {
			p1Payload, _ := json.Marshal(map[string]any{
				"item_id":  item.ID,
				"team_id":  item.TeamID,
				"title":    item.Title,
				"priority": item.Priority,
			})
			if err := items.InsertOutbox(ctx, "notify_p1_created", fmt.Sprintf("notify_p1_created:%s", item.ID), p1Payload); err != nil {
				return err
			}
		}
		body, err := json.Marshal(item)
		if err != nil {
			return fmt.Errorf("encode created item: %w", err)
		}
		if err := s.storeIdempotency(ctx, tx, user.ID, idempotencyKey, 201, body); err != nil {
			return err
		}
		resp = MutationResponse{StatusCode: 201, Body: body}
		return nil
	})
	if err != nil {
		return MutationResponse{}, err
	}
	return resp, nil
}

func (s *ItemService) Patch(ctx context.Context, user model.User, itemID string, expectedVersion int, patch map[string]json.RawMessage, requestHash string) (MutationResponse, error) {
	if !isUUID(itemID) {
		return MutationResponse{}, ErrNotFound
	}
	if expectedVersion < 1 {
		return MutationResponse{}, NewAppError(KindValidation, "If-Match must contain a positive item version", nil)
	}
	if len(patch) == 0 {
		return MutationResponse{}, NewAppError(KindValidation, "PATCH body must contain at least one field", nil)
	}
	if requestHash == "" {
		return MutationResponse{}, NewAppError(KindValidation, "request body hash is required", nil)
	}
	for field := range patch {
		switch field {
		case "title", "description", "priority", "status", "assignee_id", "custom_fields":
		default:
			return MutationResponse{}, NewAppError(KindValidation, "unsupported patch field", map[string]any{"field": field})
		}
	}
	claimAttempt := false
	if rawAssignee, exists := patch["assignee_id"]; exists && string(rawAssignee) != "null" {
		var assigneeID string
		if err := json.Unmarshal(rawAssignee, &assigneeID); err != nil || !isUUID(assigneeID) {
			return MutationResponse{}, NewAppError(KindValidation, "assignee_id must be a UUID or null", nil)
		}
		claimAttempt = strings.EqualFold(assigneeID, user.ID)
	}

	var resp MutationResponse
	err := db.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		items := repo.NewItemRepository(tx)
		current, err := items.Get(ctx, itemID)
		if errors.Is(err, repo.ErrItemNotFound) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		authorizer := authz.NewAuthorizer(authz.NewPostgresMembershipStore(tx))
		authzItem := authz.Item{TeamID: current.TeamID, CreatedBy: current.CreatedBy}
		if err := authorizer.Authorize(ctx, user, authz.Action{Name: authz.ActionItemEdit}, authzItem); err != nil {
			return err
		}
		if _, assigning := patch["assignee_id"]; assigning {
			action := authz.Action{Name: authz.ActionItemAssign}
			if claimAttempt {
				action.Name = authz.ActionItemClaim
			}
			if err := authorizer.Authorize(ctx, user, action, authzItem); err != nil {
				return err
			}
		}
		if rawStatus, transitioning := patch["status"]; transitioning {
			var target string
			if err := json.Unmarshal(rawStatus, &target); err != nil || strings.TrimSpace(target) == "" {
				return NewAppError(KindValidation, "status must be a valid target state", nil)
			}
			if err := authorizer.Authorize(ctx, user, authz.Action{Name: authz.ActionItemTransition, TargetState: target}, authzItem); err != nil {
				return err
			}
		}

		current, err = items.GetForUpdate(ctx, itemID)
		if errors.Is(err, repo.ErrItemNotFound) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if claimAttempt && current.AssigneeID != nil && !strings.EqualFold(*current.AssigneeID, user.ID) {
			return alreadyClaimed(current.AssigneeID)
		}
		if current.Version != expectedVersion {
			if err := decorateItem(ctx, authorizer, user, &current, time.Now()); err != nil {
				return err
			}
			return NewAppError(KindVersionConflict, "work item version does not match If-Match", map[string]any{"current_item": current})
		}

		idempotencyKey := "patch:" + itemID + ":" + strconv.Itoa(expectedVersion)
		if replayed, err := s.checkIdempotency(ctx, tx, user.ID, idempotencyKey, "PATCH:/items/"+itemID, requestHash); err != nil {
			return err
		} else if replayed != nil {
			resp = *replayed
			return nil
		}

		updated, changes, err := applyPatch(ctx, items, authorizer, current, patch)
		if err != nil {
			return err
		}
		if len(changes) == 0 {
			return NewAppError(KindValidation, "PATCH contains no changed fields", nil)
		}
		updated, err = items.Update(ctx, updated, expectedVersion, claimAttempt)
		if errors.Is(err, repo.ErrItemNotFound) {
			latest, readErr := items.Get(ctx, itemID)
			if readErr != nil {
				return ErrNotFound
			}
			if claimAttempt && latest.AssigneeID != nil && !strings.EqualFold(*latest.AssigneeID, user.ID) {
				return alreadyClaimed(latest.AssigneeID)
			}
			if decorateErr := decorateItem(ctx, authorizer, user, &latest, time.Now()); decorateErr != nil {
				return decorateErr
			}
			return NewAppError(KindVersionConflict, "work item version does not match If-Match", map[string]any{"current_item": latest})
		}
		if err != nil {
			return err
		}
		for _, change := range changes {
			field := change.field
			oldJSON, marshalErr := json.Marshal(change.oldValue)
			if marshalErr != nil {
				return fmt.Errorf("encode old %s event value: %w", field, marshalErr)
			}
			newJSON, marshalErr := json.Marshal(change.newValue)
			if marshalErr != nil {
				return fmt.Errorf("encode new %s event value: %w", field, marshalErr)
			}
			if err := items.InsertEvent(ctx, model.ItemEvent{
				ItemID: updated.ID, ActorID: user.ID, Type: "field_changed", Field: &field,
				OldValue: oldJSON, NewValue: newJSON,
			}); err != nil {
				return err
			}
		}
		if err := insertItemOutbox(ctx, items, "item.updated", updated); err != nil {
			return err
		}
		body, err := json.Marshal(updated)
		if err != nil {
			return fmt.Errorf("encode patched item: %w", err)
		}
		if err := s.storeIdempotency(ctx, tx, user.ID, idempotencyKey, 200, body); err != nil {
			return err
		}
		resp = MutationResponse{StatusCode: 200, Body: body}
		return nil
	})
	if err != nil {
		return MutationResponse{}, err
	}
	return resp, nil
}

func (s *ItemService) Claim(ctx context.Context, user model.User, itemID, idempotencyKey, requestHash string) (MutationResponse, error) {
	if !isUUID(itemID) {
		return MutationResponse{}, ErrNotFound
	}
	var resp MutationResponse
	err := db.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		if replayed, err := s.checkIdempotency(ctx, tx, user.ID, idempotencyKey, "POST:/items/"+itemID+"/claim", requestHash); err != nil {
			return err
		} else if replayed != nil {
			resp = *replayed
			return nil
		}

		items := repo.NewItemRepository(tx)
		authorizer := authz.NewAuthorizer(authz.NewPostgresMembershipStore(tx))

		current, err := items.Get(ctx, itemID)
		if errors.Is(err, repo.ErrItemNotFound) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}

		authzItem := authz.Item{TeamID: current.TeamID, CreatedBy: current.CreatedBy}
		if err := authorizer.Authorize(ctx, user, authz.Action{Name: authz.ActionItemClaim}, authzItem); err != nil {
			return err
		}

		updated, claimed, err := items.Claim(ctx, itemID, user.ID)
		if err != nil {
			return err
		}
		if !claimed {
			latest, readErr := items.Get(ctx, itemID)
			if readErr != nil {
				return ErrNotFound
			}
			if latest.AssigneeID != nil {
				return alreadyClaimed(latest.AssigneeID, &latest)
			}
			return NewAppError(KindConflict, fmt.Sprintf("work item cannot be claimed in status %q", latest.Status), nil)
		}

		eventID, err := items.InsertEventWithID(ctx, model.ItemEvent{
			ItemID:   updated.ID,
			ActorID:  user.ID,
			Type:     "claimed",
			Field:    stringPointer("assignee_id"),
			NewValue: json.RawMessage(`"` + user.ID + `"`),
		})
		if err != nil {
			return err
		}

		if err := insertItemOutbox(ctx, items, "item.claimed", updated); err != nil {
			return err
		}

		assignPayload, _ := json.Marshal(map[string]any{
			"item_id":     updated.ID,
			"team_id":     updated.TeamID,
			"assignee_id": user.ID,
			"actor_id":    user.ID,
			"event_id":    eventID,
		})
		if err := items.InsertOutbox(ctx, "notify_assignment", fmt.Sprintf("notify_assignment:%d", eventID), assignPayload); err != nil {
			return err
		}

		body, err := json.Marshal(updated)
		if err != nil {
			return fmt.Errorf("encode claimed item: %w", err)
		}

		if err := s.storeIdempotency(ctx, tx, user.ID, idempotencyKey, 200, body); err != nil {
			return err
		}

		resp = MutationResponse{StatusCode: 200, Body: body}
		return nil
	})
	if err != nil {
		return MutationResponse{}, err
	}
	return resp, nil
}

func (s *ItemService) Assign(ctx context.Context, user model.User, itemID string, expectedVersion int, assigneeID, reason, idempotencyKey, requestHash string) (MutationResponse, error) {
	if !isUUID(itemID) {
		return MutationResponse{}, ErrNotFound
	}
	assigneeID = strings.TrimSpace(assigneeID)
	if !isUUID(assigneeID) {
		return MutationResponse{}, NewAppError(KindValidation, "assignee_id must be a UUID", nil)
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return MutationResponse{}, NewAppError(KindValidation, "reason is required", nil)
	}
	if expectedVersion < 1 {
		return MutationResponse{}, NewAppError(KindValidation, "If-Match must contain a positive item version", nil)
	}

	var resp MutationResponse
	err := db.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		items := repo.NewItemRepository(tx)
		authorizer := authz.NewAuthorizer(authz.NewPostgresMembershipStore(tx))

		current, err := items.GetForUpdate(ctx, itemID)
		if errors.Is(err, repo.ErrItemNotFound) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}

		authzItem := authz.Item{TeamID: current.TeamID, CreatedBy: current.CreatedBy}
		if err := authorizer.Authorize(ctx, user, authz.Action{Name: authz.ActionItemAssign}, authzItem); err != nil {
			return err
		}

		if idempotencyKey != "" {
			if replayed, err := s.checkIdempotency(ctx, tx, user.ID, idempotencyKey, "POST:/items/"+itemID+"/assign", requestHash); err != nil {
				return err
			} else if replayed != nil {
				resp = *replayed
				return nil
			}
		}

		if current.Version != expectedVersion {
			if err := decorateItem(ctx, authorizer, user, &current, time.Now()); err != nil {
				return err
			}
			return NewAppError(KindVersionConflict, "work item version does not match If-Match", map[string]any{"current_item": current})
		}

		err = authorizer.Authorize(ctx, model.User{ID: assigneeID}, authz.Action{Name: authz.ActionItemView}, authz.Item{TeamID: current.TeamID})
		if errors.Is(err, ErrNotFound) {
			return NewAppError(KindValidation, "assignee must be a member of the item's team", nil)
		}
		if err != nil {
			return err
		}

		updated, err := items.Assign(ctx, itemID, assigneeID, expectedVersion)
		if errors.Is(err, repo.ErrItemNotFound) {
			latest, readErr := items.Get(ctx, itemID)
			if readErr != nil {
				return ErrNotFound
			}
			if decorateErr := decorateItem(ctx, authorizer, user, &latest, time.Now()); decorateErr != nil {
				return decorateErr
			}
			return NewAppError(KindVersionConflict, "work item version does not match If-Match", map[string]any{"current_item": latest})
		}
		if err != nil {
			return err
		}

		var oldAssigneeJSON json.RawMessage
		if current.AssigneeID != nil {
			oldAssigneeJSON = json.RawMessage(`"` + *current.AssigneeID + `"`)
		}
		newAssigneeJSON := json.RawMessage(`"` + assigneeID + `"`)

		eventID, err := items.InsertEventWithID(ctx, model.ItemEvent{
			ItemID:   updated.ID,
			ActorID:  user.ID,
			Type:     "assigned",
			Field:    stringPointer("assignee_id"),
			OldValue: oldAssigneeJSON,
			NewValue: newAssigneeJSON,
			Reason:   &reason,
		})
		if err != nil {
			return err
		}

		if err := insertItemOutbox(ctx, items, "item.assigned", updated); err != nil {
			return err
		}

		assignPayload, _ := json.Marshal(map[string]any{
			"item_id":     updated.ID,
			"team_id":     updated.TeamID,
			"assignee_id": assigneeID,
			"actor_id":    user.ID,
			"event_id":    eventID,
		})
		if err := items.InsertOutbox(ctx, "notify_assignment", fmt.Sprintf("notify_assignment:%d", eventID), assignPayload); err != nil {
			return err
		}

		body, err := json.Marshal(updated)
		if err != nil {
			return fmt.Errorf("encode assigned item: %w", err)
		}

		if err := s.storeIdempotency(ctx, tx, user.ID, idempotencyKey, 200, body); err != nil {
			return err
		}

		resp = MutationResponse{StatusCode: 200, Body: body}
		return nil
	})
	if err != nil {
		return MutationResponse{}, err
	}
	return resp, nil
}

func (s *ItemService) Transition(ctx context.Context, user model.User, itemID string, expectedVersion int, targetState string, reason *string, idempotencyKey, requestHash string) (MutationResponse, error) {
	if !isUUID(itemID) {
		return MutationResponse{}, ErrNotFound
	}
	targetState = strings.TrimSpace(targetState)
	if targetState == "" {
		return MutationResponse{}, NewAppError(KindValidation, "to is required", nil)
	}
	if expectedVersion < 1 {
		return MutationResponse{}, NewAppError(KindValidation, "If-Match must contain a positive item version", nil)
	}

	var resp MutationResponse
	err := db.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		if idempotencyKey != "" {
			if replayed, err := s.checkIdempotency(ctx, tx, user.ID, idempotencyKey, "POST:/items/"+itemID+"/transition", requestHash); err != nil {
				return err
			} else if replayed != nil {
				resp = *replayed
				return nil
			}
		}

		items := repo.NewItemRepository(tx)
		authorizer := authz.NewAuthorizer(authz.NewPostgresMembershipStore(tx))

		current, err := items.GetForUpdate(ctx, itemID)
		if errors.Is(err, repo.ErrItemNotFound) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}

		authzItem := authz.Item{TeamID: current.TeamID, CreatedBy: current.CreatedBy}
		if err := authorizer.Authorize(ctx, user, authz.Action{Name: authz.ActionItemTransition, TargetState: targetState}, authzItem); err != nil {
			return err
		}

		if current.Version != expectedVersion {
			if err := decorateItem(ctx, authorizer, user, &current, time.Now()); err != nil {
				return err
			}
			return NewAppError(KindVersionConflict, "work item version does not match If-Match", map[string]any{"current_item": current})
		}

		if !statemachine.CanTransition(current.Status, targetState) {
			return NewAppError(KindIllegalTransition, "requested state transition is not allowed", map[string]any{
				"allowed_next_states": statemachine.Allowed(current.Status),
			})
		}

		hasApproved := false
		if targetState == statemachine.StateResolved {
			var err error
			hasApproved, err = items.HasApprovedApproval(ctx, itemID)
			if err != nil {
				return err
			}
		}

		preconditionErr := statemachine.CheckPreconditions(current.Status, targetState, statemachine.ItemSnapshot{
			Status:           current.Status,
			Priority:         current.Priority,
			AssigneeID:       current.AssigneeID,
			ApprovalApproved: hasApproved,
		})
		if preconditionErr != nil {
			return NewAppError(KindValidation, preconditionErr.Error(), nil)
		}

		if targetState == statemachine.StatePendingApproval {
			if _, err := items.CreateApproval(ctx, itemID, user.ID, reason); err != nil {
				return fmt.Errorf("create approval: %w", err)
			}
		}

		var resolvedAt *time.Time
		if targetState == statemachine.StateResolved {
			now := time.Now().UTC()
			resolvedAt = &now
		} else if current.Status == statemachine.StateResolved && targetState == statemachine.StateInProgress {
			resolvedAt = nil
		} else {
			resolvedAt = current.ResolvedAt
		}

		updated, err := items.Transition(ctx, itemID, targetState, expectedVersion, resolvedAt)
		if errors.Is(err, repo.ErrItemNotFound) {
			latest, readErr := items.Get(ctx, itemID)
			if readErr != nil {
				return ErrNotFound
			}
			if decorateErr := decorateItem(ctx, authorizer, user, &latest, time.Now()); decorateErr != nil {
				return decorateErr
			}
			return NewAppError(KindVersionConflict, "work item version does not match If-Match", map[string]any{"current_item": latest})
		}
		if err != nil {
			return err
		}

		oldStatusJSON := json.RawMessage(`"` + current.Status + `"`)
		newStatusJSON := json.RawMessage(`"` + targetState + `"`)
		if err := items.InsertEvent(ctx, model.ItemEvent{
			ItemID:   updated.ID,
			ActorID:  user.ID,
			Type:     "status_changed",
			Field:    stringPointer("status"),
			OldValue: oldStatusJSON,
			NewValue: newStatusJSON,
			Reason:   reason,
		}); err != nil {
			return err
		}

		if err := insertItemOutbox(ctx, items, "item.transitioned", updated); err != nil {
			return err
		}

		body, err := json.Marshal(updated)
		if err != nil {
			return fmt.Errorf("encode transitioned item: %w", err)
		}

		if err := s.storeIdempotency(ctx, tx, user.ID, idempotencyKey, 200, body); err != nil {
			return err
		}

		resp = MutationResponse{StatusCode: 200, Body: body}
		return nil
	})
	if err != nil {
		return MutationResponse{}, err
	}
	return resp, nil
}

func (s *ItemService) DecideApproval(ctx context.Context, user model.User, itemID, approvalID, decision, reason, idempotencyKey, requestHash string) (MutationResponse, error) {
	if !isUUID(itemID) || !isUUID(approvalID) {
		return MutationResponse{}, ErrNotFound
	}
	decision = strings.ToLower(strings.TrimSpace(decision))
	if decision != "approved" && decision != "rejected" {
		return MutationResponse{}, NewAppError(KindValidation, "decision must be 'approved' or 'rejected'", nil)
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return MutationResponse{}, NewAppError(KindValidation, "reason is required", nil)
	}

	endpoint := fmt.Sprintf("POST:/items/%s/approvals/%s/decide", itemID, approvalID)

	var resp MutationResponse
	err := db.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		if idempotencyKey != "" {
			if replayed, err := s.checkIdempotency(ctx, tx, user.ID, idempotencyKey, endpoint, requestHash); err != nil {
				return err
			} else if replayed != nil {
				resp = *replayed
				return nil
			}
		}

		items := repo.NewItemRepository(tx)
		authorizer := authz.NewAuthorizer(authz.NewPostgresMembershipStore(tx))

		current, err := items.GetForUpdate(ctx, itemID)
		if errors.Is(err, repo.ErrItemNotFound) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}

		approval, err := items.GetApproval(ctx, approvalID, itemID)
		if errors.Is(err, repo.ErrApprovalNotFound) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if approval.Decision != nil {
			return NewAppError(KindConflict, "approval has already been decided", nil)
		}

		authzItem := authz.Item{
			TeamID:              current.TeamID,
			CreatedBy:           current.CreatedBy,
			ApprovalRequestedBy: approval.RequestedBy,
		}
		if err := authorizer.Authorize(ctx, user, authz.Action{Name: authz.ActionApprovalDecide}, authzItem); err != nil {
			return err
		}

		if current.Status != statemachine.StatePendingApproval {
			return NewAppError(KindConflict, "work item is not pending approval", nil)
		}

		if _, err := items.DecideApproval(ctx, approvalID, user.ID, decision, reason); err != nil {
			if errors.Is(err, repo.ErrApprovalAlreadyDecided) {
				return NewAppError(KindConflict, "approval has already been decided", nil)
			}
			return err
		}

		var nextStatus string
		var resolvedAt *time.Time
		if decision == "approved" {
			nextStatus = statemachine.StateResolved
			now := time.Now().UTC()
			resolvedAt = &now
		} else {
			nextStatus = statemachine.StateInProgress
			resolvedAt = nil
		}

		updated, err := items.UpdateItemStatus(ctx, itemID, nextStatus, resolvedAt)
		if err != nil {
			return err
		}

		oldStatusJSON := json.RawMessage(`"` + current.Status + `"`)
		newStatusJSON := json.RawMessage(`"` + nextStatus + `"`)
		if err := items.InsertEvent(ctx, model.ItemEvent{
			ItemID:   updated.ID,
			ActorID:  user.ID,
			Type:     "approval_decided",
			Field:    stringPointer("status"),
			OldValue: oldStatusJSON,
			NewValue: newStatusJSON,
			Reason:   &reason,
		}); err != nil {
			return err
		}

		if err := insertItemOutbox(ctx, items, "item.approval_decided", updated); err != nil {
			return err
		}

		body, err := json.Marshal(updated)
		if err != nil {
			return fmt.Errorf("encode decided approval item: %w", err)
		}

		if err := s.storeIdempotency(ctx, tx, user.ID, idempotencyKey, 200, body); err != nil {
			return err
		}

		resp = MutationResponse{StatusCode: 200, Body: body}
		return nil
	})
	if err != nil {
		return MutationResponse{}, err
	}
	return resp, nil
}

func stringPointer(s string) *string {
	return &s
}

type fieldChange struct {
	field    string
	oldValue any
	newValue any
}

func applyPatch(ctx context.Context, items *repo.ItemRepository, authorizer *authz.Authorizer, current model.WorkItem, patch map[string]json.RawMessage) (model.WorkItem, []fieldChange, error) {
	updated := current
	changes := make([]fieldChange, 0, len(patch)+2)
	addChange := func(field string, oldValue, newValue any) {
		if !reflect.DeepEqual(oldValue, newValue) {
			changes = append(changes, fieldChange{field: field, oldValue: oldValue, newValue: newValue})
		}
	}

	if raw, ok := patch["title"]; ok {
		var value string
		if err := json.Unmarshal(raw, &value); err != nil || strings.TrimSpace(value) == "" || len(value) > 500 {
			return model.WorkItem{}, nil, NewAppError(KindValidation, "title must be non-empty and at most 500 characters", nil)
		}
		addChange("title", current.Title, value)
		updated.Title = value
	}
	if raw, ok := patch["description"]; ok {
		var value string
		if err := json.Unmarshal(raw, &value); err != nil || len(value) > 30000 {
			return model.WorkItem{}, nil, NewAppError(KindValidation, "description must be a string of at most 30000 characters", nil)
		}
		addChange("description", current.Description, value)
		updated.Description = value
	}
	if raw, ok := patch["priority"]; ok {
		var value int16
		if err := json.Unmarshal(raw, &value); err != nil || slaForPriority(value) == 0 {
			return model.WorkItem{}, nil, NewAppError(KindValidation, "priority must be an integer between 1 and 4", nil)
		}
		addChange("priority", current.Priority, value)
		updated.Priority = value
		if value != current.Priority {
			dueAt := current.CreatedAt.Add(slaForPriority(value))
			addChange("due_at", current.DueAt, &dueAt)
			updated.DueAt = &dueAt
		}
	}
	if raw, ok := patch["status"]; ok {
		var value string
		if err := json.Unmarshal(raw, &value); err != nil || !statemachine.CanTransition(current.Status, value) {
			return model.WorkItem{}, nil, NewAppError(KindIllegalTransition, "requested state transition is not allowed", map[string]any{"allowed_next_states": statemachine.Allowed(current.Status)})
		}
		addChange("status", current.Status, value)
		updated.Status = value
		if value == "resolved" {
			now := time.Now().UTC()
			updated.ResolvedAt = &now
		} else if current.Status == "resolved" && value == "in_progress" {
			updated.ResolvedAt = nil
		}
		addChange("resolved_at", current.ResolvedAt, updated.ResolvedAt)
	}
	if raw, ok := patch["assignee_id"]; ok {
		var value *string
		if string(raw) != "null" {
			var assigneeID string
			if err := json.Unmarshal(raw, &assigneeID); err != nil || !isUUID(assigneeID) {
				return model.WorkItem{}, nil, NewAppError(KindValidation, "assignee_id must be a UUID or null", nil)
			}
			value = &assigneeID
		}
		if value != nil {
			err := authorizer.Authorize(ctx, model.User{ID: *value}, authz.Action{Name: authz.ActionItemView}, authz.Item{TeamID: current.TeamID})
			if errors.Is(err, ErrNotFound) {
				return model.WorkItem{}, nil, NewAppError(KindValidation, "assignee must be a member of the item's team", nil)
			}
			if err != nil {
				return model.WorkItem{}, nil, err
			}
		}
		addChange("assignee_id", current.AssigneeID, value)
		updated.AssigneeID = value
	}
	if raw, ok := patch["custom_fields"]; ok {
		var rawFields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &rawFields); err != nil || rawFields == nil {
			return model.WorkItem{}, nil, NewAppError(KindValidation, "custom_fields must be a JSON object", nil)
		}
		mergedFields := make(map[string]json.RawMessage, len(current.CustomFields)+len(rawFields))
		for key, value := range current.CustomFields {
			encoded, err := json.Marshal(value)
			if err != nil {
				return model.WorkItem{}, nil, fmt.Errorf("encode existing custom field %s: %w", key, err)
			}
			mergedFields[key] = encoded
		}
		for key, value := range rawFields {
			mergedFields[key] = value
		}
		schemas, err := items.ListFieldSchemas(ctx, current.TeamID)
		if err != nil {
			return model.WorkItem{}, nil, err
		}
		fields, err := validateCustomFields(schemas, mergedFields)
		if err != nil {
			return model.WorkItem{}, nil, err
		}
		addChange("custom_fields", current.CustomFields, fields)
		updated.CustomFields = fields
	}
	return updated, changes, nil
}

func validateIdempotencyKey(key, requestHash string) error {
	if strings.TrimSpace(key) == "" || len(key) > 200 {
		return NewAppError(KindValidation, "Idempotency-Key is required and must be at most 200 characters", nil)
	}
	if requestHash == "" {
		return NewAppError(KindValidation, "request body hash is required", nil)
	}
	return nil
}

func (s *ItemService) checkIdempotency(ctx context.Context, tx pgx.Tx, userID, key, endpoint, requestHash string) (*MutationResponse, error) {
	if key == "" {
		return nil, nil
	}
	result, err := repo.BeginIdempotency(ctx, tx, userID, key, endpoint, requestHash)
	if err != nil {
		return nil, err
	}
	switch result.State {
	case repo.IdempotencyReplay:
		return &MutationResponse{
			StatusCode: result.StatusCode,
			Body:       result.Response,
			Replayed:   true,
		}, nil
	case repo.IdempotencyKeyReuse:
		return nil, NewAppError(KindIdempotencyKeyReuse, "idempotency key was already used with a different request body", nil)
	case repo.IdempotencyInProgress:
		return nil, NewAppError(KindRequestInProgress, "request with this idempotency key is currently in progress", nil)
	default:
		return nil, nil
	}
}

func (s *ItemService) storeIdempotency(ctx context.Context, tx pgx.Tx, userID, key string, statusCode int, body []byte) error {
	if key == "" {
		return nil
	}
	return repo.StoreIdempotencyResponse(ctx, tx, userID, key, statusCode, body)
}

func HashRequest(method, path string, body []byte) string {
	h := sha256.New()
	h.Write([]byte(strings.ToUpper(strings.TrimSpace(method))))
	h.Write([]byte(":"))
	h.Write([]byte(path))
	h.Write([]byte(":"))
	h.Write(body)
	return hex.EncodeToString(h.Sum(nil))
}

func IsUUID(value string) bool {
	return isUUID(value)
}

func alreadyClaimed(winnerID *string, item ...*model.WorkItem) error {
	details := map[string]any{
		"winner": map[string]any{"assignee_id": winnerID},
	}
	if len(item) > 0 && item[0] != nil {
		details["current_item"] = item[0]
	}
	return NewAppError(KindAlreadyClaimed, "work item is already claimed", details)
}

func insertItemOutbox(ctx context.Context, items *repo.ItemRepository, jobType string, item model.WorkItem) error {
	payload, err := json.Marshal(map[string]any{"item_id": item.ID, "team_id": item.TeamID, "version": item.Version})
	if err != nil {
		return fmt.Errorf("encode item outbox payload: %w", err)
	}
	return items.InsertOutbox(ctx, jobType, fmt.Sprintf("%s:%s:v%d", jobType, item.ID, item.Version), payload)
}

func slaForPriority(priority int16) time.Duration {
	switch priority {
	case 1:
		return time.Hour
	case 2:
		return 4 * time.Hour
	case 3:
		return 24 * time.Hour
	case 4:
		return 72 * time.Hour
	default:
		return 0
	}
}

func timePointer(value time.Time) *time.Time {
	return &value
}

func isUUID(value string) bool {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
		return false
	}
	compact := strings.ReplaceAll(value, "-", "")
	decoded, err := hex.DecodeString(compact)
	return err == nil && len(decoded) == 16
}

func HashRequestBody(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func parsePositiveVersion(value string) (int, error) {
	value = strings.Trim(value, "\"")
	version, err := strconv.Atoi(value)
	if err != nil || version < 1 {
		return 0, fmt.Errorf("invalid item version")
	}
	return version, nil
}
