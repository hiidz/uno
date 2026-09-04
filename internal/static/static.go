// Package static serves the embedded SPA build, falling back to
// index.html for any path that isn't a real file so React Router can
// resolve client-side routes (e.g. a hard reload on /configure).
package static

import (
	"io/fs"
	"net/http"
	"strings"
)

// assetsPrefix is Vite's output directory. Everything under it is named
// with a content hash, so a changed file arrives under a new name and the
// old one can be cached indefinitely. Files outside it — index.html,
// favicon.svg, icons.svg — keep their names across builds and must not be.
const assetsPrefix = "/assets/"

// Handler serves fsys as a single-page app: any request that doesn't
// resolve to a real file is rewritten to "/" so the SPA's own router
// handles it.
func Handler(fsys fs.FS) http.Handler {
	fileServer := http.FileServerFS(fsys)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "."
		}
		if info, err := fs.Stat(fsys, path); err != nil || info.IsDir() {
			r = cloneWithPath(r, "/")
		}
		w.Header().Set("Cache-Control", cacheControl(r.URL.Path))
		fileServer.ServeHTTP(w, r)
	})
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
