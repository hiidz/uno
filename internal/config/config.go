// Package config loads runtime configuration from the environment (and an
// optional .env file), applying defaults and failing fast on missing
// required values.
package config

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

// Config is Uno's resolved runtime configuration.
type Config struct {
	DBPath              string
	Port                string
	TMDBAPIKey          string
	NuvioBaseURL        string
	NuvioPublishableKey string
	SiteBaseURL         string
	DevAuthBypassToken  string
}

// Load reads .env (if present) and returns the resolved config, applying
// defaults for any variable that isn't set. It returns an error if a
// required variable with no safe default is missing.
func Load() (Config, error) {
	_ = godotenv.Load()

	cfg := Config{
		DBPath:              getEnv("VAULT_DB", "vault.db"),
		Port:                getEnv("PORT", "8123"),
		TMDBAPIKey:          getEnv("TMDB_API_KEY", ""),
		NuvioBaseURL:        getEnv("NUVIO_BASE_URL", "https://api.nuvio.tv"),
		NuvioPublishableKey: getEnv("NUVIO_PUBLISHABLE_KEY", ""),
		SiteBaseURL:         getEnv("SITE_BASE_URL", ""),
		DevAuthBypassToken:  getEnv("DEV_AUTH_BYPASS_TOKEN", ""),
	}

	var missing []string
	if cfg.TMDBAPIKey == "" {
		missing = append(missing, "TMDB_API_KEY")
	}
	if cfg.NuvioPublishableKey == "" {
		missing = append(missing, "NUVIO_PUBLISHABLE_KEY")
	}
	if cfg.SiteBaseURL == "" {
		missing = append(missing, "SITE_BASE_URL")
	}
	if len(missing) > 0 {
		return Config{}, fmt.Errorf("missing required environment variable(s): %v", missing)
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
