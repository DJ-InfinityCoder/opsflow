package model

import "time"

type AnalyticsSummary struct {
	TeamID           string         `json:"team_id"`
	TotalItems       int            `json:"total_items"`
	OpenItems        int            `json:"open_items"`
	ByStatus         map[string]int `json:"by_status"`
	ByPriority       map[int]int    `json:"by_priority"`
	SLABreachedCount int            `json:"sla_breached_count"`
	SLAWarningCount  int            `json:"sla_warning_count"`
	MTTRSeconds      float64        `json:"mttr_seconds"`
}

type FeedEvent struct {
	ID        int64     `json:"id"`
	ItemID    string    `json:"item_id"`
	ItemTitle string    `json:"item_title"`
	TeamID    string    `json:"team_id"`
	ActorID   string    `json:"actor_id"`
	ActorName string    `json:"actor_name"`
	Type      string    `json:"type"`
	Field     *string   `json:"field,omitempty"`
	Reason    *string   `json:"reason,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}
