// Package api implements Uno's HTTP surface: the authenticated JSON API
// used by the configure SPA (catalogs, collections, profiles, push) and,
// via internal/addon, the public Stremio-protocol endpoints.
package api

import (
	"io/fs"
	"log"
	"net/http"

	"github.com/hiidz/uno/internal/addon"
	"github.com/hiidz/uno/internal/provider"
	"github.com/hiidz/uno/internal/static"
	"github.com/hiidz/uno/internal/vault"
	unoweb "github.com/hiidz/uno/web"
)

// Server is Uno's HTTP handler: the authenticated JSON API plus the public
// addon and SPA routes registered in routes().
type Server struct {
	vault       *vault.DB
	router      *http.ServeMux
	provider    *provider.TMDBClient
	verifier    TokenVerifier
	nuvio       NuvioClient
	addon       *addon.Server
	siteBaseURL string
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

// New builds a Server from d. Deps has no required-field enforcement at the
// type level — a struct literal with a field omitted still compiles — so
// this checks for it explicitly and fails at startup rather than as a nil-
// pointer panic on the first request that happens to reach the missing
// dependency.
func New(d Deps) *Server {
	switch {
	case d.Vault == nil:
		log.Fatal("api.New: Deps.Vault is nil")
	case d.Provider == nil:
		log.Fatal("api.New: Deps.Provider is nil")
	case d.Verifier == nil:
		log.Fatal("api.New: Deps.Verifier is nil")
	case d.Nuvio == nil:
		log.Fatal("api.New: Deps.Nuvio is nil")
	}

	s := &Server{
		vault:       d.Vault,
		router:      http.NewServeMux(),
		provider:    d.Provider,
		verifier:    d.Verifier,
		nuvio:       d.Nuvio,
		addon:       addon.New(d.Vault, d.Provider),
		siteBaseURL: d.SiteBaseURL,
	}
	s.routes()
	return s
}

func (s *Server) routes() {
	// Not profile-scoped: previewing a recipe reads nothing from the vault, so
	// there is no profile to resolve. Every other catalog route, including the
	// community list and take, lives under /api/p/{profileIndex}/.
	s.router.HandleFunc("POST /api/catalogs/preview", s.requireNuvioAuth(s.previewCatalog))
	s.router.HandleFunc("POST /api/catalogs/genre-options", s.requireNuvioAuth(s.catalogGenreOptions))
	s.router.HandleFunc("GET /api/p/{profileIndex}/catalogs", s.requireNuvioAuth(s.requireProfile(s.listUserCatalogs)))
	s.router.HandleFunc("POST /api/p/{profileIndex}/catalogs", s.requireNuvioAuth(s.requireProfile(s.createUserCatalog)))
	s.router.HandleFunc("PUT /api/p/{profileIndex}/catalogs/{catalogID}", s.requireNuvioAuth(s.requireProfile(s.updateUserCatalog)))
	s.router.HandleFunc("DELETE /api/p/{profileIndex}/catalogs/{catalogID}", s.requireNuvioAuth(s.requireProfile(s.deleteUserCatalog)))
	s.router.HandleFunc("GET /api/p/{profileIndex}/community/catalogs", s.requireNuvioAuth(s.requireProfile(s.listCommunityCatalogs)))
	s.router.HandleFunc("POST /api/p/{profileIndex}/community/catalogs/{catalogID}/take", s.requireNuvioAuth(s.requireProfile(s.takeCatalog)))

	s.router.HandleFunc("GET /api/p/{profileIndex}/collections", s.requireNuvioAuth(s.requireProfile(s.listUserCollections)))
	s.router.HandleFunc("POST /api/p/{profileIndex}/collections", s.requireNuvioAuth(s.requireProfile(s.createUserCollection)))
	s.router.HandleFunc("POST /api/p/{profileIndex}/collections/{collectionID}/duplicate", s.requireNuvioAuth(s.requireProfile(s.duplicateUserCollection)))
	s.router.HandleFunc("PUT /api/p/{profileIndex}/collections/{collectionID}", s.requireNuvioAuth(s.requireProfile(s.updateUserCollection)))
	s.router.HandleFunc("DELETE /api/p/{profileIndex}/collections/{collectionID}", s.requireNuvioAuth(s.requireProfile(s.deleteUserCollection)))
	s.router.HandleFunc("GET /api/p/{profileIndex}/community/collections", s.requireNuvioAuth(s.requireProfile(s.listCommunityCollections)))
	s.router.HandleFunc("POST /api/p/{profileIndex}/community/collections/{collectionID}/take", s.requireNuvioAuth(s.requireProfile(s.takeCollection)))

	// Selection is read here but never written here: the whole selection
	// travels in POST .../push's body and is written by that handler, in one
	// transaction, only after Nuvio has accepted the push.
	s.router.HandleFunc("GET /api/p/{profileIndex}/catalogs/selection", s.requireNuvioAuth(s.requireProfile(s.listCurrentCatalogSelection)))

	s.router.HandleFunc("GET /api/p/{profileIndex}/collections/selection", s.requireNuvioAuth(s.requireProfile(s.listCurrentCollectionSelection)))

	s.router.HandleFunc("POST /api/p/{profileIndex}/push", s.requireNuvioAuth(s.requireProfile(s.push)))

	s.router.HandleFunc("GET /api/genres/{type}", s.requireNuvioAuth(s.listGenres))
	s.router.HandleFunc("GET /api/certifications/{type}", s.requireNuvioAuth(s.listCertifications))
	s.router.HandleFunc("GET /api/languages", s.requireNuvioAuth(s.listLanguages))
	s.router.HandleFunc("GET /api/countries", s.requireNuvioAuth(s.listCountries))
	s.router.HandleFunc("GET /api/watch-providers/{type}", s.requireNuvioAuth(s.listWatchProviders))
	s.router.HandleFunc("GET /api/watch-regions", s.requireNuvioAuth(s.listWatchRegions))

	s.router.HandleFunc("GET /api/profiles", s.requireNuvioAuth(s.listProfiles))
	s.router.HandleFunc("POST /api/profiles/select", s.requireNuvioAuth(s.selectProfile))
	s.router.HandleFunc("GET /api/health", s.health)

	s.router.HandleFunc("GET "+addon.ManifestPathPattern, s.addon.Public(s.addon.ManifestHandler))
	s.router.HandleFunc("GET /u/{token}/catalog/{type}/{rest...}", s.addon.Public(s.addon.CatalogHandler))

	// Everything else: the embedded SPA build (web:embed.go), with the
	// existing routes above taking precedence since ServeMux matches the
	// most specific registered pattern first.
	distFS, err := fs.Sub(unoweb.DistFS, "dist")
	if err != nil {
		log.Fatalf("failed to open embedded web/dist: %v", err)
	}
	s.router.Handle("/", static.Gzip(static.Handler(distFS)))
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}
