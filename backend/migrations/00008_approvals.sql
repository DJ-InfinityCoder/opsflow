-- +goose Up
CREATE TABLE approvals (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    item_id uuid NOT NULL REFERENCES work_items(id),
    requested_by uuid NOT NULL REFERENCES users(id),
    decided_by uuid REFERENCES users(id),
    decision text CHECK (decision IS NULL OR decision IN ('approved', 'rejected')),
    reason text,
    created_at timestamptz NOT NULL DEFAULT now(),
    decided_at timestamptz
);

ALTER TABLE approvals ENABLE ROW LEVEL SECURITY;

-- +goose Down
DROP TABLE approvals;
