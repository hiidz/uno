package api

import (
	"context"
	"encoding/json"

	"github.com/hiidz/uno/internal/nuvio"
	"github.com/hiidz/uno/internal/provider"
	"github.com/hiidz/uno/internal/tmdbkey"
	"github.com/hiidz/uno/internal/vault"
)

// TokenVerifier is the subset of nuvio.Verifier's method set requireNuvioAuth
// needs to turn a bearer token into trusted claims. Declared here, at the
// consumer, so a test can fake verification without standing up the whole
// Nuvio RPC client.
type TokenVerifier interface {
	Verify(ctx context.Context, token string) (nuvio.Claims, error)
}

// NuvioClient is the subset of nuvio.Client's method set the profiles and
// push handlers need. Declared here, at the consumer, for the same reason
// as TokenVerifier above — doRPC and any method not called from internal/api
// deliberately stay off this interface.
type NuvioClient interface {
	ListProfiles(ctx context.Context, accessToken string) ([]nuvio.NuvioProfile, error)
	ListAddons(ctx context.Context, accessToken string, profileID int) ([]nuvio.NuvioAddon, error)
	PushAddons(ctx context.Context, accessToken string, profileID int, addons []nuvio.NuvioAddon) error
	PullCollections(ctx context.Context, accessToken string, profileID int) ([]json.RawMessage, error)
	PushCollections(ctx context.Context, accessToken string, profileID int, collections []json.RawMessage) error
	PullHomeOrder(ctx context.Context, accessToken string, profileID int) (json.RawMessage, error)
	PushHomeOrder(ctx context.Context, accessToken string, profileID int, settings json.RawMessage) error
	AvatarImages(ctx context.Context, accessToken string) (map[string]string, error)
}

// Compile-time assertions: a signature drift in internal/nuvio becomes a
// build error here, not a surprise at the New() call site in cmd/server.
var (
	_ TokenVerifier = (*nuvio.Client)(nil)
	_ TokenVerifier = (*nuvio.Verifier)(nil)
	_ NuvioClient   = (*nuvio.Client)(nil)
)

// Deps is New's constructor argument: every dependency the Server needs,
// named rather than positional. Verifier and Nuvio are typically the same
// *nuvio.Client value — see cmd/server/main.go for why passing a second,
// independently constructed Verifier would be wrong.
type Deps struct {
	Vault       *vault.DB
	Provider    *provider.TMDBClient
	Verifier    TokenVerifier
	Nuvio       NuvioClient
	SiteBaseURL string
	// NuvioBaseURL is the Nuvio origin the SPA's Content-Security-Policy
	// lets it call, since login and refresh go from the browser straight to
	// Nuvio. It must match the VITE_NUVIO_BASE_URL the SPA was built with.
	NuvioBaseURL string
	// Access says which Nuvio accounts may use the builder API; the zero
	// value admits every account.
	Access Access
	// Keys gives each request its account's own TMDB key on a server in
	// per-account key mode, and is nil on a server with one shared key.
	Keys *tmdbkey.Keys
}
