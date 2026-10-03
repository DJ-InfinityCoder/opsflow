-- +goose Up
CREATE TABLE work_items (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id uuid NOT NULL REFERENCES teams(id),
    title text NOT NULL,
    description text NOT NULL,
    status text NOT NULL CHECK (status IN ('new', 'triaged', 'in_progress', 'pending_approval', 'resolved', 'closed')),
    priority smallint NOT NULL CHECK (priority BETWEEN 1 AND 4),
    created_by uuid NOT NULL REFERENCES users(id),
    assignee_id uuid REFERENCES users(id),
    custom_fields jsonb NOT NULL DEFAULT '{}'::jsonb,
    due_at timestamptz,
    version integer NOT NULL DEFAULT 1,
    search tsvector GENERATED ALWAYS AS (to_tsvector('english', title || ' ' || description)) STORED,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    resolved_at timestamptz
);

CREATE INDEX work_items_team_status_priority_updated_id_idx
    ON work_items (team_id, status, priority, updated_at DESC, id);
CREATE INDEX work_items_unassigned_team_created_idx
    ON work_items (team_id, created_at)
    WHERE assignee_id IS NULL AND status IN ('new', 'triaged');
CREATE INDEX work_items_assignee_status_idx
    ON work_items (assignee_id, status);
CREATE INDEX work_items_search_gin_idx
    ON work_items USING gin (search);

ALTER TABLE work_items ENABLE ROW LEVEL SECURITY;

-- +goose Down
DROP TABLE work_items;
