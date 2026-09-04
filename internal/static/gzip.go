package static

import (
	"log"
	"net/http"

	"github.com/klauspost/compress/gzhttp"
)

// compressible is the allow-list of media types worth gzipping, kept
// explicit because gzhttp's default filter excludes archives, audio and
// video but still compresses fonts.
//
// That distinction is the whole point here: the build is font-heavy, and
// woff2/woff are already compressed. Measured over a real `vite build`,
// gzip returns -0.1% on woff2 and 0.2% on woff, which together are ~38%
// of the bytes served — while JS gives 69% and CSS 77%.
//
// Types are matched exactly, so entries carry no charset directive; that
// form matches with or without one. Go has no MIME entry for .woff2, so
// fonts are typed by content sniffing ("wOF2") rather than extension.
var compressible = []string{
	"text/html",
	"text/css",
	"text/javascript",
	"application/javascript",
	"application/json",
	"application/wasm",
	"image/svg+xml",
	"text/plain",
	"text/xml",
	"application/xml",
}

// Gzip compresses responses for clients that advertise support.
//
// gzhttp rather than a hand-rolled middleware because the decision to
// compress depends on the Content-Type, which the file server only sets
// once it starts writing — so it has to be deferred to WriteHeader
// rather than made up front.
//
// Range requests are passed through uncompressed: the file server's
// byte-range math is over the uncompressed file, which gzip would break.
func Gzip(next http.Handler) http.Handler {
	// MinSize is gzhttp's default; below it the gzip envelope costs more
	// than it saves.
	wrapper, err := gzhttp.NewWrapper(
		gzhttp.ContentTypes(compressible),
		gzhttp.MinSize(gzhttp.DefaultMinSize),
	)
	if err != nil {
		log.Fatalf("failed to build gzip wrapper: %v", err)
	}
	gzipped := wrapper(next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Range") != "" {
			// gzhttp is bypassed here, so it cannot add Vary itself. A
			// shared cache that keyed this response without it could hand
			// a gzipped body to a client that asked for none — and
			// /assets/ is cached for a year.
			w.Header().Add("Vary", "Accept-Encoding")
			next.ServeHTTP(w, r)
			return
		}
		gzipped.ServeHTTP(w, r)
	})
}
