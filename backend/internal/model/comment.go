package model

import "time"

type Comment struct {
	ID         string    `json:"id"`
	ItemID     string    `json:"item_id"`
	AuthorID   string    `json:"author_id"`
	AuthorName string    `json:"author_name"`
	Body       string    `json:"body"`
	Mentions   []string  `json:"mentions"`
	CreatedAt  time.Time `json:"created_at"`
}
