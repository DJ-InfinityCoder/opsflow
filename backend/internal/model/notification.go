package model

import "time"

type Notification struct {
	ID            string     `json:"id"`
	UserID        string     `json:"user_id"`
	ItemID        string     `json:"item_id"`
	Type          string     `json:"type"`
	SourceEventID *int64     `json:"source_event_id,omitempty"`
	DedupeKey     *string    `json:"dedupe_key,omitempty"`
	ReadAt        *time.Time `json:"read_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
}
