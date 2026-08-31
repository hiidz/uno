package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/hiidz/uno/internal/provider"
	"github.com/hiidz/uno/internal/vault"
)

func (s *Server) listGenres(w http.ResponseWriter, r *http.Request) {
	catalogType := r.PathValue("type")

	genres, err := s.provider.Genres(r.Context(), catalogType)
	if err != nil {
		if errors.Is(err, provider.ErrInvalidCatalogType) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		http.Error(w, "failed to fetch genres", http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, genres)
}

func (s *Server) listLanguages(w http.ResponseWriter, r *http.Request) {
	languages, err := s.provider.Languages(r.Context())
	if err != nil {
		http.Error(w, "failed to fetch languages", http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, languages)
}

func (s *Server) listCountries(w http.ResponseWriter, r *http.Request) {
	countries, err := s.provider.Countries(r.Context())
	if err != nil {
		http.Error(w, "failed to fetch countries", http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, countries)
}

func (s *Server) listCertifications(w http.ResponseWriter, r *http.Request) {
	catalogType := r.PathValue("type")

	certifications, err := s.provider.Certifications(r.Context(), catalogType)
	if err != nil {
		if errors.Is(err, provider.ErrInvalidCatalogType) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		http.Error(w, "failed to fetch certifications", http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, certifications)
}

// validateCatalogParams checks the TMDB-specific recipe rules for a
// catalog's params, dispatching on catalog type. provider is constrained to
// "tmdb" here as well as in vault.CatalogForm.Validate() — this is the path
// that actually parses params, so an unrecognized provider must be rejected
// here too rather than passed through, or its params never get validated at
// all. Every error returned is wrapped in vault.ErrInvalidInput so callers
// can route it through the same errors.Is switch as a vault-layer failure.
func (s *Server) validateCatalogParams(catalogType, catalogProvider, params string) error {
	if catalogProvider != "tmdb" {
		return fmt.Errorf("%w: provider must be %q", vault.ErrInvalidInput, "tmdb")
	}

	switch catalogType {
	case "movie":
		var p provider.TMDBMovieParams
		if err := json.Unmarshal([]byte(params), &p); err != nil {
			return fmt.Errorf("%w: invalid params: %v", vault.ErrInvalidInput, err)
		}
		if err := p.Validate(); err != nil {
			return fmt.Errorf("%w: %v", vault.ErrInvalidInput, err)
		}
		return nil
	case "series":
		var p provider.TMDBTVParams
		if err := json.Unmarshal([]byte(params), &p); err != nil {
			return fmt.Errorf("%w: invalid params: %v", vault.ErrInvalidInput, err)
		}
		if err := p.Validate(); err != nil {
			return fmt.Errorf("%w: %v", vault.ErrInvalidInput, err)
		}
		return nil
	default:
		// CatalogForm.Validate() should already reject anything else,
		// but this is defensive in case that check changes independently.
		return fmt.Errorf("%w: cannot validate params: unknown catalog type %q", vault.ErrInvalidInput, catalogType)
	}
}
