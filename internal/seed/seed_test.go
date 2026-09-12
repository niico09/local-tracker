package seed

import "testing"

// TestLoadTop100 proves the embedded placeholder list is well formed: 100
// entries, unique stable ids, non-empty titles and a valid kind each.
func TestLoadTop100(t *testing.T) {
	entries, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(entries) != 100 {
		t.Fatalf("entries = %d, want 100", len(entries))
	}
	seen := make(map[string]bool, len(entries))
	for _, e := range entries {
		if e.ExternalID == "" || e.Title == "" {
			t.Fatalf("entry %+v has an empty external_id or title", e)
		}
		if seen[e.ExternalID] {
			t.Fatalf("duplicate external_id %q", e.ExternalID)
		}
		seen[e.ExternalID] = true
		if !e.Kind.Valid() {
			t.Fatalf("entry %q kind = %q, want a valid kind", e.ExternalID, e.Kind)
		}
	}
}
