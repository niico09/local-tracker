package web

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// handleBackup streams a fresh database snapshot as a file download. The
// snapshot is produced by the wired backup function and removed right after it
// is served, so temp space never accumulates stale copies.
func handleBackup(backup func(ctx context.Context) (string, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path, err := backup(r.Context())
		if err != nil {
			fail(w, r, err)
			return
		}
		defer func() { _ = os.Remove(path) }()

		f, err := os.Open(path)
		if err != nil {
			fail(w, r, err)
			return
		}
		defer f.Close()

		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", `attachment; filename="tracker-`+time.Now().Format("2006-01-02")+`.db"`)
		http.ServeContent(w, r, filepath.Base(path), time.Now(), f)
	}
}
