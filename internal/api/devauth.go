package api

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/hiidz/uno/internal/nuvio"
)

// DevBypassSub is the fixed Nuvio account ID a dev-bypass token authenticates
// as. It has no meaning to Nuvio itself — it only needs to be a stable key
// for the local vault's (nuvio_user_id, profile_index) rows.
const DevBypassSub = "dev-user"

// devBypassProfileIndex is the primary profile index the dev-bypass fake
// account exposes, matching requireProfile's {profileIndex}=1..6 range.
const devBypassProfileIndex = 1

// devBypassSecondProfileIndex is a second, separately-stored profile so
// local testing can exercise switching between profiles without a real
// Nuvio account.
const devBypassSecondProfileIndex = 2

// devBypassVerifier wraps a real TokenVerifier so that presenting
// bypassToken as a bearer token authenticates as DevBypassSub without a
// real Nuvio-issued JWT — no JWKS fetch, no live Nuvio account needed. Any
// other token is verified normally by next. Intended only for local
// development; never wire this into a deployed server.
type devBypassVerifier struct {
	next  TokenVerifier
	token string
}

// NewDevBypassVerifier returns a TokenVerifier that authenticates
// bypassToken as DevBypassSub and forwards every other token to next.
func NewDevBypassVerifier(next TokenVerifier, bypassToken string) TokenVerifier {
	return &devBypassVerifier{next: next, token: bypassToken}
}

func (v *devBypassVerifier) Verify(ctx context.Context, token string) (nuvio.Claims, error) {
	if token == v.token {
		return nuvio.Claims{Sub: DevBypassSub, Exp: time.Now().Add(24 * time.Hour)}, nil
	}
	return v.next.Verify(ctx, token)
}

// devBypassNuvio wraps a NuvioClient so requests carrying the dev-bypass
// token are served from an in-memory fake profile/addon/collection store
// instead of hitting Nuvio's real API — the bypass token never leaves the
// process. Any other token passes through to next untouched. Without this,
// the bypassed identity from devBypassVerifier would still fail every
// handler that calls out to Nuvio (listProfiles, push, ...), since those
// calls forward the bearer token to Nuvio's real API.
type devBypassNuvio struct {
	next     NuvioClient
	token    string
	profiles []nuvio.NuvioProfile

	mu          sync.Mutex
	addons      map[string][]nuvio.NuvioAddon
	collections map[string][]json.RawMessage
}

// NewDevBypassNuvio returns a NuvioClient that serves requests carrying
// bypassToken from an in-memory fake account (two profiles, at
// devBypassProfileIndex and devBypassSecondProfileIndex, so profile
// switching is testable locally) and forwards every other token to next.
func NewDevBypassNuvio(next NuvioClient, bypassToken string) NuvioClient {
	return &devBypassNuvio{
		next:  next,
		token: bypassToken,
		profiles: []nuvio.NuvioProfile{
			{ID: "dev-profile", UserID: DevBypassSub, ProfileIndex: devBypassProfileIndex, Name: "Dev"},
			{ID: "dev-profile-2", UserID: DevBypassSub, ProfileIndex: devBypassSecondProfileIndex, Name: "Dev 2"},
		},
		addons:      map[string][]nuvio.NuvioAddon{},
		collections: map[string][]json.RawMessage{},
	}
}

// profileKey maps a profileID (the {profileIndex}-th caller passes
// nuvio profile "id" strings; here just the path param converted upstream)
// back to one of the two fake profiles' IDs. Both PullCollections and
// PushCollections/PushAddons receive it as an int elsewhere in the real
// Nuvio API, but the bypass only ever has these two rows, keyed by index.
func (v *devBypassNuvio) profileKey(profileID int) string {
	for _, p := range v.profiles {
		if p.ProfileIndex == profileID {
			return p.ID
		}
	}
	return v.profiles[0].ID
}

func (v *devBypassNuvio) ListProfiles(ctx context.Context, accessToken string) ([]nuvio.NuvioProfile, error) {
	if accessToken == v.token {
		return append([]nuvio.NuvioProfile(nil), v.profiles...), nil
	}
	return v.next.ListProfiles(ctx, accessToken)
}

func (v *devBypassNuvio) ListAddons(ctx context.Context, accessToken string, profileID int) ([]nuvio.NuvioAddon, error) {
	if accessToken == v.token {
		v.mu.Lock()
		defer v.mu.Unlock()
		return append([]nuvio.NuvioAddon(nil), v.addons[v.profileKey(profileID)]...), nil
	}
	return v.next.ListAddons(ctx, accessToken, profileID)
}

func (v *devBypassNuvio) PushAddons(ctx context.Context, accessToken string, profileID int, addons []nuvio.NuvioAddon) error {
	if accessToken == v.token {
		v.mu.Lock()
		defer v.mu.Unlock()
		// Copied, not aliased: the caller still owns addons.
		v.addons[v.profileKey(profileID)] = append([]nuvio.NuvioAddon(nil), addons...)
		return nil
	}
	return v.next.PushAddons(ctx, accessToken, profileID, addons)
}

func (v *devBypassNuvio) PullCollections(ctx context.Context, accessToken string, profileID int) ([]json.RawMessage, error) {
	if accessToken == v.token {
		v.mu.Lock()
		defer v.mu.Unlock()
		return append([]json.RawMessage(nil), v.collections[v.profileKey(profileID)]...), nil
	}
	return v.next.PullCollections(ctx, accessToken, profileID)
}

func (v *devBypassNuvio) PushCollections(ctx context.Context, accessToken string, profileID int, collections []json.RawMessage) error {
	if accessToken == v.token {
		v.mu.Lock()
		defer v.mu.Unlock()
		v.collections[v.profileKey(profileID)] = append([]json.RawMessage(nil), collections...)
		return nil
	}
	return v.next.PushCollections(ctx, accessToken, profileID, collections)
}

// LogDevBypassEnabled logs a loud, impossible-to-miss warning that auth is
// bypassed for a fixed token — called once at startup when the bypass is
// configured, so it can never silently end up active in a deployed server.
func LogDevBypassEnabled() {
	log.Printf("=========================================================================")
	log.Printf("  DEV AUTH BYPASS ENABLED — Authorization: Bearer <DEV_AUTH_BYPASS_TOKEN>")
	log.Printf("  authenticates as a fake account (sub=%q). Do not set this in prod.", DevBypassSub)
	log.Printf("=========================================================================")
}
