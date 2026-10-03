package repo

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"opsflow/backend/internal/model"
)

var (
	ErrItemNotFound           = errors.New("work item not found")
	ErrApprovalNotFound       = errors.New("approval not found")
	ErrApprovalAlreadyDecided = errors.New("approval already decided")
)

type ItemRepository struct {
	db DBTX
}

func NewItemRepository(db DBTX) *ItemRepository {
	return &ItemRepository{db: db}
}

const itemColumns = `id::text, team_id::text, title, description, status, priority,
created_by::text, assignee_id::text, custom_fields, due_at, version, created_at, updated_at, resolved_at`

func (r *ItemRepository) Get(ctx context.Context, id string) (model.WorkItem, error) {
	return scanWorkItem(r.db.QueryRow(ctx, `SELECT `+itemColumns+` FROM work_items WHERE id = $1::uuid`, id))
}

func (r *ItemRepository) GetForUpdate(ctx context.Context, id string) (model.WorkItem, error) {
	return scanWorkItem(r.db.QueryRow(ctx, `SELECT `+itemColumns+` FROM work_items WHERE id = $1::uuid FOR UPDATE`, id))
}

func (r *ItemRepository) Insert(ctx context.Context, item model.WorkItem) (model.WorkItem, error) {
	customFields, err := json.Marshal(item.CustomFields)
	if err != nil {
		return model.WorkItem{}, fmt.Errorf("encode custom fields: %w", err)
	}
	query := `INSERT INTO work_items (team_id, title, description, status, priority, created_by, custom_fields, due_at)
		VALUES ($1::uuid, $2, $3, $4, $5, $6::uuid, $7::jsonb, $8)
		RETURNING ` + itemColumns
	created, err := scanWorkItem(r.db.QueryRow(ctx, query,
		item.TeamID, item.Title, item.Description, item.Status, item.Priority, item.CreatedBy, customFields, item.DueAt,
	))
	if err != nil {
		return model.WorkItem{}, fmt.Errorf("insert work item: %w", err)
	}
	return created, nil
}

func (r *ItemRepository) Update(ctx context.Context, item model.WorkItem, expectedVersion int, requireUnassigned bool) (model.WorkItem, error) {
	customFields, err := json.Marshal(item.CustomFields)
	if err != nil {
		return model.WorkItem{}, fmt.Errorf("encode custom fields: %w", err)
	}
	claimCondition := ""
	if requireUnassigned {
		claimCondition = " AND assignee_id IS NULL"
	}
	query := `UPDATE work_items
		SET title = $2, description = $3, status = $4, priority = $5, assignee_id = $6::uuid,
		    custom_fields = $7::jsonb, due_at = $8, resolved_at = $9,
		    version = version + 1, updated_at = now()
		WHERE id = $1::uuid AND version = $10` + claimCondition + `
		RETURNING ` + itemColumns
	updated, err := scanWorkItem(r.db.QueryRow(ctx, query,
		item.ID, item.Title, item.Description, item.Status, item.Priority, item.AssigneeID,
		customFields, item.DueAt, item.ResolvedAt, expectedVersion,
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.WorkItem{}, ErrItemNotFound
		}
		return model.WorkItem{}, fmt.Errorf("update work item: %w", err)
	}
	return updated, nil
}

func (r *ItemRepository) InsertEvent(ctx context.Context, event model.ItemEvent) error {
	_, err := r.InsertEventWithID(ctx, event)
	return err
}

func (r *ItemRepository) InsertEventWithID(ctx context.Context, event model.ItemEvent) (int64, error) {
	var id int64
	err := r.db.QueryRow(ctx, `
		INSERT INTO item_events (item_id, actor_id, type, field, old_value, new_value, reason)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5::jsonb, $6::jsonb, $7)
		RETURNING id
	`, event.ItemID, event.ActorID, event.Type, event.Field, nullableJSON(event.OldValue), nullableJSON(event.NewValue), event.Reason).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("insert item event: %w", err)
	}
	return id, nil
}

func (r *ItemRepository) InsertOutbox(ctx context.Context, jobType, dedupeKey string, payload []byte) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO outbox_jobs (type, dedupe_key, payload)
		VALUES ($1, $2, $3::jsonb)
	`, jobType, dedupeKey, payload)
	if err != nil {
		return fmt.Errorf("insert outbox job: %w", err)
	}
	return nil
}

func nullableJSON(value json.RawMessage) any {
	if len(value) == 0 {
		return nil
	}
	return []byte(value)
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanWorkItem(row rowScanner) (model.WorkItem, error) {
	var item model.WorkItem
	var customFields []byte
	err := row.Scan(
		&item.ID, &item.TeamID, &item.Title, &item.Description, &item.Status, &item.Priority,
		&item.CreatedBy, &item.AssigneeID, &customFields, &item.DueAt, &item.Version,
		&item.CreatedAt, &item.UpdatedAt, &item.ResolvedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.WorkItem{}, ErrItemNotFound
	}
	if err != nil {
		return model.WorkItem{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(customFields))
	decoder.UseNumber()
	if err := decoder.Decode(&item.CustomFields); err != nil {
		return model.WorkItem{}, fmt.Errorf("decode work item custom fields: %w", err)
	}
	if item.CustomFields == nil {
		item.CustomFields = map[string]any{}
	}
	return item, nil
}

func (r *ItemRepository) Claim(ctx context.Context, itemID, assigneeID string) (model.WorkItem, bool, error) {
	query := `UPDATE work_items
		SET assignee_id = $2::uuid,
		    version = version + 1,
		    updated_at = now()
		WHERE id = $1::uuid
		  AND assignee_id IS NULL
		  AND status IN ('new', 'triaged')
		RETURNING ` + itemColumns
	updated, err := scanWorkItem(r.db.QueryRow(ctx, query, itemID, assigneeID))
	if err != nil {
		if errors.Is(err, ErrItemNotFound) {
			return model.WorkItem{}, false, nil
		}
		return model.WorkItem{}, false, fmt.Errorf("atomic claim work item: %w", err)
	}
	return updated, true, nil
}

func (r *ItemRepository) Assign(ctx context.Context, itemID, assigneeID string, expectedVersion int) (model.WorkItem, error) {
	query := `UPDATE work_items
		SET assignee_id = $2::uuid,
		    version = version + 1,
		    updated_at = now()
		WHERE id = $1::uuid AND version = $3
		RETURNING ` + itemColumns
	updated, err := scanWorkItem(r.db.QueryRow(ctx, query, itemID, assigneeID, expectedVersion))
	if err != nil {
		if errors.Is(err, ErrItemNotFound) {
			return model.WorkItem{}, ErrItemNotFound
		}
		return model.WorkItem{}, fmt.Errorf("assign work item: %w", err)
	}
	return updated, nil
}

func (r *ItemRepository) Transition(ctx context.Context, itemID, targetStatus string, expectedVersion int, resolvedAt *time.Time) (model.WorkItem, error) {
	query := `UPDATE work_items
		SET status = $2,
		    resolved_at = $3,
		    version = version + 1,
		    updated_at = now()
		WHERE id = $1::uuid AND version = $4
		RETURNING ` + itemColumns
	updated, err := scanWorkItem(r.db.QueryRow(ctx, query, itemID, targetStatus, resolvedAt, expectedVersion))
	if err != nil {
		if errors.Is(err, ErrItemNotFound) {
			return model.WorkItem{}, ErrItemNotFound
		}
		return model.WorkItem{}, fmt.Errorf("transition work item: %w", err)
	}
	return updated, nil
}

func (r *ItemRepository) CreateApproval(ctx context.Context, itemID, requestedBy string, reason *string) (model.Approval, error) {
	query := `INSERT INTO approvals (item_id, requested_by, reason)
		VALUES ($1::uuid, $2::uuid, $3)
		RETURNING id::text, item_id::text, requested_by::text, decided_by::text, decision, reason, created_at, decided_at`
	return scanApproval(r.db.QueryRow(ctx, query, itemID, requestedBy, reason))
}

func (r *ItemRepository) GetApproval(ctx context.Context, approvalID, itemID string) (model.Approval, error) {
	query := `SELECT id::text, item_id::text, requested_by::text, decided_by::text, decision, reason, created_at, decided_at
		FROM approvals
		WHERE id = $1::uuid AND item_id = $2::uuid`
	return scanApproval(r.db.QueryRow(ctx, query, approvalID, itemID))
}

func (r *ItemRepository) GetPendingApproval(ctx context.Context, itemID string) (*model.Approval, error) {
	query := `SELECT id::text, item_id::text, requested_by::text, decided_by::text, decision, reason, created_at, decided_at
		FROM approvals
		WHERE item_id = $1::uuid AND decision IS NULL
		ORDER BY created_at DESC LIMIT 1`
	appr, err := scanApproval(r.db.QueryRow(ctx, query, itemID))
	if err != nil {
		if errors.Is(err, ErrApprovalNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("get pending approval: %w", err)
	}
	return &appr, nil
}

func (r *ItemRepository) DecideApproval(ctx context.Context, approvalID, decidedBy, decision, reason string) (model.Approval, error) {
	query := `UPDATE approvals
		SET decided_by = $2::uuid, decision = $3, reason = $4, decided_at = now()
		WHERE id = $1::uuid AND decision IS NULL
		RETURNING id::text, item_id::text, requested_by::text, decided_by::text, decision, reason, created_at, decided_at`
	appr, err := scanApproval(r.db.QueryRow(ctx, query, approvalID, decidedBy, decision, reason))
	if err != nil {
		if errors.Is(err, ErrApprovalNotFound) {
			var exists bool
			_ = r.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM approvals WHERE id = $1::uuid)`, approvalID).Scan(&exists)
			if exists {
				return model.Approval{}, ErrApprovalAlreadyDecided
			}
			return model.Approval{}, ErrApprovalNotFound
		}
		return model.Approval{}, fmt.Errorf("decide approval: %w", err)
	}
	return appr, nil
}

func (r *ItemRepository) HasApprovedApproval(ctx context.Context, itemID string) (bool, error) {
	var exists bool
	err := r.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM approvals WHERE item_id = $1::uuid AND decision = 'approved')`, itemID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check approved approval: %w", err)
	}
	return exists, nil
}

func (r *ItemRepository) UpdateItemStatus(ctx context.Context, itemID, targetStatus string, resolvedAt *time.Time) (model.WorkItem, error) {
	query := `UPDATE work_items
		SET status = $2,
		    resolved_at = $3,
		    version = version + 1,
		    updated_at = now()
		WHERE id = $1::uuid
		RETURNING ` + itemColumns
	updated, err := scanWorkItem(r.db.QueryRow(ctx, query, itemID, targetStatus, resolvedAt))
	if err != nil {
		if errors.Is(err, ErrItemNotFound) {
			return model.WorkItem{}, ErrItemNotFound
		}
		return model.WorkItem{}, fmt.Errorf("update item status: %w", err)
	}
	return updated, nil
}

func scanApproval(row rowScanner) (model.Approval, error) {
	var a model.Approval
	err := row.Scan(&a.ID, &a.ItemID, &a.RequestedBy, &a.DecidedBy, &a.Decision, &a.Reason, &a.CreatedAt, &a.DecidedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Approval{}, ErrApprovalNotFound
	}
	if err != nil {
		return model.Approval{}, err
	}
	return a, nil
}
