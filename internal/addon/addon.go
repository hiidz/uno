// Package addon implements the Stremio-protocol addon server:
// /u/{token}/manifest.json and /u/{token}/catalog/{type}/{rest...}. It is
// public and unauthenticated by design (token-in-path, not bearer auth) —
// see internal/api's addon route registration for the trust boundary this
// package sits behind. It depends on nothing but vault and provider, never
// Nuvio.
package addon

import (
	"context"
	"errors"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/hiidz/uno/internal/httpx"
	"github.com/hiidz/uno/internal/provider"
	"github.com/hiidz/uno/internal/tmdbkey"
	"github.com/hiidz/uno/internal/vault"
)

// ID stays constant across every profile — identity in the addon protocol
// comes from the URL path (/u/{token}/...), never from this id. It is
// vault.AddonID, which every pushed source carries too.
// See docs/architecture.md.
const (
	ID = vault.AddonID
	// Name is also the display name Nuvio's own UI shows for this addon —
	// api/push.go's mergeAddon names Uno's entry with it when adding it to a
	// profile's addon list.
	Name        = "Uno Catalog"
	version     = "1.0.0"
	description = "Personal catalog rows for this profile."

	// catalogPageSize is TMDB's own fixed discover page size — declared to
	// Stremio as a hint, and the most titles the catalog route serves at once
	// (catalogWindow).
	catalogPageSize = 20

	// maxCatalogPage is TMDB's own ceiling on discover pagination: it rejects
	// any page above this one. A skip that lands past it is answered with an
	// empty page instead of a request TMDB refuses.
	maxCatalogPage = 500

	// catalogCacheMaxAge/catalogStaleRevalidate are the catalog response's
	// cacheMaxAge/staleRevalidate, the same values (3h / 1h) as the real
	// sample in docs/api/samples/catalog-response.json. They are hints the
	// Stremio addon SDK turns into a Cache-Control header; no Nuvio app reads
	// them from the body, and Uno sends no such header, so Nuvio's apps fetch
	// every catalog page afresh. The provider's page cache is what saves TMDB
	// the repeat.
	catalogCacheMaxAge     = 10800
	catalogStaleRevalidate = 3600
)

// ManifestPathPattern is this addon's manifest route pattern, in net/http's
// method-less form — the caller (internal/api's route table) prepends the
// HTTP method. Shared with ManifestPath below so the pattern and the
// concrete-URL builder can't drift apart.
const ManifestPathPattern = "/u/{token}/manifest.json"

// ConfigurePathPattern is the addon's Configure route pattern, in the same
// method-less form: the manifest's path with /manifest.json swapped for
// /configure, which is where Nuvio's addon managers send Configure.
const ConfigurePathPattern = "/u/{token}/configure"

// ManifestPath is the concrete manifest path for one profile's token.
func ManifestPath(token string) string {
	return "/u/" + token + "/manifest.json"
}

// Server holds the addon server's dependencies: reading a profile's
// current catalog selection (vault), fetching catalog pages from TMDB
// (provider), and, on a server where each account brings its own TMDB key,
// the owner's key for each request (keys, nil on a server with one shared
// key).
type Server struct {
	vault    *vault.DB
	provider *provider.TMDBClient
	keys     *tmdbkey.Keys
	// logoURL is the manifest's logo: LogoPath on this site.
	logoURL string
}

// LogoPath is the addon's logo on Uno's site: the SPA build's logo.png
// (web/public), which Nuvio's addon managers show beside the addon's name.
const LogoPath = "/logo.png"

// New builds a Server. v and p are required — the caller (api.New) already
// guarantees non-nil, but New is exported, so it checks again rather than
// relying on that guarantee holding for every future caller. keys is nil on
// a server with one shared TMDB key. siteBaseURL is the site's public base
// URL, which the manifest's logo is named under.
func New(v *vault.DB, p *provider.TMDBClient, keys *tmdbkey.Keys, siteBaseURL string) (*Server, error) {
	if v == nil || p == nil {
		return nil, errors.New("addon: vault and provider must not be nil")
	}
	return &Server{vault: v, provider: p, keys: keys, logoURL: siteBaseURL + LogoPath}, nil
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
	Name         string   `json:"name"`
	IsRequired   bool     `json:"isRequired,omitempty"`
	Options      []string `json:"options,omitempty"`
	OptionsLimit int      `json:"optionsLimit,omitempty"`
}

type manifestCatalog struct {
	Type     string          `json:"type"`
	ID       string          `json:"id"`
	Name     string          `json:"name"`
	PageSize int             `json:"pageSize,omitempty"`
	Extra    []manifestExtra `json:"extra,omitempty"`
	// ShowInHome is always explicit: Nuvio TV decides home-row eligibility
	// from this field alone and reads an absent one as "show".
	ShowInHome bool `json:"showInHome"`
}

type manifest struct {
	ID          string            `json:"id"`
	Version     string            `json:"version"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Logo        string            `json:"logo,omitempty"`
	Types       []string          `json:"types"`
	Resources   []string          `json:"resources"`
	IDPrefixes  []string          `json:"idPrefixes"`
	Catalogs    []manifestCatalog `json:"catalogs"`
	// BehaviorHints.Configurable gives the addon a Configure action in Nuvio's
	// addon managers, which open the manifest URL with /manifest.json swapped
	// for /configure (ConfigureHandler).
	BehaviorHints manifestHints `json:"behaviorHints"`
}

type manifestHints struct {
	Configurable bool `json:"configurable"`
}

// ManifestID is the manifest-facing id for one catalog, vault.ManifestID:
// the same string a pushed collection names it by in catalogSources[].catalogId.
func ManifestID(c vault.Catalog) string {
	return vault.ManifestID(c)
}

// buildManifest builds Stremio's manifest catalog list. Every catalog
// declares "skip" (pagination) as an extra and an explicit showInHome, plus
// a "genre" extra over genreNames(catalog) — see genreExtra.
func buildManifest(selection []vault.SelectedCatalog, genreNames func(vault.SelectedCatalog) []string) manifest {
	catalogs := make([]manifestCatalog, len(selection))
	for i, sc := range selection {
		extra := []manifestExtra{{Name: "skip"}}
		if genre, ok := genreExtra(sc.ShowInHome, genreNames(sc)); ok {
			extra = append(extra, genre)
		}
		catalogs[i] = manifestCatalog{
			Type:       sc.Type,
			ID:         ManifestID(sc.Catalog),
			Name:       sc.Name,
			PageSize:   catalogPageSize,
			Extra:      extra,
			ShowInHome: sc.ShowInHome,
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
		BehaviorHints: manifestHints{
			Configurable: true,
		},
	}
}

// genreExtra builds one catalog's genre extra from the genre names it can be
// narrowed by (provider.GenreExtraOptions) and its published ShowInHome. The
// two share the one extra: genre is the only required extra Nuvio
// mobile/desktop still list in Discover and collection pickers.
//
// A catalog with showInHome false gets it required — Nuvio mobile/desktop and
// Stremio keep a catalog with a required extra out of home's automatic rows —
// with provider.GenreExtraAll as its first option. Those clients default a
// required genre to its first option, so the default view stays unfiltered,
// and drop a required genre that has no options at all. A catalog on home
// gets the names as an optional extra, or no genre extra when there are none.
func genreExtra(showInHome bool, names []string) (manifestExtra, bool) {
	if showInHome {
		if len(names) == 0 {
			return manifestExtra{}, false
		}
		return manifestExtra{Name: "genre", Options: names, OptionsLimit: 1}, true
	}
	return manifestExtra{
		Name:         "genre",
		IsRequired:   true,
		Options:      append([]string{provider.GenreExtraAll}, names...),
		OptionsLimit: 1,
	}, true
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
		log.Printf("addon: manifest: resolving profile: %v", err)
		http.Error(w, "failed to resolve profile", http.StatusInternalServerError)
		return
	}

	selection, err := s.vault.GetPublishedCatalogs(r.Context(), profileID)
	if err != nil {
		log.Printf("addon: manifest: loading catalogs: %v", err)
		http.Error(w, "failed to load catalogs", http.StatusInternalServerError)
		return
	}

	// A cold genre list is fetched with the profile owner's own TMDB key, on
	// a server where each account brings one.
	ctx := provider.WithKeySource(r.Context(), s.keys.ForToken(r.Context(), token))
	m := buildManifest(selection, func(sc vault.SelectedCatalog) []string {
		return s.genreNames(ctx, sc)
	})
	m.Logo = s.logoURL
	httpx.WriteJSON(w, http.StatusOK, m)
}

// ConfigureHandler serves ConfigurePathPattern, which Nuvio's addon managers
// open for the addon's Configure action: it sends the browser to the
// builder's profile picker, which asks for sign-in first. The token is not
// looked up, since the redirect says nothing about its profile.
func (s *Server) ConfigureHandler(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/profiles", http.StatusFound)
}

// genreNames is a catalog's genre-extra option names. A TMDB failure (the
// genre list is cached after its first fetch, so only a cold one can fail)
// degrades to no names rather than failing the whole manifest.
func (s *Server) genreNames(ctx context.Context, sc vault.SelectedCatalog) []string {
	genres, err := s.provider.GenreExtraOptions(ctx, sc.Type, sc.Params)
	if err != nil {
		logTMDBFailure("genre options for catalog "+ManifestID(sc.Catalog), err)
		return nil
	}
	names := make([]string, len(genres))
	for i, g := range genres {
		names[i] = g.Name
	}
	return names
}

type catalogResponse struct {
	Metas           []provider.Meta `json:"metas"`
	CacheMaxAge     int             `json:"cacheMaxAge"`
	StaleRevalidate int             `json:"staleRevalidate"`
}

// parseManifestID splits a manifest id back into the provider and catalog id
// ManifestID joined, reporting false for anything ManifestID can't have
// written.
func parseManifestID(manifestID string) (string, uuid.UUID, bool) {
	catalogProvider, rawID, ok := strings.Cut(manifestID, "-")
	id, err := uuid.Parse(rawID)
	return catalogProvider, id, ok && err == nil && id.String() == rawID
}

// served returns the catalog a catalog route names, with its owner's
// account and sealed TMDB key, by vault.ServedCatalog: one of the token's
// profile's own catalogs that is on the TV. That lookup is also the access
// check, so a leaked or guessed catalog UUID can't pull data through a
// profile it doesn't belong to, nor a catalog the profile hasn't published.
// Anything it doesn't find is vault.ErrCatalogNotFound.
func (s *Server) served(ctx context.Context, token, catalogType, manifestID string) (vault.ServedCatalog, error) {
	catalogProvider, catalogID, ok := parseManifestID(manifestID)
	if !ok {
		return vault.ServedCatalog{}, vault.ErrCatalogNotFound
	}
	return s.vault.ServedCatalog(ctx, token, catalogID, catalogType, catalogProvider)
}

// catalogPage is one page of metas, empty and fetched from nowhere past
// TMDB's pagination ceiling. It is never nil: {"metas":null} is not a valid
// empty catalog.
func (s *Server) catalogPage(ctx context.Context, catalogType, params, genre string, page int) ([]provider.Meta, error) {
	if page > maxCatalogPage {
		return []provider.Meta{}, nil
	}
	metas, err := s.provider.FetchCatalogPage(ctx, catalogType, params, genre, page)
	if err != nil || metas != nil {
		return metas, err
	}
	return []provider.Meta{}, nil
}

// catalogWindow is the catalog's titles from the skip-th on, at most
// catalogPageSize of them: what a client holding skip titles asks for next.
// Nuvio's apps and Stremio send skip as the number of titles they hold, and a
// TMDB page holds fewer than catalogPageSize of them whenever a title without
// an IMDB id was dropped (provider.resolveMetas), so the window can start
// partway into a page and run on into the next: walkWindow finds it from page
// 1. A randomized recipe has no order to walk, and a skip past TMDB's
// pagination ceiling nothing to find, so both keep the one page skip lands
// on.
func (s *Server) catalogWindow(ctx context.Context, catalogType, params, genre string, skip int) ([]provider.Meta, error) {
	page := skip/catalogPageSize + 1
	if page > maxCatalogPage || randomizedRecipe(catalogType, params) {
		return s.catalogPage(ctx, catalogType, params, genre, page)
	}
	return s.walkWindow(ctx, catalogType, params, genre, skip)
}

// walkWindow is catalogWindow for a recipe with a fixed order: it walks the
// catalog's pages from page 1, each from the provider's page cache once a
// client has scrolled past it, until it holds catalogPageSize titles past
// skip, the catalog runs out, or it has walked twice as many pages as full
// ones would take, which bounds a cold request deep into a sparse recipe.
func (s *Server) walkWindow(ctx context.Context, catalogType, params, genre string, skip int) ([]provider.Meta, error) {
	window := []provider.Meta{}
	seen := 0
	lastPage := min(2*(skip/catalogPageSize+1), maxCatalogPage)
	for page := 1; page <= lastPage; page++ {
		metas, err := s.provider.FetchCatalogPage(ctx, catalogType, params, genre, page)
		if err != nil {
			return nil, err
		}
		if len(metas) == 0 {
			break
		}
		window = append(window, titlesFrom(metas, skip-seen)...)
		seen += len(metas)
		if len(window) >= catalogPageSize {
			break
		}
	}
	return window[:min(len(window), catalogPageSize)], nil
}

// titlesFrom is page from its from-th title on: all of it when from is
// negative, none when from is past its end.
func titlesFrom(page []provider.Meta, from int) []provider.Meta {
	if from >= len(page) {
		return nil
	}
	return page[max(from, 0):]
}

// randomizedRecipe reports whether params is a randomized recipe, whose page
// is a random pick whatever page is asked for.
func randomizedRecipe(catalogType, params string) bool {
	p, err := provider.DecodeParams(catalogType, params)
	return err == nil && p.IsRandomized()
}

// parseCatalogPath splits the catalog route's {rest...} tail into the
// manifest id and the extra props it carries: skip, the number of titles the
// client already holds, and the picked genre ("" when none). The extra props are parsed from the
// still-escaped path, because PathValue is already percent-decoded — a genre
// sent as "Sci-Fi%20%26%20Fantasy" would otherwise reach url.ParseQuery with
// a bare "&" and split in two.
func parseCatalogPath(r *http.Request) (manifestID string, skip int, genre string) {
	rest := strings.TrimSuffix(r.PathValue("rest"), ".json")
	manifestID, _, hasExtra := strings.Cut(rest, "/")
	if !hasExtra {
		return manifestID, 0, ""
	}

	escaped := r.URL.EscapedPath()
	extra := strings.TrimSuffix(escaped[strings.LastIndex(escaped, "/")+1:], ".json")
	vals, err := url.ParseQuery(extra)
	if err != nil {
		return manifestID, 0, ""
	}
	if n, err := strconv.Atoi(vals.Get("skip")); err == nil && n > 0 {
		skip = n
	}
	return manifestID, skip, vals.Get("genre")
}

// CatalogHandler serves GET /u/{token}/catalog/{type}/{rest...}, where rest
// is "{id}.json" or "{id}/{extraProps}.json" (e.g. "skip=20" for pagination,
// "genre=Action&skip=20" for a genre pick; see provider's applyGenreExtra) —
// Stremio appends extra props as an additional path segment
// rather than a query string, so a wildcard tail is the only way to capture
// both forms with one route.
//
// It serves the catalogs the manifest lists — the token's profile's
// catalogs on the TV (vault.ServedCatalog) — and answers 404 for any other.
func (s *Server) CatalogHandler(w http.ResponseWriter, r *http.Request) {
	catalogType := r.PathValue("type")
	manifestID, skip, genre := parseCatalogPath(r)

	served, err := s.served(r.Context(), r.PathValue("token"), catalogType, manifestID)
	if err != nil {
		catalogNotServed(w, r, err)
		return
	}

	// TMDB is reached with the owner's own key, on a server where each
	// account brings one.
	ctx := provider.WithKeySource(r.Context(), s.keys.Sealed(served.Account, served.SealedKey))
	metas, err := s.catalogWindow(ctx, catalogType, served.Params, genre, skip)
	if err != nil {
		// manifestID comes from the request path, so it is quoted: an
		// unescaped newline in it would otherwise forge a log line.
		logTMDBFailure("catalog "+strconv.Quote(manifestID), err)
		http.Error(w, "upstream error", http.StatusBadGateway)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, catalogResponse{
		Metas:           metas,
		CacheMaxAge:     catalogCacheMaxAge,
		StaleRevalidate: catalogStaleRevalidate,
	})
}

// catalogNotServed answers a catalog route whose lookup failed: 404 for a
// catalog the profile doesn't have, 500 for the vault failing.
func catalogNotServed(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, vault.ErrCatalogNotFound) {
		http.NotFound(w, r)
		return
	}
	log.Printf("addon: catalog: resolving catalog: %v", err)
	http.Error(w, "failed to resolve catalog", http.StatusInternalServerError)
}

// logTMDBFailure logs what, a TMDB call the addon couldn't make, and why. A
// problem with the profile owner's own TMDB key (per-account key mode: they
// have saved none, or TMDB refuses it) is said as that, apart from TMDB
// failing, since only the owner can fix it and a keyless owner's TV asks for
// every row on every load.
func logTMDBFailure(what string, err error) {
	if provider.IsKeyError(err) {
		log.Printf("addon: %s: the profile owner's TMDB key can't be used: %v", what, err)
		return
	}
	log.Printf("addon: %s: %v", what, err)
}
