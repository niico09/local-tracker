package coverfetch

import (
	"context"
	"testing"
)

func TestHostAllowlist(t *testing.T) {
	allowed := []string{
		"es.wikipedia.org", "en.wikipedia.org", "upload.wikimedia.org",
		"openlibrary.org", "covers.openlibrary.org", "ia801.us.archive.org",
	}
	for _, host := range allowed {
		if !hostAllowed(host) {
			t.Errorf("hostAllowed(%q) = false, want true", host)
		}
	}
	denied := []string{
		"localhost", "127.0.0.1", "example.com", "evilarchive.org",
		"wikipedia.org.evil.com", "archive.org.evil.com",
	}
	for _, host := range denied {
		if hostAllowed(host) {
			t.Errorf("hostAllowed(%q) = true, want false", host)
		}
	}
}

func TestFetchRejectsNonAllowlistedURLs(t *testing.T) {
	f := New()
	for _, raw := range []string{
		"http://upload.wikimedia.org/x.jpg", // plain http is never allowed
		"https://evil.example.com/x.jpg",    // host not allowlisted
		"file:///etc/passwd",                // scheme not allowed
		"",
	} {
		if _, _, err := f.Fetch(context.Background(), raw); err == nil {
			t.Errorf("Fetch(%q) = nil error, want rejection", raw)
		}
	}
}

func TestSearchRequiresQuery(t *testing.T) {
	if _, err := New().Search(context.Background(), "   "); err == nil {
		t.Error("Search(empty) = nil error, want rejection")
	}
}
