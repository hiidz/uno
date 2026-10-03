package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"slices"
	"strings"

	"github.com/google/uuid"

	"github.com/hiidz/uno/internal/addon"
	"github.com/hiidz/uno/internal/httpx"
	"github.com/hiidz/uno/internal/nuvio"
	"github.com/hiidz/uno/internal/vault"
)

// pushRequest is POST /api/p/{i}/push's body: the full pending home-screen
// selection, carried in one call rather than standalone PUT .../selection
// endpoints — selection is only ever written here, in one transaction,
// after Nuvio has accepted the push. See the "HTTP surface" section of
// docs/architecture.md.
type pushRequest struct {
	Catalogs    vault.CatalogSelectionForm    `json:"catalogs"`
	Collections vault.CollectionSelectionForm `json:"collections"`
}

// pushResult is the always-JSON response, past auth/profile resolution.
// Flat by design — success or failure, not partial-progress flags — since
// push's ordering (validate → Nuvio → local write) and its undo of what Nuvio
// already took guarantee an ordinary failure means nothing changed.
// UndoFailed marks the one case that guarantee doesn't cover: an undo that
// failed too (undoPush). Refused names why push turned the selection away
// before contacting Nuvio, when the SPA has words of its own for it.
type pushResult struct {
	Success     bool   `json:"success"`
	ManifestURL string `json:"manifest_url,omitempty"`
	Error       string `json:"error,omitempty"`
	UndoFailed  bool   `json:"undo_failed,omitempty"`
	Refused     string `json:"refused,omitempty"`
}

// The pushResult.Refused values.
const (
	// refusedEmptyCollection: the selection puts a collection with no
	// folders on Home.
	refusedEmptyCollection = "empty_collection"
	// refusedSharesAddons: the Nuvio profile uses profile 1's addons.
	refusedSharesAddons = "shares_addons"
	// refusedProfileChanged: the profile's Nuvio slot is empty now, or holds
	// another Nuvio profile.
	refusedProfileChanged = "profile_changed"
)

// push serves POST /api/p/{profileIndex}/push: an explicit, user-triggered
// sync of this profile's manifest URL and collections into Nuvio, carrying
// the full pending selection in its body.
//
// Ordering is Nuvio-first, local-write-last: validate, build the push record,
// check the selection and the live Nuvio profile (refusePush), push addons,
// push collections, and only then commit the selection and the record to
// Uno's own vault. This avoids holding a SQLite write transaction open across
// several sequential Nuvio HTTP calls, and with the undo of what Nuvio already
// took (sendPush) means an ordinary failure leaves nothing written on either
// side.
func (s *Server) push(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	profileID, ok := profileIDFrom(ctx)
	if !ok {
		http.Error(w, "missing profile", http.StatusUnauthorized)
		return
	}
	accessToken, ok := nuvioTokenFrom(ctx)
	if !ok {
		http.Error(w, "missing nuvio token", http.StatusUnauthorized)
		return
	}

	profile, err := s.vault.GetProfileByID(ctx, profileID)
	if err != nil {
		log.Printf("push: %v", err)
		http.Error(w, "failed to resolve profile", http.StatusInternalServerError)
		return
	}

	var body pushRequest
	if !decodeStrictJSON(w, r, &body) {
		return
	}

	record, err := s.pushRecord(ctx, profileID, body)
	if err != nil {
		log.Printf("push: %v", err)
		status := http.StatusInternalServerError
		if errors.Is(err, vault.ErrInvalidInput) {
			status = http.StatusBadRequest
		}
		httpx.WriteJSON(w, status, pushResult{Error: "push failed"})
		return
	}

	if s.refusePush(ctx, w, accessToken, profile, record) {
		return
	}
	status, result := s.sendPush(ctx, accessToken, profile, record)
	httpx.WriteJSON(w, status, result)
}

// refusePush answers a push that can't go ahead as it stands, and reports
// whether it did, before anything reaches Nuvio:
//   - a collection on Home with no folders: Nuvio's phone and desktop apps
//     leave one off Home, and Nuvio TV has no guard against one;
//   - a Nuvio profile slot that is empty now, or holds a Nuvio profile other
//     than the one this profile was selected as. Pushing there would leave
//     Uno's addon and collections to whoever takes the slot next; picking
//     the profile again stamps the slot's Nuvio profile afresh;
//   - a Nuvio profile that uses profile 1's addons: no Nuvio app reads its
//     own addon list, so the push would land where nothing shows it.
func (s *Server) refusePush(ctx context.Context, w http.ResponseWriter, accessToken string, profile vault.Profile, record vault.PushRecord) bool {
	if hasEmptyCollection(record) {
		httpx.WriteJSON(w, http.StatusBadRequest, pushResult{Error: "push failed", Refused: refusedEmptyCollection})
		return true
	}
	live, found, err := s.liveProfile(ctx, accessToken, profile)
	switch {
	case err != nil:
		log.Printf("push: reading the live profile: %v", err)
		httpx.WriteJSON(w, nuvioErrorStatus(err), pushResult{Error: "push failed"})
	case !found:
		httpx.WriteJSON(w, http.StatusConflict, pushResult{Error: "push failed", Refused: refusedProfileChanged})
	case live.UsesPrimaryAddons:
		httpx.WriteJSON(w, http.StatusConflict, pushResult{Error: "push failed", Refused: refusedSharesAddons})
	default:
		return false
	}
	return true
}

// hasEmptyCollection reports whether record sends a collection with no
// folders.
func hasEmptyCollection(record vault.PushRecord) bool {
	for _, raw := range record.Collections {
		var c struct {
			Folders []json.RawMessage `json:"folders"`
		}
		if json.Unmarshal(raw, &c) == nil && len(c.Folders) == 0 {
			return true
		}
	}
	return false
}

// liveProfile is the Nuvio profile in profile's slot as Nuvio lists it now,
// read with the caller's token. found is false when the slot is empty or
// holds a Nuvio profile other than the one profile was selected as.
func (s *Server) liveProfile(ctx context.Context, accessToken string, profile vault.Profile) (live nuvio.NuvioProfile, found bool, err error) {
	profiles, err := s.nuvio.ListProfiles(ctx, accessToken)
	if err != nil {
		return nuvio.NuvioProfile{}, false, err
	}
	for _, p := range profiles {
		if p.ProfileIndex == profile.NuvioProfileIndex && p.ID == profile.NuvioProfileUUID {
			return p, true, nil
		}
	}
	return nuvio.NuvioProfile{}, false, nil
}

// sendPush pushes record to profile's Nuvio profile and stores it, and
// answers how that went.
//
// Addons before collections: a pushed collection's catalogSources reference
// this addon's manifest id, so installing the addon first means a client
// that reads collections right after a push already has something to
// resolve those references against. If addons push fails, collections is
// never attempted. A failure after Nuvio took part of the push puts back
// what it took, each as it was pulled (undoPush).
func (s *Server) sendPush(ctx context.Context, accessToken string, profile vault.Profile, record vault.PushRecord) (int, pushResult) {
	slot := profile.NuvioProfileIndex
	manifestURL := s.siteBaseURL + addon.ManifestPath(profile.Token)

	pulledAddons, err := s.pushAddons(ctx, accessToken, slot, manifestURL)
	if err != nil {
		log.Printf("push: addons push failed: %v", err)
		return nuvioErrorStatus(err), pushResult{Error: "push failed"}
	}
	revertAddons := func() error { return s.nuvio.PushAddons(ctx, accessToken, slot, pulledAddons) }

	pulledCollections, err := s.pushCollections(ctx, accessToken, slot, profile.ID, record.Collections)
	if err != nil {
		log.Printf("push: collections push failed, reverting addons: %v", err)
		return undoPush(nuvioErrorStatus(err), revertAddons)
	}
	revertCollections := func() error {
		return s.nuvio.PushCollections(ctx, accessToken, slot, pulledCollections)
	}

	if err := s.vault.SavePush(ctx, profile.ID, record); err != nil {
		log.Printf("push: local commit failed after nuvio succeeded, reverting: %v", err)
		return undoPush(http.StatusInternalServerError, revertCollections, revertAddons)
	}
	return http.StatusOK, pushResult{Success: true, ManifestURL: manifestURL}
}

// undoPush runs every revert of a failed push and answers it with status. A
// revert that fails too — two independent failures back to back — leaves
// Nuvio holding part of the push, which the answer says with a 500 and
// UndoFailed.
func undoPush(status int, reverts ...func() error) (int, pushResult) {
	undone := true
	for _, revert := range reverts {
		if err := revert(); err != nil {
			log.Printf("push: compensating revert also failed: %v", err)
			undone = false
		}
	}
	if !undone {
		return http.StatusInternalServerError, pushResult{Error: "push failed", UndoFailed: true}
	}
	return status, pushResult{Error: "push failed"}
}

// listPendingPush serves GET /api/p/{profileIndex}/push/pending: what a push of
// the Home as Uno stores it would change in Nuvio, the catalogs and
// collections edited or deleted since the last push among them
// (vault.PendingPush). The builder adds its own unpushed Home edits to it.
func (s *Server) listPendingPush(w http.ResponseWriter, r *http.Request) {
	listByProfile(w, r, "listPendingPush", "failed to load pending push", s.vault.PendingPush)
}

// pushRecord checks that every id body names is one profileID may push, then
// builds what the push puts in Nuvio, once: its collections are the bytes
// sent, and the whole record is what the local write stores. The check is
// load-bearing, not a fail-fast nicety: with the write moved to the end, it is
// the only check standing between the request body and a third-party API
// call.
func (s *Server) pushRecord(ctx context.Context, profileID uuid.UUID, body pushRequest) (vault.PushRecord, error) {
	catalogIDs := make([]uuid.UUID, len(body.Catalogs.Catalogs))
	for i, c := range body.Catalogs.Catalogs {
		catalogIDs[i] = c.CatalogID
	}
	if err := s.vault.ValidateSelectionAccess(ctx, profileID, catalogIDs, body.Collections.CollectionIDs()); err != nil {
		return vault.PushRecord{}, fmt.Errorf("validation failed: %w", err)
	}
	return s.vault.BuildPushRecord(ctx, profileID, body.Catalogs, body.Collections)
}

// pushAddons runs the addons read-modify-write cycle: pull the profile's
// current addons, merge Uno's own entry into that list (mergeAddon), and push
// the complete merged list back — omitting any existing addon would delete
// it. Returns the pulled list, which a failed push puts back.
// Has no dependency on the pending selection, so it's unaffected by push's
// Nuvio-first ordering.
func (s *Server) pushAddons(ctx context.Context, accessToken string, nuvioProfileIndex int, manifestURL string) ([]nuvio.NuvioAddon, error) {
	current, err := s.nuvio.ListAddons(ctx, accessToken, nuvioProfileIndex)
	if err != nil {
		return nil, err
	}
	if err := s.nuvio.PushAddons(ctx, accessToken, nuvioProfileIndex, mergeAddon(current, manifestURL, s.siteBaseURL)); err != nil {
		return nil, err
	}
	return current, nil
}

// mergeAddon is current with this profile's addon in it once and switched
// on, every other addon as it was, in order. Entries are matched the way
// Nuvio's apps match addon URLs (addonKey): Nuvio TV saves Uno's URL without
// /manifest.json, and Nuvio's own dedup key is md5(url), so an exact match
// would add Uno a second time.
//   - The first entry for manifestURL stays where it is, with the URL and
//     name it has: a name is the user's own, set in a Nuvio app.
//   - Any further entry for it is dropped, as is one for another Uno addon
//     URL on this site (another token's): its catalogs share this addon's id,
//     which Nuvio apps resolve collection sources by.
//   - With no entry for it, one named addon.Name is appended.
func mergeAddon(current []nuvio.NuvioAddon, manifestURL, siteBaseURL string) []nuvio.NuvioAddon {
	own := addonKey(manifestURL)
	first := slices.IndexFunc(current, func(a nuvio.NuvioAddon) bool { return addonKey(a.URL) == own })
	if first < 0 {
		first = len(current)
		current = append(slices.Clone(current), nuvio.NuvioAddon{URL: manifestURL, Name: addon.Name, SortOrder: first})
	}
	unoPrefix := addonKey(siteBaseURL) + "/u/"
	merged := make([]nuvio.NuvioAddon, 0, len(current))
	for i, a := range current {
		if i == first {
			a.Enabled = true
		} else if strings.HasPrefix(addonKey(a.URL), unoPrefix) {
			continue
		}
		merged = append(merged, a)
	}
	return merged
}

// addonKey is url as Nuvio TV compares addon URLs: trimmed, without trailing
// slashes or a final /manifest.json, case-folded.
func addonKey(url string) string {
	key := strings.ToLower(strings.TrimRight(strings.TrimSpace(url), "/"))
	path, query, hasQuery := strings.Cut(key, "?")
	path = strings.TrimRight(strings.TrimSuffix(strings.TrimRight(path, "/"), "/manifest.json"), "/")
	if hasQuery {
		return path + "?" + query
	}
	return path
}

// pulledCollection is the one field of a pulled Nuvio collection push's
// merge reads: its id, for the managed-set match.
type pulledCollection struct {
	ID string `json:"id"`
}

// pushCollections runs the collections read-modify-write cycle, sending
// fresh: the push record's collections, built from the pending selection in
// the request body (vault.BuildPushRecord), since the local write hasn't
// happened yet at this point in push's sequence (see push above). Nuvio's
// blob is full-replace and holds
// collections Uno knows nothing about (its own native UI, or another
// client), so the merge has to touch only what Uno manages and leave
// everything else byte-for-byte untouched:
//
//  1. Pull the current blob as raw JSON per element — never decoded into a
//     generic map, which would round-trip numbers through float64 and
//     silently corrupt any collection Uno doesn't own.
//  2. Drop every pulled entry that is owned by this profile or was sent by
//     this profile's last push (the push record). With the closed graph,
//     everything selected is owned, so the owned set alone covers what the
//     old selection-union was for. The record covers a collection deleted
//     since the last push, whatever it held. A collection is never dropped
//     for what its folders hold: Nuvio's own collection editors build
//     sources from Uno's catalogs too.
//  3. Append fresh, the profile's pending selection as the record holds it:
//     in Home order, each pinned to the top of home as its selection entry
//     says, as the exact bytes the record keeps.
//
// Returns the pulled blob on success, which a failed push puts back.
func (s *Server) pushCollections(ctx context.Context, accessToken string, nuvioProfileIndex int, profileID uuid.UUID, fresh []json.RawMessage) ([]json.RawMessage, error) {
	pulled, err := s.nuvio.PullCollections(ctx, accessToken, nuvioProfileIndex)
	if err != nil {
		return nil, err
	}

	managed, err := s.managedCollectionIDs(ctx, profileID)
	if err != nil {
		return nil, err
	}
	foreign, err := dropManaged(pulled, managed)
	if err != nil {
		return nil, err
	}
	kept := slices.Concat(foreign, fresh)

	if err := s.nuvio.PushCollections(ctx, accessToken, nuvioProfileIndex, kept); err != nil {
		return nil, err
	}
	return pulled, nil
}

// nuvioErrorStatus classifies a push-stage error into a status code: a
// failed Nuvio call itself is 502 (upstream problem), anything else (a
// vault read failing, JSON parsing a pulled collection) is 500.
func nuvioErrorStatus(err error) int {
	if errors.Is(err, nuvio.ErrNuvioRequestFailed) {
		return http.StatusBadGateway
	}
	return http.StatusInternalServerError
}

// managedCollectionIDs is the id of every collection push owns in Nuvio's blob
// for profileID: the ones the profile owns and the ones its last push sent,
// which includes any deleted since.
func (s *Server) managedCollectionIDs(ctx context.Context, profileID uuid.UUID) (map[string]bool, error) {
	ownedIDs, err := s.vault.GetOwnedCollectionIDs(ctx, profileID)
	if err != nil {
		return nil, fmt.Errorf("loading owned collections: %w", err)
	}
	pushedIDs, err := s.vault.PushedCollectionIDs(ctx, profileID)
	if err != nil {
		return nil, fmt.Errorf("loading pushed collections: %w", err)
	}
	managed := make(map[string]bool, len(ownedIDs)+len(pushedIDs))
	for _, id := range slices.Concat(ownedIDs, pushedIDs) {
		managed[id.String()] = true
	}
	return managed, nil
}

// dropManaged is pulled without every entry whose id is in managed: what push
// leaves in Nuvio's blob untouched.
func dropManaged(pulled []json.RawMessage, managed map[string]bool) ([]json.RawMessage, error) {
	kept := make([]json.RawMessage, 0, len(pulled))
	for _, raw := range pulled {
		var parsed pulledCollection
		if err := json.Unmarshal(raw, &parsed); err != nil {
			return nil, fmt.Errorf("parsing pulled collection: %w", err)
		}
		if !managed[parsed.ID] {
			kept = append(kept, raw)
		}
	}
	return kept, nil
}
