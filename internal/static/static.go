// Package static serves the embedded SPA build, falling back to
// index.html for any path that isn't a real file so React Router can
// resolve client-side routes (e.g. a hard reload on /configure).
package static

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"net/url"
	"strings"
)

// assetsPrefix is Vite's output directory. Everything under it is named
// with a content hash, so a changed file arrives under a new name and the
// old one can be cached indefinitely. Files outside it — index.html,
// favicon.svg — keep their names across builds and must not be.
const assetsPrefix = "/assets/"

// configPath is the script index.html loads before the app, carrying the
// server's runtime settings the SPA needs. It is answered here, never from
// fsys, so one image serves whatever Nuvio backend and key its environment
// names.
const configPath = "/config.js"

// Handler serves fsys as a single-page app: any request that doesn't
// resolve to a real file is rewritten to "/" so the SPA's own router
// handles it, and configPath is answered with configScript. Every response
// carries the Content-Security-Policy built by contentSecurityPolicy, with
// nuvioBaseURL's origin as the one cross-origin endpoint the SPA may call.
func Handler(fsys fs.FS, nuvioBaseURL, nuvioPublishableKey string) (http.Handler, error) {
	u, err := url.Parse(nuvioBaseURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("static: Nuvio base URL %q is not an absolute URL", nuvioBaseURL)
	}
	csp := contentSecurityPolicy(u.Scheme + "://" + u.Host)
	script := configScript(nuvioBaseURL, nuvioPublishableKey)

	mux := http.NewServeMux()
	mux.HandleFunc("GET "+configPath, func(w http.ResponseWriter, _ *http.Request) {
		h := w.Header()
		h.Set("Content-Type", "text/javascript; charset=utf-8")
		h.Set("Cache-Control", "no-cache")
		h.Set("Content-Security-Policy", csp)
		h.Set("X-Content-Type-Options", "nosniff")
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
		h.Set("X-Content-Type-Options", "nosniff")
		fileServer.ServeHTTP(w, r)
	})
	return mux, nil
}

// configScript is the body of configPath: the Nuvio settings the SPA's
// auth client reads from window.__UNO_CONFIG__. The publishable key is
// public by design (https://nuvio.tv/docs#publishable-key).
func configScript(nuvioBaseURL, nuvioPublishableKey string) []byte {
	// A struct of two strings always marshals.
	body, _ := json.Marshal(struct {
		NuvioBaseURL        string `json:"nuvioBaseURL"`
		NuvioPublishableKey string `json:"nuvioPublishableKey"`
	}{nuvioBaseURL, nuvioPublishableKey})
	return []byte("window.__UNO_CONFIG__ = " + string(body) + ";\n")
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
// Embedded files carry a zero ModTime, so net/http emits no Last-Modified
// and no ETag; "no-cache" therefore costs a full re-fetch of the shell
// rather than a 304. At a few hundred bytes that's a fair trade for not
// having to hash the file at startup.
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
