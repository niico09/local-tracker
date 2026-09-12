// Package config resolves command, flags and environment into one Config.
// Flags override environment variables, which override defaults.
package config

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Config is the fully resolved runtime configuration.
type Config struct {
	Command    string
	Addr       string
	DataDir    string
	DBPath     string
	SessionTTL time.Duration
	UploadMax  int64
	// BackupDest is the snapshot path for `tracker backup`. It must not exist.
	BackupDest string
	// BackupUploads copies <data>/uploads beside the snapshot when true.
	BackupUploads bool
}

// Load parses `tracker [serve|seed|backup] [flags]`.
func Load(args []string) (Config, error) {
	cfg := Config{
		Command:    "serve",
		Addr:       env("TRACKER_ADDR", "0.0.0.0:8080"),
		DataDir:    env("TRACKER_DATA", "./data"),
		SessionTTL: 720 * time.Hour,
		UploadMax:  5 << 20,
	}
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		cfg.Command, args = args[0], args[1:]
	}

	fs := flag.NewFlagSet("tracker", flag.ContinueOnError)
	fs.StringVar(&cfg.Addr, "addr", cfg.Addr, "listen address")
	fs.StringVar(&cfg.DataDir, "data", cfg.DataDir, "data directory")
	dbOverride := fs.String("db", "", "database path (defaults to <data>/tracker.db)")
	fs.DurationVar(&cfg.SessionTTL, "session-ttl", cfg.SessionTTL, "session lifetime")
	fs.Int64Var(&cfg.UploadMax, "upload-max", cfg.UploadMax, "maximum upload size in bytes")
	fs.BoolVar(&cfg.BackupUploads, "uploads", false, "backup: also copy the uploads directory")
	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}

	cfg.DBPath = *dbOverride
	if cfg.DBPath == "" {
		cfg.DBPath = filepath.Join(cfg.DataDir, "tracker.db")
	}
	if fs.NArg() > 0 {
		cfg.BackupDest = fs.Arg(0)
	}
	return cfg, nil
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
