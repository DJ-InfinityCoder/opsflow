package service_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"

	"opsflow/backend/internal/model"
	"opsflow/backend/internal/repo"
	"opsflow/backend/internal/service"
	"opsflow/backend/internal/testutil"
	"opsflow/backend/internal/worker"
)

func TestHashRequestAndUUID(t *testing.T) {
	// Test IsUUID
	if !service.IsUUID("12345678-1234-1234-1234-123456789abc") {
		t.Fatalf("expected valid UUID to pass")
	}
	if service.IsUUID("not-a-uuid") {
		t.Fatalf("expected invalid string to fail UUID check")
	}
	if service.IsUUID("12345678-1234-1234-1234-123456789abg") { // 'g' is not hex
		t.Fatalf("expected non-hex to fail UUID check")
	}

	// Test HashRequest
	h1 := service.HashRequest("POST", "/items", []byte(`{"title":"test"}`))
	h2 := service.HashRequest("POST", "/items", []byte(`{"title":"test"}`))
	if h1 != h2 {
		t.Fatalf("expected deterministic hash")
	}

	hDiffBody := service.HashRequest("POST", "/items", []byte(`{"title":"diff"}`))
	if h1 == hDiffBody {
		t.Fatalf("different body must produce different hash")
	}

	hDiffPath := service.HashRequest("POST", "/other", []byte(`{"title":"test"}`))
	if h1 == hDiffPath {
		t.Fatalf("different path must produce different hash")
	}

	hDiffMethod := service.HashRequest("PUT", "/items", []byte(`{"title":"test"}`))
	if h1 == hDiffMethod {
		t.Fatalf("different method must produce different hash")
	}
}

func TestServiceIdempotencyReplayAllEndpoints(t *testing.T) {
	pool := testutil.NewTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	teamID := insertTestTeam(t, ctx, pool)
	lead := insertTestUser(t, ctx, pool)
	operator := insertUserWithEmail(t, ctx, pool, "operator-idem@example.test", "Operator", false)

	addTestMember(t, ctx, pool, teamID, lead.ID, "lead")
	addTestMember(t, ctx, pool, teamID, operator.ID, "operator")

	itemService := service.NewItemService(pool)

	// 1. Create item
	createKey := "c0000000-0000-0000-0000-000000000001"
	createBody, _ := json.Marshal(service.CreateItemInput{TeamID: teamID, Title: "Flow test", Priority: 2})
	createHash := service.HashRequest("POST", "/items", createBody)

	createResp1, err := itemService.Create(ctx, operator, service.CreateItemInput{TeamID: teamID, Title: "Flow test", Priority: 2}, createKey, createHash)
	if err != nil {
		t.Fatalf("create item: %v", err)
	}
	if createResp1.Replayed {
		t.Fatalf("first create should not be replayed")
	}

	createResp2, err := itemService.Create(ctx, operator, service.CreateItemInput{TeamID: teamID, Title: "Flow test", Priority: 2}, createKey, createHash)
	if err != nil {
		t.Fatalf("replayed create item: %v", err)
	}
	if !createResp2.Replayed {
		t.Fatalf("second create must have Replayed: true")
	}

	var item model.WorkItem
	_ = json.Unmarshal(createResp1.Body, &item)

	// 2. Claim item
	claimKey := "c0000000-0000-0000-0000-000000000002"
	claimHash := service.HashRequest("POST", "/items/"+item.ID+"/claim", nil)

	claimResp1, err := itemService.Claim(ctx, operator, item.ID, claimKey, claimHash)
	if err != nil {
		t.Fatalf("claim item: %v", err)
	}
	if claimResp1.Replayed {
		t.Fatalf("first claim should not be replayed")
	}

	claimResp2, err := itemService.Claim(ctx, operator, item.ID, claimKey, claimHash)
	if err != nil {
		t.Fatalf("second claim item: %v", err)
	}
	if !claimResp2.Replayed {
		t.Fatalf("second claim must have Replayed: true")
	}

	// 3. Transition to triaged
	transKey1 := "c0000000-0000-0000-0000-000000000003"
	transHash1 := service.HashRequest("POST", "/items/"+item.ID+"/transition", []byte(`{"to":"triaged"}`))
	transResp1, err := itemService.Transition(ctx, operator, item.ID, 2, "triaged", nil, transKey1, transHash1)
	if err != nil {
		t.Fatalf("transition triaged: %v", err)
	}
	if transResp1.Replayed {
		t.Fatalf("first transition should not be replayed")
	}

	transResp2, err := itemService.Transition(ctx, operator, item.ID, 2, "triaged", nil, transKey1, transHash1)
	if err != nil {
		t.Fatalf("second transition triaged: %v", err)
	}
	if !transResp2.Replayed {
		t.Fatalf("second transition must have Replayed: true")
	}

	// Move to in_progress (version is 3 now)
	transProgKey := "c0000000-0000-0000-0000-00000000000a"
	transProgHash := service.HashRequest("POST", "/items/"+item.ID+"/transition", []byte(`{"to":"in_progress"}`))
	if _, err := itemService.Transition(ctx, operator, item.ID, 3, "in_progress", nil, transProgKey, transProgHash); err != nil {
		t.Fatalf("transition in_progress: %v", err)
	}

	// 4. Transition to pending_approval (version is 4 now)
	reason := "Need lead signoff"
	transKey2 := "c0000000-0000-0000-0000-000000000004"
	transHash2 := service.HashRequest("POST", "/items/"+item.ID+"/transition", []byte(`{"to":"pending_approval"}`))
	transResp3, err := itemService.Transition(ctx, operator, item.ID, 4, "pending_approval", &reason, transKey2, transHash2)
	if err != nil {
		t.Fatalf("transition pending_approval: %v", err)
	}
	if transResp3.Replayed {
		t.Fatalf("first transition to pending_approval should not be replayed")
	}

	// Fetch approval ID created
	var approvalID string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM approvals WHERE item_id = $1::uuid`, item.ID).Scan(&approvalID); err != nil {
		t.Fatalf("query approval id: %v", err)
	}

	// 5. Decide approval by lead
	decideKey := "c0000000-0000-0000-0000-000000000005"
	decideHash := service.HashRequest("POST", "/items/"+item.ID+"/approvals/"+approvalID+"/decide", []byte(`{"decision":"approved","reason":"Looks great"}`))

	decideResp1, err := itemService.DecideApproval(ctx, lead, item.ID, approvalID, "approved", "Looks great", decideKey, decideHash)
	if err != nil {
		t.Fatalf("decide approval: %v", err)
	}
	if decideResp1.Replayed {
		t.Fatalf("first decide approval should not be replayed")
	}

	decideResp2, err := itemService.DecideApproval(ctx, lead, item.ID, approvalID, "approved", "Looks great", decideKey, decideHash)
	if err != nil {
		t.Fatalf("second decide approval: %v", err)
	}
	if !decideResp2.Replayed {
		t.Fatalf("second decide approval must have Replayed: true")
	}
}

func TestServiceConcurrentInProgress(t *testing.T) {
	pool := testutil.NewTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	user := insertTestUser(t, ctx, pool)
	sharedKey := "d0000000-0000-0000-0000-000000000001"
	endpoint := "POST:/items"
	requestHash := "dummy-hash-1"

	// Start a transaction in connection 1 and acquire the in-flight idempotency row
	tx1, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx1: %v", err)
	}
	defer func() { _ = tx1.Rollback(ctx) }()

	res1, err := repo.BeginIdempotency(ctx, tx1, user.ID, sharedKey, endpoint, requestHash)
	if err != nil {
		t.Fatalf("begin idempotency tx1: %v", err)
	}
	if res1.State != repo.IdempotencyNew {
		t.Fatalf("expected state new in tx1, got %v", res1.State)
	}

	// Now in connection 2 (concurrent duplicate request while tx1 is in-flight),
	// attempt BeginIdempotency with the SAME user.ID and sharedKey
	tx2, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx2: %v", err)
	}
	defer func() { _ = tx2.Rollback(ctx) }()

	res2, err := repo.BeginIdempotency(ctx, tx2, user.ID, sharedKey, endpoint, requestHash)
	if err != nil {
		t.Fatalf("begin idempotency tx2: %v", err)
	}

	// Concurrent duplicate must report in_progress (409)
	if res2.State != repo.IdempotencyInProgress {
		t.Fatalf("expected state in_progress for concurrent duplicate, got %v", res2.State)
	}
}

func TestWorkerIdempotencyCleanup(t *testing.T) {
	pool := testutil.NewTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	user := insertTestUser(t, ctx, pool)

	expiredKey := "e0000000-0000-0000-0000-000000000001"
	recentKey := "e0000000-0000-0000-0000-000000000002"

	// Insert expired key (created 25 hours ago)
	if _, err := pool.Exec(ctx, `
		INSERT INTO idempotency_keys (user_id, key, endpoint, request_hash, status_code, response_body, created_at)
		VALUES ($1::uuid, $2, 'POST:/items', 'hash1', 201, '{}'::jsonb, now() - interval '25 hours')
	`, user.ID, expiredKey); err != nil {
		t.Fatalf("insert expired key: %v", err)
	}

	// Insert recent key (created 1 hour ago)
	if _, err := pool.Exec(ctx, `
		INSERT INTO idempotency_keys (user_id, key, endpoint, request_hash, status_code, response_body, created_at)
		VALUES ($1::uuid, $2, 'POST:/items', 'hash2', 201, '{}'::jsonb, now() - interval '1 hour')
	`, user.ID, recentKey); err != nil {
		t.Fatalf("insert recent key: %v", err)
	}

	// Execute worker job of type idempotency_cleanup
	job := worker.Job{
		ID:          1,
		Type:        worker.JobTypeIdempotencyCleanup,
		Attempts:    0,
		MaxAttempts: 5,
	}

	if err := worker.ExecuteJob(ctx, pool, job, logger); err != nil {
		t.Fatalf("ExecuteJob idempotency_cleanup: %v", err)
	}

	// Verify expired key is deleted
	var expiredCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM idempotency_keys WHERE key = $1`, expiredKey).Scan(&expiredCount); err != nil {
		t.Fatalf("count expired key: %v", err)
	}
	if expiredCount != 0 {
		t.Fatalf("expected expired key to be deleted, count=%d", expiredCount)
	}

	// Verify recent key is preserved
	var recentCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM idempotency_keys WHERE key = $1`, recentKey).Scan(&recentCount); err != nil {
		t.Fatalf("count recent key: %v", err)
	}
	if recentCount != 1 {
		t.Fatalf("expected recent key to be preserved, count=%d", recentCount)
	}
}
