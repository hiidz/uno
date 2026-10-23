// Package config loads runtime configuration from the environment (and an
// optional .env file), applying defaults and failing fast on missing
// required values.
package config

import (
	"errors"
	"fmt"
	"os"
	"slices"
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
	// Community is UNO_COMMUNITY: CommunityOn, CommunityTrustedImages,
	// CommunityNoImages or CommunityOff.
	Community string
	// TrustedSites is the sites Community shares pictures from on a
	// CommunityTrustedImages server, each once, lower-cased: a site name, or
	// "*." and a name. Nil on every other setting.
	TrustedSites []string
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

// The values UNO_COMMUNITY takes: Community as it is, Community leaving out
// the image and video addresses of sites not on TrustedSites, Community
// leaving out every one, or no Community at all.
const (
	CommunityOn            = "on"
	CommunityTrustedImages = "trusted-images"
	CommunityNoImages      = "no-images"
	CommunityOff           = "off"
)

// builtInTrustedSites is the list UNO_TRUSTED_HOSTS stands for when unset,
// and "default" in it. A site may go on only when all three hold:
//  1. Policed: a film database or a big service with upload rules and a way
//     to report a picture, or a site nobody can upload to because the
//     address decides the picture. Not one person's site or a small file host.
//  2. Not the publisher's site: whoever put a picture there can't see who
//     loads it.
//  3. Stays put: its addresses can't send the viewer on to a site someone
//     else picks. Moving on to the service's own storage is fine.
//
// Passing makes a site allowed, not listed: popularity alone never puts one
// on. Each site here also loads with okhttp's default user-agent, which
// Nuvio's picture loader sends.
var builtInTrustedSites = []string{
	"image.tmdb.org", "media.themoviedb.org", // TMDB
	"assets.fanart.tv",
	"artworks.thetvdb.com",
	"flagcdn.com",
	"placehold.co",
	"raw.githubusercontent.com", "github.com", // GitHub
}

// defaultSites is the UNO_TRUSTED_HOSTS entry that stands for
// builtInTrustedSites.
const defaultSites = "default"

// DBPath reads .env (if present) and returns VAULT_DB alone, for a command
// that needs the vault and none of the server's other settings.
func DBPath() string {
	_ = godotenv.Load()
	return getEnv(envDBPath, defaultDBPath)
}

// VAULT_DB and the path it defaults to.
const (
	envDBPath     = "VAULT_DB"
	defaultDBPath = "vault.db"
)

// Load reads .env (if present) and returns the resolved config, applying
// defaults for any variable that isn't set. It returns one error naming
// every required variable that is missing and every value it can't use.
func Load() (Config, error) {
	_ = godotenv.Load()

	cfg := Config{
		DBPath:              getEnv(envDBPath, defaultDBPath),
		Port:                getEnv("PORT", "8123"),
		NuvioBaseURL:        getEnv("NUVIO_BASE_URL", "https://api.nuvio.tv"),
		NuvioPublishableKey: getEnv("NUVIO_PUBLISHABLE_KEY", ""),
		SiteBaseURL:         getEnv("SITE_BASE_URL", ""),
		DevAuthBypassToken:  getEnv("DEV_AUTH_BYPASS_TOKEN", ""),
	}

	tmdb, tmdbErr := loadTMDB()
	access, accessErr := loadAccess()
	community, sites, communityErr := loadCommunity()
	if err := errors.Join(requireSet(cfg), devBypassProblem(cfg.DevAuthBypassToken), tmdbErr, accessErr, communityErr); err != nil {
		return Config{}, err
	}
	cfg.TMDBAPIKey, cfg.PerAccountKeys, cfg.Secret = tmdb.apiKey, tmdb.perAccount, tmdb.secret
	cfg.Access = access
	cfg.Community, cfg.TrustedSites = community, sites
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

// minDevBypassTokenLen is the shortest DEV_AUTH_BYPASS_TOKEN Load takes. The
// token signs anyone who presents it in as the bypass's fake account, which
// the allowlist admits, so one short enough to guess is a way in for anyone
// who can reach the server.
const minDevBypassTokenLen = 32

// devBypassProblem refuses a DEV_AUTH_BYPASS_TOKEN that is set and shorter
// than minDevBypassTokenLen.
func devBypassProblem(token string) error {
	if token != "" && len(token) < minDevBypassTokenLen {
		return fmt.Errorf("DEV_AUTH_BYPASS_TOKEN is shorter than %d characters; use a random one (openssl rand -hex 16) or leave it empty", minDevBypassTokenLen)
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

// loadCommunity reads UNO_COMMUNITY and UNO_TRUSTED_HOSTS (trustedSites,
// then communityFor).
func loadCommunity() (string, []string, error) {
	sites, err := trustedSites(os.Getenv("UNO_TRUSTED_HOSTS"))
	if err != nil {
		return "", nil, err
	}
	return communityFor(getEnv("UNO_COMMUNITY", CommunityOn), sites)
}

// communityFor is the Community setting mode and sites, UNO_TRUSTED_HOSTS's
// entries or nil when it is unset, ask for: on CommunityTrustedImages, sites
// or builtInTrustedSites, sorted. It refuses an unknown mode, and sites set
// on any other mode, which would read as a restriction that isn't there.
func communityFor(mode string, sites []string) (string, []string, error) {
	switch mode {
	case CommunityTrustedImages:
		if sites == nil {
			sites = slices.Sorted(slices.Values(builtInTrustedSites))
		}
		return mode, sites, nil
	case CommunityOn, CommunityNoImages, CommunityOff:
		if sites != nil {
			return "", nil, fmt.Errorf("UNO_TRUSTED_HOSTS is set but UNO_COMMUNITY is %s; set UNO_COMMUNITY=%s to use it", mode, CommunityTrustedImages)
		}
		return mode, nil, nil
	}
	return "", nil, fmt.Errorf("UNO_COMMUNITY is %q; want %q, %q, %q or %q", mode, CommunityOn, CommunityTrustedImages, CommunityNoImages, CommunityOff)
}

// trustedSites is the comma-separated sites in s, trimmed, lower-cased,
// without a trailing dot, sorted and each once, with "default" standing for
// builtInTrustedSites; nil when s is empty. An entry that isn't a site
// (siteProblem), or an s with no entry, is refused rather than matching
// nothing.
func trustedSites(s string) ([]string, error) {
	if s == "" {
		return nil, nil
	}
	sites := []string{}
	for item := range strings.SplitSeq(s, ",") {
		item = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(item)), ".")
		switch item {
		case "":
			continue
		case defaultSites:
			sites = append(sites, builtInTrustedSites...)
		default:
			if err := siteProblem(item); err != nil {
				return nil, err
			}
			sites = append(sites, item)
		}
	}
	if len(sites) == 0 {
		return nil, errors.New("UNO_TRUSTED_HOSTS is set but names no site")
	}
	slices.Sort(sites)
	return slices.Compact(sites), nil
}

// siteProblem is why item, lower-cased, can't be a UNO_TRUSTED_HOSTS entry:
// anything but a site name or "*." and a name, such as an address with a
// scheme, path or port, or a bare "*".
func siteProblem(item string) error {
	if !isSiteName(strings.TrimPrefix(item, "*.")) {
		return fmt.Errorf("UNO_TRUSTED_HOSTS lists %q; an entry is a site name, like image.tmdb.org, or *. and a name", item)
	}
	return nil
}

// isSiteName reports whether s is a site name: dot-separated labels of
// letters, digits and hyphens, none empty.
func isSiteName(s string) bool {
	for label := range strings.SplitSeq(s, ".") {
		if label == "" || strings.ContainsFunc(label, func(r rune) bool {
			return (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-'
		}) {
			return false
		}
	}
	return true
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
