package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"local-tracker/internal/domain"
)

// TestBackupSnapshotIsOpenableAndCountsMatch is S6.4: the VACUUM INTO snapshot is
// a valid database with the same row counts as the source.
func TestBackupSnapshotIsOpenableAndCountsMatch(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	db, err := Open(ctx, filepath.Join(dir, "tracker.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	repos := NewRepos(db)
	seedUsers(t, repos)
	actor := domain.NewActor(1)
	for i := 0; i < 3; i++ {
		if _, err := repos.Items.Create(ctx, actor, sampleItem("Item")); err != nil {
			t.Fatalf("Create item: %v", err)
		}
	}
	if _, err := repos.Goals.Create(ctx, actor, sampleGoal("Couple", 0)); err != nil {
		t.Fatalf("Create goal: %v", err)
	}

	// The destination directory does not exist yet; Backup must create it.
	dest := filepath.Join(dir, "snapshot", "backup.db")
	if err := Backup(ctx, db, dest, filepath.Join(dir, "uploads"), false); err != nil {
		t.Fatalf("Backup: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close source: %v", err)
	}

	snap, err := Open(ctx, dest)
	if err != nil {
		t.Fatalf("open snapshot: %v", err)
	}
	defer snap.Close()

	cases := []struct {
		name  string
		query string
		want  int
	}{
		{"users", "SELECT COUNT(*) FROM users", 2},
		{"items", "SELECT COUNT(*) FROM items", 3},
		{"goals", "SELECT COUNT(*) FROM goals", 1},
	}
	for _, c := range cases {
		var n int
		if err := snap.raw.QueryRowContext(ctx, c.query).Scan(&n); err != nil {
			t.Fatalf("count %s: %v", c.name, err)
		}
		if n != c.want {
			t.Errorf("snapshot %s = %d, want %d", c.name, n, c.want)
		}
	}
}

// TestBackupRejectsExistingTarget proves the target must not pre-exist and is
// left untouched when it does.
func TestBackupRejectsExistingTarget(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	dest := filepath.Join(t.TempDir(), "existing.db")
	if err := os.WriteFile(dest, []byte("x"), 0o644); err != nil {
		t.Fatalf("seed target: %v", err)
	}
	if err := Backup(ctx, db, dest, "", false); err == nil {
		t.Fatal("Backup onto an existing target succeeded, want an error")
	}
	if data, err := os.ReadFile(dest); err != nil || string(data) != "x" {
		t.Fatalf("existing target was modified: %q, %v", data, err)
	}
}

// TestBackupCopiesUploads proves -uploads copies the uploads tree beside the
// snapshot.
func TestBackupCopiesUploads(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	db, err := Open(ctx, filepath.Join(dir, "tracker.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	uploads := filepath.Join(dir, "uploads")
	if err := os.MkdirAll(uploads, 0o755); err != nil {
		t.Fatalf("mkdir uploads: %v", err)
	}
	if err := os.WriteFile(filepath.Join(uploads, "cover.png"), []byte("img"), 0o644); err != nil {
		t.Fatalf("write upload: %v", err)
	}

	dest := filepath.Join(dir, "backup.db")
	if err := Backup(ctx, db, dest, uploads, true); err != nil {
		t.Fatalf("Backup: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dest+".uploads", "cover.png"))
	if err != nil || string(got) != "img" {
		t.Fatalf("copied upload = %q, %v; want img", got, err)
	}
}
