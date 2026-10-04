package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"opsflow/backend/internal/config"
	"opsflow/backend/internal/db"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := db.Connect(ctx, cfg.DatabaseURL, cfg.DBMaxConns, cfg.DBConnectTimeout)
	if err != nil {
		return err
	}
	defer pool.Close()

	queries := []struct {
		name string
		sql  string
	}{
		{
			name: "list_by_view",
			sql: `SELECT id, team_id, title, status, priority, assignee_id, due_at, version, updated_at
FROM work_items
WHERE team_id = (SELECT id FROM teams ORDER BY name LIMIT 1)
ORDER BY priority ASC, updated_at DESC, id ASC
LIMIT 50;`,
		},
		{
			name: "assigned_to_me_view",
			sql: `SELECT id, team_id, title, status, priority, assignee_id, due_at, version, updated_at
FROM work_items
WHERE team_id = (SELECT id FROM teams ORDER BY name LIMIT 1)
  AND assignee_id = (SELECT id FROM users WHERE email = 'marcus@opsflow.local')
ORDER BY priority ASC, updated_at DESC, id ASC
LIMIT 50;`,
		},
		{
			name: "full_text_search",
			sql: `SELECT id, team_id, title, status, priority, assignee_id, due_at, version, updated_at
FROM work_items
WHERE team_id = (SELECT id FROM teams ORDER BY name LIMIT 1)
  AND search @@ websearch_to_tsquery('english', 'payment investigation')
ORDER BY priority ASC, updated_at DESC, id ASC
LIMIT 50;`,
		},
		{
			name: "view_counts",
			sql: `SELECT
  count(*) FILTER (WHERE assignee_id = (SELECT id FROM users WHERE email = 'marcus@opsflow.local')) AS assigned_to_me,
  count(*) FILTER (WHERE assignee_id IS NULL AND status IN ('new', 'triaged')) AS team_unassigned,
  count(*) FILTER (WHERE priority <= 2 AND status NOT IN ('resolved', 'closed')) AS urgent,
  count(*) FILTER (WHERE status = 'pending_approval') AS waiting_approval,
  count(*) AS all_items
FROM work_items
WHERE team_id = (SELECT id FROM teams ORDER BY name LIMIT 1);`,
		},
		{
			name: "events_by_item",
			sql: `SELECT id, item_id, type, created_at
FROM item_events
WHERE item_id = (SELECT id FROM work_items ORDER BY updated_at DESC LIMIT 1)
ORDER BY id DESC
LIMIT 25;`,
		},
	}

	for _, query := range queries {
		explainSQL := "EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) " + query.sql
		rows, err := pool.Query(ctx, explainSQL)
		if err != nil {
			return fmt.Errorf("query %s: %w", query.name, err)
		}

		var payload []byte
		if !rows.Next() {
			rows.Close()
			return fmt.Errorf("query %s: no explain results", query.name)
		}
		if err := rows.Scan(&payload); err != nil {
			rows.Close()
			return fmt.Errorf("scan explain %s: %w", query.name, err)
		}
		rows.Close()

		var parsed []map[string]any
		if err := json.Unmarshal(payload, &parsed); err != nil {
			return fmt.Errorf("parse explain %s: %w", query.name, err)
		}
		if len(parsed) == 0 {
			return fmt.Errorf("query %s: empty explain result", query.name)
		}

		plan := parsed[0]["Plan"].(map[string]any)
		actual := plan["Actual Total Time"]
		rowsOut := plan["Actual Rows"]
		loops := plan["Actual Loops"]
		sharedHitBlocks := valueFor(plan, "Shared Hit Blocks")
		sharedReadBlocks := valueFor(plan, "Shared Read Blocks")

		fmt.Printf("=== %s ===\n", strings.ToUpper(strings.ReplaceAll(query.name, "_", " ")))
		fmt.Printf("actual_total_time=%v actual_rows=%v actual_loops=%v shared_hit_blocks=%v shared_read_blocks=%v\n",
			actual, rowsOut, loops, sharedHitBlocks, sharedReadBlocks)
		fmt.Printf("plan_node=%s\n", plan["Node Type"])
		printSummary(plan)
		fmt.Println()
	}

	return nil
}

func valueFor(plan map[string]any, key string) any {
	if v, ok := plan[key]; ok {
		return v
	}
	if child, ok := plan["Plans"].([]any); ok {
		for _, p := range child {
			if m, ok := p.(map[string]any); ok {
				if v, ok := m[key]; ok {
					return v
				}
				if r := valueFor(m, key); r != nil {
					return r
				}
			}
		}
	}
	return nil
}

func printSummary(plan map[string]any) {
	if v, ok := plan["Node Type"]; ok {
		fmt.Printf("node_type=%v |", v)
	}
	if v, ok := plan["Index Name"]; ok {
		fmt.Printf(" index=%v |", v)
	}
	if v, ok := plan["Relation Name"]; ok {
		fmt.Printf(" relation=%v |", v)
	}
	if v, ok := plan["Actual Total Time"]; ok {
		fmt.Printf(" actual_time=%v |", v)
	}
	if v, ok := plan["Actual Rows"]; ok {
		fmt.Printf(" rows=%v |", v)
	}
	if v, ok := plan["Total Cost"]; ok {
		fmt.Printf(" cost=%v |", v)
	}
	if child, ok := plan["Plans"].([]any); ok && len(child) > 0 {
		fmt.Printf(" children=%d", len(child))
	}
	fmt.Println()
	if child, ok := plan["Plans"].([]any); ok {
		for _, c := range child {
			if m, ok := c.(map[string]any); ok {
				printSummary(m)
			}
		}
	}
}
