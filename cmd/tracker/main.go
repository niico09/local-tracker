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
	"local-tracker/internal/coverfetch"
	"local-tracker/internal/seed"
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

// run dispatches on the subcommand: serve (default), seed, or backup.
func run(args []string) error {
	cfg, err := config.Load(args)
	if err != nil {
		return err
	}
	switch cfg.Command {
	case "serve":
		return serve(cfg)
	case "seed":
		return runSeed(cfg)
	case "backup":
		return runBackup(cfg)
	default:
		return fmt.Errorf("unknown command %q", cfg.Command)
	}
}

// runSeed applies the embedded Top 100 list. It is idempotent: a second run
// inserts nothing and reports the same database counts.
func runSeed(cfg config.Config) error {
	ctx := context.Background()
	db, err := storage.Open(ctx, cfg.DBPath)
	if err != nil {
		return err
	}
	defer db.Close()

	seeder := service.NewSeed(storage.NewRepos(db).Seed, seed.Load)
	result, err := seeder.Run(ctx)
	if err != nil {
		return err
	}
	log.Printf("seed: goal created=%t items inserted=%d kept=%d memberships added=%d",
		result.GoalCreated, result.ItemsInserted, result.ItemsKept, result.MembershipsAdded)
	return nil
}

// runBackup writes a WAL-safe snapshot with VACUUM INTO and, with -uploads,
// copies the uploads tree beside it.
func runBackup(cfg config.Config) error {
	if cfg.BackupDest == "" {
		return errors.New("usage: tracker backup [-uploads] <destination>")
	}
	ctx := context.Background()
	db, err := storage.Open(ctx, cfg.DBPath)
	if err != nil {
		return err
	}
	defer db.Close()

	if err := storage.Backup(ctx, db, cfg.BackupDest, filepath.Join(cfg.DataDir, "uploads"), cfg.BackupUploads); err != nil {
		return err
	}
	log.Printf("backup: wrote %s", cfg.BackupDest)
	return nil
}

// serve wires config, SQLite, the use cases and the HTTP server, then serves
// until SIGINT/SIGTERM triggers a graceful shutdown.
func serve(cfg config.Config) error {
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
	members := service.NewMembership(repos.Membership)
	progress := service.NewProgress(repos.Progress)
	reviews := service.NewReviews(repos.Reviews)
	finder := coverfetch.New()

	// backupSnapshot produces a WAL-safe snapshot in the OS temp directory for
	// the UI download route. The handler removes the file right after serving.
	backupSnapshot := func(ctx context.Context) (string, error) {
		tmp, err := os.CreateTemp("", "tracker-snapshot-*.db")
		if err != nil {
			return "", err
		}
		name := tmp.Name()
		if err := tmp.Close(); err != nil {
			return "", err
		}
		// VACUUM INTO refuses an existing destination, so the reserved name is
		// dropped right before the snapshot is written.
		if err := os.Remove(name); err != nil {
			return "", err
		}
		if err := storage.Backup(ctx, db, name, "", false); err != nil {
			return "", err
		}
		return name, nil
	}

	handler, err := web.NewServer(web.RouterDeps{
		Health:      db.Health,
		Assets:      ui.FS(),
		Auth:        auth,
		Catalog:     catalog,
		Goals:       goals,
		Membership:  members,
		Progress:    progress,
		Reviews:     reviews,
		Covers:      covers,
		UploadMax:   cfg.UploadMax,
		CoverFinder: finder,
		Backup:      backupSnapshot,
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
