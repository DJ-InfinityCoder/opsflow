-- +goose Up
CREATE TABLE notifications (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users(id),
    item_id uuid NOT NULL REFERENCES work_items(id),
    type text NOT NULL,
    source_event_id bigint NOT NULL REFERENCES item_events(id),
    read_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (user_id, source_event_id)
);

ALTER TABLE notifications ENABLE ROW LEVEL SECURITY;

-- +goose Down
DROP TABLE notifications;
