// Package api implements Uno's HTTP surface: the authenticated JSON API
// used by the configure SPA (catalogs, collections, profiles, push) and,
// via internal/addon, the public Stremio-protocol endpoints.
package api

import (
	"errors"
	"fmt"
	"io/fs"
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
	}

	addonServer, err := addon.New(d.Vault, d.Provider)
	if err != nil {
		return nil, fmt.Errorf("api: building addon server: %w", err)
	}

	s := &Server{
		vault:       d.Vault,
		router:      http.NewServeMux(),
		provider:    d.Provider,
		verifier:    d.Verifier,
		nuvio:       d.Nuvio,
		addon:       addonServer,
		siteBaseURL: d.SiteBaseURL,
	}
	if err := s.routes(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Server) routes() error {
	// Not profile-scoped: previewing a recipe reads nothing from the vault, so
	// there is no profile to resolve. Every other catalog route, including the
	// community list and take, lives under /api/p/{profileIndex}/.
	s.router.HandleFunc("POST /api/catalogs/preview", s.requireNuvioAuth(s.previewCatalog))
	s.router.HandleFunc("POST /api/catalogs/genre-options", s.requireNuvioAuth(s.catalogGenreOptions))
	s.router.HandleFunc("GET /api/p/{profileIndex}/catalogs", s.requireProfileAuth(s.listUserCatalogs))
	s.router.HandleFunc("POST /api/p/{profileIndex}/catalogs", s.requireProfileAuth(s.createUserCatalog))
	s.router.HandleFunc("PUT /api/p/{profileIndex}/catalogs/{catalogID}", s.requireProfileAuth(s.updateUserCatalog))
	s.router.HandleFunc("DELETE /api/p/{profileIndex}/catalogs/{catalogID}", s.requireProfileAuth(s.deleteUserCatalog))
	s.router.HandleFunc("GET /api/p/{profileIndex}/community/catalogs", s.requireProfileAuth(s.listCommunityCatalogs))
	s.router.HandleFunc("POST /api/p/{profileIndex}/community/catalogs/{catalogID}/take", s.requireProfileAuth(s.takeCatalog))

	s.router.HandleFunc("GET /api/p/{profileIndex}/collections", s.requireProfileAuth(s.listUserCollections))
	s.router.HandleFunc("POST /api/p/{profileIndex}/collections", s.requireProfileAuth(s.createUserCollection))
	s.router.HandleFunc("POST /api/p/{profileIndex}/collections/{collectionID}/duplicate", s.requireProfileAuth(s.duplicateUserCollection))
	s.router.HandleFunc("PUT /api/p/{profileIndex}/collections/{collectionID}", s.requireProfileAuth(s.updateUserCollection))
	s.router.HandleFunc("DELETE /api/p/{profileIndex}/collections/{collectionID}", s.requireProfileAuth(s.deleteUserCollection))
	s.router.HandleFunc("GET /api/p/{profileIndex}/community/collections", s.requireProfileAuth(s.listCommunityCollections))
	s.router.HandleFunc("POST /api/p/{profileIndex}/community/collections/{collectionID}/take", s.requireProfileAuth(s.takeCollection))

	// Selection is read here but never written here: the whole selection
	// travels in POST .../push's body and is written by that handler, in one
	// transaction, only after Nuvio has accepted the push.
	s.router.HandleFunc("GET /api/p/{profileIndex}/catalogs/selection", s.requireProfileAuth(s.listCurrentCatalogSelection))

	s.router.HandleFunc("GET /api/p/{profileIndex}/collections/selection", s.requireProfileAuth(s.listCurrentCollectionSelection))

	s.router.HandleFunc("POST /api/p/{profileIndex}/push", s.requireProfileAuth(s.push))

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
		return fmt.Errorf("api: opening embedded web/dist: %w", err)
	}
	gzipped, err := static.Gzip(static.Handler(distFS))
	if err != nil {
		return fmt.Errorf("api: building static handler: %w", err)
	}
	s.router.Handle("/", gzipped)
	return nil
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}
