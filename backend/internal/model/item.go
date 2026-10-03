package model

import (
	"encoding/json"
	"time"
)

type WorkItem struct {
	ID            string         `json:"id"`
	TeamID        string         `json:"team_id"`
	Title         string         `json:"title"`
	Description   string         `json:"description"`
	Status        string         `json:"status"`
	Priority      int16          `json:"priority"`
	CreatedBy     string         `json:"created_by"`
	AssigneeID    *string        `json:"assignee_id"`
	CustomFields  map[string]any `json:"custom_fields"`
	DueAt         *time.Time     `json:"due_at"`
	Version       int            `json:"version"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	ResolvedAt    *time.Time     `json:"resolved_at"`
	SLAState        string         `json:"sla_state,omitempty"`
	AllowedStates   []string       `json:"allowed_transitions,omitempty"`
	PendingApproval *Approval      `json:"pending_approval,omitempty"`
}

type TeamFieldSchema struct {
	TeamID   string          `json:"team_id"`
	FieldKey string          `json:"field_key"`
	Label    string          `json:"label"`
	Type     string          `json:"type"`
	Required bool            `json:"required"`
	Options  json.RawMessage `json:"options"`
}

type ItemEvent struct {
	ID        int64           `json:"id"`
	ItemID    string          `json:"item_id"`
	ActorID   string          `json:"actor_id"`
	Type      string          `json:"type"`
	Field     *string         `json:"field,omitempty"`
	OldValue  json.RawMessage `json:"old_value,omitempty"`
	NewValue  json.RawMessage `json:"new_value,omitempty"`
	Reason    *string         `json:"reason,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
}

type Approval struct {
	ID          string     `json:"id"`
	ItemID      string     `json:"item_id"`
	RequestedBy string     `json:"requested_by"`
	DecidedBy   *string    `json:"decided_by,omitempty"`
	Decision    *string    `json:"decision,omitempty"`
	Reason      *string    `json:"reason,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	DecidedAt   *time.Time `json:"decided_at,omitempty"`
}
