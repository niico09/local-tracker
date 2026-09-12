package storage

import (
	"context"
	"errors"
	"testing"
	"time"

	"local-tracker/internal/domain"
)

// insertProgress writes a progress row directly. Slice S4 owns no progress
// repository (S5 does), but the Rule 1 join must be proven against real rows, so
// the test seeds them with raw SQL.
func insertProgress(t *testing.T, repos *Repos, itemID domain.ItemID, owner domain.UserID, done bool) {
	t.Helper()
	doneInt := 0
	if done {
		doneInt = 1
	}
	if _, err := repos.Membership.db.raw.ExecContext(context.Background(),
		"INSERT INTO progress(item_id, owner_user_id, done, updated_at) VALUES (?,?,?,?)",
		int64(itemID), nullUser(owner), doneInt, formatTime(time.Now().UTC())); err != nil {
		t.Fatalf("insert progress (item %d, owner %d): %v", itemID, owner, err)
	}
}

// countProgress reports how many progress rows exist for one item.
func countProgress(t *testing.T, repos *Repos, itemID domain.ItemID) int {
	t.Helper()
	var n int
	if err := repos.Membership.db.raw.QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM progress WHERE item_id = ?", int64(itemID)).Scan(&n); err != nil {
		t.Fatalf("count progress: %v", err)
	}
	return n
}

// TestMembershipItemInCoupleAndPersonalGoalResolvesDistinctTilts is S4.3(a): the
// same item in a couple goal and a personal goal resolves two independent tilts.
func TestMembershipItemInCoupleAndPersonalGoalResolvesDistinctTilts(t *testing.T) {
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

	for _, goal := range []domain.Goal{couple, personal} {
		if err := repos.Membership.Add(ctx, actor, goal.ID, item.ID); err != nil {
			t.Fatalf("Add to goal %d: %v", goal.ID, err)
		}
	}

	// Shared tilt is done; the owner's personal tilt is still pending.
	insertProgress(t, repos, item.ID, 0, true)
	insertProgress(t, repos, item.ID, 1, false)

	coupleMembers, err := repos.Membership.ListMembers(ctx, actor, couple.ID)
	if err != nil || len(coupleMembers) != 1 {
		t.Fatalf("couple members = %d, %v; want 1", len(coupleMembers), err)
	}
	if got := coupleMembers[0]; got.TiltOwner != 0 || got.Tilt == nil || !got.Tilt.Done {
		t.Fatalf("couple member = %+v, want shared owner 0 done tilt", got)
	}
	if state := domain.DeriveState(*coupleMembers[0].Tilt); state != domain.StateCompleted {
		t.Fatalf("couple tilt state = %q, want completed", state)
	}

	personalMembers, err := repos.Membership.ListMembers(ctx, actor, personal.ID)
	if err != nil || len(personalMembers) != 1 {
		t.Fatalf("personal members = %d, %v; want 1", len(personalMembers), err)
	}
	if got := personalMembers[0]; got.TiltOwner != 1 || got.Tilt == nil || got.Tilt.Done {
		t.Fatalf("personal member = %+v, want owner 1 pending tilt", got)
	}

	if coupleMembers[0].Tilt.ID == personalMembers[0].Tilt.ID {
		t.Fatal("tilts share one progress row, want two independent records")
	}
}

// TestMembershipRule1WinsInsideCoupleGoal is S4.3(b): a standalone personal item
// added to a couple goal displays the shared tilt, not its owner's standalone
// tilt. The shared row is done and the owner's row is pending, so only the
// correct NULL-safe `IS` join can return done.
func TestMembershipRule1WinsInsideCoupleGoal(t *testing.T) {
	ctx := context.Background()
	repos := testRepos(t)
	seedUsers(t, repos)
	actor := domain.NewActor(1)

	personalItem := sampleItem("Ada's movie")
	personalItem.OwnerUserID = 1
	item, err := repos.Items.Create(ctx, actor, personalItem)
	if err != nil {
		t.Fatalf("Create personal item: %v", err)
	}
	couple, err := repos.Goals.Create(ctx, actor, sampleGoal("Couple", 0))
	if err != nil {
		t.Fatalf("Create couple goal: %v", err)
	}
	if err := repos.Membership.Add(ctx, actor, couple.ID, item.ID); err != nil {
		t.Fatalf("Add: %v", err)
	}

	insertProgress(t, repos, item.ID, 0, true)  // shared couple tilt: done
	insertProgress(t, repos, item.ID, 1, false) // owner's standalone tilt: pending

	members, err := repos.Membership.ListMembers(ctx, actor, couple.ID)
	if err != nil || len(members) != 1 {
		t.Fatalf("members = %d, %v; want 1", len(members), err)
	}
	if got := members[0]; got.TiltOwner != 0 {
		t.Fatalf("tilt owner = %d, want shared 0 (Rule 1)", got.TiltOwner)
	}
	if got := members[0]; got.Tilt == nil || !got.Tilt.Done {
		t.Fatalf("tilt = %+v, want the shared done tilt, not the owner's pending one", got.Tilt)
	}
}

// TestRemoveMembershipKeepsItemAndProgress is S4.3(c): removing a membership
// deletes only the link; the item and its progress row survive.
func TestRemoveMembershipKeepsItemAndProgress(t *testing.T) {
	ctx := context.Background()
	repos := testRepos(t)
	seedUsers(t, repos)
	actor := domain.NewActor(1)

	item, err := repos.Items.Create(ctx, actor, sampleItem("Kept"))
	if err != nil {
		t.Fatalf("Create item: %v", err)
	}
	goal, err := repos.Goals.Create(ctx, actor, sampleGoal("Couple", 0))
	if err != nil {
		t.Fatalf("Create goal: %v", err)
	}
	// A duplicate add must not create a second link.
	for i := 0; i < 2; i++ {
		if err := repos.Membership.Add(ctx, actor, goal.ID, item.ID); err != nil {
			t.Fatalf("Add #%d: %v", i+1, err)
		}
	}
	insertProgress(t, repos, item.ID, 0, true)

	if err := repos.Membership.Remove(ctx, actor, goal.ID, item.ID); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	members, err := repos.Membership.ListMembers(ctx, actor, goal.ID)
	if err != nil {
		t.Fatalf("ListMembers: %v", err)
	}
	if len(members) != 0 {
		t.Fatalf("members after remove = %d, want 0", len(members))
	}
	stored, err := repos.Items.Get(ctx, actor, item.ID)
	if err != nil || stored.Title != "Kept" {
		t.Fatalf("item after remove = %+v, %v; want it kept", stored, err)
	}
	if n := countProgress(t, repos, item.ID); n != 1 {
		t.Fatalf("progress rows after remove = %d, want 1 (no cascade)", n)
	}
}

// TestGrantedPartnerCannotModifyMembership proves membership changes respect the
// goal's read-only grant while still allowing the partner to list members.
func TestGrantedPartnerCannotModifyMembership(t *testing.T) {
	ctx := context.Background()
	repos := testRepos(t)
	seedUsers(t, repos)
	owner := domain.NewActor(1)
	partner := domain.NewActor(2)

	item, err := repos.Items.Create(ctx, owner, sampleItem("Shared item"))
	if err != nil {
		t.Fatalf("Create item: %v", err)
	}
	goal, err := repos.Goals.Create(ctx, owner, sampleGoal("Shared goal", 1))
	if err != nil {
		t.Fatalf("Create goal: %v", err)
	}
	if err := repos.Goals.SetVisibility(ctx, owner, goal.ID, domain.VisibilityShared); err != nil {
		t.Fatalf("SetVisibility: %v", err)
	}
	if err := repos.Membership.Add(ctx, owner, goal.ID, item.ID); err != nil {
		t.Fatalf("owner Add: %v", err)
	}

	if members, err := repos.Membership.ListMembers(ctx, partner, goal.ID); err != nil || len(members) != 1 {
		t.Fatalf("granted partner ListMembers = %d, %v; want 1", len(members), err)
	}
	if err := repos.Membership.Add(ctx, partner, goal.ID, item.ID); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("partner Add = %v, want ErrForbidden", err)
	}
	if err := repos.Membership.Remove(ctx, partner, goal.ID, item.ID); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("partner Remove = %v, want ErrForbidden", err)
	}
}

// TestMembershipRejectsHiddenGoalAndForeignItem proves the single access rule
// gates both the goal and the item being attached.
func TestMembershipRejectsHiddenGoalAndForeignItem(t *testing.T) {
	ctx := context.Background()
	repos := testRepos(t)
	seedUsers(t, repos)
	ownerA := domain.NewActor(1)
	partnerB := domain.NewActor(2)

	private, err := repos.Goals.Create(ctx, ownerA, sampleGoal("Ada private", 1))
	if err != nil {
		t.Fatalf("Create private goal: %v", err)
	}
	sharedItem, err := repos.Items.Create(ctx, ownerA, sampleItem("Shared"))
	if err != nil {
		t.Fatalf("Create shared item: %v", err)
	}
	if err := repos.Membership.Add(ctx, partnerB, private.ID, sharedItem.ID); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("partner Add into private goal = %v, want ErrForbidden", err)
	}
	if _, err := repos.Membership.ListMembers(ctx, partnerB, private.ID); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("partner ListMembers private goal = %v, want ErrForbidden", err)
	}

	couple, err := repos.Goals.Create(ctx, ownerA, sampleGoal("Couple", 0))
	if err != nil {
		t.Fatalf("Create couple goal: %v", err)
	}
	foreign := sampleItem("Linus personal")
	foreign.OwnerUserID = 2
	foreignItem, err := repos.Items.Create(ctx, partnerB, foreign)
	if err != nil {
		t.Fatalf("Create foreign item: %v", err)
	}
	if err := repos.Membership.Add(ctx, ownerA, couple.ID, foreignItem.ID); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("Add hidden item = %v, want ErrForbidden", err)
	}
}

// TestMembershipRepositoriesRequireActor is the structural guard for the port.
func TestMembershipRepositoriesRequireActor(t *testing.T) {
	ctx := context.Background()
	repos := testRepos(t)
	seedUsers(t, repos)
	noActor := domain.Actor{}

	calls := []struct {
		name string
		run  func() (int, error)
	}{
		{"Membership.Add", func() (int, error) { return 0, repos.Membership.Add(ctx, noActor, 1, 1) }},
		{"Membership.Remove", func() (int, error) { return 0, repos.Membership.Remove(ctx, noActor, 1, 1) }},
		{"Membership.ListMembers", func() (int, error) {
			v, err := repos.Membership.ListMembers(ctx, noActor, 1)
			return len(v), err
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
