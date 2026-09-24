package api

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"

	"github.com/hiidz/uno/internal/httpx"
	"github.com/hiidz/uno/internal/provider"
	"github.com/hiidz/uno/internal/vault"
)

// lookupList answers with fetch's result, classifying a failure the way every
// TMDB lookup route does: an unusable catalog type or an unusable query param
// is the caller's fault (400, and TMDB was never contacted), a resource TMDB
// doesn't have is a 404, anything else is upstream's (502). fetch is a
// closure so each route can pass its own path and query params.
func lookupList[T any](w http.ResponseWriter, failMsg string, fetch func() (T, error)) {
	result, err := fetch()
	if err != nil {
		if errors.Is(err, provider.ErrInvalidCatalogType) || errors.Is(err, provider.ErrInvalidParams) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if errors.Is(err, provider.ErrNotFound) {
			http.Error(w, "not found on TMDB", http.StatusNotFound)
			return
		}
		log.Printf("lookupList: %s: %v", failMsg, err)
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

// searchCompanies takes the catalog type as a query param because each
// result's title count is for that type.
func (s *Server) searchCompanies(w http.ResponseWriter, r *http.Request) {
	lookupList(w, "failed to search companies", func() ([]provider.CompanyMatch, error) {
		return s.provider.SearchCompanies(r.Context(), r.URL.Query().Get("type"), r.URL.Query().Get("q"))
	})
}

func (s *Server) searchKeywords(w http.ResponseWriter, r *http.Request) {
	lookupList(w, "failed to search keywords", func() ([]provider.Keyword, error) {
		return s.provider.SearchKeywords(r.Context(), r.URL.Query().Get("q"))
	})
}

func (s *Server) searchCollections(w http.ResponseWriter, r *http.Request) {
	lookupList(w, "failed to search collections", func() ([]provider.Collection, error) {
		return s.provider.SearchCollections(r.Context(), r.URL.Query().Get("q"))
	})
}

// searchNetworks takes no catalog type: networks filter series only.
func (s *Server) searchNetworks(w http.ResponseWriter, r *http.Request) {
	lookupList(w, "failed to search networks", func() ([]provider.NetworkMatch, error) {
		return s.provider.SearchNetworks(r.Context(), r.URL.Query().Get("q"))
	})
}

// getCompany, getKeyword, getCollection and getNetwork resolve one saved id
// back to its name for the picker. A TMDB 404 answers 404, so the picker can tell a stale id from an
// outage.
func (s *Server) getCompany(w http.ResponseWriter, r *http.Request) {
	id, ok := pathTMDBID(w, r)
	if !ok {
		return
	}
	lookupList(w, "failed to fetch company", func() (provider.Company, error) {
		return s.provider.Company(r.Context(), id)
	})
}

func (s *Server) getKeyword(w http.ResponseWriter, r *http.Request) {
	id, ok := pathTMDBID(w, r)
	if !ok {
		return
	}
	lookupList(w, "failed to fetch keyword", func() (provider.Keyword, error) {
		return s.provider.Keyword(r.Context(), id)
	})
}

func (s *Server) getCollection(w http.ResponseWriter, r *http.Request) {
	id, ok := pathTMDBID(w, r)
	if !ok {
		return
	}
	lookupList(w, "failed to fetch collection", func() (provider.Collection, error) {
		return s.provider.Collection(r.Context(), id)
	})
}

func (s *Server) getNetwork(w http.ResponseWriter, r *http.Request) {
	id, ok := pathTMDBID(w, r)
	if !ok {
		return
	}
	lookupList(w, "failed to fetch network", func() (provider.Network, error) {
		return s.provider.Network(r.Context(), id)
	})
}

// pathTMDBID parses the {id} path segment, answering 400 itself when it
// isn't an integer.
func pathTMDBID(w http.ResponseWriter, r *http.Request) (int, bool) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return 0, false
	}
	return id, true
}

// errUpstreamValidation marks a validateCatalogParams failure that is
// TMDB's fault rather than the recipe's: the lists the recipe is checked
// against couldn't be fetched, so the recipe is unjudged, not rejected.
// It is what lets writeVaultError answer 502 instead of inventing a 400 for
// a recipe nobody has actually found fault with. Its counterpart is
// vault.ErrInvalidInput, which every rejected-recipe path wraps.
var errUpstreamValidation = errors.New("cannot validate params against TMDB")

// validateCatalogParams checks the TMDB-specific recipe rules for a
// catalog's params. provider is constrained to "tmdb" here as well as in
// vault.CatalogForm.Validate() — this is the path that actually parses
// params, so an unrecognized provider must be rejected here too rather than
// passed through, or its params never get validated at all. The catalog
// type's own params shape comes from provider.DecodeParams, the one place
// that mapping lives. Every error returned is wrapped in
// vault.ErrInvalidInput so callers can route it through the same errors.Is
// switch as a vault-layer failure.
func (s *Server) validateCatalogParams(ctx context.Context, catalogType, catalogProvider, params string) error {
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
		return fmt.Errorf("%w: invalid params: %w", vault.ErrInvalidInput, err)
	}
	if err := p.Validate(); err != nil {
		return fmt.Errorf("%w: %w", vault.ErrInvalidInput, err)
	}

	// The rest of the recipe's vocabulary — genre ids, language code, watch
	// region and providers, certification country and scale — can only be
	// checked against TMDB's own published lists, so it lives behind the
	// client rather than in p.Validate(). A rejected value is the caller's
	// fault and becomes a 400; TMDB being unreachable is not, and carries
	// errUpstreamValidation so it becomes a 502.
	if err := s.provider.ValidateParams(ctx, catalogType, params); err != nil {
		if errors.Is(err, provider.ErrInvalidParams) {
			return fmt.Errorf("%w: %w", vault.ErrInvalidInput, err)
		}
		return fmt.Errorf("%w: %w", errUpstreamValidation, err)
	}
	return nil
}

// checkRecipe is what every client-supplied recipe goes through before the
// vault writes it: validateCatalogParams, then the fingerprint the row
// stores. Errors wrap vault.ErrInvalidInput or errUpstreamValidation, the
// same as validateCatalogParams, so writeVaultError classifies them.
func (s *Server) checkRecipe(ctx context.Context, catalogType, catalogProvider, params string) (string, error) {
	if err := s.validateCatalogParams(ctx, catalogType, catalogProvider, params); err != nil {
		return "", err
	}
	fingerprint, err := provider.Fingerprint(catalogType, catalogProvider, params)
	if err != nil {
		return "", fmt.Errorf("%w: %w", vault.ErrInvalidInput, err)
	}
	return fingerprint, nil
}
