package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/hiidz/uno/internal/provider"
	"github.com/hiidz/uno/internal/vault"
)

// newProfileTestServer builds a Server over db for driving requireProfile
// directly.
func newProfileTestServer(t *testing.T, db *vault.DB) *Server {
	t.Helper()
	s, err := New(Deps{
		Vault:        db,
		Provider:     provider.NewTMDBClient("key"),
		Verifier:     acceptAnyToken{},
		Nuvio:        &fakeNuvio{},
		SiteBaseURL:  "http://example.com",
		NuvioBaseURL: "https://nuvio.example.com",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

// serveRequireProfile runs one request through requireProfile for sub (none
// when empty) and the given {profileIndex}, returning the status and the
// profile ID the next handler saw.
func serveRequireProfile(s *Server, sub, index string) (int, uuid.UUID) {
	var seen uuid.UUID
	next := func(w http.ResponseWriter, r *http.Request) {
		seen, _ = profileIDFrom(r.Context())
		w.WriteHeader(http.StatusOK)
	}

	ctx := context.Background()
	if sub != "" {
		ctx = withNuvioUserID(ctx, sub)
	}
	r := httptest.NewRequestWithContext(ctx, http.MethodGet, "/api/p/"+index+"/catalogs", nil)
	r.SetPathValue("profileIndex", index)
	w := httptest.NewRecorder()
	s.requireProfile(next)(w, r)
	return w.Code, seen
}

// TestRequireProfile covers requireProfile's resolution of {profileIndex}:
// only an existing slot of the authenticated user reaches the next handler,
// carrying that slot's profile ID.
func TestRequireProfile(t *testing.T) {
	db := newTestVaultDB(t)
	s := newProfileTestServer(t, db)
	profile, err := db.ResolveOrCreateProfile(t.Context(), "user-a", 1, "nuvio-profile-1")
	if err != nil {
		t.Fatalf("ResolveOrCreateProfile: %v", err)
	}

	tests := []struct {
		name, sub, index string
		wantStatus       int
		wantProfile      uuid.UUID
	}{
		{"existing slot", "user-a", "1", http.StatusOK, profile.ID},
		{"no authenticated user", "", "1", http.StatusUnauthorized, uuid.Nil},
		{"non-numeric index", "user-a", "x", http.StatusBadRequest, uuid.Nil},
		{"index below range", "user-a", "0", http.StatusBadRequest, uuid.Nil},
		{"index above range", "user-a", "7", http.StatusBadRequest, uuid.Nil},
		{"unprovisioned slot", "user-a", "2", http.StatusNotFound, uuid.Nil},
		{"another user's slot", "user-b", "1", http.StatusNotFound, uuid.Nil},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			status, seen := serveRequireProfile(s, tc.sub, tc.index)
			if status != tc.wantStatus {
				t.Fatalf("status = %d, want %d", status, tc.wantStatus)
			}
			if seen != tc.wantProfile {
				t.Fatalf("next handler saw profile %v, want %v", seen, tc.wantProfile)
			}
		})
	}
}

// TestRequireProfileVaultFailure pins that a vault error other than a
// missing profile is a 500, not a 404.
func TestRequireProfileVaultFailure(t *testing.T) {
	db := newTestVaultDB(t)
	s := newProfileTestServer(t, db)
	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if status, _ := serveRequireProfile(s, "user-a", "1"); status != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", status, http.StatusInternalServerError)
	}
}
