// Package static serves the embedded SPA build, falling back to
// index.html for any path that isn't a real file so React Router can
// resolve client-side routes (e.g. a hard reload on /p/1/community).
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

// configBlockID is the id of the <script type="application/json"> element
// the served index.html carries the runtime settings in, so one image serves
// whatever Nuvio backend and key its environment names. JSON in a data block
// runs no code, so it needs no CSP exception the way an inline script would.
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
	// Community is "on", "no-images" or "off", as GET /api/config says it.
	Community string
}

// Handler serves fsys as a single-page app: any request that doesn't
// resolve to a real file is rewritten to "/" so the SPA's own router
// handles it, unless it asked for a file (isSPARoute), and "/" is answered
// with fsys's index.html carrying cfg as a JSON block (buildPage). Every
// response carries the Content-Security-Policy
// built by contentSecurityPolicy, with cfg.NuvioBaseURL's origin as the one
// cross-origin endpoint the SPA may call.
func Handler(fsys fs.FS, cfg Config) (http.Handler, error) {
	u, err := url.Parse(cfg.NuvioBaseURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("static: Nuvio base URL %q is not an absolute URL", cfg.NuvioBaseURL)
	}
	nuvioOrigin := u.Scheme + "://" + u.Host
	csp := contentSecurityPolicy(nuvioOrigin)
	// The SPA's first calls go to Nuvio, as anonymous CORS fetches, so the
	// page opens that connection early; without crossorigin the browser would
	// open one those fetches can't use.
	preconnect := "<" + nuvioOrigin + ">; rel=preconnect; crossorigin"
	index, err := buildPage(fsys, configJSON(cfg))
	if err != nil {
		return nil, err
	}
	etags, err := unhashedETags(fsys)
	if err != nil {
		return nil, err
	}

	mux := http.NewServeMux()
	fileServer := http.FileServerFS(fsys)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "."
		}
		if info, err := fs.Stat(fsys, path); err != nil || info.IsDir() {
			if !isSPARoute(r.URL.Path) {
				// Before Cache-Control, so a missing asset's 404 is never
				// marked immutable.
				http.NotFound(w, r)
				return
			}
			r = cloneWithPath(r, "/")
		}
		h := w.Header()
		h.Set("Cache-Control", cacheControl(r.URL.Path))
		h.Set("Content-Security-Policy", csp)
		if r.URL.Path == "/" {
			h.Set("Link", preconnect)
			index.serve(w, r)
			return
		}
		if etag, ok := etags[r.URL.Path]; ok {
			// The file server answers a matching If-None-Match with a 304.
			h.Set("ETag", etag)
		}
		fileServer.ServeHTTP(w, r)
	})
	return mux, nil
}

// unhashedETags maps the path of every file outside assetsPrefix — the ones
// that keep their names across builds, such as favicon.svg — to an ETag of
// its content. Embedded files carry a zero ModTime, so without one net/http
// sends neither Last-Modified nor ETag, and each revalidation would
// re-fetch the whole file. index.html is left out: buildPage serves it.
func unhashedETags(fsys fs.FS) (map[string]string, error) {
	etags := map[string]string{}
	err := fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return err
		case d.IsDir() && "/"+path+"/" == assetsPrefix:
			return fs.SkipDir
		case d.IsDir() || path == "index.html":
			return nil
		}
		body, err := fs.ReadFile(fsys, path)
		if err != nil {
			return err
		}
		etags["/"+path] = etagOf(body)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("static: hashing the build's unhashed files: %w", err)
	}
	return etags, nil
}

// configJSON is cfg as the SPA reads it from the page's config block.
// json.Marshal escapes <, > and &, so no value can
// close the block's <script> element early.
func configJSON(cfg Config) []byte {
	// A struct of strings always marshals.
	body, _ := json.Marshal(struct {
		NuvioBaseURL        string `json:"nuvioBaseURL"`
		NuvioPublishableKey string `json:"nuvioPublishableKey"`
		TMDBKeyMode         string `json:"tmdb_key_mode"`
		Community           string `json:"community"`
	}{cfg.NuvioBaseURL, cfg.NuvioPublishableKey, cfg.TMDBKeyMode, cfg.Community})
	return body
}

// page is a response body built once at startup and served with an ETag,
// so a revalidation that finds it unchanged is a 304.
type page struct {
	body []byte
	etag string
}

// unbuiltPage stands in for index.html when fsys has none, as on a fresh
// clone before the frontend is built. It carries only the config block, which
// is what vite dev reads from this server's "/".
const unbuiltPage = "<!doctype html><html><head></head><body></body></html>"

// buildPage is fsys's index.html, or unbuiltPage when fsys has none, with
// settings injected as the configBlockID block just before </head>.
func buildPage(fsys fs.FS, settings []byte) (*page, error) {
	html, err := fs.ReadFile(fsys, "index.html")
	if errors.Is(err, fs.ErrNotExist) {
		html, err = []byte(unbuiltPage), nil
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
// Every file outside assetsPrefix carries an ETag (buildPage for index.html,
// unhashedETags for the rest), so "no-cache" costs it a 304 when nothing
// changed.
func cacheControl(path string) string {
	if strings.HasPrefix(path, assetsPrefix) {
		return "public, max-age=31536000, immutable"
	}
	return "no-cache"
}

// isSPARoute reports whether a path no file answers is the SPA's to route,
// and so answered with index.html. A missing asset or favicon is a 404
// instead: the browser asked for a file, and HTML in its place would be cached
// or parsed as one.
func isSPARoute(path string) bool {
	return !strings.HasPrefix(path, assetsPrefix) && path != "/favicon.ico"
}

func cloneWithPath(r *http.Request, path string) *http.Request {
	r2 := r.Clone(r.Context())
	r2.URL.Path = path
	return r2
}
