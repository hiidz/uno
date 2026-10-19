// Package static serves the embedded SPA build, falling back to
// index.html for any path that isn't a real file so React Router can
// resolve client-side routes (e.g. a hard reload on /configure).
package static

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// assetsPrefix is Vite's output directory. Everything under it is named
// with a content hash, so a changed file arrives under a new name and the
// old one can be cached indefinitely. Files outside it — index.html,
// favicon.svg — keep their names across builds and must not be.
const assetsPrefix = "/assets/"

// configPath is the script that carries the server's runtime settings the
// SPA needs, the same ones as the page's configBlockID block. It is answered
// here, never from fsys, so one image serves whatever Nuvio backend and key
// its environment names.
const configPath = "/config.js"

// configBlockID is the id of the <script type="application/json"> element
// the served index.html carries the runtime settings in. JSON in a data
// block runs no code, so it needs no CSP exception the way an inline script
// would.
const configBlockID = "uno-config"

// Config is the server's runtime settings the SPA reads before it starts.
type Config struct {
	// NuvioBaseURL is the Nuvio backend the SPA signs in against.
	NuvioBaseURL string
	// NuvioPublishableKey is public by design
	// (https://nuvio.tv/docs#publishable-key).
	NuvioPublishableKey string
	// TMDBKeyMode is "shared" or "per-account", as GET /api/config says it.
	TMDBKeyMode string
}

// Handler serves fsys as a single-page app: any request that doesn't
// resolve to a real file is rewritten to "/" so the SPA's own router
// handles it, and "/" is answered with fsys's index.html carrying cfg as a
// JSON block (buildPage). configPath is answered with the same settings as a
// script that sets window.__UNO_CONFIG__. Every
// response carries the Content-Security-Policy built by
// contentSecurityPolicy, with cfg.NuvioBaseURL's origin as the one
// cross-origin endpoint the SPA may call.
//
// A build with no index.html, as on a fresh clone before the frontend is
// built, is served as it is.
func Handler(fsys fs.FS, cfg Config) (http.Handler, error) {
	u, err := url.Parse(cfg.NuvioBaseURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("static: Nuvio base URL %q is not an absolute URL", cfg.NuvioBaseURL)
	}
	csp := contentSecurityPolicy(u.Scheme + "://" + u.Host)
	settings := configJSON(cfg)
	script := []byte("window.__UNO_CONFIG__ = " + string(settings) + ";\n")
	index, err := buildPage(fsys, settings)
	if err != nil {
		return nil, err
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET "+configPath, func(w http.ResponseWriter, _ *http.Request) {
		h := w.Header()
		h.Set("Content-Type", "text/javascript; charset=utf-8")
		h.Set("Cache-Control", "no-cache")
		h.Set("Content-Security-Policy", csp)
		_, _ = w.Write(script)
	})
	fileServer := http.FileServerFS(fsys)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "."
		}
		if info, err := fs.Stat(fsys, path); err != nil || info.IsDir() {
			r = cloneWithPath(r, "/")
		}
		h := w.Header()
		h.Set("Cache-Control", cacheControl(r.URL.Path))
		h.Set("Content-Security-Policy", csp)
		if r.URL.Path == "/" && index != nil {
			index.serve(w, r)
			return
		}
		fileServer.ServeHTTP(w, r)
	})
	return mux, nil
}

// configJSON is cfg as the SPA reads it, from both configPath and the
// page's config block. json.Marshal escapes <, > and &, so no value can
// close the block's <script> element early.
func configJSON(cfg Config) []byte {
	// A struct of strings always marshals.
	body, _ := json.Marshal(struct {
		NuvioBaseURL        string `json:"nuvioBaseURL"`
		NuvioPublishableKey string `json:"nuvioPublishableKey"`
		TMDBKeyMode         string `json:"tmdb_key_mode"`
	}{cfg.NuvioBaseURL, cfg.NuvioPublishableKey, cfg.TMDBKeyMode})
	return body
}

// page is a response body built once at startup and served with an ETag,
// so a revalidation that finds it unchanged is a 304.
type page struct {
	body []byte
	etag string
}

// buildPage is fsys's index.html with settings injected as the configBlockID
// block just before </head>, or nil when fsys has no index.html.
func buildPage(fsys fs.FS, settings []byte) (*page, error) {
	html, err := fs.ReadFile(fsys, "index.html")
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("static: reading index.html: %w", err)
	}
	at := bytes.Index(html, []byte("</head>"))
	if at < 0 {
		return nil, errors.New("static: index.html has no </head> to put the config block before")
	}
	block := `<script type="application/json" id="` + configBlockID + `">` + string(settings) + "</script>\n"
	body := make([]byte, 0, len(html)+len(block))
	body = append(append(append(body, html[:at]...), block...), html[at:]...)
	return &page{body: body, etag: etagOf(body)}, nil
}

// serve answers r with p as HTML, through http.ServeContent, which answers
// a matching If-None-Match with a 304 and handles HEAD and Range.
func (p *page) serve(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("ETag", p.etag)
	http.ServeContent(w, r, "index.html", time.Time{}, bytes.NewReader(p.body))
}

// etagOf is a weak ETag for body: weak because gzip, which leaves the ETag
// as it is, serves the same content in a second encoding.
func etagOf(body []byte) string {
	sum := sha256.Sum256(body)
	return `W/"` + hex.EncodeToString(sum[:16]) + `"`
}

// contentSecurityPolicy confines the SPA to its own origin, so a script
// injected into the page can neither load more code nor send the
// localStorage refresh token anywhere but Uno and Nuvio. The exceptions
// are what the build actually needs:
//
//   - style-src 'unsafe-inline': Radix injects <style> elements at runtime.
//   - img-src https: data: — collection and folder art are arbitrary
//     user-supplied URLs, and Vite inlines small images as data: URIs.
//   - font-src data: — Vite inlines the smallest @fontsource subsets.
//   - connect-src nuvioOrigin: login and refresh go straight to Nuvio.
//
// frame-ancestors, base-uri and form-action don't fall back to
// default-src, so they are set explicitly; object-src is tightened from
// 'self' to 'none' since the SPA embeds no plugins.
func contentSecurityPolicy(nuvioOrigin string) string {
	return strings.Join([]string{
		"default-src 'self'",
		"style-src 'self' 'unsafe-inline'",
		"img-src 'self' https: data:",
		"font-src 'self' data:",
		"connect-src 'self' " + nuvioOrigin,
		"frame-ancestors 'none'",
		"base-uri 'self'",
		"form-action 'self'",
		"object-src 'none'",
	}, "; ")
}

// cacheControl keeps index.html revalidating on every load while letting
// hashed assets be cached forever. Caching the shell would strand clients
// on a stale page referencing asset hashes the new build no longer has.
//
// index.html is built at startup and served with an ETag (buildPage), so
// "no-cache" costs it a 304 when nothing changed.
func cacheControl(path string) string {
	if strings.HasPrefix(path, assetsPrefix) {
		return "public, max-age=31536000, immutable"
	}
	return "no-cache"
}

func cloneWithPath(r *http.Request, path string) *http.Request {
	r2 := r.Clone(r.Context())
	r2.URL.Path = path
	return r2
}
