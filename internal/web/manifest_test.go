package web_test

import (
	"net/http"
	"strings"
	"testing"
)

// TestManifestServedWithoutServiceWorker is S6.3/S6.4: the manifest is served
// publicly and the app registers no service worker at all (LAN HTTP, no offline
// mode).
func TestManifestServedWithoutServiceWorker(t *testing.T) {
	h := newHarness(t)

	resp := h.do(t, http.MethodGet, "/manifest.webmanifest", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /manifest.webmanifest = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "manifest") {
		t.Errorf("manifest content type = %q, want application/manifest+json", ct)
	}
	body := readBody(t, resp)
	for _, want := range []string{`"start_url"`, `"display"`, `"icons"`} {
		if !strings.Contains(body, want) {
			t.Errorf("manifest missing %s", want)
		}
	}
	lower := strings.ToLower(body)
	if strings.Contains(lower, "serviceworker") || strings.Contains(lower, "service_worker") {
		t.Error("manifest declares a service worker; none is allowed")
	}
}
