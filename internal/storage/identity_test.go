package storage

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"local-tracker/internal/domain"
)

func testRepos(t *testing.T) *Repos {
	t.Helper()
	db, err := Open(context.Background(), filepath.Join(t.TempDir(), "tracker.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return NewRepos(db)
}

func testUser(name string) domain.User {
	return domain.User{
		Name: name, PinHash: []byte("hash"), PinSalt: []byte("salt"),
		PinIter: 600_000, CreatedAt: time.Now().UTC(),
	}
}

func TestSetupCreatesExactlyTwoUsers(t *testing.T) {
	ctx := context.Background()
	repos := testRepos(t)
	actor := domain.SystemActor()

	if n, err := repos.Users.Count(ctx, actor); err != nil || n != 0 {
		t.Fatalf("initial count = %d, %v; want 0", n, err)
	}
	if err := repos.Users.Setup(ctx, actor, testUser("Ada"), testUser("Linus")); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	if n, err := repos.Users.Count(ctx, actor); err != nil || n != 2 {
		t.Fatalf("count after setup = %d, %v; want 2", n, err)
	}
	// A second wizard run must be refused (I11).
	if err := repos.Users.Setup(ctx, actor, testUser("Grace"), testUser("Alan")); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("second Setup = %v, want ErrForbidden", err)
	}
}

// TestRepositoriesRequireActor is the structural guard: every port method with
// an empty actor fails, and reads return zero rows.
func TestRepositoriesRequireActor(t *testing.T) {
	ctx := context.Background()
	repos := testRepos(t)
	if err := repos.Users.Setup(ctx, domain.SystemActor(), testUser("Ada"), testUser("Linus")); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	noActor := domain.Actor{}

	calls := []struct {
		name string
		run  func() (int, error)
	}{
		{"Users.Count", func() (int, error) { n, err := repos.Users.Count(ctx, noActor); return n, err }},
		{"Users.Setup", func() (int, error) { return 0, repos.Users.Setup(ctx, noActor, testUser("A"), testUser("B")) }},
		{"Users.FindByName", func() (int, error) { _, err := repos.Users.FindByName(ctx, noActor, "Ada"); return 0, err }},
		{"Users.FindByID", func() (int, error) { u, err := repos.Users.FindByID(ctx, noActor, 1); return boolRows(u.ID != 0), err }},
		{"Sessions.Create", func() (int, error) { return 0, repos.Sessions.Create(ctx, noActor, domain.Session{}) }},
		{"Sessions.FindByTokenHash", func() (int, error) {
			s, err := repos.Sessions.FindByTokenHash(ctx, noActor, []byte("x"))
			return len(s.TokenHash), err
		}},
		{"Sessions.Delete", func() (int, error) { return 0, repos.Sessions.Delete(ctx, noActor, []byte("x")) }},
		{"Sessions.DeleteExpired", func() (int, error) { n, err := repos.Sessions.DeleteExpired(ctx, noActor, time.Now()); return n, err }},
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

func TestSessionExpiryDeletion(t *testing.T) {
	ctx := context.Background()
	repos := testRepos(t)
	actor := domain.SystemActor()
	if err := repos.Users.Setup(ctx, actor, testUser("Ada"), testUser("Linus")); err != nil {
		t.Fatalf("Setup: %v", err)
	}

	past := time.Now().Add(-time.Hour)
	hash := domain.HashToken("expired-token")
	if err := repos.Sessions.Create(ctx, actor, domain.Session{
		TokenHash: hash, UserID: 1, CreatedAt: past, ExpiresAt: past,
	}); err != nil {
		t.Fatalf("Create session: %v", err)
	}
	if _, err := repos.Sessions.FindByTokenHash(ctx, actor, hash); err != nil {
		t.Fatalf("FindByTokenHash before expiry sweep: %v", err)
	}
	n, err := repos.Sessions.DeleteExpired(ctx, actor, time.Now())
	if err != nil || n != 1 {
		t.Fatalf("DeleteExpired = %d, %v; want 1, nil", n, err)
	}
	if _, err := repos.Sessions.FindByTokenHash(ctx, actor, hash); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("FindByTokenHash after sweep = %v, want ErrNotFound", err)
	}
}

func boolRows(b bool) int {
	if b {
		return 1
	}
	return 0
}
