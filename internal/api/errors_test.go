package api

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/hiidz/uno/internal/provider"
)

// The profile slot in the path is resolved before any profile route runs: a
// slot outside 1–6 is a 400, and a vault that fails is a 500 that says nothing
// of why.
func TestProfileSlotRouting(t *testing.T) {
	f := newRouteFixture(t)
	runSteps(t, f.s, []routeStep{
		{name: "not a number", method: http.MethodGet, path: "/api/p/x/library", wantStatus: http.StatusBadRequest},
		{name: "below range", method: http.MethodGet, path: "/api/p/0/library", wantStatus: http.StatusBadRequest},
		{name: "above range", method: http.MethodGet, path: "/api/p/7/library", wantStatus: http.StatusBadRequest},
		{name: "provisioned", method: http.MethodGet, path: "/api/p/1/library", wantStatus: http.StatusOK},
	})

	if err := f.db.Close(); err != nil {
		t.Fatal(err)
	}
	w := serve(t, f.s, http.MethodGet, "/api/p/1/library", "", false)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("with the vault down status = %d, want 500 (body %q)", w.Code, w.Body.String())
	}
	if strings.Contains(strings.ToLower(w.Body.String()), "database") {
		t.Fatalf("500 body leaked the cause: %q", w.Body.String())
	}
}

// A TMDB lookup answers what TMDB did: its 404 is a 404, an outage a 502, and
// a recipe check that couldn't reach TMDB a 502 that keeps the transport error
// to the log.
func TestTMDBFailuresReachTheClientAsStatuses(t *testing.T) {
	f := newRouteFixture(t)
	notFound := func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) }
	company := func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, `{"id":1,"name":"Marvel Studios"}`) }

	for _, tc := range []struct {
		name, method, path, body string
		tmdb                     http.HandlerFunc
		wantStatus               int
	}{
		{"company found", http.MethodGet, "/api/companies/1", "", company, http.StatusOK},
		{"company absent on TMDB", http.MethodGet, "/api/companies/1", "", notFound, http.StatusNotFound},
		{"company with TMDB down", http.MethodGet, "/api/companies/1", "", tmdbDown, http.StatusBadGateway},
		{"keyword absent on TMDB", http.MethodGet, "/api/keywords/1", "", notFound, http.StatusNotFound},
		{"collection with TMDB down", http.MethodGet, "/api/collections/1", "", tmdbDown, http.StatusBadGateway},
		{"create checked while TMDB is down", http.MethodPost, "/api/p/1/catalogs", `{"type":"movie","name":"A","provider":"tmdb","params":"{\"with_genres\":\"28\"}"}`, tmdbDown, http.StatusBadGateway},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fakeTMDB(t, tc.tmdb)
			f.s.provider = provider.NewTMDBClient("key")
			w := serve(t, f.s, tc.method, tc.path, tc.body, false)
			if w.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d (body %q)", w.Code, tc.wantStatus, w.Body.String())
			}
			if w.Code == http.StatusBadGateway && (strings.Contains(w.Body.String(), "503") || strings.Contains(w.Body.String(), "unavailable")) {
				t.Fatalf("502 body leaked TMDB's answer: %q", w.Body.String())
			}
		})
	}
}
