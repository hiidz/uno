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
	h, err := Handler(fsys, "https://nuvio.example.com/base/path")
	if err != nil {
		t.Fatalf("Handler: %v", err)
	}

	for _, path := range []string{"/configure", "/assets/app.js"} {
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
		if _, err := Handler(fstest.MapFS{}, base); err == nil {
			t.Errorf("Handler(%q): got nil error, want one", base)
		}
	}
}
