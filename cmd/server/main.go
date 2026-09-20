// Command server wires up config, vault, TMDB, and Nuvio into an api.Server
// and starts listening.
package main

import (
	"log"
	"net/http"

	"github.com/hiidz/uno/internal/api"
	"github.com/hiidz/uno/internal/config"
	"github.com/hiidz/uno/internal/nuvio"
	"github.com/hiidz/uno/internal/provider"
	"github.com/hiidz/uno/internal/vault"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	log.Println("Initializing Database")
	db, err := vault.InitDB(cfg.DBPath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	tmdb := provider.NewTMDBClient(cfg.TMDBAPIKey)
	nuvioClient := nuvio.NewClient(cfg.NuvioBaseURL, cfg.NuvioPublishableKey)

	var verifier api.TokenVerifier = nuvioClient
	var nuvioAPI api.NuvioClient = nuvioClient
	if cfg.DevAuthBypassToken != "" {
		api.LogDevBypassEnabled()
		verifier = api.NewDevBypassVerifier(nuvioClient, cfg.DevAuthBypassToken)
		nuvioAPI = api.NewDevBypassNuvio(nuvioClient, cfg.DevAuthBypassToken)
	}

	apiServer, err := api.New(api.Deps{
		Vault:       db,
		Provider:    tmdb,
		Verifier:    verifier,
		Nuvio:       nuvioAPI,
		SiteBaseURL: cfg.SiteBaseURL,
	})
	if err != nil {
		log.Fatalf("Failed to build server: %v", err)
	}

	log.Printf("Server starting on port %s", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, apiServer); err != nil {
		log.Fatalf("Server failed on port %s: %v", cfg.Port, err)
	}
}
