// Command tracker serves the local-tracker LAN application.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"local-tracker/internal/config"
	"local-tracker/internal/service"
	"local-tracker/internal/storage"
	"local-tracker/internal/ui"
	"local-tracker/internal/web"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

// run wires config, SQLite, the auth service and the HTTP server, then serves
// until SIGINT/SIGTERM triggers a graceful shutdown.
func run(args []string) error {
	cfg, err := config.Load(args)
	if err != nil {
		return err
	}
	switch cfg.Command {
	case "serve":
	case "seed", "backup":
		return fmt.Errorf("command %q is not implemented until slice S6", cfg.Command)
	default:
		return fmt.Errorf("unknown command %q", cfg.Command)
	}

	ctx := context.Background()
	db, err := storage.Open(ctx, cfg.DBPath)
	if err != nil {
		return err
	}
	defer db.Close()
	log.Printf("sqlite ready at %s (journal_mode=wal, foreign_keys=1)", cfg.DBPath)

	repos := storage.NewRepos(db)
	auth := service.NewAuth(repos.Users, repos.Sessions, cfg.SessionTTL)
	covers := service.NewCoverStore(filepath.Join(cfg.DataDir, "uploads"), cfg.UploadMax)
	catalog := service.NewCatalog(repos.Items, covers)
	goals := service.NewGoals(repos.Goals)
	handler, err := web.NewServer(web.RouterDeps{
		Health:    db.Health,
		Assets:    ui.FS(),
		Auth:      auth,
		Catalog:   catalog,
		Goals:     goals,
		Covers:    covers,
		UploadMax: cfg.UploadMax,
	})
	if err != nil {
		return err
	}

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	shutdownCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		log.Printf("listening on %s", cfg.Addr)
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
