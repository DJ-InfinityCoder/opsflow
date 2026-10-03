-- +goose Up
-- 1. Allow 'failed' in outbox_jobs status constraint
ALTER TABLE outbox_jobs DROP CONSTRAINT IF EXISTS outbox_jobs_status_check;
ALTER TABLE outbox_jobs ADD CONSTRAINT outbox_jobs_status_check 
    CHECK (status IN ('pending', 'processing', 'completed', 'dead_letter', 'failed'));

-- 2. Allow notifications without source_event_id if needed, and add dedupe_key
ALTER TABLE notifications ALTER COLUMN source_event_id DROP NOT NULL;
ALTER TABLE notifications ADD COLUMN IF NOT EXISTS dedupe_key text UNIQUE;

-- 3. Add username to users table with default from email prefix
ALTER TABLE users ADD COLUMN IF NOT EXISTS username text;
UPDATE users SET username = split_part(email, '@', 1) WHERE username IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS users_username_lower_idx ON users (lower(username));

-- 4. Indexes for comments and notifications
CREATE INDEX IF NOT EXISTS comments_item_id_created_at_idx ON comments (item_id, created_at ASC);
CREATE INDEX IF NOT EXISTS notifications_user_id_created_at_idx ON notifications (user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS item_events_created_at_idx ON item_events (created_at DESC, id DESC);

-- +goose Down
DROP INDEX IF EXISTS item_events_created_at_idx;
DROP INDEX IF EXISTS notifications_user_id_created_at_idx;
DROP INDEX IF EXISTS comments_item_id_created_at_idx;
DROP INDEX IF EXISTS users_username_lower_idx;
ALTER TABLE users DROP COLUMN IF EXISTS username;
ALTER TABLE notifications DROP COLUMN IF EXISTS dedupe_key;
ALTER TABLE notifications ALTER COLUMN source_event_id SET NOT NULL;
ALTER TABLE outbox_jobs DROP CONSTRAINT IF EXISTS outbox_jobs_status_check;
ALTER TABLE outbox_jobs ADD CONSTRAINT outbox_jobs_status_check 
    CHECK (status IN ('pending', 'processing', 'completed', 'dead_letter'));
