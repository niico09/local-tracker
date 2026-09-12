package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Backup writes a transactionally consistent snapshot of the database with
// VACUUM INTO and then atomically renames it to dest. VACUUM INTO is the
// WAL-safe choice: it never copies a half-written journal. dest must not
// pre-exist. When includeUploads is set the uploads tree is copied beside the
// snapshot to dest+".uploads".
func Backup(ctx context.Context, db *DB, dest, uploadsDir string, includeUploads bool) error {
	if db == nil || db.raw == nil {
		return errors.New("backup: database is not open")
	}
	if dest == "" {
		return errors.New("backup: destination path is required")
	}
	if _, err := os.Stat(dest); err == nil {
		return fmt.Errorf("backup: destination %s already exists", dest)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("backup: %w", err)
	}

	dir := filepath.Dir(dest)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("backup: create destination directory: %w", err)
	}

	// VACUUM INTO refuses to write to an existing file, so reserve a unique
	// temporary name in the destination directory and drop the placeholder.
	tmp, err := os.CreateTemp(dir, ".tracker-backup-*")
	if err != nil {
		return fmt.Errorf("backup: reserve temp file: %w", err)
	}
	tmpPath := tmp.Name()
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("backup: close temp file: %w", err)
	}
	if err := os.Remove(tmpPath); err != nil {
		return fmt.Errorf("backup: clear temp file: %w", err)
	}

	// The target is bound as a parameter, so a path containing quotes cannot
	// break the statement.
	if _, err := db.raw.ExecContext(ctx, "VACUUM INTO ?", tmpPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("backup: vacuum into: %w", err)
	}
	// Same-directory rename: atomic, so readers never observe a partial file.
	if err := os.Rename(tmpPath, dest); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("backup: rename: %w", err)
	}

	if includeUploads {
		if err := copyTree(uploadsDir, dest+".uploads"); err != nil {
			return fmt.Errorf("backup: copy uploads: %w", err)
		}
	}
	return nil
}

// copyTree recursively copies a directory. A missing source is not an error:
// a fresh install simply has no uploads yet.
func copyTree(src, dst string) error {
	info, err := os.Stat(src)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dst, info.Mode().Perm()); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		from := filepath.Join(src, e.Name())
		to := filepath.Join(dst, e.Name())
		if e.IsDir() {
			if err := copyTree(from, to); err != nil {
				return err
			}
			continue
		}
		if err := copyFile(from, to); err != nil {
			return err
		}
	}
	return nil
}

// copyFile copies one regular file, failing rather than overwriting.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}
