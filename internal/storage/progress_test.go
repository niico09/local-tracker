package storage

import (
	"context"
	"errors"
	"testing"
	"time"

	"local-tracker/internal/domain"
)

// goalWithTarget builds a goal with an explicit target for rollup tests.
func goalWithTarget(title string, owner domain.UserID, target int) domain.Goal {
	g := sampleGoal(title, owner)
	g.Target = &target
	return g
}

// datePtr builds a *Date at UTC midnight.
func datePtr(y int, m time.Month, d int) *domain.Date {
	v := domain.NewDate(y, m, d)
	return &v
}

// addMember links an item into a goal through the membership repo.
func addMember(t *testing.T, repos *Repos, a domain.Actor, goalID domain.GoalID, itemID domain.ItemID) {
	t.Helper()
	if err := repos.Membership.Add(context.Background(), a, goalID, itemID); err != nil {
		t.Fatalf("Add item %d to goal %d: %v", itemID, goalID, err)
	}
}

// TestProgressUpsertIsUniquePerOwner is S5.4: a repeated write for the same
// (item, owner) updates one row, and the shared and personal tilts of one item
// coexist as two independent rows.
func TestProgressUpsertIsUniquePerOwner(t *testing.T) {
	ctx := context.Background()
	repos := testRepos(t)
	seedUsers(t, repos)
	actor := domain.NewActor(1)

	item, err := repos.Items.Create(ctx, actor, sampleItem("Dune"))
	if err != nil {
		t.Fatalf("Create item: %v", err)
	}
	couple, err := repos.Goals.Create(ctx, actor, sampleGoal("Couple", 0))
	if err != nil {
		t.Fatalf("Create couple goal: %v", err)
	}
	personal, err := repos.Goals.Create(ctx, actor, sampleGoal("Mine", 1))
	if err != nil {
		t.Fatalf("Create personal goal: %v", err)
	}
	addMember(t, repos, actor, couple.ID, item.ID)
	addMember(t, repos, actor, personal.ID, item.ID)

	// Shared tilt via the couple goal, written twice: one row, last write wins.
	if _, err := repos.Progress.SetTilt(ctx, actor, couple.ID, domain.Progress{ItemID: item.ID, Done: true}); err != nil {
		t.Fatalf("SetTilt shared: %v", err)
	}
	if _, err := repos.Progress.SetTilt(ctx, actor, couple.ID, domain.Progress{ItemID: item.ID, Done: false}); err != nil {
		t.Fatalf("SetTilt shared again: %v", err)
	}
	// Personal tilt via the owner's goal.
	if _, err := repos.Progress.SetTilt(ctx, actor, personal.ID, domain.Progress{ItemID: item.ID, Done: true}); err != nil {
		t.Fatalf("SetTilt personal: %v", err)
	}

	if n := countProgress(t, repos, item.ID); n != 2 {
		t.Fatalf("progress rows = %d, want 2 (one shared + one personal)", n)
	}

	shared, err := repos.Progress.Tilt(ctx, actor, couple.ID, item.ID)
	if err != nil {
		t.Fatalf("Tilt shared: %v", err)
	}
	if shared.OwnerUserID != 0 || shared.Done {
		t.Fatalf("shared tilt = %+v, want owner 0 and done false after update", shared)
	}
	owned, err := repos.Progress.Tilt(ctx, actor, personal.ID, item.ID)
	if err != nil {
		t.Fatalf("Tilt personal: %v", err)
	}
	if owned.OwnerUserID != 1 || !owned.Done {
		t.Fatalf("personal tilt = %+v, want owner 1 and done true", owned)
	}
	if shared.ID == owned.ID {
		t.Fatal("shared and personal tilts share one row, want two records")
	}
}

// TestProgressDuplicateRawInsertRejected proves the NULL-safe expression index
// rejects a second shared tilt while still allowing one personal tilt.
func TestProgressDuplicateRawInsertRejected(t *testing.T) {
	ctx := context.Background()
	repos := testRepos(t)
	seedUsers(t, repos)
	actor := domain.NewActor(1)

	item, err := repos.Items.Create(ctx, actor, sampleItem("Dune"))
	if err != nil {
		t.Fatalf("Create item: %v", err)
	}
	insert := func(owner any) error {
		_, err := repos.Progress.db.raw.ExecContext(ctx,
			"INSERT INTO progress(item_id, owner_user_id, done, updated_at) VALUES (?,?,?,?)",
			int64(item.ID), owner, 0, formatTime(time.Now().UTC()))
		return err
	}
	if err := insert(nil); err != nil {
		t.Fatalf("first shared insert: %v", err)
	}
	if err := insert(nil); err == nil {
		t.Fatal("second shared insert succeeded, want a unique-index rejection")
	}
	if err := insert(int64(1)); err != nil {
		t.Fatalf("personal insert alongside shared: %v", err)
	}
}

// TestProgressForeignKeyEnforced proves the progress table's FK to items is live.
func TestProgressForeignKeyEnforced(t *testing.T) {
	ctx := context.Background()
	repos := testRepos(t)
	seedUsers(t, repos)

	_, err := repos.Progress.db.raw.ExecContext(ctx,
		"INSERT INTO progress(item_id, owner_user_id, done, updated_at) VALUES (?,?,?,?)",
		int64(99999), nil, 0, formatTime(time.Now().UTC()))
	if err == nil {
		t.Fatal("progress with an unknown item was accepted, want an FK violation")
	}
}

// TestProgressDoneWithNoDatesCompleted covers Rule 5: done alone is completed.
func TestProgressDoneWithNoDatesCompleted(t *testing.T) {
	ctx := context.Background()
	repos := testRepos(t)
	seedUsers(t, repos)
	actor := domain.NewActor(1)

	item, err := repos.Items.Create(ctx, actor, sampleItem("Dune"))
	if err != nil {
		t.Fatalf("Create item: %v", err)
	}
	stored, err := repos.Progress.SetTilt(ctx, actor, 0, domain.Progress{ItemID: item.ID, Done: true})
	if err != nil {
		t.Fatalf("SetTilt: %v", err)
	}
	if !stored.Done || stored.StartDate != nil || stored.EndDate != nil {
		t.Fatalf("stored tilt = %+v, want done with no dates", stored)
	}
	if state := domain.DeriveState(stored); state != domain.StateCompleted {
		t.Fatalf("state = %q, want completed", state)
	}
}

// TestProgressStartOnlyInProgress covers Rule 5: start without done is in-progress.
func TestProgressStartOnlyInProgress(t *testing.T) {
	ctx := context.Background()
	repos := testRepos(t)
	seedUsers(t, repos)
	actor := domain.NewActor(1)

	item, err := repos.Items.Create(ctx, actor, sampleItem("Dune"))
	if err != nil {
		t.Fatalf("Create item: %v", err)
	}
	stored, err := repos.Progress.SetTilt(ctx, actor, 0, domain.Progress{ItemID: item.ID, StartDate: datePtr(2024, time.January, 5)})
	if err != nil {
		t.Fatalf("SetTilt: %v", err)
	}
	if state := domain.DeriveState(stored); state != domain.StateInProgress {
		t.Fatalf("state = %q, want in_progress", state)
	}
}

// TestProgressEndBeforeStartRejected proves the write path refuses end < start.
func TestProgressEndBeforeStartRejected(t *testing.T) {
	ctx := context.Background()
	repos := testRepos(t)
	seedUsers(t, repos)
	actor := domain.NewActor(1)

	item, err := repos.Items.Create(ctx, actor, sampleItem("Dune"))
	if err != nil {
		t.Fatalf("Create item: %v", err)
	}
	_, err = repos.Progress.SetTilt(ctx, actor, 0, domain.Progress{
		ItemID:    item.ID,
		StartDate: datePtr(2024, time.February, 1),
		EndDate:   datePtr(2024, time.January, 1),
	})
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("SetTilt end<start = %v, want ErrValidation", err)
	}
}

// TestProgressStandaloneSharedDefault covers Rule 4: an item outside every goal
// writes the shared tilt, and both partners read the same row.
func TestProgressStandaloneSharedDefault(t *testing.T) {
	ctx := context.Background()
	repos := testRepos(t)
	seedUsers(t, repos)
	ownerA := domain.NewActor(1)
	partnerB := domain.NewActor(2)

	item, err := repos.Items.Create(ctx, ownerA, sampleItem("Shared"))
	if err != nil {
		t.Fatalf("Create item: %v", err)
	}
	stored, err := repos.Progress.SetTilt(ctx, ownerA, 0, domain.Progress{ItemID: item.ID, Done: true})
	if err != nil {
		t.Fatalf("SetTilt: %v", err)
	}
	if stored.OwnerUserID != 0 {
		t.Fatalf("standalone owner = %d, want shared 0", stored.OwnerUserID)
	}
	seen, err := repos.Progress.Tilt(ctx, partnerB, 0, item.ID)
	if err != nil {
		t.Fatalf("partner Tilt: %v", err)
	}
	if !seen.Done || seen.OwnerUserID != 0 {
		t.Fatalf("partner shared tilt = %+v, want the same done shared row", seen)
	}
}

// TestProgressStandalonePersonalOwner covers Rule 4's personal branch: a
// standalone item owned by A records its tilt under A, and B cannot change it.
func TestProgressStandalonePersonalOwner(t *testing.T) {
	ctx := context.Background()
	repos := testRepos(t)
	seedUsers(t, repos)
	ownerA := domain.NewActor(1)
	partnerB := domain.NewActor(2)

	personal := sampleItem("Ada private")
	personal.OwnerUserID = 1
	item, err := repos.Items.Create(ctx, ownerA, personal)
	if err != nil {
		t.Fatalf("Create personal item: %v", err)
	}
	stored, err := repos.Progress.SetTilt(ctx, ownerA, 0, domain.Progress{ItemID: item.ID, Done: true})
	if err != nil {
		t.Fatalf("owner SetTilt: %v", err)
	}
	if stored.OwnerUserID != 1 || !stored.Done {
		t.Fatalf("personal tilt = %+v, want owner 1 done", stored)
	}
	if _, err := repos.Progress.SetTilt(ctx, partnerB, 0, domain.Progress{ItemID: item.ID}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("partner SetTilt = %v, want ErrForbidden", err)
	}
}

// TestProgressRule1AndTop100 covers Rules 1 and 2: the same item in a couple goal
// and a personal goal resolves two independent tilts, the couple goal is one
// shared advance toward its target, and a granted partner reads but cannot write.
func TestProgressRule1AndTop100(t *testing.T) {
	ctx := context.Background()
	repos := testRepos(t)
	seedUsers(t, repos)
	ownerA := domain.NewActor(1)
	partnerB := domain.NewActor(2)

	item, err := repos.Items.Create(ctx, ownerA, sampleItem("Dune"))
	if err != nil {
		t.Fatalf("Create item: %v", err)
	}
	top100, err := repos.Goals.Create(ctx, ownerA, goalWithTarget("Top 100", 0, 100))
	if err != nil {
		t.Fatalf("Create couple goal: %v", err)
	}
	personal, err := repos.Goals.Create(ctx, ownerA, sampleGoal("Mine", 1))
	if err != nil {
		t.Fatalf("Create personal goal: %v", err)
	}
	if err := repos.Goals.SetVisibility(ctx, ownerA, personal.ID, domain.VisibilityShared); err != nil {
		t.Fatalf("SetVisibility: %v", err)
	}
	addMember(t, repos, ownerA, top100.ID, item.ID)
	addMember(t, repos, ownerA, personal.ID, item.ID)

	// Rule 2: either partner advancing the couple goal writes the shared tilt.
	if _, err := repos.Progress.SetTilt(ctx, partnerB, top100.ID, domain.Progress{ItemID: item.ID, Done: true}); err != nil {
		t.Fatalf("partner SetTilt couple: %v", err)
	}
	// Rule 1: the personal goal shows the owner's tilt, still pending here.
	if _, err := repos.Progress.SetTilt(ctx, ownerA, personal.ID, domain.Progress{ItemID: item.ID}); err != nil {
		t.Fatalf("SetTilt personal: %v", err)
	}

	coupleTilt, err := repos.Progress.Tilt(ctx, ownerA, top100.ID, item.ID)
	if err != nil || coupleTilt.OwnerUserID != 0 || !coupleTilt.Done {
		t.Fatalf("couple tilt = %+v, %v; want shared done", coupleTilt, err)
	}
	personalTilt, err := repos.Progress.Tilt(ctx, ownerA, personal.ID, item.ID)
	if err != nil || personalTilt.OwnerUserID != 1 || personalTilt.Done {
		t.Fatalf("personal tilt = %+v, %v; want owner 1 pending", personalTilt, err)
	}

	done, total, err := repos.Progress.GoalRollup(ctx, ownerA, top100.ID)
	if err != nil || done != 1 || total != 100 {
		t.Fatalf("Top 100 rollup = %d/%d, %v; want 1/100", done, total, err)
	}

	// The granted partner reads the personal goal's rollup read-only.
	if _, _, err := repos.Progress.GoalRollup(ctx, partnerB, personal.ID); err != nil {
		t.Fatalf("granted partner rollup: %v", err)
	}
	if _, err := repos.Progress.SetTilt(ctx, partnerB, personal.ID, domain.Progress{ItemID: item.ID, Done: true}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("granted partner SetTilt = %v, want ErrForbidden", err)
	}
}

// TestProgressGoalRollupNoTarget covers the member-count denominator (G3).
func TestProgressGoalRollupNoTarget(t *testing.T) {
	ctx := context.Background()
	repos := testRepos(t)
	seedUsers(t, repos)
	actor := domain.NewActor(1)

	goal, err := repos.Goals.Create(ctx, actor, sampleGoal("Reading", 0))
	if err != nil {
		t.Fatalf("Create goal: %v", err)
	}
	for i := 0; i < 4; i++ {
		item, err := repos.Items.Create(ctx, actor, sampleItem("Book"))
		if err != nil {
			t.Fatalf("Create item: %v", err)
		}
		addMember(t, repos, actor, goal.ID, item.ID)
		if i < 2 {
			if _, err := repos.Progress.SetTilt(ctx, actor, goal.ID, domain.Progress{ItemID: item.ID, Done: true}); err != nil {
				t.Fatalf("SetTilt: %v", err)
			}
		}
	}
	done, total, err := repos.Progress.GoalRollup(ctx, actor, goal.ID)
	if err != nil || done != 2 || total != 4 {
		t.Fatalf("rollup = %d/%d, %v; want 2/4", done, total, err)
	}
}

// TestProgressGoalRollupClamps proves a done count above the target is clamped.
func TestProgressGoalRollupClamps(t *testing.T) {
	ctx := context.Background()
	repos := testRepos(t)
	seedUsers(t, repos)
	actor := domain.NewActor(1)

	goal, err := repos.Goals.Create(ctx, actor, goalWithTarget("One", 0, 1))
	if err != nil {
		t.Fatalf("Create goal: %v", err)
	}
	for i := 0; i < 2; i++ {
		item, err := repos.Items.Create(ctx, actor, sampleItem("Item"))
		if err != nil {
			t.Fatalf("Create item: %v", err)
		}
		addMember(t, repos, actor, goal.ID, item.ID)
		if _, err := repos.Progress.SetTilt(ctx, actor, goal.ID, domain.Progress{ItemID: item.ID, Done: true}); err != nil {
			t.Fatalf("SetTilt: %v", err)
		}
	}
	done, total, err := repos.Progress.GoalRollup(ctx, actor, goal.ID)
	if err != nil || done != 1 || total != 1 {
		t.Fatalf("rollup = %d/%d, %v; want clamped 1/1", done, total, err)
	}
}

// TestProgressGoalTiltRequiresMembership proves a tilt cannot be written against
// an item that is not a member of the goal.
func TestProgressGoalTiltRequiresMembership(t *testing.T) {
	ctx := context.Background()
	repos := testRepos(t)
	seedUsers(t, repos)
	actor := domain.NewActor(1)

	goal, err := repos.Goals.Create(ctx, actor, sampleGoal("Empty", 0))
	if err != nil {
		t.Fatalf("Create goal: %v", err)
	}
	item, err := repos.Items.Create(ctx, actor, sampleItem("Loose"))
	if err != nil {
		t.Fatalf("Create item: %v", err)
	}
	if _, err := repos.Progress.SetTilt(ctx, actor, goal.ID, domain.Progress{ItemID: item.ID}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("SetTilt non-member = %v, want ErrNotFound", err)
	}
	if _, err := repos.Progress.Tilt(ctx, actor, goal.ID, item.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("Tilt non-member = %v, want ErrNotFound", err)
	}
}

// TestProgressRepositoriesRequireActor is the structural guard for the port.
func TestProgressRepositoriesRequireActor(t *testing.T) {
	ctx := context.Background()
	repos := testRepos(t)
	seedUsers(t, repos)
	noActor := domain.Actor{}

	calls := []struct {
		name string
		run  func() (int, error)
	}{
		{"Progress.Tilt", func() (int, error) {
			v, err := repos.Progress.Tilt(ctx, noActor, 0, 1)
			return boolRows(v.ItemID != 0), err
		}},
		{"Progress.SetTilt", func() (int, error) {
			_, err := repos.Progress.SetTilt(ctx, noActor, 0, domain.Progress{ItemID: 1})
			return 0, err
		}},
		{"Progress.GoalRollup", func() (int, error) {
			d, _, err := repos.Progress.GoalRollup(ctx, noActor, 1)
			return d, err
		}},
	}
	for _, c := range calls {
		rows, err := c.run()
		if !errors.Is(err, domain.ErrNoActor) {
			t.Errorf("%s err = %v, want ErrNoActor", c.name, err)
		}
		if rows != 0 {
			t.Errorf("%s returned %d rows, want 0", c.name, rows)
		}
	}
}
