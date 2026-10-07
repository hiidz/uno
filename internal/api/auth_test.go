package api

import (
	"testing"

	"github.com/hiidz/uno/internal/provider"
	"github.com/hiidz/uno/internal/vault"
)

// newProfileTestServer builds a Server over db for driving requireProfile
// directly.
func newProfileTestServer(t *testing.T, db *vault.DB) *Server {
	t.Helper()
	return newTestServer(t, db, &fakeNuvio{})
}

// newTestServer builds a Server over db and nuvioClient that authenticates every
// bearer token as acceptAnyToken's account.
func newTestServer(t *testing.T, db *vault.DB, nuvioClient NuvioClient) *Server {
	t.Helper()
	s, err := New(Deps{
		Vault:        db,
		Provider:     provider.NewTMDBClient("key"),
		Verifier:     acceptAnyToken{},
		Nuvio:        nuvioClient,
		SiteBaseURL:  "http://example.com",
		NuvioBaseURL: "https://nuvio.example.com",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}
