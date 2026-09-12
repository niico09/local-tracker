package domain

import "context"

// SeedEntry is one row of the embedded Top 100 list, ready to be applied.
type SeedEntry struct {
	ExternalID string
	Title      string
	Kind       Kind
	Year       *int
}

// SeedResult reports what one idempotent seed run changed. A second run over an
// already-seeded database reports zero inserted and every entry kept.
type SeedResult struct {
	GoalCreated      bool
	ItemsInserted    int
	ItemsKept        int
	MembershipsAdded int
}

// SeedRepository applies the Top 100 seed idempotently. Idempotency comes from
// the schema constraints (UNIQUE(external_id), UNIQUE(goals.external_key) and
// the goal_items primary key), never from a pre-check. The Top 100 goal is a
// couple goal (owner unset) measured as one shared advance (Rule 2).
type SeedRepository interface {
	Seed(ctx context.Context, a Actor, entries []SeedEntry) (SeedResult, error)
}
