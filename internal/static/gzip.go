package static

import (
	"compress/gzip"
	"net/http"
	"strings"
)

// Gzip compresses responses for clients that advertise support. Chosen over
// pre-compressing web/dist at build time — see .ref/Open_Items.md — since
// it's one middleware instead of a second build step.
//
// Range requests are passed through uncompressed: the file server's
// byte-range math is over the uncompressed file, which gzip would break.
func Gzip(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Range") != "" || !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Add("Vary", "Accept-Encoding")
		gz := gzip.NewWriter(w)
		defer gz.Close()
		next.ServeHTTP(&gzipResponseWriter{ResponseWriter: w, gz: gz}, r)
	})
}

// gzipResponseWriter strips the file server's uncompressed Content-Length —
// left in place it would no longer match the gzipped body — the moment
// either WriteHeader or the first Write fires, whichever comes first.
type gzipResponseWriter struct {
	http.ResponseWriter
	gz          *gzip.Writer
	wroteHeader bool
}

func (w *gzipResponseWriter) WriteHeader(status int) {
	w.Header().Del("Content-Length")
	w.wroteHeader = true
	w.ResponseWriter.WriteHeader(status)
}

func (w *gzipResponseWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.gz.Write(b)
}
