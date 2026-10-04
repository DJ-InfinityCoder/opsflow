-- +goose Up
INSERT INTO approvals (item_id, requested_by)
SELECT wi.id, wi.created_by
FROM work_items wi
WHERE wi.status = 'pending_approval'
  AND NOT EXISTS (
      SELECT 1
      FROM approvals a
      WHERE a.item_id = wi.id
        AND a.decision IS NULL
  );

-- +goose Down
-- Backfilled approval records are retained because decisions may have been made
-- against them after this migration was applied.
