// Package coverfetch searches public cover sources and downloads one image.
// It is the only place the server talks to the outside world: every request is
// bounded, allowlisted and size-limited.
package coverfetch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Candidate is one search hit the user may preview and adopt.
type Candidate struct {
	Label    string
	ThumbURL string
	FullURL  string
}

const (
	// searchLimit caps the candidates one query returns.
	searchLimit = 6
	// maxImage caps a downloaded image, mirroring the upload spirit.
	maxImage = 6 << 20
	// maxJSON caps every JSON response.
	maxJSON = 1 << 20
)

// Finder searches Wikipedia and Open Library and fetches candidate images.
type Finder struct {
	client *http.Client
}

// New builds the finder with a bounded client. Every redirect hop is checked
// against the same allowlist as the original URL, so a redirect cannot turn the
// fetcher into a proxy for an arbitrary host.
func New() *Finder {
	f := &Finder{}
	f.client = &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("too many redirects")
			}
			if !hostAllowed(req.URL.Hostname()) {
				return fmt.Errorf("redirect to %q is not allowed", req.URL.Host)
			}
			return nil
		},
	}
	return f
}

// hostAllowed is the allowlist for every URL this package fetches: the public
// cover sources the UI offers, plus the archive.org hosts Open Library cover
// URLs redirect to.
func hostAllowed(host string) bool {
	host = strings.ToLower(host)
	switch host {
	case "es.wikipedia.org", "en.wikipedia.org", "upload.wikimedia.org",
		"openlibrary.org", "covers.openlibrary.org":
		return true
	}
	return strings.HasSuffix(host, ".archive.org")
}

// Search queries the sources in order and returns up to searchLimit unique
// candidates. A source that fails is skipped; only an empty overall result is
// an error.
func (f *Finder) Search(ctx context.Context, query string) ([]Candidate, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, errors.New("cover search: empty query")
	}
	var out []Candidate
	if c, err := f.wikipedia(ctx, "es", query); err == nil && len(c) > 0 {
		out = append(out, c...)
	} else if c, err := f.wikipedia(ctx, "en", query); err == nil && len(c) > 0 {
		out = append(out, c...)
	}
	if c, err := f.openLibrary(ctx, query); err == nil {
		out = append(out, c...)
	}
	if len(out) > searchLimit {
		out = out[:searchLimit]
	}
	if len(out) == 0 {
		return nil, errors.New("cover search: no results")
	}
	return out, nil
}

// wikipedia reads the page's lead image, which for works is the cover or poster.
func (f *Finder) wikipedia(ctx context.Context, lang, query string) ([]Candidate, error) {
	endpoint := "https://" + lang + ".wikipedia.org/w/api.php?action=query&format=json&redirects=1" +
		"&prop=pageimages&piprop=thumbnail&pithumbsize=560&pilicense=any&titles=" + url.QueryEscape(query)
	var payload struct {
		Query struct {
			Pages map[string]struct {
				Title     string `json:"title"`
				Thumbnail *struct {
					Source string `json:"source"`
				} `json:"thumbnail"`
			} `json:"pages"`
		} `json:"query"`
	}
	if err := f.getJSON(ctx, endpoint, &payload); err != nil {
		return nil, err
	}
	var out []Candidate
	for _, page := range payload.Query.Pages {
		if page.Thumbnail == nil || page.Thumbnail.Source == "" {
			continue
		}
		out = append(out, Candidate{
			Label:    "Wikipedia · " + page.Title,
			ThumbURL: page.Thumbnail.Source,
			FullURL:  page.Thumbnail.Source,
		})
	}
	return out, nil
}

// openLibrary searches by text and maps cover ids to the covers API URLs.
func (f *Finder) openLibrary(ctx context.Context, query string) ([]Candidate, error) {
	endpoint := "https://openlibrary.org/search.json?limit=4&fields=title,cover_i&q=" + url.QueryEscape(query)
	var payload struct {
		Docs []struct {
			Title   string `json:"title"`
			CoverID int    `json:"cover_i"`
		} `json:"docs"`
	}
	if err := f.getJSON(ctx, endpoint, &payload); err != nil {
		return nil, err
	}
	var out []Candidate
	for _, doc := range payload.Docs {
		if doc.CoverID == 0 {
			continue
		}
		id := strconv.Itoa(doc.CoverID)
		out = append(out, Candidate{
			Label:    "Open Library · " + doc.Title,
			ThumbURL: "https://covers.openlibrary.org/b/id/" + id + "-M.jpg",
			FullURL:  "https://covers.openlibrary.org/b/id/" + id + "-L.jpg",
		})
	}
	return out, nil
}

// Fetch downloads one candidate image after allowlist validation. The bytes
// are bounded and the content type must claim an image before anything is
// handed to the cover store.
func (f *Finder) Fetch(ctx context.Context, raw string) ([]byte, string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || !hostAllowed(u.Hostname()) {
		return nil, "", errors.New("cover fetch: url is not allowed")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", "local-tracker/1.0 (personal LAN app)")
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("cover fetch: HTTP %d", resp.StatusCode)
	}
	// Content-Type may carry parameters; only the prefix matters here, and the
	// cover store re-sniffs the bytes before anything is stored.
	contentType := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "image/") {
		return nil, "", fmt.Errorf("cover fetch: not an image (%s)", contentType)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxImage+1))
	if err != nil {
		return nil, "", err
	}
	if len(data) > maxImage {
		return nil, "", fmt.Errorf("cover fetch: image exceeds %d bytes", maxImage)
	}
	return data, contentType, nil
}

// getJSON performs one allowlisted JSON GET with a bounded body.
func (f *Finder) getJSON(ctx context.Context, endpoint string, dst any) error {
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "https" || !hostAllowed(u.Hostname()) {
		return errors.New("cover search: url is not allowed")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "local-tracker/1.0 (personal LAN app)")
	resp, err := f.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("cover search: HTTP %d", resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, maxJSON)).Decode(dst)
}
