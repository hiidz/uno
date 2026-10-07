// Command server wires up config, vault, TMDB, and Nuvio into an api.Server
// and starts listening; `server migrate --db <path>` runs the v10→v11
// migration instead (migrate.go).
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hiidz/uno/internal/api"
	"github.com/hiidz/uno/internal/config"
	"github.com/hiidz/uno/internal/nuvio"
	"github.com/hiidz/uno/internal/provider"
	"github.com/hiidz/uno/internal/tmdbkey"
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
	if err := runCommand(context.Background(), os.Args[1:], os.Stdout); err != nil {
		log.Fatal(err)
	}
}

// runCommand runs the migrate subcommand when args name it, writing its
// report to out, and the server otherwise.
func runCommand(ctx context.Context, args []string, out io.Writer) error {
	if len(args) > 0 && args[0] == "migrate" {
		return runMigrate(ctx, args[1:], out)
	}
	return run()
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

	deps, err := apiDeps(cfg, db)
	if err != nil {
		return err
	}
	apiServer, err := api.New(deps)
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

// apiDeps builds the API server's dependencies from cfg over db: the TMDB and
// Nuvio clients, the dev auth bypass when it is configured, the access policy,
// and each account's own TMDB key on a server in per-account key mode.
func apiDeps(cfg config.Config, db *vault.DB) (api.Deps, error) {
	keys, err := accountKeys(cfg, db)
	if err != nil {
		return api.Deps{}, err
	}
	nuvioClient := nuvio.NewClient(cfg.NuvioBaseURL, cfg.NuvioPublishableKey)
	deps := api.Deps{
		Vault:        db,
		Provider:     provider.NewTMDBClient(cfg.TMDBAPIKey),
		Verifier:     nuvioClient,
		Nuvio:        nuvioClient,
		SiteBaseURL:  cfg.SiteBaseURL,
		NuvioBaseURL: cfg.NuvioBaseURL,
		Access:       api.Access{Allowlist: cfg.Access.Allowlist, Emails: cfg.Access.Emails},
		Keys:         keys,
	}
	if cfg.DevAuthBypassToken != "" {
		api.LogDevBypassEnabled()
		deps.Verifier = api.NewDevBypassVerifier(nuvioClient, cfg.DevAuthBypassToken)
		deps.Nuvio = api.NewDevBypassNuvio(nuvioClient, cfg.DevAuthBypassToken)
		deps.Access = deps.Access.WithDevBypass()
	}
	return deps, nil
}

// accountKeys is what hands each request its account's own TMDB key when
// cfg asks every account to bring one, and nil when every account shares
// cfg.TMDBAPIKey.
func accountKeys(cfg config.Config, db *vault.DB) (*tmdbkey.Keys, error) {
	if !cfg.PerAccountKeys {
		return nil, nil
	}
	box, err := tmdbkey.NewBox(cfg.Secret)
	if err != nil {
		return nil, fmt.Errorf("failed to load UNO_SECRET: %w", err)
	}
	log.Println("TMDB key mode: per-account; each Nuvio account brings its own TMDB key")
	return tmdbkey.New(box, db), nil
}
