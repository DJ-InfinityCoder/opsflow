package db_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"opsflow/backend/internal/testutil"
)

func TestItemEventsAreAppendOnly(t *testing.T) {
	pool := testutil.NewTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var userID, teamID, itemID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (email, name) VALUES ('audit@example.test', 'Audit Test') RETURNING id::text`,
	).Scan(&userID); err != nil {
		t.Fatalf("insert test user: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO teams (name) VALUES ('Audit Test Team') RETURNING id::text`,
	).Scan(&teamID); err != nil {
		t.Fatalf("insert test team: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO work_items (team_id, title, description, status, priority, created_by)
		 VALUES ($1, 'Audit Test Item', 'Append-only test', 'new', 1, $2) RETURNING id::text`,
		teamID, userID,
	).Scan(&itemID); err != nil {
		t.Fatalf("insert test work item: %v", err)
	}
	var eventID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO item_events (item_id, actor_id, type, reason)
		 VALUES ($1, $2, 'created', 'test') RETURNING id`,
		itemID, userID,
	).Scan(&eventID); err != nil {
		t.Fatalf("insert test item event: %v", err)
	}

	operations := []struct {
		name  string
		query string
	}{
		{name: "update", query: `UPDATE item_events SET reason = 'changed' WHERE id = $1`},
		{name: "delete", query: `DELETE FROM item_events WHERE id = $1`},
	}
	for _, operation := range operations {
		t.Run(operation.name, func(t *testing.T) {
			_, err := pool.Exec(ctx, operation.query, eventID)
			if err == nil {
				t.Fatal("expected append-only trigger to reject mutation")
			}
			if !strings.Contains(err.Error(), "item_events is append-only") {
				t.Fatalf("expected append-only trigger error, got: %v", err)
			}
		})
	}
}
