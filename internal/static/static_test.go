package static

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

// TestHandlerSetsSecurityHeaders covers both a real file and a path that
// falls back to index.html, and shows connect-src carries only the Nuvio
// base URL's origin, not its path.
func TestHandlerSetsSecurityHeaders(t *testing.T) {
	fsys := fstest.MapFS{
		"index.html":    {Data: []byte("<!doctype html>")},
		"assets/app.js": {Data: []byte("console.log(1)")},
	}
	h, err := Handler(fsys, "https://nuvio.example.com/base/path", "publishable-key")
	if err != nil {
		t.Fatalf("Handler: %v", err)
	}

	for _, path := range []string{"/configure", "/assets/app.js", "/config.js"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))

		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status %d, want 200", path, rec.Code)
		}
		csp := rec.Header().Get("Content-Security-Policy")
		if !strings.Contains(csp, "connect-src 'self' https://nuvio.example.com;") {
			t.Errorf("%s: CSP connect-src wrong: %q", path, csp)
		}
		if !strings.Contains(csp, "frame-ancestors 'none'") {
			t.Errorf("%s: CSP missing frame-ancestors: %q", path, csp)
		}
		if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
			t.Errorf("%s: X-Content-Type-Options = %q, want nosniff", path, got)
		}
	}
}

func TestHandlerRejectsNonAbsoluteNuvioURL(t *testing.T) {
	for _, base := range []string{"", "api.nuvio.tv", "/rest/v1", "://bad"} {
		if _, err := Handler(fstest.MapFS{}, base, ""); err == nil {
			t.Errorf("Handler(%q): got nil error, want one", base)
		}
	}
}

// TestHandlerServesConfigScript shows /config.js carries the settings the
// SPA reads, is revalidated on every load, and is never read from the
// build, even when a file of that name is in it.
func TestHandlerServesConfigScript(t *testing.T) {
	fsys := fstest.MapFS{
		"index.html": {Data: []byte("<!doctype html>")},
		"config.js":  {Data: []byte("stale")},
	}
	h, err := Handler(fsys, "https://nuvio.example.com", `key-"x`)
	if err != nil {
		t.Fatalf("Handler: %v", err)
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/config.js", nil))

	want := `window.__UNO_CONFIG__ = {"nuvioBaseURL":"https://nuvio.example.com","nuvioPublishableKey":"key-\"x"};` + "\n"
	if got := rec.Body.String(); got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
	if got := rec.Header().Get("Content-Type"); got != "text/javascript; charset=utf-8" {
		t.Errorf("Content-Type = %q", got)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-cache" {
		t.Errorf("Cache-Control = %q, want no-cache", got)
	}
}
