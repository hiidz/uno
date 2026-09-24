package api

import (
	"context"
	"errors"
	"fmt"
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

// TestLookupListClassification pins which provider failure becomes which
// status on the TMDB lookup routes.
func TestLookupListClassification(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{"success", nil, http.StatusOK},
		{"bad catalog type", fmt.Errorf("%w: got %q", provider.ErrInvalidCatalogType, "x"), http.StatusBadRequest},
		{"bad query param", fmt.Errorf("%w: search query is blank", provider.ErrInvalidParams), http.StatusBadRequest},
		{"absent on TMDB", fmt.Errorf("%w: /company/999", provider.ErrNotFound), http.StatusNotFound},
		{"TMDB unreachable", errors.New("provider: fetch /company/1: dial tcp: refused"), http.StatusBadGateway},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			lookupList(w, "failed", func() (provider.Company, error) {
				return provider.Company{ID: 1, Name: "Lucasfilm Ltd."}, tc.err
			})
			if w.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", w.Code, tc.wantStatus)
			}
			if tc.err == nil && strings.TrimSpace(w.Body.String()) != `{"id":1,"name":"Lucasfilm Ltd."}` {
				t.Fatalf("body = %s", w.Body.String())
			}
		})
	}
}

// TestEntityLookupRoutes drives the real route table for the cases that
// never reach TMDB: auth, a blank search query, a company search's missing or
// unknown catalog type, and an id that isn't a number. The body text tells the search route from the {id} route, which is
// how it shows the literal "search" segment wins.
func TestEntityLookupRoutes(t *testing.T) {
	s, err := New(Deps{
		Vault:       newTestVaultDB(t),
		Provider:    provider.NewTMDBClient("key"),
		Verifier:    acceptAnyToken{},
		Nuvio:       &fakeNuvio{},
		SiteBaseURL: "http://example.com",
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
		{name: "unauthenticated search", path: "/api/companies/search?q=marvel&type=movie", noAuth: true, wantStatus: http.StatusUnauthorized},
		{name: "unauthenticated id", path: "/api/keywords/1", noAuth: true, wantStatus: http.StatusUnauthorized},
		{name: "company search without q", path: "/api/companies/search?type=movie", wantStatus: http.StatusBadRequest, wantBody: "search query is blank"},
		{name: "company search without type", path: "/api/companies/search?q=a24", wantStatus: http.StatusBadRequest, wantBody: "invalid catalog type"},
		{name: "company search with TMDB's type word", path: "/api/companies/search?q=a24&type=tv", wantStatus: http.StatusBadRequest, wantBody: "invalid catalog type"},
		{name: "company search with neither param", path: "/api/companies/search", wantStatus: http.StatusBadRequest, wantBody: "invalid catalog type"},
		{name: "keyword search with blank q", path: "/api/keywords/search?q=%20%20", wantStatus: http.StatusBadRequest, wantBody: "search query is blank"},
		{name: "non-numeric company id", path: "/api/companies/marvel", wantStatus: http.StatusBadRequest, wantBody: "invalid id"},
		{name: "non-numeric keyword id", path: "/api/keywords/1a", wantStatus: http.StatusBadRequest, wantBody: "invalid id"},
		{name: "non-positive company id", path: "/api/companies/0", wantStatus: http.StatusBadRequest, wantBody: "is not a TMDB id"},
		{name: "unauthenticated collection search", path: "/api/collections/search?q=star", noAuth: true, wantStatus: http.StatusUnauthorized},
		{name: "unauthenticated collection id", path: "/api/collections/10", noAuth: true, wantStatus: http.StatusUnauthorized},
		{name: "collection search without q", path: "/api/collections/search", wantStatus: http.StatusBadRequest, wantBody: "search query is blank"},
		{name: "non-numeric collection id", path: "/api/collections/starwars", wantStatus: http.StatusBadRequest, wantBody: "invalid id"},
		{name: "non-positive collection id", path: "/api/collections/-1", wantStatus: http.StatusBadRequest, wantBody: "is not a TMDB id"},
		{name: "unauthenticated network search", path: "/api/networks/search?q=hbo", noAuth: true, wantStatus: http.StatusUnauthorized},
		{name: "unauthenticated network id", path: "/api/networks/49", noAuth: true, wantStatus: http.StatusUnauthorized},
		{name: "network search with blank q", path: "/api/networks/search?q=%20", wantStatus: http.StatusBadRequest, wantBody: "search query is blank"},
		{name: "non-numeric network id", path: "/api/networks/hbo", wantStatus: http.StatusBadRequest, wantBody: "invalid id"},
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
		name, catalogType, catalogProvider, params string
		ctx                                        context.Context
		wantErr                                    error
	}{
		{"clean recipe", "movie", "tmdb", `{"sort_by":"popularity.desc"}`, t.Context(), nil},
		{"other provider", "movie", "mdblist", `{}`, t.Context(), vault.ErrInvalidInput},
		{"unknown catalog type", "anime", "tmdb", `{}`, t.Context(), vault.ErrInvalidInput},
		{"undecodable params", "movie", "tmdb", `{`, t.Context(), vault.ErrInvalidInput},
		{"recipe rule broken", "movie", "tmdb", `{"sort_by":"bogus.desc"}`, t.Context(), vault.ErrInvalidInput},
		{"vocabulary rejected", "series", "tmdb", `{"with_collection":"10"}`, t.Context(), vault.ErrInvalidInput},
		{"TMDB unreachable", "movie", "tmdb", `{"with_genres":"28"}`, cancelled, errUpstreamValidation},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := s.validateCatalogParams(tc.ctx, tc.catalogType, tc.catalogProvider, tc.params)
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
