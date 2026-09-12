package storage

import (
	"context"
	"testing"
)

func TestMigrateIsIdempotent(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	scalar := func(query string) int {
		t.Helper()
		var got int
		if err := db.raw.QueryRowContext(ctx, query).Scan(&got); err != nil {
			t.Fatalf("query %q: %v", query, err)
		}
		return got
	}

	// 6 domain tables + schema_migrations, ledger holds one applied version.
	if got := scalar("SELECT COUNT(*) FROM schema_migrations"); got != 1 {
		t.Errorf("schema_migrations rows = %d, want 1", got)
	}
	if got := scalar("SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'"); got != 7 {
		t.Errorf("table count = %d, want 7", got)
	}

	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
	if got := scalar("SELECT COUNT(*) FROM schema_migrations"); got != 1 {
		t.Errorf("schema_migrations after re-run = %d, want 1", got)
	}
}

func TestSchemaIncludesG1AndNullSafeUniqueIndex(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	count := func(query string, args ...any) int {
		t.Helper()
		var got int
		if err := db.raw.QueryRowContext(ctx, query, args...).Scan(&got); err != nil {
			t.Fatalf("query %q: %v", query, err)
		}
		return got
	}

	if got := count("SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = 'ux_progress_item_owner'"); got != 1 {
		t.Errorf("ux_progress_item_owner count = %d, want 1", got)
	}
	if got := count("SELECT COUNT(*) FROM pragma_table_info('items') WHERE name = 'owner_user_id'"); got != 1 {
		t.Errorf("items.owner_user_id count = %d, want 1 (G1)", got)
	}
	if got := count("SELECT COUNT(*) FROM pragma_table_info('goals') WHERE name = 'external_key'"); got != 1 {
		t.Errorf("goals.external_key count = %d, want 1", got)
	}
}
