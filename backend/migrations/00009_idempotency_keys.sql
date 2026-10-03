-- +goose Up
CREATE TABLE idempotency_keys (
    user_id uuid NOT NULL REFERENCES users(id),
    key text NOT NULL,
    endpoint text NOT NULL,
    request_hash text NOT NULL,
    status_code integer NOT NULL,
    response_body jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, key)
);

ALTER TABLE idempotency_keys ENABLE ROW LEVEL SECURITY;

-- +goose Down
DROP TABLE idempotency_keys;
