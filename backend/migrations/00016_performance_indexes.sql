-- +goose Up
CREATE INDEX IF NOT EXISTS work_items_team_assignee_priority_updated_id_idx
    ON work_items (team_id, assignee_id, priority ASC, updated_at DESC, id ASC);

CREATE INDEX IF NOT EXISTS work_items_updated_at_desc_idx
    ON work_items (updated_at DESC);

-- +goose Down
DROP INDEX IF EXISTS work_items_team_assignee_priority_updated_id_idx;
DROP INDEX IF EXISTS work_items_updated_at_desc_idx;
