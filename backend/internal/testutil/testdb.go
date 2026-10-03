package testutil

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"

	"opsflow/backend/internal/db"
)

var (
	sharedAdminMu   sync.Mutex
	sharedAdminPool *pgxpool.Pool
)

func isConnectionLimit(err error) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "53300" {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "53300") ||
		strings.Contains(msg, "remaining connection slots") ||
		strings.Contains(msg, "too many clients")
}

func getAdminPool(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	sharedAdminMu.Lock()
	defer sharedAdminMu.Unlock()

	if sharedAdminPool != nil {
		return sharedAdminPool, nil
	}

	adminConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse admin db url: %w", err)
	}
	adminConfig.MaxConns = 1
	adminConfig.ConnConfig.ConnectTimeout = 10 * time.Second

	for attempt := 0; attempt < 6; attempt++ {
		p, err := pgxpool.NewWithConfig(ctx, adminConfig)
		if err != nil {
			if isConnectionLimit(err) && attempt < 5 {
				time.Sleep(time.Duration(attempt+1) * 350 * time.Millisecond)
				continue
			}
			return nil, fmt.Errorf("create admin pool: %w", err)
		}
		if err := p.Ping(ctx); err != nil {
			p.Close()
			if isConnectionLimit(err) && attempt < 5 {
				time.Sleep(time.Duration(attempt+1) * 350 * time.Millisecond)
				continue
			}
			return nil, fmt.Errorf("ping admin pool: %w", err)
		}
		sharedAdminPool = p
		break
	}
	return sharedAdminPool, nil
}

func NewTestDB(t testing.TB) *pgxpool.Pool {
	return NewTestDBWithMaxConns(t, 2)
}

func NewTestDBWithMaxConns(t testing.TB, maxConns int32) *pgxpool.Pool {
	t.Helper()
	_ = godotenv.Load("../../.env", "../.env", ".env")
	databaseURL := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if databaseURL == "" {
		databaseURL = strings.TrimSpace(os.Getenv("DATABASE_URL"))
	}
	if databaseURL == "" {
		t.Fatal("TEST_DATABASE_URL or DATABASE_URL is required for database tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	adminPool, err := getAdminPool(ctx, databaseURL)
	if err != nil {
		t.Fatalf("get admin pool: %v", err)
	}

	schema, err := newSchemaName()
	if err != nil {
		t.Fatalf("generate test schema name: %v", err)
	}
	for attempt := 0; attempt < 6; attempt++ {
		if _, err := adminPool.Exec(ctx, "CREATE SCHEMA "+pgx.Identifier{schema}.Sanitize()); err != nil {
			if isConnectionLimit(err) && attempt < 5 {
				time.Sleep(time.Duration(attempt+1) * 350 * time.Millisecond)
				continue
			}
			t.Fatalf("create isolated test schema %s: %v", schema, err)
		}
		break
	}

	schemaConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		_, _ = adminPool.Exec(context.Background(), "DROP SCHEMA "+pgx.Identifier{schema}.Sanitize()+" CASCADE")
		t.Fatalf("parse test database URL for isolated schema: %v", err)
	}
	if maxConns < 1 {
		maxConns = 2
	}
	schemaConfig.MaxConns = maxConns
	schemaConfig.MinConns = 0
	schemaConfig.MaxConnIdleTime = 2 * time.Second
	schemaConfig.MaxConnLifetime = 30 * time.Second
	schemaConfig.ConnConfig.ConnectTimeout = 10 * time.Second
	if schemaConfig.ConnConfig.RuntimeParams == nil {
		schemaConfig.ConnConfig.RuntimeParams = make(map[string]string)
	}
	schemaConfig.ConnConfig.RuntimeParams["search_path"] = schema

	pool, err := pgxpool.NewWithConfig(ctx, schemaConfig)
	if err != nil {
		_, _ = adminPool.Exec(context.Background(), "DROP SCHEMA "+pgx.Identifier{schema}.Sanitize()+" CASCADE")
		t.Fatalf("create pool pinned to test schema: %v", err)
	}
	for attempt := 0; attempt < 6; attempt++ {
		if err := pool.Ping(ctx); err != nil {
			if isConnectionLimit(err) && attempt < 5 {
				time.Sleep(time.Duration(attempt+1) * 350 * time.Millisecond)
				continue
			}
			pool.Close()
			_, _ = adminPool.Exec(context.Background(), "DROP SCHEMA "+pgx.Identifier{schema}.Sanitize()+" CASCADE")
			t.Fatalf("connect to isolated test schema: %v", err)
		}
		break
	}
	if err := db.RunMigrations(ctx, pool, "up"); err != nil {
		pool.Close()
		_, _ = adminPool.Exec(context.Background(), "DROP SCHEMA "+pgx.Identifier{schema}.Sanitize()+" CASCADE")
		t.Fatalf("apply migrations to isolated test schema %s: %v", schema, err)
	}

	t.Cleanup(func() {
		pool.Close()
		time.Sleep(100 * time.Millisecond)
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		for attempt := 0; attempt < 6; attempt++ {
			if _, err := adminPool.Exec(cleanupCtx, "DROP SCHEMA "+pgx.Identifier{schema}.Sanitize()+" CASCADE"); err != nil {
				if isConnectionLimit(err) && attempt < 5 {
					time.Sleep(time.Duration(attempt+1) * 350 * time.Millisecond)
					continue
				}
				t.Errorf("drop isolated test schema %s: %v", schema, err)
			}
			break
		}
	})

	return pool
}

func newSchemaName() (string, error) {
	var randomBytes [8]byte
	if _, err := rand.Read(randomBytes[:]); err != nil {
		return "", fmt.Errorf("read random bytes: %w", err)
	}
	return "test_" + hex.EncodeToString(randomBytes[:]), nil
}
