package storage

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"local-tracker/internal/domain"
	"local-tracker/internal/seed"
)

// countRows runs an ungated COUNT for test assertions.
func countRows(t *testing.T, repos *Repos, q string) int {
	t.Helper()
	var n int
	if err := repos.Seed.db.raw.QueryRowContext(context.Background(), q).Scan(&n); err != nil {
		t.Fatalf("count %q: %v", q, err)
	}
	return n
}

// TestSeedIsIdempotent is S6.4: a second seed run inserts nothing and every
// count is unchanged.
func TestSeedIsIdempotent(t *testing.T) {
	ctx := context.Background()
	repos := testRepos(t)
	seedUsers(t, repos)
	entries, err := seed.Load()
	if err != nil {
		t.Fatalf("seed.Load: %v", err)
	}

	first, err := repos.Seed.Seed(ctx, domain.SystemActor(), entries)
	if err != nil {
		t.Fatalf("first Seed: %v", err)
	}
	if first.ItemsInserted != 100 || first.ItemsKept != 0 || first.MembershipsAdded != 100 || !first.GoalCreated {
		t.Fatalf("first seed result = %+v, want 100 inserted / 100 memberships / goal created", first)
	}
	items1 := countRows(t, repos, "SELECT COUNT(*) FROM items")
	members1 := countRows(t, repos, "SELECT COUNT(*) FROM goal_items")
	goals1 := countRows(t, repos, "SELECT COUNT(*) FROM goals")

	second, err := repos.Seed.Seed(ctx, domain.SystemActor(), entries)
	if err != nil {
		t.Fatalf("second Seed: %v", err)
	}
	if second.ItemsInserted != 0 || second.ItemsKept != 100 || second.MembershipsAdded != 0 || second.GoalCreated {
		t.Fatalf("second seed result = %+v, want 0 inserted / 100 kept / no memberships / no goal", second)
	}

	items2 := countRows(t, repos, "SELECT COUNT(*) FROM items")
	members2 := countRows(t, repos, "SELECT COUNT(*) FROM goal_items")
	goals2 := countRows(t, repos, "SELECT COUNT(*) FROM goals")
	if items1 != items2 || members1 != members2 || goals1 != goals2 {
		t.Fatalf("counts changed between runs: items %d->%d, memberships %d->%d, goals %d->%d",
			items1, items2, members1, members2, goals1, goals2)
	}
	if items1 != 100 || members1 != 100 || goals1 != 1 {
		t.Fatalf("seeded counts = items %d, memberships %d, goals %d; want 100/100/1", items1, members1, goals1)
	}
}

// TestSeedTop100IsOneSharedAdvance is S6.4 / Rule 2: the seeded Top 100 goal is
// a couple goal (owner NULL, target 100) and either partner advances the same
// shared tilt.
func TestSeedTop100IsOneSharedAdvance(t *testing.T) {
	ctx := context.Background()
	repos := testRepos(t)
	seedUsers(t, repos)
	entries, err := seed.Load()
	if err != nil {
		t.Fatalf("seed.Load: %v", err)
	}
	if _, err := repos.Seed.Seed(ctx, domain.SystemActor(), entries); err != nil {
		t.Fatalf("Seed: %v", err)
	}

	var (
		goalID int64
		owner  sql.NullInt64
		target sql.NullInt64
	)
	if err := repos.Seed.db.raw.QueryRowContext(ctx,
		"SELECT id, owner_user_id, target FROM goals WHERE external_key = ?", top100ExternalKey).
		Scan(&goalID, &owner, &target); err != nil {
		t.Fatalf("load top100 goal: %v", err)
	}
	if owner.Valid {
		t.Fatalf("top100 owner = %d, want NULL (couple goal)", owner.Int64)
	}
	if !target.Valid || target.Int64 != 100 {
		t.Fatalf("top100 target = %v, want 100", target)
	}

	var itemID int64
	if err := repos.Seed.db.raw.QueryRowContext(ctx, "SELECT id FROM items ORDER BY id LIMIT 1").Scan(&itemID); err != nil {
		t.Fatalf("load seeded item: %v", err)
	}

	ownerA := domain.NewActor(1)
	partnerB := domain.NewActor(2)
	if _, err := repos.Progress.SetTilt(ctx, partnerB, domain.GoalID(goalID), domain.Progress{ItemID: domain.ItemID(itemID), Done: true}); err != nil {
		t.Fatalf("partner SetTilt: %v", err)
	}
	done, total, err := repos.Progress.GoalRollup(ctx, ownerA, domain.GoalID(goalID))
	if err != nil || done != 1 || total != 100 {
		t.Fatalf("owner rollup = %d/%d, %v; want 1/100", done, total, err)
	}
	donePartner, _, err := repos.Progress.GoalRollup(ctx, partnerB, domain.GoalID(goalID))
	if err != nil || donePartner != 1 {
		t.Fatalf("partner rollup done = %d, %v; want the shared 1", donePartner, err)
	}
}

// TestSeedRequiresActor keeps I1 structural for the seed port.
func TestSeedRequiresActor(t *testing.T) {
	ctx := context.Background()
	repos := testRepos(t)
	_, err := repos.Seed.Seed(ctx, domain.Actor{}, []domain.SeedEntry{{ExternalID: "x", Title: "X", Kind: domain.KindSeries}})
	if !errors.Is(err, domain.ErrNoActor) {
		t.Fatalf("Seed without actor = %v, want ErrNoActor", err)
	}
}
