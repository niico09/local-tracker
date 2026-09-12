// Command tracker serves the local-tracker LAN application.
package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"local-tracker/internal/storage"
	"local-tracker/internal/ui"
	"local-tracker/internal/web"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

// run wires the SQLite store and HTTP router, then serves until SIGINT/SIGTERM
// triggers a graceful shutdown.
func run(args []string) error {
	if len(args) > 0 && args[0] == "serve" {
		args = args[1:]
	}

	fs := flag.NewFlagSet("tracker", flag.ContinueOnError)
	addr := fs.String("addr", envOr("TRACKER_ADDR", "0.0.0.0:8080"), "listen address")
	dataDir := fs.String("data", envOr("TRACKER_DATA", "./data"), "data directory")
	dbOverride := fs.String("db", "", "database path (defaults to <data>/tracker.db)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	dbPath := *dbOverride
	if dbPath == "" {
		dbPath = filepath.Join(*dataDir, "tracker.db")
	}

	ctx := context.Background()
	db, err := storage.Open(ctx, dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	log.Printf("sqlite ready at %s (journal_mode=wal, foreign_keys=1)", dbPath)

	srv := &http.Server{
		Addr:              *addr,
		Handler:           web.NewRouter(web.RouterDeps{Health: db.Health, Assets: ui.FS()}),
		ReadHeaderTimeout: 5 * time.Second,
	}

	shutdownCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		log.Printf("listening on %s", *addr)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-shutdownCtx.Done():
		log.Print("shutting down")
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutdown)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
