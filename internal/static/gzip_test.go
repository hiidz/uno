package static

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestGzip covers which responses Gzip compresses: an allow-listed type at
// or above the minimum size, for a client that accepts gzip. An
// already-compressed font, a body too small to gain, a client that didn't
// ask, and a range request all pass through as written, and the range
// request still carries Vary so no shared cache serves it gzipped.
func TestGzip(t *testing.T) {
	large := strings.Repeat("console.log('uno');\n", 200)

	tests := []struct {
		name, contentType, body string
		acceptGzip, rangeReq    bool
		wantGzip                bool
	}{
		{name: "script", contentType: "text/javascript; charset=utf-8", body: large, acceptGzip: true, wantGzip: true},
		{name: "font", contentType: "font/woff2", body: large, acceptGzip: true},
		{name: "small body", contentType: "text/javascript", body: "console.log(1)", acceptGzip: true},
		{name: "client without gzip", contentType: "text/javascript", body: large},
		{name: "range request", contentType: "text/javascript", body: large, acceptGzip: true, rangeReq: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h, err := Gzip(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", tc.contentType)
				io.WriteString(w, tc.body)
			}))
			if err != nil {
				t.Fatalf("Gzip: %v", err)
			}
			req := httptest.NewRequest(http.MethodGet, "/assets/app.js", nil)
			if tc.acceptGzip {
				req.Header.Set("Accept-Encoding", "gzip")
			}
			if tc.rangeReq {
				req.Header.Set("Range", "bytes=0-99")
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)

			gzipped := w.Header().Get("Content-Encoding") == "gzip"
			if gzipped != tc.wantGzip {
				t.Fatalf("gzipped = %v, want %v", gzipped, tc.wantGzip)
			}
			body := w.Body.String()
			if gzipped {
				zr, err := gzip.NewReader(w.Body)
				if err != nil {
					t.Fatalf("gzip.NewReader: %v", err)
				}
				raw, err := io.ReadAll(zr)
				if err != nil {
					t.Fatalf("decompressing: %v", err)
				}
				body = string(raw)
			}
			if body != tc.body {
				t.Fatalf("body = %d bytes, want the %d written", len(body), len(tc.body))
			}
			if (tc.wantGzip || tc.rangeReq) && !strings.Contains(strings.Join(w.Header().Values("Vary"), ","), "Accept-Encoding") {
				t.Fatalf("Vary = %q, want it to include Accept-Encoding", w.Header().Values("Vary"))
			}
		})
	}
}
