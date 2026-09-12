package service

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"

	"local-tracker/internal/domain"
)

// coverNamePattern is the only filename shape accepted for stored covers: 16
// random bytes in lowercase hex plus an extension derived from the detected
// MIME type. No client string can ever reach the filesystem.
var coverNamePattern = regexp.MustCompile(`^[a-f0-9]{32}\.(jpg|png|gif|webp)$`)

// coverExtensions maps a sniffed MIME type to the extension stored on disk.
var coverExtensions = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/gif":  ".gif",
	"image/webp": ".webp",
}

// CoverStore owns cover files under one directory. Filenames are always
// server-generated, so path traversal is structurally impossible.
type CoverStore struct {
	dir string
	max int64
}

// NewCoverStore wires the upload directory and size limit. A non-positive limit
// falls back to 5 MiB.
func NewCoverStore(dir string, max int64) *CoverStore {
	if max <= 0 {
		max = 5 << 20
	}
	return &CoverStore{dir: dir, max: max}
}

// ValidCoverName reports whether name matches the only accepted filename shape.
func ValidCoverName(name string) bool { return coverNamePattern.MatchString(name) }

// Save validates size and sniffed MIME type, then writes the bytes under a
// fresh random name. Nothing is written when validation fails.
func (s *CoverStore) Save(data []byte) (string, error) {
	if int64(len(data)) > s.max {
		return "", fmt.Errorf("%w: cover exceeds %d bytes", domain.ErrValidation, s.max)
	}
	ext, ok := coverExtensions[http.DetectContentType(data)]
	if !ok {
		return "", fmt.Errorf("%w: cover must be a JPEG, PNG, GIF or WebP image", domain.ErrValidation)
	}
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	name := hex.EncodeToString(buf) + ext
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(s.dir, name), data, 0o644); err != nil {
		return "", err
	}
	return name, nil
}

// Remove deletes a stored cover. Missing files are not an error.
func (s *CoverStore) Remove(name string) error {
	if !ValidCoverName(name) {
		return nil
	}
	if err := os.Remove(filepath.Join(s.dir, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// Open returns the stored cover for a name that matches the strict pattern.
// Anything else, including filesystem errors, is reported as not found so the
// HTTP layer can answer 404 without leaking why.
func (s *CoverStore) Open(name string) (*os.File, error) {
	if !ValidCoverName(name) {
		return nil, domain.ErrNotFound
	}
	f, err := os.Open(filepath.Join(s.dir, name))
	if err != nil {
		return nil, domain.ErrNotFound
	}
	return f, nil
}
