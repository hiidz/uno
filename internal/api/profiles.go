package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/hiidz/uno/internal/addon"
	"github.com/hiidz/uno/internal/httpx"
	"github.com/hiidz/uno/internal/nuvio"
	"github.com/hiidz/uno/internal/vault"
)

// errProfileIndexNotOnAccount indicates the requested profile_index has no
// matching profile in the caller's live Nuvio account.
var errProfileIndexNotOnAccount = errors.New("profile index not found on this account")

// resolveSelectedProfile matches profileIndex against the caller's live
// Nuvio profile list, then resolves (or creates) the corresponding Uno
// profile. Kept in api, not nuvio: it composes nuvio and vault, and api is
// the only package that depends on both.
func (s *Server) resolveSelectedProfile(ctx context.Context, sub, token string, profileIndex int) (vault.Profile, error) {
	profiles, err := s.nuvio.ListProfiles(ctx, token)
	if err != nil {
		return vault.Profile{}, err
	}

	var matched *nuvio.NuvioProfile
	for i := range profiles {
		if profiles[i].ProfileIndex == profileIndex {
			matched = &profiles[i]
			break
		}
	}
	if matched == nil {
		return vault.Profile{}, errProfileIndexNotOnAccount
	}

	return s.vault.ResolveOrCreateProfile(ctx, sub, matched.ProfileIndex, matched.ID)
}

func (s *Server) listProfiles(w http.ResponseWriter, r *http.Request) {
	token, ok := nuvioTokenFrom(r.Context())
	if !ok {
		http.Error(w, "missing nuvio token", http.StatusUnauthorized)
		return
	}

	profiles, err := s.nuvio.ListProfiles(r.Context(), token)
	if err != nil {
		writeNuvioError(w, err, "failed to list profiles")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, profiles)
}

func (s *Server) selectProfile(w http.ResponseWriter, r *http.Request) {
	sub, ok := nuvioUserIDFrom(r.Context())
	if !ok {
		http.Error(w, "missing nuvio user id", http.StatusUnauthorized)
		return
	}
	token, ok := nuvioTokenFrom(r.Context())
	if !ok {
		http.Error(w, "missing nuvio token", http.StatusUnauthorized)
		return
	}

	var body struct {
		ProfileIndex int `json:"profile_index"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}

	profile, err := s.resolveSelectedProfile(r.Context(), sub, token, body.ProfileIndex)
	if err != nil {
		if errors.Is(err, errProfileIndexNotOnAccount) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeNuvioError(w, err, "failed to resolve profile")
		return
	}

	// manifest_url is computable at selection time — it's just the site's
	// base URL plus this profile's token — so it's returned here rather
	// than waiting on a first push.
	httpx.WriteJSON(w, http.StatusOK, struct {
		vault.Profile
		ManifestURL string `json:"manifest_url"`
	}{profile, s.siteBaseURL + addon.ManifestPath(profile.Token)})
}
