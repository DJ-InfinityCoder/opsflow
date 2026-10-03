package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"opsflow/backend/internal/model"
	"opsflow/backend/internal/service"
	"opsflow/backend/internal/testutil"
)

func TestStep7StatemachineActions(t *testing.T) {
	pool := testutil.NewTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	teamID := insertTestTeam(t, ctx, pool)

	lead := insertUserWithEmail(t, ctx, pool, "lead-sm@example.test", "Lead User", false)
	operator := insertUserWithEmail(t, ctx, pool, "operator-sm@example.test", "Operator User", false)
	otherOperator := insertUserWithEmail(t, ctx, pool, "other-op-sm@example.test", "Other Operator", false)
	reporter := insertUserWithEmail(t, ctx, pool, "reporter-sm@example.test", "Reporter User", false)

	addTestMember(t, ctx, pool, teamID, lead.ID, "lead")
	addTestMember(t, ctx, pool, teamID, operator.ID, "operator")
	addTestMember(t, ctx, pool, teamID, otherOperator.ID, "operator")
	addTestMember(t, ctx, pool, teamID, reporter.ID, "reporter")

	itemService := service.NewItemService(pool)

	// Create a work item with status 'new' and priority 1
	createResp, err := itemService.Create(ctx, operator, service.CreateItemInput{
		TeamID:   teamID,
		Title:    "Payment Gateway Outage",
		Priority: 1,
	}, "create-item-sm-1", service.HashRequestBody([]byte("test")))
	if err != nil {
		t.Fatalf("create item: %v", err)
	}
	var item model.WorkItem
	if err := json.Unmarshal(createResp.Body, &item); err != nil {
		t.Fatalf("decode created item: %v", err)
	}
	if item.Status != "new" || item.Version != 1 {
		t.Fatalf("unexpected new item state: status=%s version=%d", item.Status, item.Version)
	}

	// ----------------------------------------------------
	// 1. POST /items/:id/claim
	// ----------------------------------------------------
	t.Run("claim atomic and already claimed", func(t *testing.T) {
		// First claim by operator succeeds
		claimResp, err := itemService.Claim(ctx, operator, item.ID, "claim-key-1", service.HashRequestBody([]byte("claim-1")))
		if err != nil {
			t.Fatalf("operator claim item: %v", err)
		}
		var claimedItem model.WorkItem
		if err := json.Unmarshal(claimResp.Body, &claimedItem); err != nil {
			t.Fatalf("decode claimed item: %v", err)
		}
		if claimedItem.AssigneeID == nil || *claimedItem.AssigneeID != operator.ID || claimedItem.Version != 2 {
			t.Fatalf("unexpected claimed item state: %+v", claimedItem)
		}

		// Second claim by otherOperator returns 409 already_claimed with current assignee info
		_, err = itemService.Claim(ctx, otherOperator, item.ID, "claim-key-2", service.HashRequestBody([]byte("claim-2")))
		var appErr *service.AppError
		if !errors.As(err, &appErr) || appErr.Kind != service.KindAlreadyClaimed {
			t.Fatalf("expected already_claimed conflict, got %v", err)
		}

		// Reporter cannot claim (forbidden)
		_, err = itemService.Claim(ctx, reporter, item.ID, "claim-key-3", service.HashRequestBody([]byte("claim-3")))
		if !errors.Is(err, service.ErrForbidden) {
			t.Fatalf("expected forbidden for reporter claim, got %v", err)
		}

		// Verify audit event
		events, err := itemService.Events(ctx, operator, item.ID, "", 10)
		if err != nil {
			t.Fatalf("get events: %v", err)
		}
		foundClaimEvent := false
		for _, e := range events.Events {
			if e.Type == "claimed" {
				foundClaimEvent = true
				break
			}
		}
		if !foundClaimEvent {
			t.Fatal("expected claimed audit event in item_events")
		}
		item = claimedItem
	})

	// ----------------------------------------------------
	// 2. POST /items/:id/assign (lead only; If-Match; reason required)
	// ----------------------------------------------------
	t.Run("assign lead only with reason and If-Match", func(t *testing.T) {
		// Operator trying to assign should get 403 Forbidden
		_, err := itemService.Assign(ctx, operator, item.ID, item.Version, otherOperator.ID, "Reassigning to colleague", "", "")
		if !errors.Is(err, service.ErrForbidden) {
			t.Fatalf("expected operator assign to be forbidden, got %v", err)
		}

		// Missing reason should return validation error
		_, err = itemService.Assign(ctx, lead, item.ID, item.Version, otherOperator.ID, "   ", "", "")
		if !errors.Is(err, service.ErrValidation) {
			t.Fatalf("expected validation error for empty reason, got %v", err)
		}

		// Stale version should return 409 version_conflict
		_, err = itemService.Assign(ctx, lead, item.ID, item.Version+99, otherOperator.ID, "Reassigning", "", "")
		if !errors.Is(err, service.ErrVersionConflict) {
			t.Fatalf("expected version conflict, got %v", err)
		}

		// Valid assign by lead
		assignResp, err := itemService.Assign(ctx, lead, item.ID, item.Version, otherOperator.ID, "Reassigning to on-call", "assign-1", service.HashRequestBody([]byte("assign")))
		if err != nil {
			t.Fatalf("lead assign: %v", err)
		}
		var assignedItem model.WorkItem
		if err := json.Unmarshal(assignResp.Body, &assignedItem); err != nil {
			t.Fatalf("decode assigned item: %v", err)
		}
		if assignedItem.AssigneeID == nil || *assignedItem.AssigneeID != otherOperator.ID || assignedItem.Version != item.Version+1 {
			t.Fatalf("unexpected assigned item: %+v", assignedItem)
		}
		item = assignedItem
	})

	// ----------------------------------------------------
	// 3. POST /items/:id/transition
	// ----------------------------------------------------
	t.Run("transition state machine and preconditions", func(t *testing.T) {
		// Illegal transition: new -> in_progress (skipping triaged)
		_, err := itemService.Transition(ctx, operator, item.ID, item.Version, "in_progress", nil, "", "")
		var appErr *service.AppError
		if !errors.As(err, &appErr) || appErr.Kind != service.KindIllegalTransition {
			t.Fatalf("expected illegal transition, got %v", err)
		}

		// Valid transition: new -> triaged
		transResp, err := itemService.Transition(ctx, operator, item.ID, item.Version, "triaged", nil, "", "")
		if err != nil {
			t.Fatalf("transition new -> triaged: %v", err)
		}
		if err := json.Unmarshal(transResp.Body, &item); err != nil {
			t.Fatalf("decode item: %v", err)
		}
		if item.Status != "triaged" {
			t.Fatalf("expected triaged, got %s", item.Status)
		}

		// Valid transition: triaged -> in_progress (already has assignee otherOperator)
		transResp, err = itemService.Transition(ctx, otherOperator, item.ID, item.Version, "in_progress", nil, "", "")
		if err != nil {
			t.Fatalf("transition triaged -> in_progress: %v", err)
		}
		if err := json.Unmarshal(transResp.Body, &item); err != nil {
			t.Fatalf("decode item: %v", err)
		}
		if item.Status != "in_progress" {
			t.Fatalf("expected in_progress, got %s", item.Status)
		}

		// Valid transition: in_progress -> pending_approval (creates approvals row)
		reason := "Requesting approval for payment refund"
		transResp, err = itemService.Transition(ctx, otherOperator, item.ID, item.Version, "pending_approval", &reason, "", "")
		if err != nil {
			t.Fatalf("transition in_progress -> pending_approval: %v", err)
		}
		if err := json.Unmarshal(transResp.Body, &item); err != nil {
			t.Fatalf("decode item: %v", err)
		}
		if item.Status != "pending_approval" {
			t.Fatalf("expected pending_approval, got %s", item.Status)
		}

		// Direct transition to resolved without approval should fail
		_, err = itemService.Transition(ctx, lead, item.ID, item.Version, "resolved", nil, "", "")
		if err == nil {
			t.Fatal("expected direct transition to resolved to fail without approved decision")
		}

		// Verify approvals row was created
		var approvalID, requestedBy string
		if err := pool.QueryRow(ctx, `SELECT id::text, requested_by::text FROM approvals WHERE item_id = $1::uuid AND decision IS NULL`, item.ID).Scan(&approvalID, &requestedBy); err != nil {
			t.Fatalf("query approvals row: %v", err)
		}
		if requestedBy != otherOperator.ID {
			t.Fatalf("expected approval requester to be %s, got %s", otherOperator.ID, requestedBy)
		}

		// ----------------------------------------------------
		// 4. POST /items/:id/approvals/:aid/decide
		// ----------------------------------------------------
		// Requester (otherOperator) cannot decide approval (separation of duties)
		_, err = itemService.DecideApproval(ctx, otherOperator, item.ID, approvalID, "approved", "Self approving", "", "")
		if !errors.Is(err, service.ErrForbidden) {
			t.Fatalf("expected requester decision to be forbidden, got %v", err)
		}

		// Non-lead operator cannot decide approval
		_, err = itemService.DecideApproval(ctx, operator, item.ID, approvalID, "approved", "Operator approving", "", "")
		if !errors.Is(err, service.ErrForbidden) {
			t.Fatalf("expected non-lead decision to be forbidden, got %v", err)
		}

		// Rejection flow: lead rejects with reason -> moves item back to in_progress
		rejectResp, err := itemService.DecideApproval(ctx, lead, item.ID, approvalID, "rejected", "Need more transaction logs", "decide-reject-1", service.HashRequestBody([]byte("reject")))
		if err != nil {
			t.Fatalf("lead reject approval: %v", err)
		}
		var rejectedItem model.WorkItem
		if err := json.Unmarshal(rejectResp.Body, &rejectedItem); err != nil {
			t.Fatalf("decode rejected item: %v", err)
		}
		if rejectedItem.Status != "in_progress" {
			t.Fatalf("expected in_progress after rejection, got %s", rejectedItem.Status)
		}
		item = rejectedItem

		// Second transition from in_progress to pending_approval
		reason2 := "Re-submitting with additional logs"
		transResp, err = itemService.Transition(ctx, otherOperator, item.ID, item.Version, "pending_approval", &reason2, "", "")
		if err != nil {
			t.Fatalf("re-transition to pending_approval: %v", err)
		}
		if err := json.Unmarshal(transResp.Body, &item); err != nil {
			t.Fatalf("decode item: %v", err)
		}

		// Query second approval row
		var approval2ID string
		if err := pool.QueryRow(ctx, `SELECT id::text FROM approvals WHERE item_id = $1::uuid AND decision IS NULL`, item.ID).Scan(&approval2ID); err != nil {
			t.Fatalf("query second approval row: %v", err)
		}

		// Approval flow: lead approves with reason -> moves item to resolved
		approveResp, err := itemService.DecideApproval(ctx, lead, item.ID, approval2ID, "approved", "Approved after reviewing logs", "decide-approve-1", service.HashRequestBody([]byte("approve")))
		if err != nil {
			t.Fatalf("lead approve approval: %v", err)
		}
		var approvedItem model.WorkItem
		if err := json.Unmarshal(approveResp.Body, &approvedItem); err != nil {
			t.Fatalf("decode approved item: %v", err)
		}
		if approvedItem.Status != "resolved" || approvedItem.ResolvedAt == nil {
			t.Fatalf("expected resolved status with resolved_at, got status=%s resolved_at=%v", approvedItem.Status, approvedItem.ResolvedAt)
		}
		item = approvedItem

		// Resolved -> Closed transition
		closeResp, err := itemService.Transition(ctx, lead, item.ID, item.Version, "closed", nil, "", "")
		if err != nil {
			t.Fatalf("transition resolved -> closed: %v", err)
		}
		var closedItem model.WorkItem
		if err := json.Unmarshal(closeResp.Body, &closedItem); err != nil {
			t.Fatalf("decode closed item: %v", err)
		}
		if closedItem.Status != "closed" {
			t.Fatalf("expected closed status, got %s", closedItem.Status)
		}
	})
}

func insertUserWithEmail(t *testing.T, ctx context.Context, pool *pgxpool.Pool, email, name string, isAdmin bool) model.User {
	t.Helper()
	var user model.User
	if err := pool.QueryRow(ctx, `
		INSERT INTO users (email, name, is_system_admin)
		VALUES ($1, $2, $3)
		RETURNING id::text, email, name, is_system_admin
	`, email, name, isAdmin).Scan(&user.ID, &user.Email, &user.Name, &user.IsSystemAdmin); err != nil {
		t.Fatalf("create user %s: %v", email, err)
	}
	return user
}

func addTestMember(t *testing.T, ctx context.Context, pool *pgxpool.Pool, teamID, userID, role string) {
	t.Helper()
	if _, err := pool.Exec(ctx, `INSERT INTO team_members (team_id, user_id, role) VALUES ($1, $2, $3)`, teamID, userID, role); err != nil {
		t.Fatalf("add %s member: %v", role, err)
	}
}
