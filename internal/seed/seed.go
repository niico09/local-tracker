// Package seed loads the embedded Top 100 series list.
//
// The shipped list is a PLACEHOLDER: no authoritative Top 100 series list was
// supplied, so every title is generic. Replace top100.json with the real list
// before relying on it. Only external_id must stay unique and stable, because
// idempotency is keyed on it.
package seed

import (
	_ "embed"
	"encoding/json"
	"fmt"

	"local-tracker/internal/domain"
)

//go:embed top100.json
var top100JSON []byte

// file mirrors the JSON envelope. The note is documentation for whoever edits
// the list; only entries are loaded into the database.
type file struct {
	Note    string  `json:"_note"`
	Entries []entry `json:"entries"`
}

// entry is one row of the data file. year is optional.
type entry struct {
	ExternalID string `json:"external_id"`
	Title      string `json:"title"`
	Kind       string `json:"kind"`
	Year       *int   `json:"year"`
}

// Load parses the embedded list into domain entries. It rejects an empty list
// or a malformed row so a corrupted embed fails loudly instead of silently
// seeding nothing.
func Load() ([]domain.SeedEntry, error) {
	var f file
	if err := json.Unmarshal(top100JSON, &f); err != nil {
		return nil, fmt.Errorf("parse top100.json: %w", err)
	}
	if len(f.Entries) == 0 {
		return nil, fmt.Errorf("top100.json: no entries")
	}
	out := make([]domain.SeedEntry, 0, len(f.Entries))
	for i, e := range f.Entries {
		if e.ExternalID == "" || e.Title == "" {
			return nil, fmt.Errorf("top100.json entry %d: external_id and title are required", i)
		}
		kind := domain.Kind(e.Kind)
		if !kind.Valid() {
			return nil, fmt.Errorf("top100.json entry %q: invalid kind %q", e.ExternalID, e.Kind)
		}
		out = append(out, domain.SeedEntry{
			ExternalID: e.ExternalID,
			Title:      e.Title,
			Kind:       kind,
			Year:       e.Year,
		})
	}
	return out, nil
}
