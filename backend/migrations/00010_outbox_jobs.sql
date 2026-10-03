-- +goose Up
CREATE TABLE outbox_jobs (
    id bigserial PRIMARY KEY,
    type text NOT NULL,
    dedupe_key text NOT NULL UNIQUE,
    payload jsonb NOT NULL,
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'processing', 'completed', 'dead_letter')),
    attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    max_attempts integer NOT NULL DEFAULT 5 CHECK (max_attempts > 0),
    run_at timestamptz NOT NULL DEFAULT now(),
    locked_at timestamptz,
    last_error text,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX outbox_jobs_status_run_at_idx ON outbox_jobs (status, run_at);

ALTER TABLE outbox_jobs ENABLE ROW LEVEL SECURITY;

-- +goose Down
DROP TABLE outbox_jobs;
