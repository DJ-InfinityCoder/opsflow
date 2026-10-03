package testutil

import (
	"context"
	"testing"
	"time"
)

var (
	firstTestReady  = make(chan struct{}, 1)
	secondTestReady = make(chan struct{}, 1)
)

func TestNewTestDBIsolationFirst(t *testing.T) {
	t.Parallel()
	assertIsolatedSchema(t, "first", firstTestReady, secondTestReady)
}

func TestNewTestDBIsolationSecond(t *testing.T) {
	t.Parallel()
	assertIsolatedSchema(t, "second", secondTestReady, firstTestReady)
}

func assertIsolatedSchema(t *testing.T, value string, ready, peerReady chan struct{}) {
	t.Helper()
	pool := NewTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if _, err := pool.Exec(ctx, `CREATE TABLE isolation_probe (value text NOT NULL)`); err != nil {
		t.Fatalf("create probe table: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO isolation_probe (value) VALUES ($1)`, value); err != nil {
		t.Fatalf("insert probe row: %v", err)
	}

	ready <- struct{}{}
	select {
	case <-peerReady:
	case <-ctx.Done():
		t.Fatal("parallel schema isolation test timed out waiting for its peer")
	}

	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM isolation_probe`).Scan(&count); err != nil {
		t.Fatalf("query probe table: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected one row in this test's isolated schema, got %d", count)
	}
}
