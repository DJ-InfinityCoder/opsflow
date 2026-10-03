package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	stdhttp "net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"opsflow/backend/internal/authn"
	"opsflow/backend/internal/config"
	apphttp "opsflow/backend/internal/http"
	"opsflow/backend/internal/model"
	"opsflow/backend/internal/repo"
	"opsflow/backend/internal/service"
	"opsflow/backend/internal/testutil"
)

type httpErrorEnvelope struct {
	Error struct {
		Code      string         `json:"code"`
		Message   string         `json:"message"`
		RequestID string         `json:"request_id"`
		Details   map[string]any `json:"details"`
	} `json:"error"`
}

func setupTestRouter(t *testing.T, pool *pgxpool.Pool) (stdhttp.Handler, *service.AuthService) {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := config.Config{
		AppEnv:    "development",
		JWTSecret: "test-secret-12345678901234567890123456789012",
	}
	jwtAuthenticator, err := authn.NewHMACJWT(cfg.JWTSecret)
	if err != nil {
		t.Fatalf("configure jwt: %v", err)
	}
	authService := service.NewAuthService(repo.NewUserRepository(pool), jwtAuthenticator, jwtAuthenticator)
	handler := apphttp.NewRouter(pool, logger, cfg, authService)
	return handler, authService
}

func loginUser(t *testing.T, ctx context.Context, authService *service.AuthService, email string) string {
	t.Helper()
	res, err := authService.DevLogin(ctx, email)
	if err != nil {
		t.Fatalf("login %s: %v", email, err)
	}
	return res.Token
}

func insertTeam(t *testing.T, ctx context.Context, pool *pgxpool.Pool, name string) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(ctx, `INSERT INTO teams (name) VALUES ($1) RETURNING id::text`, name).Scan(&id); err != nil {
		t.Fatalf("insert team: %v", err)
	}
	return id
}

func insertUser(t *testing.T, ctx context.Context, pool *pgxpool.Pool, email, name string) model.User {
	t.Helper()
	var user model.User
	if err := pool.QueryRow(ctx, `
		INSERT INTO users (email, name) VALUES ($1, $2)
		RETURNING id::text, email, name, is_system_admin
	`, email, name).Scan(&user.ID, &user.Email, &user.Name, &user.IsSystemAdmin); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	return user
}

func addMember(t *testing.T, ctx context.Context, pool *pgxpool.Pool, teamID, userID, role string) {
	t.Helper()
	if _, err := pool.Exec(ctx, `INSERT INTO team_members (team_id, user_id, role) VALUES ($1, $2, $3)`, teamID, userID, role); err != nil {
		t.Fatalf("insert member: %v", err)
	}
}

func TestHttpIdempotencyKeyRequiredAndUUID(t *testing.T) {
	pool := testutil.NewTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	handler, authService := setupTestRouter(t, pool)
	teamID := insertTeam(t, ctx, pool, "Ops Team")
	user := insertUser(t, ctx, pool, "lead@example.test", "Lead User")
	addMember(t, ctx, pool, teamID, user.ID, "lead")
	token := loginUser(t, ctx, authService, user.Email)

	bodyBytes, _ := json.Marshal(map[string]any{
		"team_id":  teamID,
		"title":    "New item",
		"priority": 1,
	})

	t.Run("POST /items without Idempotency-Key returns 422", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/items", bytes.NewReader(bodyBytes))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != stdhttp.StatusUnprocessableEntity {
			t.Fatalf("expected 422, got %d: %s", rec.Code, rec.Body.String())
		}
		var errEnv httpErrorEnvelope
		_ = json.Unmarshal(rec.Body.Bytes(), &errEnv)
		if errEnv.Error.Code != "validation_error" {
			t.Fatalf("expected validation_error code, got %s", errEnv.Error.Code)
		}
	})

	t.Run("POST /items with non-UUID Idempotency-Key returns 422", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/items", bytes.NewReader(bodyBytes))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", "not-a-valid-uuid")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != stdhttp.StatusUnprocessableEntity {
			t.Fatalf("expected 422, got %d: %s", rec.Code, rec.Body.String())
		}
		var errEnv httpErrorEnvelope
		_ = json.Unmarshal(rec.Body.Bytes(), &errEnv)
		if errEnv.Error.Code != "validation_error" {
			t.Fatalf("expected validation_error code, got %s", errEnv.Error.Code)
		}
	})

	t.Run("POST /claim without Idempotency-Key returns 422", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/items/11111111-1111-1111-1111-111111111111/claim", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != stdhttp.StatusUnprocessableEntity {
			t.Fatalf("expected 422, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /transition without Idempotency-Key returns 422", func(t *testing.T) {
		transBody, _ := json.Marshal(map[string]any{"to": "triaged"})
		req := httptest.NewRequest("POST", "/items/11111111-1111-1111-1111-111111111111/transition", bytes.NewReader(transBody))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("If-Match", `"1"`)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != stdhttp.StatusUnprocessableEntity {
			t.Fatalf("expected 422, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /approvals/:aid/decide without Idempotency-Key returns 422", func(t *testing.T) {
		decideBody, _ := json.Marshal(map[string]any{"decision": "approved", "reason": "Looks good"})
		req := httptest.NewRequest("POST", "/items/11111111-1111-1111-1111-111111111111/approvals/22222222-2222-2222-2222-222222222222/decide", bytes.NewReader(decideBody))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != stdhttp.StatusUnprocessableEntity {
			t.Fatalf("expected 422, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}

func TestHttpIdempotencyReplayAndHashMismatch(t *testing.T) {
	pool := testutil.NewTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	handler, authService := setupTestRouter(t, pool)
	teamID := insertTeam(t, ctx, pool, "Ops Team")
	user := insertUser(t, ctx, pool, "lead@example.test", "Lead User")
	addMember(t, ctx, pool, teamID, user.ID, "lead")
	token := loginUser(t, ctx, authService, user.Email)

	idempotencyKey := "a0000000-0000-0000-0000-000000000001"
	body1, _ := json.Marshal(map[string]any{
		"team_id":     teamID,
		"title":       "Incident A",
		"description": "First description",
		"priority":    2,
	})

	// 1. Initial request -> 201 Created
	req1 := httptest.NewRequest("POST", "/items", bytes.NewReader(body1))
	req1.Header.Set("Authorization", "Bearer "+token)
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set("Idempotency-Key", idempotencyKey)
	rec1 := httptest.NewRecorder()

	handler.ServeHTTP(rec1, req1)

	if rec1.Code != stdhttp.StatusCreated {
		t.Fatalf("first request expected 201, got %d: %s", rec1.Code, rec1.Body.String())
	}
	if rec1.Header().Get("Idempotent-Replay") == "true" {
		t.Fatalf("initial request should not have Idempotent-Replay: true")
	}

	var item1 model.WorkItem
	if err := json.Unmarshal(rec1.Body.Bytes(), &item1); err != nil {
		t.Fatalf("decode created item: %v", err)
	}

	// 2. Replay with same key and same body -> 201 Created, Idempotent-Replay: true
	req2 := httptest.NewRequest("POST", "/items", bytes.NewReader(body1))
	req2.Header.Set("Authorization", "Bearer "+token)
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Idempotency-Key", idempotencyKey)
	rec2 := httptest.NewRecorder()

	handler.ServeHTTP(rec2, req2)

	if rec2.Code != stdhttp.StatusCreated {
		t.Fatalf("replayed request expected 201, got %d: %s", rec2.Code, rec2.Body.String())
	}
	if rec2.Header().Get("Idempotent-Replay") != "true" {
		t.Fatalf("expected Idempotent-Replay: true, got %q", rec2.Header().Get("Idempotent-Replay"))
	}
	var item2 model.WorkItem
	if err := json.Unmarshal(rec2.Body.Bytes(), &item2); err != nil {
		t.Fatalf("decode replayed item: %v", err)
	}
	if item1.ID != item2.ID {
		t.Fatalf("expected replayed item ID %s, got %s", item1.ID, item2.ID)
	}

	// 3. Different body with same key -> 422 idempotency_key_reuse
	body2, _ := json.Marshal(map[string]any{
		"team_id":     teamID,
		"title":       "Incident B Differing Body",
		"description": "Different body",
		"priority":    2,
	})
	req3 := httptest.NewRequest("POST", "/items", bytes.NewReader(body2))
	req3.Header.Set("Authorization", "Bearer "+token)
	req3.Header.Set("Content-Type", "application/json")
	req3.Header.Set("Idempotency-Key", idempotencyKey)
	rec3 := httptest.NewRecorder()

	handler.ServeHTTP(rec3, req3)

	if rec3.Code != stdhttp.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for key reuse, got %d: %s", rec3.Code, rec3.Body.String())
	}
	var errEnv httpErrorEnvelope
	if err := json.Unmarshal(rec3.Body.Bytes(), &errEnv); err != nil {
		t.Fatalf("decode error envelope: %v", err)
	}
	if errEnv.Error.Code != "idempotency_key_reuse" {
		t.Fatalf("expected code idempotency_key_reuse, got %q", errEnv.Error.Code)
	}
}

func TestHttpIdempotencyUserIsolation(t *testing.T) {
	pool := testutil.NewTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	handler, authService := setupTestRouter(t, pool)
	teamID := insertTeam(t, ctx, pool, "Ops Team")

	userA := insertUser(t, ctx, pool, "user-a@example.test", "User A")
	userB := insertUser(t, ctx, pool, "user-b@example.test", "User B")
	addMember(t, ctx, pool, teamID, userA.ID, "lead")
	addMember(t, ctx, pool, teamID, userB.ID, "lead")

	tokenA := loginUser(t, ctx, authService, userA.Email)
	tokenB := loginUser(t, ctx, authService, userB.Email)

	sharedKey := "b0000000-0000-0000-0000-000000000002"
	bodyA, _ := json.Marshal(map[string]any{"team_id": teamID, "title": "User A Item", "priority": 1})
	bodyB, _ := json.Marshal(map[string]any{"team_id": teamID, "title": "User B Item", "priority": 2})

	// User A creates item with sharedKey
	reqA := httptest.NewRequest("POST", "/items", bytes.NewReader(bodyA))
	reqA.Header.Set("Authorization", "Bearer "+tokenA)
	reqA.Header.Set("Content-Type", "application/json")
	reqA.Header.Set("Idempotency-Key", sharedKey)
	recA := httptest.NewRecorder()
	handler.ServeHTTP(recA, reqA)
	if recA.Code != stdhttp.StatusCreated {
		t.Fatalf("User A create expected 201, got %d: %s", recA.Code, recA.Body.String())
	}
	var itemA model.WorkItem
	_ = json.Unmarshal(recA.Body.Bytes(), &itemA)

	// User B creates item with the SAME sharedKey
	// Must NOT replay User A's response, and must NOT fail with key reuse
	reqB := httptest.NewRequest("POST", "/items", bytes.NewReader(bodyB))
	reqB.Header.Set("Authorization", "Bearer "+tokenB)
	reqB.Header.Set("Content-Type", "application/json")
	reqB.Header.Set("Idempotency-Key", sharedKey)
	recB := httptest.NewRecorder()
	handler.ServeHTTP(recB, reqB)
	if recB.Code != stdhttp.StatusCreated {
		t.Fatalf("User B create with same key expected 201, got %d: %s", recB.Code, recB.Body.String())
	}
	if recB.Header().Get("Idempotent-Replay") == "true" {
		t.Fatalf("User B should not receive Idempotent-Replay: true")
	}
	var itemB model.WorkItem
	_ = json.Unmarshal(recB.Body.Bytes(), &itemB)

	if itemA.ID == itemB.ID {
		t.Fatalf("User B got User A's item! Idempotency key must be scoped to user_id")
	}
	if itemB.Title != "User B Item" {
		t.Fatalf("expected User B's item title, got %s", itemB.Title)
	}
}
