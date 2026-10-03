package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	stdhttp "net/http"
	"net/http/httptest"
	"testing"
	"time"

	"opsflow/backend/internal/model"
	"opsflow/backend/internal/repo"
	"opsflow/backend/internal/service"
	"opsflow/backend/internal/testutil"
	"opsflow/backend/internal/worker"
)

func TestStep9CommentsAndMentions(t *testing.T) {
	pool := testutil.NewTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	handler, authService := setupTestRouter(t, pool)
	teamID := insertTeam(t, ctx, pool, "Comments Team")

	lead := insertUser(t, ctx, pool, "lead-comm@example.test", "Lead User")
	operator := insertUser(t, ctx, pool, "op-comm@example.test", "Operator User")
	_ = insertUser(t, ctx, pool, "other-comm@example.test", "Other User")

	addMember(t, ctx, pool, teamID, lead.ID, "lead")
	addMember(t, ctx, pool, teamID, operator.ID, "operator")
	// otherUser is NOT a member of teamID

	leadToken := loginUser(t, ctx, authService, lead.Email)
	itemService := service.NewItemService(pool)

	// Create an item
	createResp, err := itemService.Create(ctx, lead, service.CreateItemInput{
		TeamID:   teamID,
		Title:    "Issue for comments",
		Priority: 2,
	}, "comm-create-key-1", service.HashRequest("POST", "/items", []byte("body")))
	if err != nil {
		t.Fatalf("create item: %v", err)
	}
	var item model.WorkItem
	_ = json.Unmarshal(createResp.Body, &item)

	// 1. Comment mentioning a valid team member (@op-comm)
	commentBody, _ := json.Marshal(map[string]any{
		"body": "Hey @op-comm, can you check this out?",
	})
	req := httptest.NewRequest("POST", fmt.Sprintf("/items/%s/comments", item.ID), bytes.NewReader(commentBody))
	req.Header.Set("Authorization", "Bearer "+leadToken)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusCreated {
		t.Fatalf("create comment expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var comment model.Comment
	if err := json.Unmarshal(rec.Body.Bytes(), &comment); err != nil {
		t.Fatalf("decode comment: %v", err)
	}
	if len(comment.Mentions) != 1 || comment.Mentions[0] != operator.ID {
		t.Fatalf("expected mentions to contain operator ID %s, got %v", operator.ID, comment.Mentions)
	}

	// Verify item_event 'commented' was inserted
	var eventCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM item_events WHERE item_id = $1::uuid AND type = 'commented'`, item.ID).Scan(&eventCount); err != nil {
		t.Fatalf("count commented events: %v", err)
	}
	if eventCount != 1 {
		t.Fatalf("expected 1 commented event, got %d", eventCount)
	}

	// Verify outbox_jobs contains notify_mention
	var outboxCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM outbox_jobs WHERE type = 'notify_mention'`).Scan(&outboxCount); err != nil {
		t.Fatalf("count notify_mention outbox jobs: %v", err)
	}
	if outboxCount != 1 {
		t.Fatalf("expected 1 notify_mention outbox job, got %d", outboxCount)
	}

	// 2. Comment mentioning a user who is NOT a member of the team (@other-comm) -> 422
	invalidBody, _ := json.Marshal(map[string]any{
		"body": "Hey @other-comm, take a look",
	})
	reqInvalid := httptest.NewRequest("POST", fmt.Sprintf("/items/%s/comments", item.ID), bytes.NewReader(invalidBody))
	reqInvalid.Header.Set("Authorization", "Bearer "+leadToken)
	reqInvalid.Header.Set("Content-Type", "application/json")
	recInvalid := httptest.NewRecorder()
	handler.ServeHTTP(recInvalid, reqInvalid)

	if recInvalid.Code != stdhttp.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for non-team member mention, got %d: %s", recInvalid.Code, recInvalid.Body.String())
	}

	// 3. GET /items/:id/comments
	reqGet := httptest.NewRequest("GET", fmt.Sprintf("/items/%s/comments", item.ID), nil)
	reqGet.Header.Set("Authorization", "Bearer "+leadToken)
	recGet := httptest.NewRecorder()
	handler.ServeHTTP(recGet, reqGet)

	if recGet.Code != stdhttp.StatusOK {
		t.Fatalf("get comments expected 200, got %d: %s", recGet.Code, recGet.Body.String())
	}
	var getResp struct {
		Comments []model.Comment `json:"comments"`
	}
	_ = json.Unmarshal(recGet.Body.Bytes(), &getResp)
	if len(getResp.Comments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(getResp.Comments))
	}
}

func TestStep9OutboxWorkerAndNotifications(t *testing.T) {
	pool := testutil.NewTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler, authService := setupTestRouter(t, pool)
	teamID := insertTeam(t, ctx, pool, "Outbox Team")

	lead := insertUser(t, ctx, pool, "lead-out@example.test", "Lead Outbox")
	operator := insertUser(t, ctx, pool, "op-out@example.test", "Operator Outbox")
	addMember(t, ctx, pool, teamID, lead.ID, "lead")
	addMember(t, ctx, pool, teamID, operator.ID, "operator")

	opToken := loginUser(t, ctx, authService, operator.Email)
	itemService := service.NewItemService(pool)

	// Create item with Priority 1 -> should create outbox job notify_p1_created
	createResp, err := itemService.Create(ctx, operator, service.CreateItemInput{
		TeamID:   teamID,
		Title:    "P1 Incident",
		Priority: 1,
	}, "out-create-key-1", service.HashRequest("POST", "/items", []byte("p1")))
	if err != nil {
		t.Fatalf("create P1 item: %v", err)
	}
	var item model.WorkItem
	_ = json.Unmarshal(createResp.Body, &item)

	// Check notify_p1_created was enqueued
	var p1JobCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM outbox_jobs WHERE type = 'notify_p1_created'`).Scan(&p1JobCount); err != nil {
		t.Fatalf("count p1 jobs: %v", err)
	}
	if p1JobCount != 1 {
		t.Fatalf("expected 1 notify_p1_created job, got %d", p1JobCount)
	}

	// Claim item -> should create outbox job notify_assignment
	claimResp, err := itemService.Claim(ctx, operator, item.ID, "out-claim-key-1", service.HashRequest("POST", "/items/"+item.ID+"/claim", nil))
	if err != nil {
		t.Fatalf("claim item: %v", err)
	}
	if claimResp.StatusCode != 200 {
		t.Fatalf("claim expected 200, got %d", claimResp.StatusCode)
	}

	var assignJobCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM outbox_jobs WHERE type = 'notify_assignment'`).Scan(&assignJobCount); err != nil {
		t.Fatalf("count assign jobs: %v", err)
	}
	if assignJobCount != 1 {
		t.Fatalf("expected 1 notify_assignment job, got %d", assignJobCount)
	}

	// Run worker ProcessNextJob until all pending jobs are processed
	for {
		processed, err := worker.ProcessNextJob(ctx, pool, logger)
		if err != nil {
			t.Fatalf("worker process next job: %v", err)
		}
		if !processed {
			break
		}
	}

	// Verify all jobs in outbox are completed
	var pendingCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM outbox_jobs WHERE status = 'pending'`).Scan(&pendingCount); err != nil {
		t.Fatalf("count pending jobs: %v", err)
	}
	if pendingCount != 0 {
		t.Fatalf("expected 0 pending jobs, got %d", pendingCount)
	}

	// Verify notifications table has the delivered notification for operator
	var notifCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE user_id = $1::uuid`, operator.ID).Scan(&notifCount); err != nil {
		t.Fatalf("count notifications for operator: %v", err)
	}
	if notifCount < 1 {
		t.Fatalf("expected at least 1 notification for operator, got %d", notifCount)
	}

	// Test GET /notifications
	reqNotif := httptest.NewRequest("GET", "/notifications", nil)
	reqNotif.Header.Set("Authorization", "Bearer "+opToken)
	recNotif := httptest.NewRecorder()
	handler.ServeHTTP(recNotif, reqNotif)

	if recNotif.Code != stdhttp.StatusOK {
		t.Fatalf("get notifications expected 200, got %d: %s", recNotif.Code, recNotif.Body.String())
	}
	var notifPage struct {
		Notifications []model.Notification `json:"notifications"`
	}
	_ = json.Unmarshal(recNotif.Body.Bytes(), &notifPage)
	if len(notifPage.Notifications) == 0 {
		t.Fatalf("expected notifications in response, got 0")
	}

	// Test POST /notifications/read
	readBody, _ := json.Marshal(map[string]any{
		"ids": []string{notifPage.Notifications[0].ID},
	})
	reqRead := httptest.NewRequest("POST", "/notifications/read", bytes.NewReader(readBody))
	reqRead.Header.Set("Authorization", "Bearer "+opToken)
	reqRead.Header.Set("Content-Type", "application/json")
	recRead := httptest.NewRecorder()
	handler.ServeHTTP(recRead, reqRead)

	if recRead.Code != stdhttp.StatusOK {
		t.Fatalf("mark read expected 200, got %d: %s", recRead.Code, recRead.Body.String())
	}

	// Verify notification now has read_at set
	var readAt *time.Time
	if err := pool.QueryRow(ctx, `SELECT read_at FROM notifications WHERE id = $1::uuid`, notifPage.Notifications[0].ID).Scan(&readAt); err != nil {
		t.Fatalf("check read_at: %v", err)
	}
	if readAt == nil {
		t.Fatalf("expected read_at to be non-nil after marking read")
	}
}

func TestStep9FeedAnalyticsAndAdminJobs(t *testing.T) {
	pool := testutil.NewTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	handler, authService := setupTestRouter(t, pool)
	teamID := insertTeam(t, ctx, pool, "Analytics Team")

	lead := insertUser(t, ctx, pool, "lead-an@example.test", "Lead Analytics")
	operator := insertUser(t, ctx, pool, "op-an@example.test", "Operator Analytics")
	admin := insertUser(t, ctx, pool, "admin-an@example.test", "Admin User")
	outsideUser := insertUser(t, ctx, pool, "outside@example.test", "Outside User")

	// Update admin to be system admin
	if _, err := pool.Exec(ctx, `UPDATE users SET is_system_admin = true WHERE id = $1::uuid`, admin.ID); err != nil {
		t.Fatalf("set system admin: %v", err)
	}

	addMember(t, ctx, pool, teamID, lead.ID, "lead")
	addMember(t, ctx, pool, teamID, operator.ID, "operator")

	leadToken := loginUser(t, ctx, authService, lead.Email)
	opToken := loginUser(t, ctx, authService, operator.Email)
	adminToken := loginUser(t, ctx, authService, admin.Email)
	outsideToken := loginUser(t, ctx, authService, outsideUser.Email)

	itemService := service.NewItemService(pool)

	// Create an item to generate feed events
	_, err := itemService.Create(ctx, lead, service.CreateItemInput{
		TeamID:   teamID,
		Title:    "Analytics Item",
		Priority: 2,
	}, "an-create-key-1", service.HashRequest("POST", "/items", []byte("1")))
	if err != nil {
		t.Fatalf("create item: %v", err)
	}

	// 1. GET /feed?team=
	reqFeed := httptest.NewRequest("GET", fmt.Sprintf("/feed?team=%s", teamID), nil)
	reqFeed.Header.Set("Authorization", "Bearer "+leadToken)
	recFeed := httptest.NewRecorder()
	handler.ServeHTTP(recFeed, reqFeed)

	if recFeed.Code != stdhttp.StatusOK {
		t.Fatalf("get feed expected 200, got %d: %s", recFeed.Code, recFeed.Body.String())
	}
	var feedResp struct {
		Events []model.FeedEvent `json:"events"`
	}
	_ = json.Unmarshal(recFeed.Body.Bytes(), &feedResp)
	if len(feedResp.Events) == 0 {
		t.Fatalf("expected feed events, got 0")
	}

	// 2. GET /analytics/summary?team=
	// Lead should succeed (200)
	reqLeadAn := httptest.NewRequest("GET", fmt.Sprintf("/analytics/summary?team=%s", teamID), nil)
	reqLeadAn.Header.Set("Authorization", "Bearer "+leadToken)
	recLeadAn := httptest.NewRecorder()
	handler.ServeHTTP(recLeadAn, reqLeadAn)
	if recLeadAn.Code != stdhttp.StatusOK {
		t.Fatalf("lead analytics expected 200, got %d: %s", recLeadAn.Code, recLeadAn.Body.String())
	}
	var summary model.AnalyticsSummary
	_ = json.Unmarshal(recLeadAn.Body.Bytes(), &summary)
	if summary.TotalItems < 1 {
		t.Fatalf("expected at least 1 total item in analytics, got %d", summary.TotalItems)
	}

	// Operator should be forbidden (403)
	reqOpAn := httptest.NewRequest("GET", fmt.Sprintf("/analytics/summary?team=%s", teamID), nil)
	reqOpAn.Header.Set("Authorization", "Bearer "+opToken)
	recOpAn := httptest.NewRecorder()
	handler.ServeHTTP(recOpAn, reqOpAn)
	if recOpAn.Code != stdhttp.StatusForbidden {
		t.Fatalf("operator analytics expected 403, got %d: %s", recOpAn.Code, recOpAn.Body.String())
	}

	// Outside user should receive 404
	reqOutAn := httptest.NewRequest("GET", fmt.Sprintf("/analytics/summary?team=%s", teamID), nil)
	reqOutAn.Header.Set("Authorization", "Bearer "+outsideToken)
	recOutAn := httptest.NewRecorder()
	handler.ServeHTTP(recOutAn, reqOutAn)
	if recOutAn.Code != stdhttp.StatusNotFound {
		t.Fatalf("outside user analytics expected 404, got %d: %s", recOutAn.Code, recOutAn.Body.String())
	}

	// 3. GET /admin/jobs?status=failed
	// Insert a failed job into outbox_jobs
	if _, err := pool.Exec(ctx, `
		INSERT INTO outbox_jobs (type, dedupe_key, payload, status, attempts, max_attempts, last_error)
		VALUES ('test_job', 'test:dedupe:failed', '{}'::jsonb, 'failed', 5, 5, 'fatal error')
	`); err != nil {
		t.Fatalf("insert failed job: %v", err)
	}

	// Admin accesses /admin/jobs -> 200 OK
	reqAdminJobs := httptest.NewRequest("GET", "/admin/jobs?status=failed", nil)
	reqAdminJobs.Header.Set("Authorization", "Bearer "+adminToken)
	recAdminJobs := httptest.NewRecorder()
	handler.ServeHTTP(recAdminJobs, reqAdminJobs)
	if recAdminJobs.Code != stdhttp.StatusOK {
		t.Fatalf("admin jobs expected 200, got %d: %s", recAdminJobs.Code, recAdminJobs.Body.String())
	}
	var adminResp struct {
		Jobs []model.OutboxJob `json:"jobs"`
	}
	_ = json.Unmarshal(recAdminJobs.Body.Bytes(), &adminResp)
	if len(adminResp.Jobs) != 1 {
		t.Fatalf("expected 1 failed job in admin view, got %d", len(adminResp.Jobs))
	}

	// Non-admin accesses /admin/jobs -> 403 Forbidden
	reqNonAdminJobs := httptest.NewRequest("GET", "/admin/jobs?status=failed", nil)
	reqNonAdminJobs.Header.Set("Authorization", "Bearer "+leadToken)
	recNonAdminJobs := httptest.NewRecorder()
	handler.ServeHTTP(recNonAdminJobs, reqNonAdminJobs)
	if recNonAdminJobs.Code != stdhttp.StatusForbidden {
		t.Fatalf("non-admin jobs expected 403, got %d: %s", recNonAdminJobs.Code, recNonAdminJobs.Body.String())
	}
}

func TestStep9WorkerDeadLetterAndReclaim(t *testing.T) {
	pool := testutil.NewTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// 1. Dead-letter test: insert job that fails execution with attempts = 4, max_attempts = 5
	// We insert an invalid JSON payload into notify_assignment so ExecuteJob returns an error!
	if _, err := pool.Exec(ctx, `
		INSERT INTO outbox_jobs (type, dedupe_key, payload, status, attempts, max_attempts)
		VALUES ('notify_assignment', 'failing:job:1', '{"item_id":"00000000-0000-0000-0000-000000000001","assignee_id":"00000000-0000-0000-0000-000000000002"}'::jsonb, 'pending', 4, 5)
	`); err != nil {
		t.Fatalf("insert failing job: %v", err)
	}

	// Process the job
	processed, err := worker.ProcessNextJob(ctx, pool, logger)
	if err != nil {
		t.Fatalf("process failing job: %v", err)
	}
	if !processed {
		t.Fatalf("expected failing job to be processed")
	}

	// Check status is now 'failed' (dead-lettered)
	var finalStatus string
	var finalAttempts int
	if err := pool.QueryRow(ctx, `SELECT status, attempts FROM outbox_jobs WHERE dedupe_key = 'failing:job:1'`).Scan(&finalStatus, &finalAttempts); err != nil {
		t.Fatalf("query failing job: %v", err)
	}
	if finalStatus != "failed" {
		t.Fatalf("expected status 'failed', got %q", finalStatus)
	}
	if finalAttempts != 5 {
		t.Fatalf("expected attempts 5, got %d", finalAttempts)
	}

	// 2. Reclaim test: insert a job with status='processing' and locked_at = 10 minutes ago
	if _, err := pool.Exec(ctx, `
		INSERT INTO outbox_jobs (type, dedupe_key, payload, status, locked_at)
		VALUES ('test_reclaim', 'stale:job:1', '{}'::jsonb, 'processing', now() - interval '10 minutes')
	`); err != nil {
		t.Fatalf("insert stale job: %v", err)
	}

	outboxRepo := repo.NewOutboxRepository(pool)
	reclaimed, err := outboxRepo.ReclaimStaleJobs(ctx, 5*time.Minute)
	if err != nil {
		t.Fatalf("reclaim stale jobs: %v", err)
	}
	if reclaimed != 1 {
		t.Fatalf("expected 1 reclaimed job, got %d", reclaimed)
	}

	// Check status reverted to 'pending' and locked_at is NULL
	var reclaimedStatus string
	var lockedAt *time.Time
	if err := pool.QueryRow(ctx, `SELECT status, locked_at FROM outbox_jobs WHERE dedupe_key = 'stale:job:1'`).Scan(&reclaimedStatus, &lockedAt); err != nil {
		t.Fatalf("query reclaimed job: %v", err)
	}
	if reclaimedStatus != "pending" {
		t.Fatalf("expected reclaimed status 'pending', got %q", reclaimedStatus)
	}
	if lockedAt != nil {
		t.Fatalf("expected locked_at to be NULL, got %v", lockedAt)
	}
}
