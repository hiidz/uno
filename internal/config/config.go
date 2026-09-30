// Package config loads runtime configuration from the environment (and an
// optional .env file), applying defaults and failing fast on missing
// required values.
package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"unicode"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
)

// Config is Uno's resolved runtime configuration.
type Config struct {
	DBPath     string
	Port       string
	TMDBAPIKey string
	// PerAccountKeys is TMDB_KEY_MODE=per-account: each Nuvio account brings
	// its own TMDB key, sealed under Secret, and TMDBAPIKey is empty.
	PerAccountKeys      bool
	Secret              []byte
	NuvioBaseURL        string
	NuvioPublishableKey string
	SiteBaseURL         string
	DevAuthBypassToken  string
	Access              Access
}

// Access says which Nuvio accounts may sign in: every account, or, with
// Allowlist set, only those whose email address is in Emails, lower-cased.
type Access struct {
	Allowlist bool
	Emails    []string
}

// The access modes UNO_ACCESS takes.
const (
	accessOpen      = "open"
	accessAllowlist = "allowlist"
)

// Load reads .env (if present) and returns the resolved config, applying
// defaults for any variable that isn't set. It returns one error naming
// every required variable that is missing and every value it can't use.
func Load() (Config, error) {
	_ = godotenv.Load()

	cfg := Config{
		DBPath:              getEnv("VAULT_DB", "vault.db"),
		Port:                getEnv("PORT", "8123"),
		NuvioBaseURL:        getEnv("NUVIO_BASE_URL", "https://api.nuvio.tv"),
		NuvioPublishableKey: getEnv("NUVIO_PUBLISHABLE_KEY", ""),
		SiteBaseURL:         getEnv("SITE_BASE_URL", ""),
		DevAuthBypassToken:  getEnv("DEV_AUTH_BYPASS_TOKEN", ""),
	}

	tmdb, tmdbErr := loadTMDB()
	access, accessErr := loadAccess()
	if err := errors.Join(requireSet(cfg), tmdbErr, accessErr); err != nil {
		return Config{}, err
	}
	cfg.TMDBAPIKey, cfg.PerAccountKeys, cfg.Secret = tmdb.apiKey, tmdb.perAccount, tmdb.secret
	cfg.Access = access
	return cfg, nil
}

// requireSet refuses cfg when a required variable with no safe default is
// empty, naming every one that is.
func requireSet(cfg Config) error {
	var missing []string
	if cfg.NuvioPublishableKey == "" {
		missing = append(missing, "NUVIO_PUBLISHABLE_KEY")
	}
	if cfg.SiteBaseURL == "" {
		missing = append(missing, "SITE_BASE_URL")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required environment variable(s): %v", missing)
	}
	return nil
}

// loadAccess reads UNO_ACCESS and UNO_ALLOWED_EMAILS (allowedEmails, then
// accessFor).
func loadAccess() (Access, error) {
	emails, err := allowedEmails(os.Getenv("UNO_ALLOWED_EMAILS"))
	if err != nil {
		return Access{}, err
	}
	return accessFor(getEnv("UNO_ACCESS", accessOpen), emails)
}

// accessFor is the Access mode and emails ask for. It refuses an unknown
// mode, an allowlist naming no one (which would shut everyone out), and
// emails listed while access is open (which would read as a restriction
// that isn't there).
func accessFor(mode string, emails []string) (Access, error) {
	switch mode {
	case accessOpen:
		if len(emails) > 0 {
			return Access{}, errors.New("UNO_ALLOWED_EMAILS is set but UNO_ACCESS is open; set UNO_ACCESS=allowlist to use it")
		}
		return Access{}, nil
	case accessAllowlist:
		if len(emails) == 0 {
			return Access{}, errors.New("UNO_ACCESS is allowlist but UNO_ALLOWED_EMAILS names no one")
		}
		return Access{Allowlist: true, Emails: emails}, nil
	}
	return Access{}, fmt.Errorf("UNO_ACCESS is %q; want %q or %q", mode, accessOpen, accessAllowlist)
}

// allowedEmails is the comma-separated email addresses in s, trimmed and
// lower-cased, empty items dropped. An entry that isn't an address is
// refused by name (emailProblem) rather than matching nobody.
func allowedEmails(s string) ([]string, error) {
	var emails []string
	for item := range strings.SplitSeq(s, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if err := emailProblem(item); err != nil {
			return nil, err
		}
		emails = append(emails, strings.ToLower(item))
	}
	return emails, nil
}

// emailProblem is why item can't be an allowlist entry: a Nuvio account id,
// which the list doesn't take, or anything else that isn't an address
// (isEmailAddress).
func emailProblem(item string) error {
	if _, err := uuid.Parse(item); err == nil {
		return fmt.Errorf("UNO_ALLOWED_EMAILS lists %q, a Nuvio account id; the list takes email addresses", item)
	}
	if !isEmailAddress(item) {
		return fmt.Errorf("UNO_ALLOWED_EMAILS lists %q, which is not an email address", item)
	}
	return nil
}

// isEmailAddress reports whether s reads as one email address: exactly one
// @, with text on both sides, and no spaces.
func isEmailAddress(s string) bool {
	local, domain, _ := strings.Cut(s, "@")
	return local != "" && domain != "" && !strings.Contains(domain, "@") && !strings.ContainsFunc(s, unicode.IsSpace)
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
