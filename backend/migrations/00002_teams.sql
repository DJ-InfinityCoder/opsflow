-- +goose Up
CREATE TABLE teams (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL UNIQUE
);

ALTER TABLE teams ENABLE ROW LEVEL SECURITY;

-- +goose Down
DROP TABLE teams;
