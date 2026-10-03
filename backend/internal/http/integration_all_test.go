package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	stdhttp "net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"opsflow/backend/internal/authz"
	"opsflow/backend/internal/db"
	"opsflow/backend/internal/model"
	"opsflow/backend/internal/repo"
	"opsflow/backend/internal/service"
	"opsflow/backend/internal/statemachine"
	"opsflow/backend/internal/testutil"
	"opsflow/backend/internal/worker"
)

// 1. Concurrent claim: 50 goroutines claim one item; exactly 1 succeeds, 49 get already_claimed, exactly 1 audit event.
// Pool: MaxConns <= 15 (here 12) so total connections stay under 20.
func TestIntegrationConcurrentClaim(t *testing.T) {
	pool := testutil.NewTestDBWithMaxConns(t, 12)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	handler, authService := setupTestRouter(t, pool)
	teamID := insertTeam(t, ctx, pool, "Concurrent Claim Team")

	const numClaimants = 50

	// Batch insert 50 users in one query
	var userVals []string
	for i := 0; i < numClaimants; i++ {
		userVals = append(userVals, fmt.Sprintf("('claimant-%d@example.test', 'Claimant %d')", i, i))
	}
	userRows, err := pool.Query(ctx, fmt.Sprintf(`
		INSERT INTO users (email, name) VALUES %s
		RETURNING id::text, email
	`, strings.Join(userVals, ",")))
	if err != nil {
		t.Fatalf("batch insert claimants: %v", err)
	}

	type userInfo struct {
		id    string
		email string
	}
	var users []userInfo
	for userRows.Next() {
		var u userInfo
		if err := userRows.Scan(&u.id, &u.email); err != nil {
			t.Fatalf("scan claimant: %v", err)
		}
		users = append(users, u)
	}
	userRows.Close()
	if len(users) != numClaimants {
		t.Fatalf("expected %d users, got %d", numClaimants, len(users))
	}

	// Batch insert team memberships as operators
	var memVals []string
	for _, u := range users {
		memVals = append(memVals, fmt.Sprintf("('%s', '%s', 'operator')", teamID, u.id))
	}
	if _, err := pool.Exec(ctx, fmt.Sprintf(`INSERT INTO team_members (team_id, user_id, role) VALUES %s`, strings.Join(memVals, ","))); err != nil {
		t.Fatalf("batch insert memberships: %v", err)
	}

	// Pre-generate tokens
	tokens := make([]string, numClaimants)
	for i, u := range users {
		tokens[i] = loginUser(t, ctx, authService, u.email)
	}

	// Create 1 unclaimed item (status 'new', assignee_id NULL)
	itemService := service.NewItemService(pool)
	firstUser := model.User{ID: users[0].id, Email: users[0].email}
	createResp, err := itemService.Create(ctx, firstUser, service.CreateItemInput{
		TeamID:   teamID,
		Title:    "Race condition test item",
		Priority: 2,
	}, "race-item-create-key-1", service.HashRequest("POST", "/items", []byte("claim-race")))
	if err != nil {
		t.Fatalf("create race item: %v", err)
	}
	var item model.WorkItem
	_ = json.Unmarshal(createResp.Body, &item)

	// Launch 50 goroutines claiming the item concurrently
	type claimResult struct {
		code int
		body string
	}
	results := make([]claimResult, numClaimants)
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(numClaimants)

	for i := 0; i < numClaimants; i++ {
		go func(idx int) {
			defer wg.Done()
			<-start

			req := httptest.NewRequest("POST", fmt.Sprintf("/items/%s/claim", item.ID), nil)
			req.Header.Set("Authorization", "Bearer "+tokens[idx])
			req.Header.Set("Idempotency-Key", fmt.Sprintf("00000000-0000-0000-0000-%012d", idx+1))
			req.RemoteAddr = fmt.Sprintf("192.0.2.%d:1234", idx+1)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			results[idx] = claimResult{code: rec.Code, body: rec.Body.String()}
		}(i)
	}

	close(start)
	wg.Wait()

	// Verify exactly 1 succeeded (200), and exactly 49 received 409 already_claimed
	successCount := 0
	alreadyClaimedCount := 0
	for _, res := range results {
		if res.code == stdhttp.StatusOK {
			successCount++
		} else if res.code == stdhttp.StatusConflict {
			if strings.Contains(res.body, "already_claimed") {
				alreadyClaimedCount++
			}
		} else {
			t.Errorf("unexpected status code: %d, body: %s", res.code, res.body)
		}
	}

	if successCount != 1 {
		t.Fatalf("expected exactly 1 claim success, got %d", successCount)
	}
	if alreadyClaimedCount != numClaimants-1 {
		t.Fatalf("expected exactly %d already_claimed, got %d", numClaimants-1, alreadyClaimedCount)
	}

	// Verify DB state: exactly 1 audit event for assignment/claim
	var auditCount int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM item_events 
		WHERE item_id = $1::uuid AND (type = 'assigned' OR type = 'claimed')
	`, item.ID).Scan(&auditCount); err != nil {
		t.Fatalf("count audit events: %v", err)
	}
	if auditCount != 1 {
		t.Fatalf("expected exactly 1 audit event, got %d", auditCount)
	}

	// Verify item has assignee set
	var assigneeID *string
	if err := pool.QueryRow(ctx, `SELECT assignee_id::text FROM work_items WHERE id = $1::uuid`, item.ID).Scan(&assigneeID); err != nil {
		t.Fatalf("query assignee: %v", err)
	}
	if assigneeID == nil {
		t.Fatalf("expected item assignee to be set, got nil")
	}
}

// 2. Stale version: two PATCH calls with the same If-Match; second returns 409 and the first change is intact.
func TestIntegrationStaleVersion(t *testing.T) {
	pool := testutil.NewTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	handler, authService := setupTestRouter(t, pool)
	teamID := insertTeam(t, ctx, pool, "Stale Version Team")
	op := insertUser(t, ctx, pool, "stale-op@example.test", "Stale Operator")
	addMember(t, ctx, pool, teamID, op.ID, "operator")
	token := loginUser(t, ctx, authService, op.Email)

	// Create initial item with version 1
	createBody, _ := json.Marshal(map[string]any{
		"team_id":  teamID,
		"title":    "Initial Title",
		"priority": 2,
	})
	createReq := httptest.NewRequest("POST", "/items", bytes.NewReader(createBody))
	createReq.Header.Set("Authorization", "Bearer "+token)
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("Idempotency-Key", "00000000-0000-0000-0001-000000000001")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != stdhttp.StatusCreated {
		t.Fatalf("expected 201 created, got %d: %s", createRec.Code, createRec.Body.String())
	}

	var item model.WorkItem
	_ = json.Unmarshal(createRec.Body.Bytes(), &item)
	if item.Version != 1 {
		t.Fatalf("expected version 1, got %d", item.Version)
	}

	// 1. First PATCH with If-Match: 1 -> succeeds, version becomes 2
	patch1Body, _ := json.Marshal(map[string]any{"title": "First Update Title"})
	patch1Req := httptest.NewRequest("PATCH", fmt.Sprintf("/items/%s", item.ID), bytes.NewReader(patch1Body))
	patch1Req.Header.Set("Authorization", "Bearer "+token)
	patch1Req.Header.Set("Content-Type", "application/json")
	patch1Req.Header.Set("If-Match", "1")
	patch1Rec := httptest.NewRecorder()
	handler.ServeHTTP(patch1Rec, patch1Req)
	if patch1Rec.Code != stdhttp.StatusOK {
		t.Fatalf("patch 1 expected 200, got %d: %s", patch1Rec.Code, patch1Rec.Body.String())
	}
	var patchedItem model.WorkItem
	_ = json.Unmarshal(patch1Rec.Body.Bytes(), &patchedItem)
	if patchedItem.Version != 2 || patchedItem.Title != "First Update Title" {
		t.Fatalf("expected version 2 with 'First Update Title', got version %d, title %q", patchedItem.Version, patchedItem.Title)
	}

	// 2. Second PATCH with the SAME If-Match: 1 (now stale!) -> returns 409 version_conflict
	patch2Body, _ := json.Marshal(map[string]any{"title": "Second Update Title"})
	patch2Req := httptest.NewRequest("PATCH", fmt.Sprintf("/items/%s", item.ID), bytes.NewReader(patch2Body))
	patch2Req.Header.Set("Authorization", "Bearer "+token)
	patch2Req.Header.Set("Content-Type", "application/json")
	patch2Req.Header.Set("If-Match", "1")
	patch2Rec := httptest.NewRecorder()
	handler.ServeHTTP(patch2Rec, patch2Req)
	if patch2Rec.Code != stdhttp.StatusConflict {
		t.Fatalf("patch 2 expected 409 conflict, got %d: %s", patch2Rec.Code, patch2Rec.Body.String())
	}
	if !strings.Contains(patch2Rec.Body.String(), "version_conflict") {
		t.Fatalf("expected error code version_conflict, got: %s", patch2Rec.Body.String())
	}

	// 3. Confirm first change remains intact
	getReq := httptest.NewRequest("GET", fmt.Sprintf("/items/%s", item.ID), nil)
	getReq.Header.Set("Authorization", "Bearer "+token)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)
	if getRec.Code != stdhttp.StatusOK {
		t.Fatalf("get item expected 200, got %d", getRec.Code)
	}
	var currentItem model.WorkItem
	_ = json.Unmarshal(getRec.Body.Bytes(), &currentItem)
	if currentItem.Version != 2 || currentItem.Title != "First Update Title" {
		t.Fatalf("expected version 2 and title 'First Update Title' intact, got version %d, title %q", currentItem.Version, currentItem.Title)
	}
}

// 3. Idempotency: same key twice => identical response and one side effect; same key with different body => 422; concurrent same-key requests => one execution.
func TestIntegrationIdempotency(t *testing.T) {
	pool := testutil.NewTestDBWithMaxConns(t, 12)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	handler, authService := setupTestRouter(t, pool)
	teamID := insertTeam(t, ctx, pool, "Idempotency Team")
	op := insertUser(t, ctx, pool, "idem-op@example.test", "Idem Operator")
	addMember(t, ctx, pool, teamID, op.ID, "operator")
	token := loginUser(t, ctx, authService, op.Email)

	// 3a. Same key twice => identical response and one side effect
	key1 := "a0000000-0000-0000-0000-000000000001"
	body1, _ := json.Marshal(map[string]any{
		"team_id":  teamID,
		"title":    "Idempotent Single Creation",
		"priority": 2,
	})

	req1 := httptest.NewRequest("POST", "/items", bytes.NewReader(body1))
	req1.Header.Set("Authorization", "Bearer "+token)
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set("Idempotency-Key", key1)
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)
	if rec1.Code != stdhttp.StatusCreated {
		t.Fatalf("first request expected 201, got %d: %s", rec1.Code, rec1.Body.String())
	}

	req2 := httptest.NewRequest("POST", "/items", bytes.NewReader(body1))
	req2.Header.Set("Authorization", "Bearer "+token)
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Idempotency-Key", key1)
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	if rec2.Code != stdhttp.StatusCreated {
		t.Fatalf("second request expected 201, got %d: %s", rec2.Code, rec2.Body.String())
	}
	if rec2.Header().Get("Idempotent-Replay") != "true" {
		t.Fatalf("expected Idempotent-Replay: true header")
	}
	var item1, item2 model.WorkItem
	if err := json.Unmarshal(rec1.Body.Bytes(), &item1); err != nil {
		t.Fatalf("unmarshal rec1: %v", err)
	}
	if err := json.Unmarshal(rec2.Body.Bytes(), &item2); err != nil {
		t.Fatalf("unmarshal rec2: %v", err)
	}
	if item1.ID != item2.ID || item1.Title != item2.Title || item1.Version != item2.Version {
		t.Fatalf("expected identical replay item data, got item1=%+v, item2=%+v", item1, item2)
	}

	var count1 int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM work_items WHERE title = 'Idempotent Single Creation'`).Scan(&count1); err != nil {
		t.Fatalf("count items: %v", err)
	}
	if count1 != 1 {
		t.Fatalf("expected exactly 1 item created, got %d", count1)
	}

	// 3b. Same key with different body => 422 idempotency_key_reuse
	bodyDiff, _ := json.Marshal(map[string]any{
		"team_id":  teamID,
		"title":    "Completely Different Body",
		"priority": 1,
	})
	reqDiff := httptest.NewRequest("POST", "/items", bytes.NewReader(bodyDiff))
	reqDiff.Header.Set("Authorization", "Bearer "+token)
	reqDiff.Header.Set("Content-Type", "application/json")
	reqDiff.Header.Set("Idempotency-Key", key1)
	recDiff := httptest.NewRecorder()
	handler.ServeHTTP(recDiff, reqDiff)
	if recDiff.Code != stdhttp.StatusUnprocessableEntity {
		t.Fatalf("different body expected 422, got %d: %s", recDiff.Code, recDiff.Body.String())
	}
	if !strings.Contains(recDiff.Body.String(), "idempotency_key_reuse") {
		t.Fatalf("expected idempotency_key_reuse error code, got: %s", recDiff.Body.String())
	}

	// 3c. Concurrent same-key requests => exactly one execution
	keyConcurrent := "a0000000-0000-0000-0000-000000000002"
	bodyConcurrent, _ := json.Marshal(map[string]any{
		"team_id":  teamID,
		"title":    "Concurrent Same Key Item",
		"priority": 2,
	})

	const numConcurr = 8
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(numConcurr)
	statuses := make([]int, numConcurr)
	bodies := make([]string, numConcurr)

	for i := 0; i < numConcurr; i++ {
		go func(idx int) {
			defer wg.Done()
			<-start

			req := httptest.NewRequest("POST", "/items", bytes.NewReader(bodyConcurrent))
			req.RemoteAddr = fmt.Sprintf("192.168.10.%d:1234", idx+1)
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Idempotency-Key", keyConcurrent)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			statuses[idx] = rec.Code
			bodies[idx] = rec.Body.String()
		}(i)
	}

	close(start)
	wg.Wait()

	// Every request must be either 201 (created or replayed) or 409 (request_in_progress)
	createdOrReplayed := 0
	inProgress := 0
	for idx, code := range statuses {
		if code == stdhttp.StatusCreated {
			createdOrReplayed++
		} else if code == stdhttp.StatusConflict {
			inProgress++
		} else {
			t.Errorf("unexpected concurrent idempotency status: %d body: %s", code, bodies[idx])
		}
	}
	if createdOrReplayed < 1 {
		t.Fatalf("expected at least 1 request to succeed with 201, got %d", createdOrReplayed)
	}

	var countConcurrent int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM work_items WHERE title = 'Concurrent Same Key Item'`).Scan(&countConcurrent); err != nil {
		t.Fatalf("count items: %v", err)
	}
	if countConcurrent != 1 {
		t.Fatalf("expected exactly 1 item in DB for concurrent same-key, got %d", countConcurrent)
	}
}

// 4. State machine: table-driven over every (from, to, role) combination.
func TestIntegrationStateMachine(t *testing.T) {
	pool := testutil.NewTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	teamID := insertTeam(t, ctx, pool, "State Machine Team")
	reporter := insertUser(t, ctx, pool, "sm-rep@example.test", "SM Reporter")
	operator := insertUser(t, ctx, pool, "sm-op@example.test", "SM Operator")
	lead := insertUser(t, ctx, pool, "sm-lead@example.test", "SM Lead")

	addMember(t, ctx, pool, teamID, reporter.ID, "reporter")
	addMember(t, ctx, pool, teamID, operator.ID, "operator")
	addMember(t, ctx, pool, teamID, lead.ID, "lead")

	states := []string{
		statemachine.StateNew,
		statemachine.StateTriaged,
		statemachine.StateInProgress,
		statemachine.StatePendingApproval,
		statemachine.StateResolved,
		statemachine.StateClosed,
	}

	roles := []struct {
		name string
		user model.User
	}{
		{"reporter", reporter},
		{"operator", operator},
		{"lead", lead},
	}

	// 1. Table-driven evaluation over all 6 x 6 x 3 = 108 combinations
	authorizer := authz.NewAuthorizer(authz.NewPostgresMembershipStore(pool))
	authzItem := authz.Item{TeamID: teamID}

	for _, from := range states {
		for _, to := range states {
			isLegalTransition := statemachine.CanTransition(from, to)

			for _, r := range roles {
				testName := fmt.Sprintf("%s/%s->%s", r.name, from, to)
				t.Run(testName, func(t *testing.T) {
					authzErr := authorizer.Authorize(ctx, r.user, authz.Action{
						Name:        authz.ActionItemTransition,
						TargetState: to,
					}, authzItem)

					if r.name == "reporter" {
						// Reporter can never transition items
						if !errors.Is(authzErr, service.ErrForbidden) {
							t.Fatalf("expected ErrForbidden for reporter transition, got %v", authzErr)
						}
						return
					}

					if r.name == "operator" && to == statemachine.StateResolved {
						// Operator can never transition to resolved (requires lead)
						if !errors.Is(authzErr, service.ErrForbidden) {
							t.Fatalf("expected ErrForbidden for operator transition to resolved, got %v", authzErr)
						}
						return
					}

					// Role is authorized
					if authzErr != nil {
						t.Fatalf("expected authz to succeed for %s to %s, got %v", r.name, to, authzErr)
					}

					// Verify transition legal check
					if !isLegalTransition {
						if statemachine.CanTransition(from, to) {
							t.Fatalf("expected state machine to disallow %s -> %s", from, to)
						}
					} else {
						if !statemachine.CanTransition(from, to) {
							t.Fatalf("expected state machine to allow %s -> %s", from, to)
						}
					}
				})
			}
		}
	}

	// 2. Execute live transitions through item service to verify database enforcement
	itemService := service.NewItemService(pool)
	createResp, err := itemService.Create(ctx, lead, service.CreateItemInput{
		TeamID:   teamID,
		Title:    "Lifecycle transitions",
		Priority: 2,
	}, "sm-lifecycle-key-1", service.HashRequest("POST", "/items", []byte("lifecycle")))
	if err != nil {
		t.Fatalf("create item: %v", err)
	}
	var item model.WorkItem
	_ = json.Unmarshal(createResp.Body, &item)

	// Illegal jump: new -> in_progress (allowed next is only triaged) -> 422 illegal_transition
	_, err = itemService.Transition(ctx, operator, item.ID, 1, "in_progress", nil, "sm-illegal-1", "hash")
	if !errors.Is(err, service.ErrIllegalTransition) {
		t.Fatalf("expected illegal transition for new -> in_progress, got %v", err)
	}

	// Legal: new -> triaged (version 1 -> 2)
	trResp, err := itemService.Transition(ctx, operator, item.ID, 1, "triaged", nil, "sm-triaged-1", "hash")
	if err != nil {
		t.Fatalf("new -> triaged failed: %v", err)
	}
	_ = json.Unmarshal(trResp.Body, &item)
	if item.Status != statemachine.StateTriaged {
		t.Fatalf("expected status triaged, got %s", item.Status)
	}

	// Assign item so triaged -> in_progress precondition is satisfied
	_, err = itemService.Claim(ctx, operator, item.ID, "sm-claim-1", "hash")
	if err != nil {
		t.Fatalf("claim item: %v", err)
	}

	// Legal: triaged -> in_progress (version 3 -> 4)
	progResp, err := itemService.Transition(ctx, operator, item.ID, 3, "in_progress", nil, "sm-inprog-1", "hash")
	if err != nil {
		t.Fatalf("triaged -> in_progress failed: %v", err)
	}
	_ = json.Unmarshal(progResp.Body, &item)
	if item.Status != statemachine.StateInProgress {
		t.Fatalf("expected status in_progress, got %s", item.Status)
	}

	// Legal: in_progress -> pending_approval (version 4 -> 5)
	reason := "Need lead review"
	paResp, err := itemService.Transition(ctx, operator, item.ID, 4, "pending_approval", &reason, "sm-pa-1", "hash")
	if err != nil {
		t.Fatalf("in_progress -> pending_approval failed: %v", err)
	}
	_ = json.Unmarshal(paResp.Body, &item)
	if item.Status != statemachine.StatePendingApproval {
		t.Fatalf("expected status pending_approval, got %s", item.Status)
	}

	// Operator attempting to transition to resolved without approval decision -> 403 Forbidden (operator not lead)
	_, err = itemService.Transition(ctx, operator, item.ID, 5, "resolved", nil, "sm-res-op", "hash")
	if !errors.Is(err, service.ErrForbidden) {
		t.Fatalf("expected ErrForbidden for operator transitioning to resolved, got %v", err)
	}

	// Lead attempting to transition to resolved directly without approval decision -> 422 ErrApprovalRequired
	_, err = itemService.Transition(ctx, lead, item.ID, 5, "resolved", nil, "sm-res-lead", "hash")
	if !errors.Is(err, service.ErrValidation) && !strings.Contains(err.Error(), "requires an approved decision") {
		t.Fatalf("expected validation/approval required error for unresolved approval, got %v", err)
	}
}

// 5. Authz: cross-team access returns 404, reporter transition returns 403, requester self-approval is denied.
func TestIntegrationAuthz(t *testing.T) {
	pool := testutil.NewTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	handler, authService := setupTestRouter(t, pool)

	teamA := insertTeam(t, ctx, pool, "Team Alpha")
	teamB := insertTeam(t, ctx, pool, "Team Beta")

	userA := insertUser(t, ctx, pool, "user-alpha@example.test", "User Alpha")
	userB := insertUser(t, ctx, pool, "user-beta@example.test", "User Beta")
	leadA := insertUser(t, ctx, pool, "lead-alpha@example.test", "Lead Alpha")
	leadA2 := insertUser(t, ctx, pool, "lead-alpha2@example.test", "Second Lead Alpha")

	addMember(t, ctx, pool, teamA, userA.ID, "reporter")
	addMember(t, ctx, pool, teamA, leadA.ID, "lead")
	addMember(t, ctx, pool, teamA, leadA2.ID, "lead")
	addMember(t, ctx, pool, teamB, userB.ID, "operator")

	tokenA := loginUser(t, ctx, authService, userA.Email)
	tokenLeadA := loginUser(t, ctx, authService, leadA.Email)
	tokenLeadA2 := loginUser(t, ctx, authService, leadA2.Email)

	// Create item in Team B
	itemService := service.NewItemService(pool)
	createRespB, err := itemService.Create(ctx, userB, service.CreateItemInput{
		TeamID:   teamB,
		Title:    "Team Beta Item",
		Priority: 2,
	}, "authz-item-b", "hash")
	if err != nil {
		t.Fatalf("create team B item: %v", err)
	}
	var itemB model.WorkItem
	_ = json.Unmarshal(createRespB.Body, &itemB)

	// 5a. Cross-team access returns 404 (userA belongs only to Team A, accessing Team B item)
	getReq := httptest.NewRequest("GET", fmt.Sprintf("/items/%s", itemB.ID), nil)
	getReq.Header.Set("Authorization", "Bearer "+tokenA)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)
	if getRec.Code != stdhttp.StatusNotFound {
		t.Fatalf("cross-team GET expected 404, got %d: %s", getRec.Code, getRec.Body.String())
	}

	claimReq := httptest.NewRequest("POST", fmt.Sprintf("/items/%s/claim", itemB.ID), nil)
	claimReq.Header.Set("Authorization", "Bearer "+tokenA)
	claimReq.Header.Set("Idempotency-Key", "b0000000-0000-0000-0000-000000000001")
	claimRec := httptest.NewRecorder()
	handler.ServeHTTP(claimRec, claimReq)
	if claimRec.Code != stdhttp.StatusNotFound {
		t.Fatalf("cross-team claim expected 404, got %d: %s", claimRec.Code, claimRec.Body.String())
	}

	// 5b. Reporter transition returns 403 (userA is reporter in Team A, trying to transition item in Team A)
	createRespA, err := itemService.Create(ctx, leadA, service.CreateItemInput{
		TeamID:   teamA,
		Title:    "Team Alpha Item",
		Priority: 2,
	}, "authz-item-a", "hash")
	if err != nil {
		t.Fatalf("create team A item: %v", err)
	}
	var itemA model.WorkItem
	_ = json.Unmarshal(createRespA.Body, &itemA)

	transBody, _ := json.Marshal(map[string]any{"to": "triaged"})
	transReq := httptest.NewRequest("POST", fmt.Sprintf("/items/%s/transition", itemA.ID), bytes.NewReader(transBody))
	transReq.Header.Set("Authorization", "Bearer "+tokenA) // reporter
	transReq.Header.Set("Content-Type", "application/json")
	transReq.Header.Set("If-Match", "1")
	transReq.Header.Set("Idempotency-Key", "b0000000-0000-0000-0000-000000000002")
	transRec := httptest.NewRecorder()
	handler.ServeHTTP(transRec, transReq)
	if transRec.Code != stdhttp.StatusForbidden {
		t.Fatalf("reporter transition expected 403 forbidden, got %d: %s", transRec.Code, transRec.Body.String())
	}

	// 5c. Requester self-approval is denied
	// Move itemA to triaged -> assign to leadA -> in_progress -> pending_approval
	_, _ = itemService.Transition(ctx, leadA, itemA.ID, 1, "triaged", nil, "t-triaged", "h1")
	_, _ = itemService.Claim(ctx, leadA, itemA.ID, "t-claim", "h2")
	_, _ = itemService.Transition(ctx, leadA, itemA.ID, 3, "in_progress", nil, "t-inprog", "h3")
	reasonReq := "Signoff required"
	paResp, err := itemService.Transition(ctx, leadA, itemA.ID, 4, "pending_approval", &reasonReq, "t-pa", "h4")
	if err != nil {
		t.Fatalf("move to pending_approval: %v", err)
	}
	_ = json.Unmarshal(paResp.Body, &itemA)

	// Fetch created approval ID
	var approvalID string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM approvals WHERE item_id = $1::uuid`, itemA.ID).Scan(&approvalID); err != nil {
		t.Fatalf("query approval id: %v", err)
	}

	// leadA is the requester of this approval. Self-approval must be denied (403)!
	decideBody, _ := json.Marshal(map[string]any{"decision": "approved", "reason": "Self approving"})
	selfReq := httptest.NewRequest("POST", fmt.Sprintf("/items/%s/approvals/%s/decide", itemA.ID, approvalID), bytes.NewReader(decideBody))
	selfReq.Header.Set("Authorization", "Bearer "+tokenLeadA) // same lead who requested!
	selfReq.Header.Set("Content-Type", "application/json")
	selfReq.Header.Set("Idempotency-Key", "b0000000-0000-0000-0000-000000000003")
	selfRec := httptest.NewRecorder()
	handler.ServeHTTP(selfRec, selfReq)
	if selfRec.Code != stdhttp.StatusForbidden {
		t.Fatalf("self-approval expected 403 forbidden, got %d: %s", selfRec.Code, selfRec.Body.String())
	}

	// leadA2 (different lead on the same team) CAN decide the approval
	otherLeadReq := httptest.NewRequest("POST", fmt.Sprintf("/items/%s/approvals/%s/decide", itemA.ID, approvalID), bytes.NewReader(decideBody))
	otherLeadReq.Header.Set("Authorization", "Bearer "+tokenLeadA2)
	otherLeadReq.Header.Set("Content-Type", "application/json")
	otherLeadReq.Header.Set("Idempotency-Key", "b0000000-0000-0000-0000-000000000004")
	otherLeadRec := httptest.NewRecorder()
	handler.ServeHTTP(otherLeadRec, otherLeadReq)
	if otherLeadRec.Code != stdhttp.StatusOK {
		t.Fatalf("different lead approval decision expected 200, got %d: %s", otherLeadRec.Code, otherLeadRec.Body.String())
	}
}

// 6. Audit atomicity: inject a failure after the item update and before commit; assert neither the update nor the event persisted.
func TestIntegrationAuditAtomicity(t *testing.T) {
	pool := testutil.NewTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	teamID := insertTeam(t, ctx, pool, "Atomicity Team")
	user := insertUser(t, ctx, pool, "atom-user@example.test", "Atom User")
	addMember(t, ctx, pool, teamID, user.ID, "operator")

	// Create item with initial title
	itemService := service.NewItemService(pool)
	createResp, err := itemService.Create(ctx, user, service.CreateItemInput{
		TeamID:   teamID,
		Title:    "Original Unchanged Title",
		Priority: 2,
	}, "atom-create-key-1", "hash")
	if err != nil {
		t.Fatalf("create item: %v", err)
	}
	var item model.WorkItem
	_ = json.Unmarshal(createResp.Body, &item)

	// Execute transaction where we update item and insert event, then inject failure before commit
	simulatedErr := errors.New("simulated system failure before commit")
	txErr := db.WithTx(ctx, pool, func(tx pgx.Tx) error {
		// Update item
		tag, err := tx.Exec(ctx, `UPDATE work_items SET title = 'Tampered Title In Aborted Tx', version = version + 1 WHERE id = $1`, item.ID)
		if err != nil || tag.RowsAffected() == 0 {
			return fmt.Errorf("update item in tx: %w", err)
		}

		// Insert audit event
		_, err = tx.Exec(ctx, `
			INSERT INTO item_events (item_id, actor_id, type, field, old_value, new_value)
			VALUES ($1, $2, 'updated', 'title', '"Original Unchanged Title"', '"Tampered Title In Aborted Tx"')
		`, item.ID, user.ID)
		if err != nil {
			return fmt.Errorf("insert event in tx: %w", err)
		}

		// Inject failure right before commit
		return simulatedErr
	})

	if !errors.Is(txErr, simulatedErr) {
		t.Fatalf("expected simulated error from tx, got %v", txErr)
	}

	// Assert: work item title was NOT updated
	var currentTitle string
	var currentVersion int
	if err := pool.QueryRow(ctx, `SELECT title, version FROM work_items WHERE id = $1`, item.ID).Scan(&currentTitle, &currentVersion); err != nil {
		t.Fatalf("query item: %v", err)
	}
	if currentTitle != "Original Unchanged Title" || currentVersion != 1 {
		t.Fatalf("expected title 'Original Unchanged Title' and version 1, got title %q and version %d", currentTitle, currentVersion)
	}

	// Assert: audit event was NOT persisted
	var eventCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM item_events WHERE item_id = $1 AND new_value::text LIKE '%Tampered Title In Aborted Tx%'`, item.ID).Scan(&eventCount); err != nil {
		t.Fatalf("query events: %v", err)
	}
	if eventCount != 0 {
		t.Fatalf("expected 0 audit events persisted from rolled back transaction, got %d", eventCount)
	}
}

// 7. Outbox: simulate worker crash mid-job (leave locked_at), confirm reclaim and retry; deliver the same job twice and confirm exactly one notification row.
func TestIntegrationOutbox(t *testing.T) {
	pool := testutil.NewTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	teamID := insertTeam(t, ctx, pool, "Outbox Test Team")
	user := insertUser(t, ctx, pool, "outbox-user@example.test", "Outbox User")
	addMember(t, ctx, pool, teamID, user.ID, "operator")

	itemService := service.NewItemService(pool)
	createResp, err := itemService.Create(ctx, user, service.CreateItemInput{
		TeamID:   teamID,
		Title:    "Outbox Item",
		Priority: 2,
	}, "outbox-create-item", "hash")
	if err != nil {
		t.Fatalf("create item: %v", err)
	}
	var item model.WorkItem
	_ = json.Unmarshal(createResp.Body, &item)

	// Fetch an event ID from item_events
	var eventID int64
	if err := pool.QueryRow(ctx, `SELECT id FROM item_events WHERE item_id = $1 LIMIT 1`, item.ID).Scan(&eventID); err != nil {
		t.Fatalf("get event id: %v", err)
	}

	// 7a. Simulate worker crash mid-job (status='processing', locked_at 10 minutes ago)
	payload, _ := json.Marshal(map[string]any{
		"item_id":     item.ID,
		"team_id":     teamID,
		"assignee_id": user.ID,
		"actor_id":    user.ID,
		"event_id":    eventID,
	})
	if _, err := pool.Exec(ctx, `
		INSERT INTO outbox_jobs (type, dedupe_key, payload, status, locked_at, run_at, attempts, max_attempts)
		VALUES ('notify_assignment', 'crash-test-key-1', $1::jsonb, 'processing', now() - interval '10 minutes', now() - interval '10 minutes', 1, 5)
	`, payload); err != nil {
		t.Fatalf("insert crashed job: %v", err)
	}

	outboxRepo := repo.NewOutboxRepository(pool)
	reclaimed, err := outboxRepo.ReclaimStaleJobs(ctx, 5*time.Minute)
	if err != nil {
		t.Fatalf("reclaim stale jobs: %v", err)
	}
	if reclaimed < 1 {
		t.Fatalf("expected at least 1 job reclaimed, got %d", reclaimed)
	}

	// Check status in DB is now pending and locked_at is cleared
	var jobStatus string
	var lockedAt *time.Time
	if err := pool.QueryRow(ctx, `SELECT status, locked_at FROM outbox_jobs WHERE dedupe_key = 'crash-test-key-1'`).Scan(&jobStatus, &lockedAt); err != nil {
		t.Fatalf("query reclaimed job: %v", err)
	}
	if jobStatus != "pending" || lockedAt != nil {
		t.Fatalf("expected job status 'pending' with nil locked_at, got status=%s, locked_at=%v", jobStatus, lockedAt)
	}

	// Worker processes reclaimed job
	processed, err := worker.ProcessNextJob(ctx, pool, logger)
	if err != nil {
		t.Fatalf("process reclaimed job: %v", err)
	}
	if !processed {
		t.Fatalf("expected worker to process reclaimed job")
	}

	// Confirm job completed
	if err := pool.QueryRow(ctx, `SELECT status FROM outbox_jobs WHERE dedupe_key = 'crash-test-key-1'`).Scan(&jobStatus); err != nil {
		t.Fatalf("query completed job: %v", err)
	}
	if jobStatus != "completed" {
		t.Fatalf("expected job status 'completed', got %s", jobStatus)
	}

	// 7b. Deliver same job twice and confirm exactly one notification row
	notifRepo := repo.NewNotificationRepository(pool)
	_, inserted1, err := notifRepo.Insert(ctx, user.ID, item.ID, "assignment", &eventID, nil)
	if err != nil {
		t.Fatalf("first notification insert: %v", err)
	}
	// Note: previous worker execution delivered notification for (user.ID, eventID), so inserted1 may be false (idempotent)!
	_ = inserted1

	_, inserted2, err := notifRepo.Insert(ctx, user.ID, item.ID, "assignment", &eventID, nil)
	if err != nil {
		t.Fatalf("second notification insert: %v", err)
	}
	if inserted2 {
		t.Fatalf("expected second delivery to be ignored by ON CONFLICT DO NOTHING, got inserted=true")
	}

	// Confirm exactly 1 notification row in DB
	var notifCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE user_id = $1::uuid AND source_event_id = $2`, user.ID, eventID).Scan(&notifCount); err != nil {
		t.Fatalf("count notifications: %v", err)
	}
	if notifCount != 1 {
		t.Fatalf("expected exactly 1 notification row for user and source_event_id, got %d", notifCount)
	}
}

// 8. Audit immutability: UPDATE and DELETE on item_events fail.
func TestIntegrationAuditImmutability(t *testing.T) {
	pool := testutil.NewTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	teamID := insertTeam(t, ctx, pool, "Immutability Team")
	user := insertUser(t, ctx, pool, "immutable-user@example.test", "Audit User")
	addMember(t, ctx, pool, teamID, user.ID, "operator")

	itemService := service.NewItemService(pool)
	createResp, err := itemService.Create(ctx, user, service.CreateItemInput{
		TeamID:   teamID,
		Title:    "Immutable Audit Item",
		Priority: 2,
	}, "immutable-create-key", "hash")
	if err != nil {
		t.Fatalf("create item: %v", err)
	}
	var item model.WorkItem
	_ = json.Unmarshal(createResp.Body, &item)

	var eventID int64
	if err := pool.QueryRow(ctx, `SELECT id FROM item_events WHERE item_id = $1 LIMIT 1`, item.ID).Scan(&eventID); err != nil {
		t.Fatalf("query event id: %v", err)
	}

	// Attempt UPDATE on item_events -> must fail with append-only trigger exception
	_, updateErr := pool.Exec(ctx, `UPDATE item_events SET reason = 'unauthorized modification' WHERE id = $1`, eventID)
	if updateErr == nil {
		t.Fatalf("expected UPDATE on item_events to fail with append-only exception, but it succeeded")
	}
	if !strings.Contains(updateErr.Error(), "item_events is append-only") {
		t.Fatalf("expected 'item_events is append-only' error, got %v", updateErr)
	}

	// Attempt DELETE on item_events -> must fail with append-only trigger exception
	_, deleteErr := pool.Exec(ctx, `DELETE FROM item_events WHERE id = $1`, eventID)
	if deleteErr == nil {
		t.Fatalf("expected DELETE on item_events to fail with append-only exception, but it succeeded")
	}
	if !strings.Contains(deleteErr.Error(), "item_events is append-only") {
		t.Fatalf("expected 'item_events is append-only' error, got %v", deleteErr)
	}

	// Verify event row is still present and unmodified
	var currentReason *string
	if err := pool.QueryRow(ctx, `SELECT reason FROM item_events WHERE id = $1`, eventID).Scan(&currentReason); err != nil {
		t.Fatalf("query event after tamper attempts: %v", err)
	}
	if currentReason != nil && *currentReason == "unauthorized modification" {
		t.Fatalf("audit event was illegally modified")
	}
}
