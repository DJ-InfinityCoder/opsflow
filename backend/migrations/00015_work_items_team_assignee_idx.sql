-- +goose Up
CREATE INDEX work_items_team_assignee_id_idx
    ON work_items (team_id, assignee_id);

-- +goose Down
DROP INDEX work_items_team_assignee_id_idx;
