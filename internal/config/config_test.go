package config

import (
	"maps"
	"reflect"
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
		"UNO_ACCESS", "UNO_ALLOWED_EMAILS", "TMDB_KEY_MODE", "UNO_SECRET",
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
// the dev auth bypass stays off and access is open.
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
	if !reflect.DeepEqual(cfg, want) {
		t.Fatalf("cfg = %+v\nwant  %+v", cfg, want)
	}
}

func TestLoadOverrides(t *testing.T) {
	vars := map[string]string{
		"VAULT_DB":              "/data/vault.db",
		"PORT":                  "9000",
		"NUVIO_BASE_URL":        "https://nuvio.example",
		"DEV_AUTH_BYPASS_TOKEN": devBypassToken,
		"UNO_ACCESS":            "allowlist",
		"UNO_ALLOWED_EMAILS":    " Someone@Example.com, ,other+tag@example.org ",
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
		DevAuthBypassToken:  devBypassToken,
		Access:              Access{Allowlist: true, Emails: []string{"someone@example.com", "other+tag@example.org"}},
	}
	if !reflect.DeepEqual(cfg, want) {
		t.Fatalf("cfg = %+v\nwant  %+v", cfg, want)
	}
}

// devBypassToken is a DEV_AUTH_BYPASS_TOKEN of the shortest length Load takes.
const devBypassToken = "0123456789abcdef0123456789abcdef"

// A DEV_AUTH_BYPASS_TOKEN short enough to guess stops the start; one of the
// shortest length taken, or none, doesn't.
func TestLoadRefusesAShortDevBypassToken(t *testing.T) {
	for token, refused := range map[string]bool{
		"":                  false,
		"dev":               true,
		devBypassToken[:31]: true,
		devBypassToken:      false,
	} {
		vars := map[string]string{"DEV_AUTH_BYPASS_TOKEN": token}
		maps.Copy(vars, required)
		setEnv(t, vars)

		_, err := Load()
		if got := err != nil && strings.Contains(err.Error(), "DEV_AUTH_BYPASS_TOKEN is shorter than 32"); got != refused || (!refused && err != nil) {
			t.Errorf("a %d-character token: err = %v, want refused %t", len(token), err, refused)
		}
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

// UNO_ACCESS and UNO_ALLOWED_EMAILS must agree, and every entry must be an
// email address: an unknown mode, an allowlist naming no one, emails listed
// under open access, a Nuvio account id and anything else that isn't one
// address each stop the start with an error naming the variable.
func TestLoadRefusesAccessItCantUse(t *testing.T) {
	for _, tc := range []struct {
		name, mode, emails, want string
	}{
		{"unknown mode", "closed", "", `UNO_ACCESS is "closed"`},
		{"allowlist of no one", "allowlist", " , ", "names no one"},
		{"emails under open access", "open", "someone@example.com", "UNO_ALLOWED_EMAILS is set"},
		{"emails under the default mode", "", "someone@example.com", "UNO_ALLOWED_EMAILS is set"},
		{"an account id", "allowlist", "d7f23542-5f70-4d44-9a81-733f69adbac9", `lists "d7f23542-5f70-4d44-9a81-733f69adbac9", a Nuvio account id; the list takes email addresses`},
		{"no @", "allowlist", "someone", `lists "someone", which is not an email address`},
		{"nothing before the @", "allowlist", "@example.com", `lists "@example.com", which is not an email address`},
		{"nothing after the @", "allowlist", "someone@", `lists "someone@", which is not an email address`},
		{"two @", "allowlist", "a@b@example.com", `lists "a@b@example.com", which is not an email address`},
		{"a space inside", "allowlist", "some one@example.com", `lists "some one@example.com", which is not an email address`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			vars := map[string]string{"UNO_ACCESS": tc.mode, "UNO_ALLOWED_EMAILS": tc.emails}
			maps.Copy(vars, required)
			setEnv(t, vars)

			if _, err := Load(); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want one containing %q", err, tc.want)
			}
		})
	}
}

// In per-account mode there is no shared key and UNO_SECRET is decoded; in
// shared mode UNO_SECRET is ignored.
func TestLoadTMDBKeyModes(t *testing.T) {
	secret := "AQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQE=" // 32 bytes of 0x01
	vars := map[string]string{"TMDB_KEY_MODE": "per-account", "UNO_SECRET": secret}
	maps.Copy(vars, required)
	vars["TMDB_API_KEY"] = ""
	setEnv(t, vars)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.PerAccountKeys || cfg.TMDBAPIKey != "" || len(cfg.Secret) != 32 || cfg.Secret[0] != 1 {
		t.Errorf("per-account: %+v, want the decoded secret and no shared key", cfg)
	}

	vars = map[string]string{"TMDB_KEY_MODE": "shared", "UNO_SECRET": "not base64"}
	maps.Copy(vars, required)
	setEnv(t, vars)
	if cfg, err = Load(); err != nil || cfg.PerAccountKeys || cfg.Secret != nil || cfg.TMDBAPIKey != "tmdb-key" {
		t.Errorf("shared with a stray secret = %+v, %v; want shared, the secret ignored", cfg, err)
	}
}

// A mode the variables don't support stops the start, naming why.
func TestLoadRefusesTMDBKeyModesItCantUse(t *testing.T) {
	secret := "AQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQE="
	for _, tc := range []struct {
		name, mode, apiKey, secret, want string
	}{
		{"unknown mode", "mine", "k", "", `TMDB_KEY_MODE is "mine"`},
		{"shared without a key", "shared", "", "", "TMDB_API_KEY"},
		{"per-account with a shared key", "per-account", "k", secret, "TMDB_API_KEY is set"},
		{"per-account without a secret", "per-account", "", "", "UNO_SECRET must be 32 random bytes"},
		{"per-account with a short secret", "per-account", "", "AQEB", "UNO_SECRET must be 32 random bytes"},
		{"per-account with a secret that isn't base64", "per-account", "", "%%%", "UNO_SECRET must be 32 random bytes"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			vars := map[string]string{}
			maps.Copy(vars, required)
			vars["TMDB_KEY_MODE"], vars["TMDB_API_KEY"], vars["UNO_SECRET"] = tc.mode, tc.apiKey, tc.secret
			setEnv(t, vars)
			if _, err := Load(); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want one containing %q", err, tc.want)
			}
		})
	}
}
