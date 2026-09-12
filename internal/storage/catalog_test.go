package storage

import (
	"context"
	"errors"
	"testing"

	"local-tracker/internal/domain"
)

func seedUsers(t *testing.T, repos *Repos) {
	t.Helper()
	if err := repos.Users.Setup(context.Background(), domain.SystemActor(), testUser("Ada"), testUser("Linus")); err != nil {
		t.Fatalf("Setup: %v", err)
	}
}

func sampleItem(title string) domain.Item {
	year := 2021
	return domain.Item{Title: title, Kind: domain.KindMovie, Year: &year}
}

func TestItemCRUDRoundTrip(t *testing.T) {
	ctx := context.Background()
	repos := testRepos(t)
	seedUsers(t, repos)
	actor := domain.NewActor(1)

	created, err := repos.Items.Create(ctx, actor, sampleItem("Dune"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID == 0 {
		t.Fatal("Create did not assign an id")
	}

	got, err := repos.Items.Get(ctx, actor, created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Title != "Dune" || got.Kind != domain.KindMovie || got.Year == nil || *got.Year != 2021 {
		t.Fatalf("Get = %+v, want Dune/movie/2021", got)
	}
	if got.OwnerUserID != 0 {
		t.Fatalf("shared item owner = %d, want 0", got.OwnerUserID)
	}

	list, err := repos.Items.List(ctx, actor)
	if err != nil || len(list) != 1 {
		t.Fatalf("List = %d items, %v; want 1", len(list), err)
	}

	got.Title = "Dune Part Two"
	if err := repos.Items.Update(ctx, actor, got); err != nil {
		t.Fatalf("Update: %v", err)
	}
	updated, err := repos.Items.Get(ctx, actor, created.ID)
	if err != nil {
		t.Fatalf("Get after update: %v", err)
	}
	if updated.Title != "Dune Part Two" {
		t.Fatalf("updated title = %q, want Dune Part Two", updated.Title)
	}

	cover := "deadbeefdeadbeefdeadbeefdeadbeef.png"
	if err := repos.Items.SetCover(ctx, actor, created.ID, cover); err != nil {
		t.Fatalf("SetCover: %v", err)
	}
	withCover, err := repos.Items.Get(ctx, actor, created.ID)
	if err != nil {
		t.Fatalf("Get after cover: %v", err)
	}
	if withCover.CoverPath != cover {
		t.Fatalf("cover = %q, want %q", withCover.CoverPath, cover)
	}

	if err := repos.Items.Delete(ctx, actor, created.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := repos.Items.Get(ctx, actor, created.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("Get after delete = %v, want ErrNotFound", err)
	}
}

// TestPersonalItemHiddenFromPartner proves the single access rule hides a
// personal item from the other profile on both read and list.
func TestPersonalItemHiddenFromPartner(t *testing.T) {
	ctx := context.Background()
	repos := testRepos(t)
	seedUsers(t, repos)
	ownerA := domain.NewActor(1)
	partnerB := domain.NewActor(2)

	personal := sampleItem("Ada private")
	personal.OwnerUserID = 1
	created, err := repos.Items.Create(ctx, ownerA, personal)
	if err != nil {
		t.Fatalf("Create personal: %v", err)
	}

	if _, err := repos.Items.Get(ctx, partnerB, created.ID); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("partner Get = %v, want ErrForbidden", err)
	}
	list, err := repos.Items.List(ctx, partnerB)
	if err != nil {
		t.Fatalf("partner List: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("partner List = %d items, want 0", len(list))
	}
	if err := repos.Items.Delete(ctx, partnerB, created.ID); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("partner Delete = %v, want ErrForbidden", err)
	}
	// The owner still sees it.
	if _, err := repos.Items.Get(ctx, ownerA, created.ID); err != nil {
		t.Fatalf("owner Get: %v", err)
	}
}

// TestItemRepositoriesRequireActor is the structural guard for the catalog port.
func TestItemRepositoriesRequireActor(t *testing.T) {
	ctx := context.Background()
	repos := testRepos(t)
	seedUsers(t, repos)
	noActor := domain.Actor{}

	calls := []struct {
		name string
		run  func() (int, error)
	}{
		{"Items.List", func() (int, error) { v, err := repos.Items.List(ctx, noActor); return len(v), err }},
		{"Items.Get", func() (int, error) { v, err := repos.Items.Get(ctx, noActor, 1); return boolRows(v.ID != 0), err }},
		{"Items.Create", func() (int, error) { _, err := repos.Items.Create(ctx, noActor, sampleItem("x")); return 0, err }},
		{"Items.Update", func() (int, error) { return 0, repos.Items.Update(ctx, noActor, sampleItem("x")) }},
		{"Items.SetCover", func() (int, error) { return 0, repos.Items.SetCover(ctx, noActor, 1, "x.png") }},
		{"Items.Delete", func() (int, error) { return 0, repos.Items.Delete(ctx, noActor, 1) }},
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
