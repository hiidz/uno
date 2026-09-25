package config

import (
	"maps"
	"strings"
	"testing"
)

// setEnv sets every variable Load reads, with an empty value standing for
// unset (Load treats the two the same).
func setEnv(t *testing.T, vars map[string]string) {
	t.Helper()
	for _, key := range []string{
		"VAULT_DB", "PORT", "TMDB_API_KEY", "NUVIO_BASE_URL",
		"NUVIO_PUBLISHABLE_KEY", "SITE_BASE_URL", "DEV_AUTH_BYPASS_TOKEN",
	} {
		t.Setenv(key, vars[key])
	}
}

var required = map[string]string{
	"TMDB_API_KEY":          "tmdb-key",
	"NUVIO_PUBLISHABLE_KEY": "publishable-key",
	"SITE_BASE_URL":         "https://uno.example",
}

// With only the required variables set, everything else takes its default,
// and the dev auth bypass stays off.
func TestLoadDefaults(t *testing.T) {
	setEnv(t, required)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := Config{
		DBPath:              "vault.db",
		Port:                "8123",
		TMDBAPIKey:          "tmdb-key",
		NuvioBaseURL:        "https://api.nuvio.tv",
		NuvioPublishableKey: "publishable-key",
		SiteBaseURL:         "https://uno.example",
	}
	if cfg != want {
		t.Fatalf("cfg = %+v\nwant  %+v", cfg, want)
	}
}

func TestLoadOverrides(t *testing.T) {
	vars := map[string]string{
		"VAULT_DB":              "/data/vault.db",
		"PORT":                  "9000",
		"NUVIO_BASE_URL":        "https://nuvio.example",
		"DEV_AUTH_BYPASS_TOKEN": "dev-secret",
	}
	maps.Copy(vars, required)
	setEnv(t, vars)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := Config{
		DBPath:              "/data/vault.db",
		Port:                "9000",
		TMDBAPIKey:          "tmdb-key",
		NuvioBaseURL:        "https://nuvio.example",
		NuvioPublishableKey: "publishable-key",
		SiteBaseURL:         "https://uno.example",
		DevAuthBypassToken:  "dev-secret",
	}
	if cfg != want {
		t.Fatalf("cfg = %+v\nwant  %+v", cfg, want)
	}
}

// A required variable with no safe default fails the load, and the error
// names every one that is missing, not just the first.
func TestLoadNamesEveryMissingVariable(t *testing.T) {
	for missing := range required {
		t.Run(missing, func(t *testing.T) {
			vars := map[string]string{}
			for k, v := range required {
				if k != missing {
					vars[k] = v
				}
			}
			setEnv(t, vars)

			_, err := Load()
			if err == nil || !strings.Contains(err.Error(), missing) {
				t.Fatalf("err = %v, want one naming %s", err, missing)
			}
		})
	}

	t.Run("all", func(t *testing.T) {
		setEnv(t, nil)
		_, err := Load()
		if err == nil {
			t.Fatal("err = nil, want one")
		}
		for missing := range required {
			if !strings.Contains(err.Error(), missing) {
				t.Errorf("err = %v, want it to name %s", err, missing)
			}
		}
	})
}
