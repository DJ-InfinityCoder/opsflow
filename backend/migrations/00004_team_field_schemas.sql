-- +goose Up
CREATE TABLE team_field_schemas (
    team_id uuid NOT NULL REFERENCES teams(id),
    field_key text NOT NULL,
    label text NOT NULL,
    type text NOT NULL,
    required boolean NOT NULL DEFAULT false,
    options jsonb NOT NULL DEFAULT '[]'::jsonb,
    PRIMARY KEY (team_id, field_key)
);

ALTER TABLE team_field_schemas ENABLE ROW LEVEL SECURITY;

-- +goose Down
DROP TABLE team_field_schemas;
