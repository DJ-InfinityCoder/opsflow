-- +goose Up
CREATE TABLE item_events (
    id bigserial PRIMARY KEY,
    item_id uuid NOT NULL REFERENCES work_items(id),
    actor_id uuid NOT NULL REFERENCES users(id),
    type text NOT NULL,
    field text,
    old_value jsonb,
    new_value jsonb,
    reason text,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX item_events_item_id_id_idx ON item_events (item_id, id);

-- +goose StatementBegin
CREATE FUNCTION prevent_item_events_mutation() RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'item_events is append-only'
        USING ERRCODE = '55000';
    RETURN OLD;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER item_events_append_only
    BEFORE UPDATE OR DELETE ON item_events
    FOR EACH ROW EXECUTE FUNCTION prevent_item_events_mutation();

ALTER TABLE item_events ENABLE ROW LEVEL SECURITY;

-- +goose Down
DROP TABLE item_events;
DROP FUNCTION prevent_item_events_mutation();
