package static

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

// shell is the smallest index.html a build carries: Handler puts the config
// block before its </head>.
const shell = "<!doctype html><html><head><title>Uno</title></head><body></body></html>"

// testConfig is a server's settings for tests, the key carrying a quote and
// a closing script tag so the encoding shows.
var testConfig = Config{
	NuvioBaseURL:        "https://nuvio.example.com",
	NuvioPublishableKey: `key-"x</script>`,
	TMDBKeyMode:         "per-account",
}

// testSettings is testConfig as JSON, the block's and the script's payload,
// with < and > escaped the way encoding/json writes them.
var testSettings = strings.NewReplacer("<", `\`+"u003c", ">", `\`+"u003e").Replace(
	`{"nuvioBaseURL":"https://nuvio.example.com","nuvioPublishableKey":"key-\"x</script>","tmdb_key_mode":"per-account"}`)

func newTestHandler(t *testing.T, fsys fstest.MapFS, cfg Config) http.Handler {
	t.Helper()
	h, err := Handler(fsys, cfg)
	if err != nil {
		t.Fatalf("Handler: %v", err)
	}
	return h
}

// TestHandlerSetsSecurityHeaders covers both a real file and a path that
// falls back to index.html, and shows connect-src carries only the Nuvio
// base URL's origin, not its path.
func TestHandlerSetsSecurityHeaders(t *testing.T) {
	fsys := fstest.MapFS{
		"index.html":    {Data: []byte(shell)},
		"assets/app.js": {Data: []byte("console.log(1)")},
	}
	h := newTestHandler(t, fsys, Config{NuvioBaseURL: "https://nuvio.example.com/base/path"})

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
	}
}

func TestHandlerRejectsNonAbsoluteNuvioURL(t *testing.T) {
	for _, base := range []string{"", "api.nuvio.tv", "/rest/v1", "://bad"} {
		if _, err := Handler(fstest.MapFS{}, Config{NuvioBaseURL: base}); err == nil {
			t.Errorf("Handler(%q): got nil error, want one", base)
		}
	}
}

// TestHandlerServesConfigScript shows /config.js carries the settings the
// SPA reads, is revalidated on every load, and is never read from the
// build, even when a file of that name is in it.
func TestHandlerServesConfigScript(t *testing.T) {
	fsys := fstest.MapFS{
		"index.html": {Data: []byte(shell)},
		"config.js":  {Data: []byte("stale")},
	}
	h := newTestHandler(t, fsys, testConfig)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/config.js", nil))

	want := "window.__UNO_CONFIG__ = " + testSettings + ";\n"
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

// TestHandlerServesThePageWithItsConfigBlock shows index.html, at "/" and
// at any SPA route, carries the settings as a JSON block before </head>,
// preconnects to Nuvio, and has an ETag a revalidation is answered against with a 304.
func TestHandlerServesThePageWithItsConfigBlock(t *testing.T) {
	h := newTestHandler(t, fstest.MapFS{"index.html": {Data: []byte(shell)}}, testConfig)

	wantBody := strings.Replace(shell, "</head>",
		`<script type="application/json" id="uno-config">`+testSettings+"</script>\n</head>", 1)
	var etag string
	for _, path := range []string{"/", "/configure"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK || rec.Body.String() != wantBody {
			t.Fatalf("GET %s = %d %q, want 200 %q", path, rec.Code, rec.Body.String(), wantBody)
		}
		if got := rec.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
			t.Errorf("GET %s: Content-Type = %q", path, got)
		}
		if got := rec.Header().Get("Cache-Control"); got != "no-cache" {
			t.Errorf("GET %s: Cache-Control = %q, want no-cache", path, got)
		}
		if got, want := rec.Header().Get("Link"), "<https://nuvio.example.com>; rel=preconnect; crossorigin"; got != want {
			t.Errorf("GET %s: Link = %q, want %q", path, got, want)
		}
		etag = rec.Header().Get("ETag")
		if etag == "" {
			t.Fatalf("GET %s: no ETag", path)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("If-None-Match", etag)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotModified {
		t.Errorf("revalidating with its ETag = %d, want 304", rec.Code)
	}
}

// A build with no index.html, as on a fresh clone, still serves the config
// block at "/"; a page with nowhere to put the block makes no handler.
func TestHandlerPageInjectionPoint(t *testing.T) {
	h := newTestHandler(t, fstest.MapFS{".gitkeep": {}}, testConfig)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `<script type="application/json" id="uno-config">`+testSettings+"</script>") {
		t.Errorf("GET / with no index.html = %d %q, want 200 with the config block", rec.Code, rec.Body.String())
	}
	if _, err := Handler(fstest.MapFS{"index.html": {Data: []byte("<!doctype html>")}}, testConfig); err == nil {
		t.Error("a page with no </head>: got a handler, want an error")
	}
}

// A file the build doesn't have is a 404 under /assets/ and at
// /favicon.ico, with no year-long cache header, while any other unknown
// path is still the SPA's page.
func TestHandlerAnswersAMissingFileWithNotFound(t *testing.T) {
	h := newTestHandler(t, fstest.MapFS{"index.html": {Data: []byte(shell)}}, testConfig)
	for _, path := range []string{"/assets/gone-abc123.js", "/assets/", "/favicon.ico"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", path, rec.Code)
		}
		if got := rec.Header().Get("Cache-Control"); got != "" {
			t.Errorf("GET %s: Cache-Control = %q, want none", path, got)
		}
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/no/such/route", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "uno-config") {
		t.Errorf("GET /no/such/route = %d, want the page", rec.Code)
	}
}

// A file outside /assets/ revalidates on every load and is answered with a
// 304 when its ETag still matches.
func TestHandlerRevalidatesAnUnhashedFile(t *testing.T) {
	fsys := fstest.MapFS{
		"index.html":    {Data: []byte(shell)},
		"favicon.svg":   {Data: []byte("<svg/>")},
		"assets/app.js": {Data: []byte("console.log(1)")},
	}
	h := newTestHandler(t, fsys, testConfig)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/favicon.svg", nil))
	etag := rec.Header().Get("ETag")
	if rec.Code != http.StatusOK || etag == "" || rec.Header().Get("Cache-Control") != "no-cache" {
		t.Fatalf("GET /favicon.svg = %d, ETag %q, Cache-Control %q; want 200, an ETag, no-cache",
			rec.Code, etag, rec.Header().Get("Cache-Control"))
	}

	req := httptest.NewRequest(http.MethodGet, "/favicon.svg", nil)
	req.Header.Set("If-None-Match", etag)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotModified {
		t.Errorf("revalidating /favicon.svg with its ETag = %d, want 304", rec.Code)
	}
}
