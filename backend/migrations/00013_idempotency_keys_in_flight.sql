-- +goose Up
ALTER TABLE idempotency_keys ALTER COLUMN status_code DROP NOT NULL;
ALTER TABLE idempotency_keys ALTER COLUMN response_body DROP NOT NULL;
CREATE INDEX IF NOT EXISTS idempotency_keys_created_at_idx ON idempotency_keys (created_at);

-- +goose Down
DROP INDEX IF EXISTS idempotency_keys_created_at_idx;
ALTER TABLE idempotency_keys ALTER COLUMN status_code SET NOT NULL;
ALTER TABLE idempotency_keys ALTER COLUMN response_body SET NOT NULL;
