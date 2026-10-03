-- Run with psql against the seeded Supabase database after applying migrations.
-- Uses one seeded team to inspect the list endpoint's sort and page plan.
EXPLAIN (ANALYZE, BUFFERS)
SELECT id, team_id, title, status, priority, assignee_id, due_at, version, updated_at
FROM work_items
WHERE team_id = (SELECT id FROM teams ORDER BY name LIMIT 1)
ORDER BY priority ASC, updated_at DESC, id ASC
LIMIT 50;

-- Full-text-search variant used when the `q` filter is present.
EXPLAIN (ANALYZE, BUFFERS)
SELECT id, team_id, title, status, priority, assignee_id, due_at, version, updated_at
FROM work_items
WHERE team_id = (SELECT id FROM teams ORDER BY name LIMIT 1)
  AND search @@ websearch_to_tsquery('english', 'payment investigation')
ORDER BY priority ASC, updated_at DESC, id ASC
LIMIT 50;
