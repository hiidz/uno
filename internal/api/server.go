// Package api implements Uno's HTTP surface: the authenticated JSON API
// used by the configure SPA (catalogs, collections, profiles, push) and,
// via internal/addon, the public Stremio-protocol endpoints.
package api

import (
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"

	"github.com/hiidz/uno/internal/addon"
	"github.com/hiidz/uno/internal/provider"
	"github.com/hiidz/uno/internal/static"
	"github.com/hiidz/uno/internal/tmdbkey"
	"github.com/hiidz/uno/internal/vault"
	unoweb "github.com/hiidz/uno/web"
)

// Server is Uno's HTTP handler: the authenticated JSON API plus the public
// addon and SPA routes registered in routes().
type Server struct {
	vault        *vault.DB
	router       *http.ServeMux
	provider     *provider.TMDBClient
	verifier     TokenVerifier
	nuvio        NuvioClient
	addon        *addon.Server
	siteBaseURL  string
	nuvioBaseURL string
	// admission is the access policy's allowlist, nil when every account is
	// admitted.
	admission *admission
	// keys gives each request its account's own TMDB key, nil on a server
	// with one shared key.
	keys *tmdbkey.Keys
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

// New builds a Server from d. Deps has no required-field enforcement at the
// type level — a struct literal with a field omitted still compiles — so
// this checks for it explicitly and reports the missing dependency rather
// than leaving a nil-pointer panic for the first request that happens to
// reach it. Every error here is a startup misconfiguration; cmd/server is
// where that becomes a fatal.
func New(d Deps) (*Server, error) {
	switch {
	case d.Vault == nil:
		return nil, errors.New("api: Deps.Vault is nil")
	case d.Provider == nil:
		return nil, errors.New("api: Deps.Provider is nil")
	case d.Verifier == nil:
		return nil, errors.New("api: Deps.Verifier is nil")
	case d.Nuvio == nil:
		return nil, errors.New("api: Deps.Nuvio is nil")
	case d.SiteBaseURL == "":
		// The one dep whose absence never panics: it would instead push a
		// relative manifest URL into the user's Nuvio account.
		return nil, errors.New("api: Deps.SiteBaseURL is empty")
	}

	addonServer, err := addon.New(d.Vault, d.Provider, d.Keys, d.SiteBaseURL)
	if err != nil {
		return nil, fmt.Errorf("api: building addon server: %w", err)
	}

	s := &Server{
		vault:        d.Vault,
		router:       http.NewServeMux(),
		provider:     d.Provider,
		verifier:     d.Verifier,
		nuvio:        d.Nuvio,
		addon:        addonServer,
		siteBaseURL:  d.SiteBaseURL,
		nuvioBaseURL: d.NuvioBaseURL,
		admission:    d.Access.admission(),
		keys:         d.Keys,
	}
	if err := s.routes(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Server) routes() error {
	// Not profile-scoped: previewing a recipe reads nothing from the vault, so
	// there is no profile to resolve. Every other catalog route, including
	// publishing and Community, lives under /api/p/{profileIndex}/.
	s.router.HandleFunc("POST /api/catalogs/preview", s.requireNuvioAuth(s.previewCatalog))
	s.router.HandleFunc("POST /api/catalogs/genre-options", s.requireNuvioAuth(s.catalogGenreOptions))
	s.router.HandleFunc("GET /api/p/{profileIndex}/library", s.requireProfileAuth(s.getLibrary))
	s.router.HandleFunc("POST /api/p/{profileIndex}/catalogs", s.requireProfileAuth(s.createUserCatalog))
	s.router.HandleFunc("PUT /api/p/{profileIndex}/catalogs/{catalogID}", s.requireProfileAuth(s.updateUserCatalog))
	s.router.HandleFunc("DELETE /api/p/{profileIndex}/catalogs/{catalogID}", s.requireProfileAuth(s.deleteUserCatalog))
	s.router.HandleFunc("POST /api/p/{profileIndex}/catalogs/{catalogID}/duplicate", s.requireProfileAuth(s.duplicateUserCatalog))
	s.router.HandleFunc("POST /api/p/{profileIndex}/catalogs/{catalogID}/publish", s.requireProfileAuth(s.publishCatalog))
	s.router.HandleFunc("POST /api/p/{profileIndex}/catalogs/{catalogID}/unpublish", s.requireProfileAuth(s.unpublishCatalog))
	s.router.HandleFunc("GET /api/p/{profileIndex}/catalogs/{catalogID}/changes-since-publish", s.requireProfileAuth(s.catalogChanges))
	s.router.HandleFunc("POST /api/p/{profileIndex}/catalogs/{catalogID}/acknowledge-release", s.requireProfileAuth(s.acknowledgeReleasedCatalog))

	s.router.HandleFunc("POST /api/p/{profileIndex}/collections", s.requireProfileAuth(s.createUserCollection))
	s.router.HandleFunc("POST /api/p/{profileIndex}/collections/{collectionID}/duplicate", s.requireProfileAuth(s.duplicateUserCollection))
	s.router.HandleFunc("PUT /api/p/{profileIndex}/collections/{collectionID}", s.requireProfileAuth(s.updateUserCollection))
	s.router.HandleFunc("DELETE /api/p/{profileIndex}/collections/{collectionID}", s.requireProfileAuth(s.deleteUserCollection))
	s.router.HandleFunc("POST /api/p/{profileIndex}/collections/{collectionID}/publish", s.requireProfileAuth(s.publishCollection))
	s.router.HandleFunc("POST /api/p/{profileIndex}/collections/{collectionID}/unpublish", s.requireProfileAuth(s.unpublishCollection))
	s.router.HandleFunc("GET /api/p/{profileIndex}/collections/{collectionID}/changes-since-publish", s.requireProfileAuth(s.collectionChanges))
	s.router.HandleFunc("POST /api/p/{profileIndex}/collections/{collectionID}/acknowledge-release", s.requireProfileAuth(s.acknowledgeReleasedCollection))

	// Community: other profiles' live publications, by publication id.
	s.router.HandleFunc("GET /api/p/{profileIndex}/community", s.requireProfileAuth(s.listCommunityPage))
	s.router.HandleFunc("GET /api/p/{profileIndex}/community/{publicationID}", s.requireProfileAuth(s.getPublication))
	s.router.HandleFunc("POST /api/p/{profileIndex}/community/{publicationID}/subscribe", s.requireProfileAuth(s.subscribe))
	s.router.HandleFunc("GET /api/p/{profileIndex}/community/{publicationID}/changes", s.requireProfileAuth(s.updateChanges))
	s.router.HandleFunc("POST /api/p/{profileIndex}/community/{publicationID}/update", s.requireProfileAuth(s.updateSubscription))
	s.router.HandleFunc("POST /api/p/{profileIndex}/community/{publicationID}/duplicate", s.requireProfileAuth(s.duplicatePublication))

	// Released: the caller's rows whose publication ended, until acknowledged.
	s.router.HandleFunc("GET /api/p/{profileIndex}/released", s.requireProfileAuth(s.listReleased))

	s.router.HandleFunc("POST /api/p/{profileIndex}/export", s.requireProfileAuth(s.exportBundle))
	s.router.HandleFunc("POST /api/p/{profileIndex}/import/check", s.requireProfileAuth(s.checkImport))
	s.router.HandleFunc("POST /api/p/{profileIndex}/import", s.requireProfileAuth(s.importBundle))

	// The Home selection travels whole in push's body and is written only by
	// that handler, in one transaction, once Nuvio has accepted the push. It
	// is read back from the library, as each row's home_position.
	s.router.HandleFunc("POST /api/p/{profileIndex}/push", s.requireProfileAuth(s.push))

	s.router.HandleFunc("GET /api/genres/{type}", s.requireNuvioAuth(s.listGenres))
	s.router.HandleFunc("GET /api/certifications/{type}", s.requireNuvioAuth(s.listCertifications))
	s.router.HandleFunc("GET /api/languages", s.requireNuvioAuth(s.listLanguages))
	s.router.HandleFunc("GET /api/countries", s.requireNuvioAuth(s.listCountries))
	s.router.HandleFunc("GET /api/watch-providers/{type}", s.requireNuvioAuth(s.listWatchProviders))
	s.router.HandleFunc("GET /api/watch-regions", s.requireNuvioAuth(s.listWatchRegions))
	s.router.HandleFunc("GET /api/companies/search", s.requireNuvioAuth(s.searchCompanies))
	s.router.HandleFunc("GET /api/companies/{id}", s.requireNuvioAuth(s.getCompany))
	s.router.HandleFunc("GET /api/keywords/search", s.requireNuvioAuth(s.searchKeywords))
	s.router.HandleFunc("GET /api/keywords/{id}", s.requireNuvioAuth(s.getKeyword))
	s.router.HandleFunc("GET /api/collections/search", s.requireNuvioAuth(s.searchCollections))
	s.router.HandleFunc("GET /api/collections/{id}", s.requireNuvioAuth(s.getCollection))
	s.router.HandleFunc("GET /api/networks/search", s.requireNuvioAuth(s.searchNetworks))
	s.router.HandleFunc("GET /api/networks/{id}", s.requireNuvioAuth(s.getNetwork))

	// The signed-in account's own TMDB key, on a server where each account
	// brings one (perAccountKeys: a 404 otherwise).
	s.router.HandleFunc("GET /api/config", s.config)
	s.router.HandleFunc("GET /api/account/tmdb-key", s.requireNuvioAuth(s.perAccountKeys(s.getTMDBKey)))
	s.router.HandleFunc("PUT /api/account/tmdb-key", s.requireNuvioAuth(s.perAccountKeys(s.putTMDBKey)))
	s.router.HandleFunc("DELETE /api/account/tmdb-key", s.requireNuvioAuth(s.perAccountKeys(s.deleteTMDBKey)))

	s.router.HandleFunc("GET /api/profiles", s.requireNuvioAuth(s.listProfiles))
	s.router.HandleFunc("POST /api/profiles/select", s.requireNuvioAuth(s.selectProfile))
	s.router.HandleFunc("GET /api/health", s.health)

	s.router.HandleFunc("GET "+addon.ManifestPathPattern, s.addon.Public(s.addon.ManifestHandler))
	s.router.HandleFunc("GET /u/{token}/catalog/{type}/{rest...}", s.addon.Public(s.addon.CatalogHandler))
	s.router.HandleFunc("GET "+addon.ConfigurePathPattern, s.addon.Public(s.addon.ConfigureHandler))

	// Everything else: the embedded SPA build (web:embed.go), with the
	// existing routes above taking precedence since ServeMux matches the
	// most specific registered pattern first.
	distFS, err := fs.Sub(unoweb.DistFS, "dist")
	if err != nil {
		return fmt.Errorf("api: opening embedded web/dist: %w", err)
	}
	spa, err := static.Handler(distFS, s.nuvioBaseURL)
	if err != nil {
		return fmt.Errorf("api: building static handler: %w", err)
	}
	gzipped, err := static.Gzip(spa)
	if err != nil {
		return fmt.Errorf("api: building static handler: %w", err)
	}
	s.router.Handle("/", gzipped)
	return nil
}

// health is the liveness probe: a plain-text 200 that reads nothing, so it
// answers even while the vault or Nuvio is unreachable. The write failure is
// logged rather than reported — the status line is already on the wire, so
// there is nothing left to tell the client.
func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte("ok")); err != nil {
		log.Printf("api: health: writing response: %v", err)
	}
}
