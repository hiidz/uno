// Package static serves the embedded SPA build, falling back to
// index.html for any path that isn't a real file so React Router can
// resolve client-side routes (e.g. a hard reload on /configure).
package static

import (
	"io/fs"
	"net/http"
	"strings"
)

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
		fileServer.ServeHTTP(w, r)
	})
}

func cloneWithPath(r *http.Request, path string) *http.Request {
	r2 := r.Clone(r.Context())
	r2.URL.Path = path
	return r2
}
