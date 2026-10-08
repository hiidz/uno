package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hiidz/uno/internal/nuvio"
	"github.com/hiidz/uno/internal/provider"
	"github.com/hiidz/uno/internal/vault"
)

// acceptAnyToken is a TokenVerifier that authenticates every bearer token.
type acceptAnyToken struct{}

func (acceptAnyToken) Verify(context.Context, string) (nuvio.Claims, error) {
	return nuvio.Claims{Sub: "test-sub"}, nil
}

// TestEntityLookupRoutes drives the real route table for the cases that
// never reach TMDB, one per check the four lookups share: auth, a blank search
// query, an id that isn't a positive number, and a company search's catalog
// type. The blank query's words tell the search route from the {id} route,
// which is how it shows the literal "search" segment wins.
func TestEntityLookupRoutes(t *testing.T) {
	s, err := New(Deps{
		Vault:        newTestVaultDB(t),
		Provider:     provider.NewTMDBClient("key"),
		Verifier:     acceptAnyToken{},
		Nuvio:        &fakeNuvio{},
		SiteBaseURL:  "http://example.com",
		NuvioBaseURL: "https://nuvio.example.com",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	tests := []struct {
		name, path string
		noAuth     bool
		wantStatus int
		wantBody   string
	}{
		{name: "unauthenticated id", path: "/api/keywords/1", noAuth: true, wantStatus: http.StatusUnauthorized},
		{name: "company search with TMDB's type word", path: "/api/companies/search?q=a24&type=tv", wantStatus: http.StatusBadRequest, wantBody: "invalid catalog type"},
		{name: "keyword search with blank q", path: "/api/keywords/search?q=%20%20", wantStatus: http.StatusBadRequest, wantBody: "search query is blank"},
		{name: "non-numeric company id", path: "/api/companies/marvel", wantStatus: http.StatusBadRequest, wantBody: "invalid id"},
		{name: "non-positive network id", path: "/api/networks/0", wantStatus: http.StatusBadRequest, wantBody: "is not a TMDB id"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			if !tc.noAuth {
				req.Header.Set("Authorization", "Bearer token")
			}
			w := httptest.NewRecorder()
			s.ServeHTTP(w, req)

			if w.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d (body %q)", w.Code, tc.wantStatus, w.Body.String())
			}
			if !strings.Contains(w.Body.String(), tc.wantBody) {
				t.Fatalf("body = %q, want it to contain %q", w.Body.String(), tc.wantBody)
			}
		})
	}
}

// TestValidateCatalogParams covers how validateCatalogParams classifies a
// recipe: anything wrong with the recipe itself wraps vault.ErrInvalidInput
// (a 400), and TMDB failing to serve a list wraps errUpstreamValidation (a
// 502). The upstream case uses a cancelled context, so the TMDB request
// fails before it is sent.
func TestValidateCatalogParams(t *testing.T) {
	s := newProfileTestServer(t, newTestVaultDB(t))
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()

	tests := []struct {
		name, catalogType, params string
		ctx                       context.Context
		wantErr                   error
	}{
		{"clean recipe", "movie", `{"sort_by":"popularity.desc"}`, t.Context(), nil},
		{"unknown catalog type", "anime", `{}`, t.Context(), vault.ErrInvalidInput},
		{"recipe rule broken", "movie", `{"sort_by":"bogus.desc"}`, t.Context(), vault.ErrInvalidInput},
		{"TMDB unreachable", "movie", `{"with_genres":"28"}`, cancelled, errUpstreamValidation},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := s.validateCatalogParams(tc.ctx, tc.catalogType, tc.params)
			if tc.wantErr == nil {
				if err != nil {
					t.Fatalf("err = %v, want nil", err)
				}
				return
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want it to wrap %v", err, tc.wantErr)
			}
			if errors.Is(tc.wantErr, errUpstreamValidation) && errors.Is(err, vault.ErrInvalidInput) {
				t.Fatalf("err = %v, an unreachable TMDB must not read as a rejected recipe", err)
			}
		})
	}
}
