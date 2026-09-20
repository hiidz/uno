package api

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/hiidz/uno/internal/httpx"
	"github.com/hiidz/uno/internal/provider"
	"github.com/hiidz/uno/internal/vault"
)

// lookupList answers with fetch's result, classifying a failure the way every
// TMDB lookup route does: an unusable catalog type is the caller's fault
// (400, and TMDB was never contacted), anything else is upstream's (502).
// fetch is a closure so each route can pass its own path and query params.
func lookupList[T any](w http.ResponseWriter, failMsg string, fetch func() (T, error)) {
	result, err := fetch()
	if err != nil {
		if errors.Is(err, provider.ErrInvalidCatalogType) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		http.Error(w, failMsg, http.StatusBadGateway)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, result)
}

func (s *Server) listGenres(w http.ResponseWriter, r *http.Request) {
	lookupList(w, "failed to fetch genres", func() ([]provider.Genre, error) {
		return s.provider.Genres(r.Context(), r.PathValue("type"))
	})
}

func (s *Server) listLanguages(w http.ResponseWriter, r *http.Request) {
	lookupList(w, "failed to fetch languages", func() ([]provider.Language, error) {
		return s.provider.Languages(r.Context())
	})
}

func (s *Server) listCountries(w http.ResponseWriter, r *http.Request) {
	lookupList(w, "failed to fetch countries", func() ([]provider.Country, error) {
		return s.provider.Countries(r.Context())
	})
}

// listWatchProviders backs the builder's streaming-service picker. The
// region is a query param rather than a path segment because it is optional
// here: TMDB returns every service it knows about when watch_region is
// omitted, which is what the picker shows before a region is chosen.
func (s *Server) listWatchProviders(w http.ResponseWriter, r *http.Request) {
	lookupList(w, "failed to fetch watch providers", func() ([]provider.WatchProvider, error) {
		return s.provider.WatchProviders(r.Context(), r.PathValue("type"), r.URL.Query().Get("region"))
	})
}

func (s *Server) listWatchRegions(w http.ResponseWriter, r *http.Request) {
	lookupList(w, "failed to fetch watch regions", func() ([]provider.WatchRegion, error) {
		return s.provider.WatchRegions(r.Context())
	})
}

func (s *Server) listCertifications(w http.ResponseWriter, r *http.Request) {
	lookupList(w, "failed to fetch certifications", func() (map[string][]provider.Certification, error) {
		return s.provider.Certifications(r.Context(), r.PathValue("type"))
	})
}

// validateCatalogParams checks the TMDB-specific recipe rules for a
// catalog's params. provider is constrained to "tmdb" here as well as in
// vault.CatalogForm.Validate() — this is the path that actually parses
// params, so an unrecognized provider must be rejected here too rather than
// passed through, or its params never get validated at all. The catalog
// type's own params shape comes from provider.DecodeParams, the one place
// that mapping lives. Every error returned is wrapped in
// vault.ErrInvalidInput so callers can route it through the same errors.Is
// switch as a vault-layer failure.
func (s *Server) validateCatalogParams(catalogType, catalogProvider, params string) error {
	if catalogProvider != "tmdb" {
		return fmt.Errorf("%w: provider must be %q", vault.ErrInvalidInput, "tmdb")
	}

	p, err := provider.DecodeParams(catalogType, params)
	if err != nil {
		if errors.Is(err, provider.ErrInvalidCatalogType) {
			// CatalogForm.Validate() should already reject anything else,
			// but this is defensive in case that check changes independently.
			return fmt.Errorf("%w: cannot validate params: unknown catalog type %q", vault.ErrInvalidInput, catalogType)
		}
		return fmt.Errorf("%w: invalid params: %v", vault.ErrInvalidInput, err)
	}
	if err := p.Validate(); err != nil {
		return fmt.Errorf("%w: %v", vault.ErrInvalidInput, err)
	}
	return nil
}
