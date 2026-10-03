package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"opsflow/backend/internal/model"
	"opsflow/backend/internal/service"
	"opsflow/backend/internal/testutil"
)

func TestItemServiceLifecycle(t *testing.T) {
	pool := testutil.NewTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	teamID := insertTestTeam(t, ctx, pool)
	user := insertTestUser(t, ctx, pool)
	var otherUser model.User
	if err := pool.QueryRow(ctx, `
		INSERT INTO users (email, name) VALUES ('item-service-other@example.test', 'Other Item Service User')
		RETURNING id::text, email, name, is_system_admin
	`).Scan(&otherUser.ID, &otherUser.Email, &otherUser.Name, &otherUser.IsSystemAdmin); err != nil {
		t.Fatalf("insert second user: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO team_members (team_id, user_id, role) VALUES ($1, $2, 'operator')`, teamID, user.ID); err != nil {
		t.Fatalf("insert operator membership: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO team_members (team_id, user_id, role) VALUES ($1, $2, 'operator')`, teamID, otherUser.ID); err != nil {
		t.Fatalf("insert second operator membership: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO team_field_schemas (team_id, field_key, label, type, required, options)
		VALUES ($1, 'transaction_id', 'Transaction ID', 'text', true, '[]'::jsonb)
	`, teamID); err != nil {
		t.Fatalf("insert team field schema: %v", err)
	}

	items := service.NewItemService(pool)
	input := service.CreateItemInput{
		TeamID: teamID, Title: "Payment investigation", Description: "Investigate delayed settlement", Priority: 1,
		CustomFields: map[string]json.RawMessage{"transaction_id": json.RawMessage(`"txn-123"`)},
	}
	createBody, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("encode create request for idempotency hash: %v", err)
	}
	invalidInput := input
	invalidInput.CustomFields = map[string]json.RawMessage{}
	if _, err := items.Create(ctx, user, invalidInput, "create-missing-required-field", service.HashRequestBody([]byte("missing-required"))); !errors.Is(err, service.ErrValidation) {
		t.Fatalf("expected required-field validation error, got %v", err)
	}
	created, err := items.Create(ctx, user, input, "create-payment-item-1", service.HashRequestBody(createBody))
	if err != nil {
		t.Fatalf("create work item: %v", err)
	}
	if created.StatusCode != 201 {
		t.Fatalf("expected status 201, got %d", created.StatusCode)
	}
	var item model.WorkItem
	if err := json.Unmarshal(created.Body, &item); err != nil {
		t.Fatalf("decode created work item: %v", err)
	}
	if item.Status != "new" || item.Version != 1 || item.DueAt == nil {
		t.Fatalf("unexpected created item state: %+v", item)
	}
	remainingSLA := item.DueAt.Sub(item.CreatedAt)
	if remainingSLA < time.Hour-time.Minute || remainingSLA > time.Hour {
		t.Fatalf("expected approximately one-hour priority-1 SLA, got %s", remainingSLA)
	}

	replay, err := items.Create(ctx, user, input, "create-payment-item-1", service.HashRequestBody(createBody))
	if err != nil {
		t.Fatalf("replay create request: %v", err)
	}
	var replayedItem model.WorkItem
	if err := json.Unmarshal(replay.Body, &replayedItem); err != nil {
		t.Fatalf("decode replayed item: %v", err)
	}
	if !replay.Replayed || replayedItem.ID != item.ID || replayedItem.Version != item.Version {
		t.Fatalf("expected exact idempotent replay, got %+v", replay)
	}

	patchBody := []byte(`{"title":"Settlement investigation"}`)
	patch := map[string]json.RawMessage{"title": json.RawMessage(`"Settlement investigation"`)}
	updated, err := items.Patch(ctx, user, item.ID, 1, patch, service.HashRequestBody(patchBody))
	if err != nil {
		t.Fatalf("patch work item: %v", err)
	}
	var patched model.WorkItem
	if err := json.Unmarshal(updated.Body, &patched); err != nil {
		t.Fatalf("decode patched item: %v", err)
	}
	if patched.Version != 2 || patched.Title != "Settlement investigation" {
		t.Fatalf("unexpected patched item: %+v", patched)
	}
	_, err = items.Patch(ctx, user, item.ID, 2,
		map[string]json.RawMessage{"status": json.RawMessage(`"in_progress"`)},
		service.HashRequestBody([]byte(`{"status":"in_progress"}`)))
	if !errors.Is(err, service.ErrIllegalTransition) {
		t.Fatalf("expected illegal transition error, got %v", err)
	}
	claimBody := []byte(`{"assignee_id":"` + user.ID + `"}`)
	claimed, err := items.Patch(ctx, user, item.ID, 2,
		map[string]json.RawMessage{"assignee_id": json.RawMessage(`"` + user.ID + `"`)},
		service.HashRequestBody(claimBody))
	if err != nil {
		t.Fatalf("claim work item: %v", err)
	}
	var claimedItem model.WorkItem
	if err := json.Unmarshal(claimed.Body, &claimedItem); err != nil {
		t.Fatalf("decode claimed work item: %v", err)
	}
	if claimedItem.Version != 3 || claimedItem.AssigneeID == nil || *claimedItem.AssigneeID != user.ID {
		t.Fatalf("unexpected claimed item: %+v", claimedItem)
	}
	otherClaimBody := []byte(`{"assignee_id":"` + otherUser.ID + `"}`)
	_, err = items.Patch(ctx, otherUser, item.ID, 3,
		map[string]json.RawMessage{"assignee_id": json.RawMessage(`"` + otherUser.ID + `"`)},
		service.HashRequestBody(otherClaimBody))
	var claimedError *service.AppError
	if !errors.As(err, &claimedError) || claimedError.Kind != service.KindAlreadyClaimed {
		t.Fatalf("expected already-claimed conflict, got %v", err)
	}
	current, err := items.Get(ctx, user, item.ID)
	if err != nil {
		t.Fatalf("get work item: %v", err)
	}
	if len(current.AllowedStates) != 1 || current.AllowedStates[0] != "triaged" {
		t.Fatalf("unexpected allowed transitions: %v", current.AllowedStates)
	}

	if _, err := pool.Exec(ctx, `UPDATE work_items SET version = version + 1 WHERE id = $1::uuid`, item.ID); err != nil {
		t.Fatalf("advance item version for stale-write test: %v", err)
	}
	staleBody := []byte(`{"description":"stale update"}`)
	_, err = items.Patch(ctx, user, item.ID, 3,
		map[string]json.RawMessage{"description": json.RawMessage(`"stale update"`)},
		service.HashRequestBody(staleBody))
	if !errors.Is(err, service.ErrVersionConflict) {
		t.Fatalf("expected version conflict, got %v", err)
	}

	events, err := items.Events(ctx, user, item.ID, "", 100)
	if err != nil {
		t.Fatalf("list item events: %v", err)
	}
	if len(events.Events) != 3 || events.Events[0].Type != "field_changed" || events.Events[1].Type != "field_changed" || events.Events[2].Type != "created" {
		t.Fatalf("unexpected item event timeline: %+v", events.Events)
	}

	counts, err := items.Counts(ctx, user)
	if err != nil {
		t.Fatalf("count views: %v", err)
	}
	if counts.All != 1 || counts.Urgent != 1 || counts.AssignedToMe != 1 {
		t.Fatalf("unexpected smart-view counts: %+v", counts)
	}

	for index, priority := range []int16{2, 1} {
		additionalInput := service.CreateItemInput{
			TeamID: teamID, Title: fmt.Sprintf("Additional work item %d", index), Description: "Cursor pagination fixture", Priority: priority,
			CustomFields: map[string]json.RawMessage{"transaction_id": json.RawMessage(fmt.Sprintf(`"txn-%d"`, index))},
		}
		body, err := json.Marshal(additionalInput)
		if err != nil {
			t.Fatalf("encode additional create request: %v", err)
		}
		if _, err := items.Create(ctx, user, additionalInput, fmt.Sprintf("create-additional-item-%d", index), service.HashRequestBody(body)); err != nil {
			t.Fatalf("create additional work item: %v", err)
		}
	}
	counts, err = items.Counts(ctx, user)
	if err != nil {
		t.Fatalf("recount views after item creation: %v", err)
	}
	if counts.All != 3 || counts.Urgent != 3 || counts.AssignedToMe != 1 {
		t.Fatalf("unexpected updated smart-view counts: %+v", counts)
	}
	firstPage, err := items.List(ctx, user, service.ItemListInput{View: "all", Limit: 1, LimitSet: true})
	if err != nil {
		t.Fatalf("list first page: %v", err)
	}
	if len(firstPage.Items) != 1 || firstPage.NextCursor == "" {
		t.Fatalf("expected one item and a next cursor, got %+v", firstPage)
	}
	secondPage, err := items.List(ctx, user, service.ItemListInput{View: "all", Cursor: firstPage.NextCursor, Limit: 1, LimitSet: true})
	if err != nil {
		t.Fatalf("list second page: %v", err)
	}
	if len(secondPage.Items) != 1 || secondPage.Items[0].ID == firstPage.Items[0].ID {
		t.Fatalf("expected a distinct item on the second page, got %+v", secondPage)
	}
	if secondPage.Items[0].Priority < firstPage.Items[0].Priority {
		t.Fatalf("priority order regressed across pages: %d then %d", firstPage.Items[0].Priority, secondPage.Items[0].Priority)
	}
	searchResult, err := items.List(ctx, user, service.ItemListInput{View: "all", Query: "settlement", Limit: 10})
	if err != nil {
		t.Fatalf("full-text search: %v", err)
	}
	if len(searchResult.Items) != 1 || searchResult.Items[0].ID != item.ID {
		t.Fatalf("unexpected full-text search results: %+v", searchResult.Items)
	}
}

func insertTestTeam(t *testing.T, ctx context.Context, pool *pgxpool.Pool) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(ctx, `INSERT INTO teams (name) VALUES ('Item Service Team') RETURNING id::text`).Scan(&id); err != nil {
		t.Fatalf("insert team: %v", err)
	}
	return id
}

func insertTestUser(t *testing.T, ctx context.Context, pool *pgxpool.Pool) model.User {
	t.Helper()
	var user model.User
	if err := pool.QueryRow(ctx, `
		INSERT INTO users (email, name) VALUES ('item-service@example.test', 'Item Service')
		RETURNING id::text, email, name, is_system_admin
	`).Scan(&user.ID, &user.Email, &user.Name, &user.IsSystemAdmin); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	return user
}
