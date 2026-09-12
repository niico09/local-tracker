package ui

import (
	"bytes"
	"io/fs"
	"strings"
	"testing"
)

func TestGeneratedAppCSSIsNonTrivial(t *testing.T) {
	data, err := files.ReadFile("static/app.css")
	if err != nil {
		t.Fatalf("read embedded static/app.css: %v", err)
	}
	if len(data) < 1000 {
		t.Errorf("app.css is %d bytes; run `make css`", len(data))
	}
	if !bytes.Contains(data, []byte("tailwindcss")) {
		t.Error("app.css missing tailwindcss marker; run `make css`")
	}
}

// TestManifestEmbeddedAndNoServiceWorker proves the manifest ships in the binary
// and that no service worker is embedded anywhere: the app is served over LAN
// HTTP and claims no offline behavior.
func TestManifestEmbeddedAndNoServiceWorker(t *testing.T) {
	data, err := files.ReadFile("manifest.webmanifest")
	if err != nil {
		t.Fatalf("read embedded manifest.webmanifest: %v", err)
	}
	if !bytes.Contains(data, []byte(`"start_url"`)) {
		t.Error("manifest missing start_url")
	}

	err = fs.WalkDir(files, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		name := strings.ToLower(d.Name())
		if name == "sw.js" || strings.Contains(name, "service-worker") || strings.Contains(name, "serviceworker") {
			t.Errorf("embedded service worker found at %s; none is allowed", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk embedded fs: %v", err)
	}
}
