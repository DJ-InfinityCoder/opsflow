-- +goose Up
CREATE INDEX work_items_team_priority_updated_id_idx
    ON work_items (team_id, priority ASC, updated_at DESC, id ASC);

-- +goose Down
DROP INDEX work_items_team_priority_updated_id_idx;
