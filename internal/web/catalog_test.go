package web_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"

	"local-tracker/internal/domain"
	"local-tracker/internal/service"
)

// signIn runs first-run setup and logs in as the first profile.
func signIn(t *testing.T, h *harness) {
	t.Helper()
	if resp := h.do(t, http.MethodPost, "/setup", setupForm()); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("POST /setup = %d, want 303", resp.StatusCode)
	}
	if resp := h.do(t, http.MethodPost, "/login", loginForm("Ada", "1234")); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("POST /login = %d, want 303", resp.StatusCode)
	}
}

func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return string(b)
}

func createItem(t *testing.T, h *harness, title string) string {
	t.Helper()
	resp := h.do(t, http.MethodPost, "/catalog", url.Values{"title": {title}, "kind": {"movie"}})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("create %q = %d, want 303", title, resp.StatusCode)
	}
	location := resp.Header.Get("Location")
	if !strings.HasPrefix(location, "/catalog/") {
		t.Fatalf("create Location = %q, want /catalog/{id}", location)
	}
	return location
}

func uploadCover(t *testing.T, h *harness, path, filename string, data []byte) *http.Response {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, err := mw.CreateFormFile("cover", filename)
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := fw.Write(data); err != nil {
		t.Fatalf("write multipart part: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close multipart: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, h.srv.URL+path, &body)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := h.client.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	return resp
}

func assertNoUploads(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return // nothing was created, which is exactly the point
		}
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("uploads dir has %d files, want 0", len(entries))
	}
}

// TestCatalogCreateAndView covers create -> detail -> list with a plain form.
func TestCatalogCreateAndView(t *testing.T) {
	h := newHarness(t)
	signIn(t, h)

	if resp := h.do(t, http.MethodGet, "/catalog", nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /catalog = %d, want 200", resp.StatusCode)
	}

	resp := h.do(t, http.MethodPost, "/catalog", url.Values{"title": {"Dune"}, "kind": {"movie"}, "year": {"2021"}})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("POST /catalog = %d, want 303", resp.StatusCode)
	}
	detail := resp.Header.Get("Location")

	if got := readBody(t, h.do(t, http.MethodGet, detail, nil)); !strings.Contains(got, "Dune") {
		t.Fatalf("detail page missing title: %s", got)
	}
	if got := readBody(t, h.do(t, http.MethodGet, "/catalog", nil)); !strings.Contains(got, "Dune") {
		t.Fatalf("list page missing title: %s", got)
	}

	// Edit round-trips the stored values.
	if got := readBody(t, h.do(t, http.MethodGet, detail+"/edit", nil)); !strings.Contains(got, "Dune") {
		t.Fatalf("edit page missing title: %s", got)
	}
	resp = h.do(t, http.MethodPost, detail+"/edit", url.Values{"title": {"Dune Part Two"}, "kind": {"movie"}, "year": {"2024"}})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("POST edit = %d, want 303", resp.StatusCode)
	}
	if got := readBody(t, h.do(t, http.MethodGet, detail, nil)); !strings.Contains(got, "Dune Part Two") {
		t.Fatalf("detail page missing updated title: %s", got)
	}
}

// TestCatalogNewBeatsIDWildcard proves the literal /catalog/new route wins over
// /catalog/{id} (a wildcard match would fail to parse "new" and 404).
func TestCatalogNewBeatsIDWildcard(t *testing.T) {
	h := newHarness(t)
	signIn(t, h)

	resp := h.do(t, http.MethodGet, "/catalog/new", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /catalog/new = %d, want 200", resp.StatusCode)
	}
	if got := readBody(t, resp); !strings.Contains(got, "Add item") {
		t.Fatalf("unexpected /catalog/new body: %s", got)
	}
}

// TestCoverUploadAcceptedAndServed exercises the full upload + serve path.
func TestCoverUploadAcceptedAndServed(t *testing.T) {
	h := newHarness(t)
	signIn(t, h)
	detail := createItem(t, h, "Dune")

	resp := uploadCover(t, h, detail+"/cover", "cover.png", pngBytes())
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("cover upload = %d, want 303", resp.StatusCode)
	}
	resp.Body.Close()

	items, err := h.repos.Items.List(context.Background(), domain.NewActor(1))
	if err != nil || len(items) != 1 {
		t.Fatalf("load item = %d, %v; want 1", len(items), err)
	}
	if !service.ValidCoverName(items[0].CoverPath) {
		t.Fatalf("stored cover path %q is not a generated name", items[0].CoverPath)
	}

	resp = h.do(t, http.MethodGet, "/uploads/"+items[0].CoverPath, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET cover = %d, want 200", resp.StatusCode)
	}
	if cc := resp.Header.Get("Cache-Control"); cc != "public, max-age=86400" {
		t.Fatalf("Cache-Control = %q, want public, max-age=86400", cc)
	}
	resp.Body.Close()
}

// TestCoverUploadRejectsOversizeWithoutWriting proves a too-large upload is
// refused and leaves the uploads directory empty.
func TestCoverUploadRejectsOversizeWithoutWriting(t *testing.T) {
	h := newHarness(t)
	signIn(t, h)
	detail := createItem(t, h, "Oversize")

	big := append(pngBytes(), bytes.Repeat([]byte{0x00}, 6<<20)...)
	resp := uploadCover(t, h, detail+"/cover", "big.png", big)
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversize upload = %d, want 413", resp.StatusCode)
	}
	resp.Body.Close()
	assertNoUploads(t, h.uploadsDir)
}

// TestCoverUploadRejectsNonImageWithoutWriting proves a non-image is refused
// and leaves no file behind.
func TestCoverUploadRejectsNonImageWithoutWriting(t *testing.T) {
	h := newHarness(t)
	signIn(t, h)
	detail := createItem(t, h, "Not an image")

	resp := uploadCover(t, h, detail+"/cover", "notes.txt", []byte("this is not an image at all"))
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("non-image upload = %d, want 422", resp.StatusCode)
	}
	resp.Body.Close()
	assertNoUploads(t, h.uploadsDir)
}

// TestUploadsRejectsUngeneratedName proves that a name that is not the generated
// shape is a 404, never a filesystem read.
func TestUploadsRejectsUngeneratedName(t *testing.T) {
	h := newHarness(t)

	for _, name := range []string{"tracker.db", "abc.png", "deadbeefdeadbeefdeadbeefdeadbeef.exe"} {
		resp := h.do(t, http.MethodGet, "/uploads/"+name, nil)
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("GET /uploads/%s = %d, want 404", name, resp.StatusCode)
		}
		resp.Body.Close()
	}
}

// pngBytes is a minimal PNG signature; DetectContentType keys on the signature.
func pngBytes() []byte {
	return append([]byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}, bytes.Repeat([]byte{0x01}, 64)...)
}
