// Package addon implements the Stremio-protocol addon server:
// /u/{token}/manifest.json and /u/{token}/catalog/{type}/{rest...}. It is
// public and unauthenticated by design (token-in-path, not bearer auth) —
// see internal/api's addon route registration for the trust boundary this
// package sits behind. It depends on nothing but vault and provider, never
// Nuvio.
package addon

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/hiidz/uno/internal/provider"
	"github.com/hiidz/uno/internal/vault"
)

// ID stays constant across every profile — identity in the addon protocol
// comes from the URL path (/u/{token}/...), never from this id.
// See docs/architecture.md.
const (
	ID = "hiidz.uno.catalog"
	// Name is also the display name Nuvio's own UI shows for this addon —
	// api/push.go's pushAddons sets it when upserting Uno's manifest URL
	// into a profile's addon list.
	Name        = "Uno Catalog"
	version     = "1.0.0"
	description = "Personal catalog rows for this profile."

	// catalogPageSize is TMDB's own fixed discover page size — declared to
	// Stremio as a hint, and used again in the catalog route's skip->page
	// conversion.
	catalogPageSize = 20

	// catalogCacheMaxAge/catalogStaleRevalidate tell Stremio how long it may
	// serve a catalog response before refetching — the same values (3h / 1h)
	// as the real sample in docs/api/samples/catalog-response.json.
	// There's no server-side response cache, so these are the only thing
	// keeping Stremio from re-hitting TMDB on every reopen of the app.
	catalogCacheMaxAge     = 10800
	catalogStaleRevalidate = 3600
)

// ManifestPathPattern is this addon's manifest route pattern, in net/http's
// method-less form — the caller (internal/api's route table) prepends the
// HTTP method. Shared with ManifestPath below so the pattern and the
// concrete-URL builder can't drift apart.
const ManifestPathPattern = "/u/{token}/manifest.json"

// ManifestPath is the concrete manifest path for one profile's token.
func ManifestPath(token string) string {
	return "/u/" + token + "/manifest.json"
}

// Server holds the addon server's two dependencies: reading a profile's
// current catalog selection (vault) and fetching catalog pages from TMDB
// (provider).
type Server struct {
	vault    *vault.DB
	provider *provider.TMDBClient
}

// New builds a Server. Both arguments are required — the caller (api.New)
// already guarantees non-nil, but New is exported, so it checks again
// rather than relying on that guarantee holding for every future caller.
func New(v *vault.DB, p *provider.TMDBClient) *Server {
	if v == nil || p == nil {
		log.Fatal("addon.New: vault and provider must not be nil")
	}
	return &Server{vault: v, provider: p}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("failed to encode response: %v", err)
	}
}

// Public wraps a handler on the public, unauthenticated addon surface:
// recovers panics (nothing else on this path can, since it's deliberately
// outside requireNuvioAuth) and opens CORS — see the "Addon server —
// public, unauthenticated, CORS-open, cacheable" section of
// docs/architecture.md.
func (s *Server) Public(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("addon: panic: %v", rec)
				http.Error(w, "internal error", http.StatusInternalServerError)
			}
		}()
		w.Header().Set("Access-Control-Allow-Origin", "*")
		next(w, r)
	}
}

type manifestExtra struct {
	Name       string `json:"name"`
	IsRequired bool   `json:"isRequired,omitempty"`
}

type manifestCatalog struct {
	Type     string          `json:"type"`
	ID       string          `json:"id"`
	Name     string          `json:"name"`
	PageSize int             `json:"pageSize,omitempty"`
	Extra    []manifestExtra `json:"extra,omitempty"`
}

type manifest struct {
	ID          string            `json:"id"`
	Version     string            `json:"version"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Types       []string          `json:"types"`
	Resources   []string          `json:"resources"`
	IDPrefixes  []string          `json:"idPrefixes"`
	Catalogs    []manifestCatalog `json:"catalogs"`
}

// ManifestID is the manifest-facing id for one catalog: provider-prefixed so
// it's the same string that would later round-trip as Nuvio's collections
// catalogSources[].catalogId — see the "Push wire shape" section of
// docs/data-model.md and the sample in
// docs/api/samples/collections-basic.json.
func ManifestID(c vault.Catalog) string {
	return c.Provider + "-" + c.ID.String()
}

// buildManifest builds Stremio's manifest catalog list. Every catalog
// declares "skip" (pagination) as an extra; a catalog with
// ShowInHome == false also declares a required "genre" extra, which is the
// Stremio mechanism for keeping a catalog out of the home screen's automatic
// rows while leaving it reachable from Discover.
func buildManifest(selection []vault.SelectedCatalog) manifest {
	catalogs := make([]manifestCatalog, len(selection))
	for i, sc := range selection {
		extra := []manifestExtra{{Name: "skip"}}
		if !sc.ShowInHome {
			extra = append(extra, manifestExtra{Name: "genre", IsRequired: true})
		}
		catalogs[i] = manifestCatalog{
			Type:     sc.Type,
			ID:       ManifestID(sc.Catalog),
			Name:     sc.Name,
			PageSize: catalogPageSize,
			Extra:    extra,
		}
	}

	return manifest{
		ID:          ID,
		Version:     version,
		Name:        Name,
		Description: description,
		Types:       []string{"movie", "series"},
		Resources:   []string{"catalog"},
		IDPrefixes:  []string{"tt"},
		Catalogs:    catalogs,
	}
}

// ManifestHandler serves GET /u/{token}/manifest.json. An unknown token is a
// 404, not an empty manifest — a mistyped link should fail loudly rather
// than silently installing an addon with no catalogs.
func (s *Server) ManifestHandler(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")

	profileID, err := s.vault.ResolveProfileID(r.Context(), token)
	if err != nil {
		if errors.Is(err, vault.ErrProfileNotFound) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "failed to resolve profile", http.StatusInternalServerError)
		return
	}

	selection, err := s.vault.GetPublishedCatalogs(r.Context(), profileID)
	if err != nil {
		http.Error(w, "failed to load catalogs", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, buildManifest(selection))
}

type catalogResponse struct {
	Metas           []provider.Meta `json:"metas"`
	CacheMaxAge     int             `json:"cacheMaxAge"`
	StaleRevalidate int             `json:"staleRevalidate"`
}

// findSelectedCatalog looks up a catalog in one profile's selection by the
// same id/type pair the manifest handed out (ManifestID + Type).
//
// This is also the access check: a catalog id that exists but isn't in this
// profile's current selection is indistinguishable from one that doesn't
// exist at all. A leaked or guessed catalog UUID can't be used to pull data
// through a profile it was never shared with.
func findSelectedCatalog(selection []vault.SelectedCatalog, catalogType, manifestID string) (vault.Catalog, bool) {
	for _, sc := range selection {
		if sc.Type == catalogType && ManifestID(sc.Catalog) == manifestID {
			return sc.Catalog, true
		}
	}
	return vault.Catalog{}, false
}

// CatalogHandler serves GET /u/{token}/catalog/{type}/{rest...}, where rest
// is "{id}.json" or "{id}/{extraProps}.json" (e.g. Stremio's "skip=20" for
// pagination) — Stremio appends extra props as an additional path segment
// rather than a query string, so a wildcard tail is the only way to capture
// both forms with one route.
func (s *Server) CatalogHandler(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	catalogType := r.PathValue("type")

	profileID, err := s.vault.ResolveProfileID(r.Context(), token)
	if err != nil {
		if errors.Is(err, vault.ErrProfileNotFound) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "failed to resolve profile", http.StatusInternalServerError)
		return
	}

	rest := strings.TrimSuffix(r.PathValue("rest"), ".json")
	manifestID, extra, _ := strings.Cut(rest, "/")

	selection, err := s.vault.GetPublishedCatalogs(r.Context(), profileID)
	if err != nil {
		http.Error(w, "failed to load catalogs", http.StatusInternalServerError)
		return
	}

	catalog, ok := findSelectedCatalog(selection, catalogType, manifestID)
	if !ok {
		http.NotFound(w, r)
		return
	}

	page := 1
	if extra != "" {
		if vals, err := url.ParseQuery(extra); err == nil {
			if skip, err := strconv.Atoi(vals.Get("skip")); err == nil && skip > 0 {
				page = skip/catalogPageSize + 1
			}
		}
	}

	metas, err := s.provider.FetchCatalogPage(r.Context(), catalog.Type, catalog.Params, page)
	if err != nil {
		log.Printf("addon: catalog %s: %v", manifestID, err)
		http.Error(w, "upstream error", http.StatusBadGateway)
		return
	}
	if metas == nil {
		metas = []provider.Meta{} // {"metas":null} is not a valid empty catalog
	}

	writeJSON(w, http.StatusOK, catalogResponse{
		Metas:           metas,
		CacheMaxAge:     catalogCacheMaxAge,
		StaleRevalidate: catalogStaleRevalidate,
	})
}
