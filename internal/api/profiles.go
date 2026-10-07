package api

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"slices"

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
	// The vault row is keyed on sub (the verified JWT subject) but carries
	// matched.ID from Nuvio's response. The RPC is called with this caller's
	// own token and Nuvio scopes it, so the two agree — this refuses to bind
	// them if that ever stops being true, rather than filing another
	// account's profile under this one. Answered the same way a missing
	// index is, so the response says nothing about the other account.
	if matched.UserID != sub {
		return vault.Profile{}, fmt.Errorf("%w: profile index %d does not belong to the authenticated account", errProfileIndexNotOnAccount, profileIndex)
	}

	return s.vault.ResolveOrCreateProfile(ctx, sub, matched.ProfileIndex, matched.ID)
}

func (s *Server) listProfiles(w http.ResponseWriter, r *http.Request) {
	token, _ := nuvioTokenFrom(r.Context()) // guaranteed by requireNuvioAuth

	profiles, err := s.nuvio.ListProfiles(r.Context(), token)
	if err != nil {
		writeNuvioError(w, err, "failed to list profiles")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, s.withAvatarImages(r.Context(), token, profiles))
}

// pickerProfile is one profile as GET /api/profiles answers it: Nuvio's row,
// plus the picture Nuvio's apps show for it, "" when they show its colour.
type pickerProfile struct {
	nuvio.NuvioProfile
	AvatarImageURL string `json:"avatar_image_url"`
}

// withAvatarImages is profiles with the picture each shows: its own upload,
// else its built-in avatar's image.
func (s *Server) withAvatarImages(ctx context.Context, token string, profiles []nuvio.NuvioProfile) []pickerProfile {
	images := s.avatarImages(ctx, token, profiles)
	out := make([]pickerProfile, len(profiles))
	for i, p := range profiles {
		out[i] = pickerProfile{NuvioProfile: p, AvatarImageURL: cmp.Or(p.AvatarURL, images[p.AvatarID])}
	}
	return out
}

// avatarImages is Nuvio's built-in avatar images by id, asked for only when
// a profile uses one. A failure to list them is logged and leaves every
// profile its colour: the picker still works without them.
func (s *Server) avatarImages(ctx context.Context, token string, profiles []nuvio.NuvioProfile) map[string]string {
	if !slices.ContainsFunc(profiles, func(p nuvio.NuvioProfile) bool { return p.AvatarID != "" }) {
		return nil
	}
	images, err := s.nuvio.AvatarImages(ctx, token)
	if err != nil {
		log.Printf("listProfiles: avatar images: %v", err)
		return nil
	}
	return images
}

// selectedProfile is POST /api/profiles/select's answer.
type selectedProfile struct {
	ManifestURL string `json:"manifest_url"`
}

func (s *Server) selectProfile(w http.ResponseWriter, r *http.Request) {
	sub, _ := nuvioUserIDFrom(r.Context())  // guaranteed by requireNuvioAuth
	token, _ := nuvioTokenFrom(r.Context()) // guaranteed by requireNuvioAuth

	var body struct {
		ProfileIndex int `json:"profile_index"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}

	profile, err := s.resolveSelectedProfile(r.Context(), sub, token, body.ProfileIndex)
	if err != nil {
		if errors.Is(err, errProfileIndexNotOnAccount) {
			httpx.WriteError(w, http.StatusBadRequest, errProfileIndexNotOnAccount.Error())
			return
		}
		writeNuvioError(w, err, "failed to resolve profile")
		return
	}

	// manifest_url is computable at selection time — it's just the site's
	// base URL plus this profile's token — so it's returned here rather
	// than waiting on a first push. It is all the builder is told of the
	// profile.
	httpx.WriteJSON(w, http.StatusOK, selectedProfile{ManifestURL: s.siteBaseURL + addon.ManifestPath(profile.Token)})
}
