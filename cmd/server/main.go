// Command server wires up config, vault, TMDB, and Nuvio into an api.Server
// and starts listening.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/hiidz/uno/internal/api"
	"github.com/hiidz/uno/internal/config"
	"github.com/hiidz/uno/internal/nuvio"
	"github.com/hiidz/uno/internal/provider"
	"github.com/hiidz/uno/internal/vault"
)

// shutdownGracePeriod bounds how long a drain waits for requests already in
// flight. It matches the server's WriteTimeout below, the longest a handler
// can hold a request, so a push — a multi-step, non-atomic sequence against
// Nuvio that leaves a profile in the state pushResult.UndoFailed reports if
// it is cut in half — gets the whole span it is allowed. A container runtime
// that sends SIGKILL sooner (10s after SIGTERM, for `docker stop`) cuts the
// drain shorter.
const shutdownGracePeriod = 60 * time.Second

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

// run holds every startup step, so each one's cleanup can unwind through a
// defer: log.Fatal in main calls os.Exit, which runs no defer, and returning
// the error here is what lets db.Close and the signal handler release.
func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	log.Println("Initializing Database")
	db, err := vault.InitDB(cfg.DBPath)
	if err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			log.Printf("Failed to close database: %v", err)
		}
	}()

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
		return fmt.Errorf("failed to build server: %w", err)
	}

	// Timeouts are set explicitly because http.ListenAndServe's zero-value
	// server has none: a client that opens a connection and never finishes
	// its headers otherwise holds it forever. WriteTimeout is the loosest of
	// the three — it bounds the whole handler, and the addon's catalog route
	// fans one request out to a discover call plus up to 20 external_ids
	// lookups, each with the provider's own 10s client timeout.
	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           apiServer,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		MaxHeaderBytes:    1 << 20, // 1 MiB
	}

	// ctx is done on the first SIGINT/SIGTERM; a second one kills the process
	// outright, since stop restores the default disposition.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Buffered so the goroutine can hand over its error and exit even when
	// nothing is left reading — a shutdown is one of those cases.
	listenErr := make(chan error, 1)
	go func() {
		log.Printf("Server starting on port %s", cfg.Port)
		listenErr <- srv.ListenAndServe()
	}()

	select {
	case err := <-listenErr:
		// ErrServerClosed can only arrive here from a Shutdown started
		// elsewhere; a listen that fails on its own is fatal.
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("server failed on port %s: %w", cfg.Port, err)
		}
		return nil
	case <-ctx.Done():
	}

	log.Println("Shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownGracePeriod)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown did not finish cleanly: %w", err)
	}
	return nil
}
