package web

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"local-tracker/internal/domain"
	"local-tracker/internal/ui"
)

var updateGolden = flag.Bool("update", false, "rewrite golden files")

// TestGoldenProgressFragments pins the stable HTMX fragments so an accidental
// markup change is caught. Refresh with:
//
//	go test ./internal/web -run Golden -update
//
// and then re-run without -update.
func TestGoldenProgressFragments(t *testing.T) {
	tmpls, err := parseTemplates(ui.FS())
	if err != nil {
		t.Fatalf("parseTemplates: %v", err)
	}
	cases := []struct {
		name, page, fragment string
		data                 pageData
	}{
		{
			name: "progress_goal", page: "goal_progress", fragment: "progress_goal",
			data: pageData{Rollup: &rollupView{Done: 3, Total: 4}},
		},
		{
			name: "progress_tilt", page: "catalog_tilt", fragment: "progress_tilt",
			data: pageData{Tilt: &tiltView{ItemID: 1, Owner: 0, State: domain.StateCompleted, Done: true}},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := tmpls[c.page].ExecuteTemplate(&buf, c.fragment, c.data); err != nil {
				t.Fatalf("execute %s: %v", c.fragment, err)
			}
			path := filepath.Join("testdata", c.name+".golden.html")
			if *updateGolden {
				if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
					t.Fatalf("write golden: %v", err)
				}
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read golden (run with -update): %v", err)
			}
			if buf.String() != string(want) {
				t.Errorf("golden mismatch for %s:\n got: %q\nwant: %q", c.name, buf.String(), want)
			}
		})
	}
}
