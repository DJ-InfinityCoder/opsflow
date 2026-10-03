-- +goose Up
CREATE TABLE comments (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    item_id uuid NOT NULL REFERENCES work_items(id),
    author_id uuid NOT NULL REFERENCES users(id),
    body text NOT NULL,
    mentions uuid[] NOT NULL DEFAULT '{}',
    created_at timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE comments ENABLE ROW LEVEL SECURITY;

-- +goose Down
DROP TABLE comments;
