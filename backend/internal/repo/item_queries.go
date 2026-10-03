package repo

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"opsflow/backend/internal/model"
)

type ItemCursor struct {
	Priority  int16     `json:"p"`
	UpdatedAt time.Time `json:"u"`
	ID        string    `json:"i"`
}

type ItemListFilter struct {
	TeamIDs    []string
	UserID     string
	View       string
	TeamID     string
	Status     string
	Priority   int16
	AssigneeID string
	Query      string
	Cursor     *ItemCursor
	Limit      int
}

type ItemPage struct {
	Items      []model.WorkItem
	HasMore    bool
	NextCursor string
}

type ViewCounts struct {
	AssignedToMe    int64 `json:"assigned_to_me"`
	TeamUnassigned  int64 `json:"team_unassigned"`
	Urgent          int64 `json:"urgent"`
	WaitingApproval int64 `json:"waiting_approval"`
	All             int64 `json:"all"`
}

func (r *ItemRepository) List(ctx context.Context, filter ItemListFilter) (ItemPage, error) {
	teamIDs, err := uuidArray(filter.TeamIDs)
	if err != nil {
		return ItemPage{}, err
	}
	args := []any{teamIDs}
	conditions := []string{"team_id = ANY($1::uuid[])"}
	add := func(value any, expression string) string {
		args = append(args, value)
		return fmt.Sprintf(expression, len(args))
	}
	if filter.TeamID != "" {
		conditions = append(conditions, add(filter.TeamID, "team_id = $%d::uuid"))
	}
	if filter.Status != "" {
		conditions = append(conditions, add(filter.Status, "status = $%d"))
	}
	if filter.Priority > 0 {
		conditions = append(conditions, add(filter.Priority, "priority = $%d"))
	}
	if filter.AssigneeID != "" {
		conditions = append(conditions, add(filter.AssigneeID, "assignee_id = $%d::uuid"))
	}
	if filter.Query != "" {
		conditions = append(conditions, add(filter.Query, "search @@ websearch_to_tsquery('english', $%d)"))
	}
	switch filter.View {
	case "", "all":
	case "assigned_to_me":
		conditions = append(conditions, add(filter.UserID, "assignee_id = $%d::uuid"))
	case "team_unassigned":
		conditions = append(conditions, "assignee_id IS NULL AND status IN ('new', 'triaged')")
	case "urgent":
		conditions = append(conditions, "priority <= 2 AND status NOT IN ('resolved', 'closed')")
	case "waiting_approval":
		conditions = append(conditions, "status = 'pending_approval'")
	default:
		return ItemPage{}, fmt.Errorf("invalid item view %q", filter.View)
	}
	if filter.Cursor != nil {
		cursor := filter.Cursor
		args = append(args, cursor.Priority, cursor.UpdatedAt, cursor.UpdatedAt, cursor.ID)
		priorityArg := len(args) - 3
		updatedArg := len(args) - 2
		idUpdatedArg := len(args) - 1
		idArg := len(args)
		conditions = append(conditions, fmt.Sprintf(
			"(priority > $%d OR (priority = $%d AND updated_at < $%d) OR (priority = $%d AND updated_at = $%d AND id > $%d::uuid))",
			priorityArg, priorityArg, updatedArg, priorityArg, idUpdatedArg, idArg,
		))
	}
	args = append(args, filter.Limit+1)
	limitArg := len(args)
	query := `SELECT ` + itemColumns + ` FROM work_items WHERE ` + strings.Join(conditions, " AND ") +
		fmt.Sprintf(" ORDER BY priority ASC, updated_at DESC, id ASC LIMIT $%d", limitArg)
	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return ItemPage{}, fmt.Errorf("list work items: %w", err)
	}
	defer rows.Close()

	page := ItemPage{Items: make([]model.WorkItem, 0, filter.Limit+1)}
	for rows.Next() {
		item, err := scanWorkItem(rows)
		if err != nil {
			return ItemPage{}, fmt.Errorf("scan listed work item: %w", err)
		}
		page.Items = append(page.Items, item)
	}
	if err := rows.Err(); err != nil {
		return ItemPage{}, fmt.Errorf("iterate work items: %w", err)
	}
	if len(page.Items) > filter.Limit {
		page.HasMore = true
		page.Items = page.Items[:filter.Limit]
		last := page.Items[len(page.Items)-1]
		cursorBytes, err := json.Marshal(ItemCursor{Priority: last.Priority, UpdatedAt: last.UpdatedAt, ID: last.ID})
		if err != nil {
			return ItemPage{}, fmt.Errorf("encode list cursor: %w", err)
		}
		page.NextCursor = base64.RawURLEncoding.EncodeToString(cursorBytes)
	}
	return page, nil
}

func (r *ItemRepository) Counts(ctx context.Context, teamIDs []string, userID string) (ViewCounts, error) {
	uuidIDs, err := uuidArray(teamIDs)
	if err != nil {
		return ViewCounts{}, err
	}
	var counts ViewCounts
	err = r.db.QueryRow(ctx, `
		SELECT
			count(*) FILTER (WHERE assignee_id = $2::uuid),
			count(*) FILTER (WHERE assignee_id IS NULL AND status IN ('new', 'triaged')),
			count(*) FILTER (WHERE priority <= 2 AND status NOT IN ('resolved', 'closed')),
			count(*) FILTER (WHERE status = 'pending_approval'),
			count(*)
		FROM work_items
		WHERE team_id = ANY($1::uuid[])
	`, uuidIDs, userID).Scan(
		&counts.AssignedToMe, &counts.TeamUnassigned, &counts.Urgent, &counts.WaitingApproval, &counts.All,
	)
	if err != nil {
		return ViewCounts{}, fmt.Errorf("count smart views: %w", err)
	}
	return counts, nil
}

func (r *ItemRepository) ListEvents(ctx context.Context, itemID string, beforeID int64, limit int) ([]model.ItemEvent, bool, error) {
	args := []any{itemID}
	condition := "item_id = $1::uuid"
	if beforeID > 0 {
		args = append(args, beforeID)
		condition += " AND id < $2"
	}
	args = append(args, limit+1)
	rows, err := r.db.Query(ctx, `
		SELECT id, item_id::text, actor_id::text, type, field, old_value, new_value, reason, created_at
		FROM item_events WHERE `+condition+fmt.Sprintf(" ORDER BY id DESC LIMIT $%d", len(args)), args...)
	if err != nil {
		return nil, false, fmt.Errorf("list item events: %w", err)
	}
	defer rows.Close()

	events := make([]model.ItemEvent, 0, limit+1)
	for rows.Next() {
		var event model.ItemEvent
		if err := rows.Scan(&event.ID, &event.ItemID, &event.ActorID, &event.Type, &event.Field, &event.OldValue, &event.NewValue, &event.Reason, &event.CreatedAt); err != nil {
			return nil, false, fmt.Errorf("scan item event: %w", err)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, false, fmt.Errorf("iterate item events: %w", err)
	}
	hasMore := len(events) > limit
	if hasMore {
		events = events[:limit]
	}
	return events, hasMore, nil
}

func (r *ItemRepository) ListFieldSchemas(ctx context.Context, teamID string) ([]model.TeamFieldSchema, error) {
	rows, err := r.db.Query(ctx, `
		SELECT team_id::text, field_key, label, type, required, options
		FROM team_field_schemas WHERE team_id = $1::uuid ORDER BY field_key
	`, teamID)
	if err != nil {
		return nil, fmt.Errorf("list team field schemas: %w", err)
	}
	defer rows.Close()
	schemas := make([]model.TeamFieldSchema, 0)
	for rows.Next() {
		var schema model.TeamFieldSchema
		if err := rows.Scan(&schema.TeamID, &schema.FieldKey, &schema.Label, &schema.Type, &schema.Required, &schema.Options); err != nil {
			return nil, fmt.Errorf("scan team field schema: %w", err)
		}
		schemas = append(schemas, schema)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate team field schemas: %w", err)
	}
	return schemas, nil
}

func (r *ItemRepository) ListTeamMembers(ctx context.Context, teamID string) ([]TeamMember, error) {
	rows, err := r.db.Query(ctx, `
		SELECT u.id::text, u.email, u.name, u.is_system_admin, tm.role
		FROM team_members tm JOIN users u ON u.id = tm.user_id
		WHERE tm.team_id = $1::uuid
		ORDER BY tm.role, u.name, u.id
	`, teamID)
	if err != nil {
		return nil, fmt.Errorf("list team members: %w", err)
	}
	defer rows.Close()
	members := make([]TeamMember, 0)
	for rows.Next() {
		var member TeamMember
		if err := rows.Scan(&member.User.ID, &member.User.Email, &member.User.Name, &member.User.IsSystemAdmin, &member.Role); err != nil {
			return nil, fmt.Errorf("scan team member: %w", err)
		}
		members = append(members, member)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate team members: %w", err)
	}
	return members, nil
}

type TeamMember struct {
	User model.User `json:"user"`
	Role string     `json:"role"`
}

func (r *ItemRepository) TeamExists(ctx context.Context, teamID string) (bool, error) {
	var exists bool
	if err := r.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM teams WHERE id = $1::uuid)`, teamID).Scan(&exists); err != nil {
		return false, fmt.Errorf("check team existence: %w", err)
	}
	return exists, nil
}

func uuidArray(values []string) ([]pgtype.UUID, error) {
	result := make([]pgtype.UUID, 0, len(values))
	for _, value := range values {
		var parsed pgtype.UUID
		if err := parsed.Scan(value); err != nil {
			return nil, fmt.Errorf("parse team UUID %q: %w", value, err)
		}
		result = append(result, parsed)
	}
	return result, nil
}

func DecodeItemCursor(value string) (*ItemCursor, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("decode cursor: %w", err)
	}
	var cursor ItemCursor
	if err := json.Unmarshal(decoded, &cursor); err != nil {
		return nil, fmt.Errorf("parse cursor: %w", err)
	}
	if cursor.Priority < 1 || cursor.Priority > 4 || cursor.ID == "" || cursor.UpdatedAt.IsZero() {
		return nil, fmt.Errorf("cursor is missing required item position")
	}
	return &cursor, nil
}

func DecodeEventCursor(value string) (int64, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return 0, fmt.Errorf("decode event cursor: %w", err)
	}
	id, err := strconv.ParseInt(string(decoded), 10, 64)
	if err != nil || id < 1 {
		return 0, fmt.Errorf("invalid event cursor")
	}
	return id, nil
}

func EncodeEventCursor(id int64) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(id, 10)))
}
