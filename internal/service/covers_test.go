package service

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"local-tracker/internal/domain"
)

// pngBytes is a minimal PNG signature with trailing data. DetectContentType
// keys on the signature, which is all the store needs.
func pngBytes() []byte {
	return append([]byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}, bytes.Repeat([]byte{0x01}, 64)...)
}

func TestCoverStoreSaveValid(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "uploads")
	store := NewCoverStore(dir, 5<<20)

	name, err := store.Save(pngBytes())
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if !ValidCoverName(name) {
		t.Fatalf("Save returned invalid name %q", name)
	}
	if filepath.Ext(name) != ".png" {
		t.Fatalf("extension = %q, want .png", filepath.Ext(name))
	}
	if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
		t.Fatalf("stored file missing: %v", err)
	}
}

// TestCoverStoreRejectsOversizeWithoutWriting proves a rejected upload leaves
// no file behind.
func TestCoverStoreRejectsOversizeWithoutWriting(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "uploads")
	store := NewCoverStore(dir, 16)

	oversize := append(pngBytes(), make([]byte, 64)...)
	if _, err := store.Save(oversize); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("oversize Save = %v, want ErrValidation", err)
	}
	assertDirEmpty(t, dir)
}

// TestCoverStoreRejectsNonImageWithoutWriting proves a non-image is rejected
// before any write.
func TestCoverStoreRejectsNonImageWithoutWriting(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "uploads")
	store := NewCoverStore(dir, 5<<20)

	if _, err := store.Save([]byte("this is plainly not an image")); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("non-image Save = %v, want ErrValidation", err)
	}
	assertDirEmpty(t, dir)
}

// TestCoverStoreOpenRejectsBadNames proves nothing but the generated shape is
// ever joined to a path.
func TestCoverStoreOpenRejectsBadNames(t *testing.T) {
	store := NewCoverStore(t.TempDir(), 5<<20)
	bad := []string{"../tracker.db", "abc.png", "deadbeefdeadbeefdeadbeefdeadbeef.exe", "", "DEADBEEFDEADBEEFDEADBEEFDEADBEEF.png"}
	for _, name := range bad {
		if _, err := store.Open(name); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("Open(%q) = %v, want ErrNotFound", name, err)
		}
	}
}

func assertDirEmpty(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return // nothing was created, which is exactly the point
		}
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("directory %s has %d files, want 0", dir, len(entries))
	}
}
