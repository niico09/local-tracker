package domain

import "time"

// Rating is one profile's 1–5 score on one catalog item. Scores are personal:
// the partner may read them but never write them.
type Rating struct {
	ItemID    ItemID
	UserID    UserID
	Score     int
	UpdatedAt time.Time
}

// Note is the couple's shared free-text note attached to one catalog item.
type Note struct {
	ItemID    ItemID
	Body      string
	UpdatedAt time.Time
}
