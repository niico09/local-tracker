package storage

import (
	"context"
	"errors"
	"testing"

	"local-tracker/internal/domain"
)

func sampleGoal(title string, owner domain.UserID) domain.Goal {
	return domain.Goal{Title: title, OwnerUserID: owner, Visibility: domain.VisibilityPrivate}
}

func TestGoalCRUDRoundTrip(t *testing.T) {
	ctx := context.Background()
	repos := testRepos(t)
	seedUsers(t, repos)
	actor := domain.NewActor(1)

	couple, err := repos.Goals.Create(ctx, actor, sampleGoal("Watch together", 0))
	if err != nil {
		t.Fatalf("Create couple: %v", err)
	}
	if couple.ID == 0 {
		t.Fatal("Create did not assign an id")
	}
	if couple.OwnerUserID != 0 || couple.Target != nil || couple.Visibility != domain.VisibilityPrivate {
		t.Fatalf("couple goal = %+v, want owner 0, no target, private", couple)
	}

	target := 12
	personal := sampleGoal("Books", 1)
	personal.Target = &target
	personal.Visibility = domain.VisibilityShared
	created, err := repos.Goals.Create(ctx, actor, personal)
	if err != nil {
		t.Fatalf("Create personal: %v", err)
	}
	if created.Target == nil || *created.Target != 12 {
		t.Fatalf("personal target = %v, want 12", created.Target)
	}
	if created.Visibility != domain.VisibilityShared || created.OwnerUserID != 1 {
		t.Fatalf("personal goal = %+v, want owner 1 shared", created)
	}

	list, err := repos.Goals.List(ctx, actor)
	if err != nil || len(list) != 2 {
		t.Fatalf("List = %d goals, %v; want 2", len(list), err)
	}

	// Update keeps ownership and can clear the target.
	created.Title = "Books 2026"
	created.Target = nil
	if err := repos.Goals.Update(ctx, actor, created); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, err := repos.Goals.Get(ctx, actor, created.ID)
	if err != nil {
		t.Fatalf("Get after update: %v", err)
	}
	if got.Title != "Books 2026" || got.Target != nil {
		t.Fatalf("updated goal = %+v, want title Books 2026 and no target", got)
	}
	if got.Visibility != domain.VisibilityShared {
		t.Fatalf("visibility = %q, want shared", got.Visibility)
	}

	// SetVisibility revokes the grant.
	if err := repos.Goals.SetVisibility(ctx, actor, created.ID, domain.VisibilityPrivate); err != nil {
		t.Fatalf("SetVisibility: %v", err)
	}
	if got, _ := repos.Goals.Get(ctx, actor, created.ID); got.Visibility != domain.VisibilityPrivate {
		t.Fatalf("visibility after revoke = %q, want private", got.Visibility)
	}

	if err := repos.Goals.Delete(ctx, actor, created.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := repos.Goals.Get(ctx, actor, created.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("Get after delete = %v, want ErrNotFound", err)
	}
}

// TestGoalPrivateHiddenFromPartner proves the single access rule hides a private
// personal goal from the other profile on both read and list.
func TestGoalPrivateHiddenFromPartner(t *testing.T) {
	ctx := context.Background()
	repos := testRepos(t)
	seedUsers(t, repos)
	ownerA := domain.NewActor(1)
	partnerB := domain.NewActor(2)

	created, err := repos.Goals.Create(ctx, ownerA, sampleGoal("Ada private", 1))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if _, err := repos.Goals.Get(ctx, partnerB, created.ID); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("partner Get = %v, want ErrForbidden", err)
	}
	list, err := repos.Goals.List(ctx, partnerB)
	if err != nil {
		t.Fatalf("partner List: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("partner List = %d goals, want 0", len(list))
	}
	if _, err := repos.Goals.Get(ctx, ownerA, created.ID); err != nil {
		t.Fatalf("owner Get: %v", err)
	}
}

// TestGrantedGoalIsReadOnlyForPartner proves a shared personal goal is readable
// by the partner but every mutation is refused.
func TestGrantedGoalIsReadOnlyForPartner(t *testing.T) {
	ctx := context.Background()
	repos := testRepos(t)
	seedUsers(t, repos)
	ownerA := domain.NewActor(1)
	partnerB := domain.NewActor(2)

	created, err := repos.Goals.Create(ctx, ownerA, sampleGoal("Shared goal", 1))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := repos.Goals.SetVisibility(ctx, ownerA, created.ID, domain.VisibilityShared); err != nil {
		t.Fatalf("SetVisibility: %v", err)
	}

	if _, err := repos.Goals.Get(ctx, partnerB, created.ID); err != nil {
		t.Fatalf("granted partner Get: %v", err)
	}
	list, err := repos.Goals.List(ctx, partnerB)
	if err != nil || len(list) != 1 {
		t.Fatalf("granted partner List = %d goals, %v; want 1", len(list), err)
	}

	mutated := created
	mutated.Title = "stolen"
	if err := repos.Goals.Update(ctx, partnerB, mutated); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("partner Update = %v, want ErrForbidden", err)
	}
	if err := repos.Goals.SetVisibility(ctx, partnerB, created.ID, domain.VisibilityPrivate); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("partner SetVisibility = %v, want ErrForbidden", err)
	}
	if err := repos.Goals.Delete(ctx, partnerB, created.ID); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("partner Delete = %v, want ErrForbidden", err)
	}

	stored, err := repos.Goals.Get(ctx, ownerA, created.ID)
	if err != nil || stored.Title != "Shared goal" {
		t.Fatalf("stored goal = %+v, %v; want unchanged title", stored, err)
	}
}

// TestCoupleGoalVisibleToBoth proves a goal with no owner is shared and both
// partners may modify it.
func TestCoupleGoalVisibleToBoth(t *testing.T) {
	ctx := context.Background()
	repos := testRepos(t)
	seedUsers(t, repos)
	ownerA := domain.NewActor(1)
	partnerB := domain.NewActor(2)

	created, err := repos.Goals.Create(ctx, ownerA, sampleGoal("Couple goal", 0))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := repos.Goals.Get(ctx, partnerB, created.ID); err != nil {
		t.Fatalf("partner Get couple: %v", err)
	}
	created.Title = "Couple v2"
	if err := repos.Goals.Update(ctx, partnerB, created); err != nil {
		t.Fatalf("partner Update couple: %v", err)
	}
	if got, _ := repos.Goals.Get(ctx, ownerA, created.ID); got.Title != "Couple v2" {
		t.Fatalf("couple title = %q, want Couple v2", got.Title)
	}
}

// TestGoalRepositoriesRequireActor is the structural guard for the goal port.
func TestGoalRepositoriesRequireActor(t *testing.T) {
	ctx := context.Background()
	repos := testRepos(t)
	seedUsers(t, repos)
	noActor := domain.Actor{}

	calls := []struct {
		name string
		run  func() (int, error)
	}{
		{"Goals.List", func() (int, error) { v, err := repos.Goals.List(ctx, noActor); return len(v), err }},
		{"Goals.Get", func() (int, error) { v, err := repos.Goals.Get(ctx, noActor, 1); return boolRows(v.ID != 0), err }},
		{"Goals.Create", func() (int, error) { _, err := repos.Goals.Create(ctx, noActor, sampleGoal("x", 0)); return 0, err }},
		{"Goals.Update", func() (int, error) { return 0, repos.Goals.Update(ctx, noActor, sampleGoal("x", 0)) }},
		{"Goals.SetVisibility", func() (int, error) {
			return 0, repos.Goals.SetVisibility(ctx, noActor, 1, domain.VisibilityShared)
		}},
		{"Goals.Delete", func() (int, error) { return 0, repos.Goals.Delete(ctx, noActor, 1) }},
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
