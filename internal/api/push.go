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
	"time"

	"github.com/google/uuid"

	"github.com/hiidz/uno/internal/addon"
	"github.com/hiidz/uno/internal/httpx"
	"github.com/hiidz/uno/internal/nuvio"
	"github.com/hiidz/uno/internal/vault"
)

// pushRequest is POST /api/p/{i}/push's body: the full pending home screen
// as one ordered list of rows. The Home selection is only ever written here,
// in one transaction, after Nuvio has accepted the push. See the "HTTP
// surface" section of docs/architecture.md.
type pushRequest struct {
	Rows []pushRow `json:"rows"`
}

// pushRow is one row of the pending Home, in Home order: a catalog, with
// whether it gets a home row or is in Discover only, or a collection, with
// whether Nuvio shows it first.
type pushRow struct {
	CatalogID    *uuid.UUID `json:"catalog_id,omitempty"`
	ShowInHome   bool       `json:"show_in_home,omitempty"`
	CollectionID *uuid.UUID `json:"collection_id,omitempty"`
	PinToTop     bool       `json:"pin_to_top,omitempty"`
}

// id is the catalog or collection the row names; check has already refused a
// row that names neither or both.
func (row pushRow) id() uuid.UUID {
	if row.CatalogID != nil {
		return *row.CatalogID
	}
	return *row.CollectionID
}

// check refuses, as vault.ErrInvalidInput, a row naming neither a catalog nor a
// collection, or both, and a row naming one an earlier row already names: a
// home screen holds each row once.
func (body pushRequest) check() error {
	seen := make(map[uuid.UUID]bool, len(body.Rows))
	for i, row := range body.Rows {
		if (row.CatalogID == nil) == (row.CollectionID == nil) {
			return fmt.Errorf("%w: home row %d must name one catalog or one collection", vault.ErrInvalidInput, i)
		}
		if seen[row.id()] {
			return fmt.Errorf("%w: home row %d repeats a row already on the home screen", vault.ErrInvalidInput, i)
		}
		seen[row.id()] = true
	}
	return nil
}

// selection is body as the vault's Home selection, each entry's Position its
// row's place in Rows, or vault.ErrInvalidInput for a body check refuses.
func (body pushRequest) selection() (vault.PushedHome, error) {
	if err := body.check(); err != nil {
		return vault.PushedHome{}, err
	}
	var home vault.PushedHome
	for i, row := range body.Rows {
		if row.CatalogID != nil {
			home.Catalogs = append(home.Catalogs, vault.SelectedCatalogInput{CatalogID: *row.CatalogID, ShowInHome: row.ShowInHome, Position: i})
		} else {
			home.Collections = append(home.Collections, vault.SelectedCollectionInput{CollectionID: *row.CollectionID, PinToTop: row.PinToTop, Position: i})
		}
	}
	return home, nil
}

// pushResult is push's JSON answer once its body has decoded: Success, the
// marker the SPA tells it from any other body by, true only on a 200. Flat by
// design â€” success or failure, not partial-progress flags â€” since
// push's ordering (validate â†’ Nuvio â†’ local write) and its undo of what Nuvio
// already took guarantee an ordinary failure means nothing changed.
// UndoFailed marks the one case that guarantee doesn't cover: an undo that
// failed too (undoPush). Refused names why push turned the selection away
// before contacting Nuvio, when the SPA has words of its own for it.
type pushResult struct {
	Success    bool   `json:"success"`
	UndoFailed bool   `json:"undo_failed,omitempty"`
	Refused    string `json:"refused,omitempty"`
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
	// refusedHomeOrderUnreadable: Nuvio's home-order list for the profile
	// couldn't be read (errHomeOrderUnreadable).
	refusedHomeOrderUnreadable = "home_order_unreadable"
	// refusedTooManyCatalogs: the selection gives Nuvio more than
	// maxPushedCatalogs catalogs.
	refusedTooManyCatalogs = "too_many_catalogs"
)

// maxPushedCatalogs bounds the catalogs one push gives Nuvio: those on Home
// and those its collections' folders use. Only listed catalogs count toward
// a profile's own limit, so a profile's collections can hold thousands, and
// the public addon serves every one the push record holds: the manifest lists
// them all, and each catalog request reads the whole record. This is five
// times the listed-catalog limit, far past any real Home.
const maxPushedCatalogs = 1_000

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
//
// One push runs at a time per profile (lockPush): two interleaved pushes
// would each full-replace Nuvio's lists step by step, and one's undo would
// put back what the other replaced.
func (s *Server) push(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	profile, _ := profileFrom(ctx) // guaranteed by requireProfile

	var body pushRequest
	if !decodeJSON(w, r, &body) {
		return
	}

	unlock, err := s.lockPush(ctx, profile.ID)
	if err != nil {
		log.Printf("push: waiting for the profile's push in flight: %v", err)
		httpx.WriteJSON(w, http.StatusServiceUnavailable, pushResult{})
		return
	}
	defer unlock()
	s.pushLocked(ctx, w, profile, body)
}

// pushLockWait bounds how long a push waits for the same profile's push in
// flight. It, the live-profile read, pushStepsBudget and pushSettleBudget
// together stay under the server's 60s WriteTimeout, so the SPA always hears
// how a push ended.
const pushLockWait = 5 * time.Second

// lockPush takes profileID's push lock, waiting up to pushLockWait for a push
// already in flight, and returns its release.
func (s *Server) lockPush(ctx context.Context, profileID uuid.UUID) (unlock func(), err error) {
	held, _ := s.pushLocks.LoadOrStore(profileID, make(chan struct{}, 1))
	lock := held.(chan struct{})
	wait, cancel := context.WithTimeout(ctx, pushLockWait)
	defer cancel()
	select {
	case lock <- struct{}{}:
		return func() { <-lock }, nil
	case <-wait.Done():
		return nil, wait.Err()
	}
}

// pushLocked is push once it holds the profile's push lock.
func (s *Server) pushLocked(ctx context.Context, w http.ResponseWriter, profile vault.Profile, body pushRequest) {
	accessToken, _ := nuvioTokenFrom(ctx) // guaranteed by requireNuvioAuth

	record, err := s.pushRecord(ctx, profile.ID, body)
	if err != nil {
		log.Printf("push: %v", err)
		status := http.StatusInternalServerError
		if errors.Is(err, vault.ErrInvalidInput) {
			status = http.StatusBadRequest
		}
		httpx.WriteJSON(w, status, pushResult{})
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
//   - more than maxPushedCatalogs catalogs for Nuvio (recordRefusal);
//   - a Nuvio profile slot that is empty now, or holds a Nuvio profile other
//     than the one this profile was selected as. Pushing there would leave
//     Uno's addon and collections to whoever takes the slot next; picking
//     the profile again stamps the slot's Nuvio profile afresh;
//   - a Nuvio profile that uses profile 1's addons: no Nuvio app reads its
//     own addon list, so the push would land where nothing shows it.
func (s *Server) refusePush(ctx context.Context, w http.ResponseWriter, accessToken string, profile vault.Profile, record vault.PushRecord) bool {
	if refused := recordRefusal(record); refused != "" {
		httpx.WriteJSON(w, http.StatusBadRequest, pushResult{Refused: refused})
		return true
	}
	live, found, err := s.liveProfile(ctx, accessToken, profile)
	switch {
	case err != nil:
		log.Printf("push: reading the live profile: %v", err)
		httpx.WriteJSON(w, nuvioErrorStatus(err), pushResult{})
	case !found:
		httpx.WriteJSON(w, http.StatusConflict, pushResult{Refused: refusedProfileChanged})
	case live.UsesPrimaryAddons:
		httpx.WriteJSON(w, http.StatusConflict, pushResult{Refused: refusedSharesAddons})
	default:
		return false
	}
	return true
}

// recordRefusal is why record itself can't be pushed, as a pushResult.Refused
// value, or "" when nothing in it stops the push: a collection with no
// folders, or more than maxPushedCatalogs catalogs for Nuvio.
func recordRefusal(record vault.PushRecord) string {
	switch {
	case hasEmptyCollection(record):
		return refusedEmptyCollection
	case len(record.Catalogs) > maxPushedCatalogs:
		return refusedTooManyCatalogs
	}
	return ""
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
// The steps run in order, each only after the one before succeeded:
//  1. Pull and merge the home-order list, writing nothing: a list push can't
//     read stops the push before Nuvio is touched.
//  2. Addons. A pushed collection's catalogSources reference this addon's
//     manifest id, so installing the addon first means a client that reads
//     collections right after a push already has something to resolve those
//     references against.
//  3. Collections.
//  4. The home-order list, whose rows name the addon's catalogs and the
//     collections the steps before put there.
//
// A failure after Nuvio took part of the push puts back what it took, each
// as it was pulled, newest first (undoPush).
//
// The push runs detached from ctx's cancellation: a client that drops
// mid-push would otherwise cancel the undo and the local commit with it,
// leaving Nuvio holding part of the push. The steps run within
// pushStepsBudget, and the local commit or the undo within its own
// pushSettleBudget, so a step that runs out of time still leaves the undo
// time of its own.
func (s *Server) sendPush(ctx context.Context, accessToken string, profile vault.Profile, record vault.PushRecord) (int, pushResult) {
	ctx = context.WithoutCancel(ctx)
	stepsCtx, cancelSteps := context.WithTimeout(ctx, pushStepsBudget)
	defer cancelSteps()
	run := &pushRun{
		s: s, ctx: stepsCtx, accessToken: accessToken, slot: profile.NuvioProfileIndex, profileID: profile.ID,
		manifestURL: s.siteBaseURL + addon.ManifestPath(profile.Token), record: record,
	}
	var reverts []pushRevert
	for _, step := range []pushStep{run.prepareHomeOrder, run.pushAddons, run.pushCollections, run.pushHomeOrder} {
		revert, err := step()
		if err != nil {
			return failedPush(ctx, err, reverts...)
		}
		reverts = slices.Insert(reverts, 0, revert)
	}

	settleCtx, cancelSettle := context.WithTimeout(ctx, pushSettleBudget)
	defer cancelSettle()
	if err := s.vault.SavePush(settleCtx, profile.ID, record); err != nil {
		log.Printf("push: local commit failed after nuvio succeeded, reverting: %v", err)
		return undoPush(settleCtx, http.StatusInternalServerError, reverts...)
	}
	return http.StatusOK, pushResult{Success: true}
}

// pushStepsBudget bounds a push's steps against Nuvio; pushSettleBudget bounds
// what follows them, the local commit or the undo (see pushLockWait).
const (
	pushStepsBudget  = 25 * time.Second
	pushSettleBudget = 15 * time.Second
)

// pushRevert puts back what one step of a push wrote to Nuvio, as it was
// pulled.
type pushRevert func(ctx context.Context) error

// pushStep is one step of a push: it returns how to put back what it wrote
// to Nuvio, which a later step's failure runs.
type pushStep func() (revert pushRevert, err error)

// pushRun is one push as its steps run: what they write, and what the home
// order step read and merged before the others wrote anything.
type pushRun struct {
	s           *Server
	ctx         context.Context
	accessToken string
	slot        int
	profileID   uuid.UUID
	manifestURL string
	record      vault.PushRecord

	// managed is the collections push owns in Nuvio (managedCollectionIDs),
	// read once for both the home-order and the collections merge.
	managed         map[string]bool
	pulledHomeOrder json.RawMessage
	homeOrder       json.RawMessage
}

// prepareHomeOrder pulls the profile's home-order list, reads the collections
// push manages, and merges the push's home rows into the list (mergeHomeOrder),
// writing nothing.
func (r *pushRun) prepareHomeOrder() (pushRevert, error) {
	pulled, err := r.s.nuvio.PullHomeOrder(r.ctx, r.accessToken, r.slot)
	if err != nil {
		return nil, fmt.Errorf("pulling home order: %w", err)
	}
	r.managed, err = r.s.managedCollectionIDs(r.ctx, r.profileID)
	if err != nil {
		return nil, err
	}
	pinned, rows := r.record.HomeRows()
	r.homeOrder, err = mergeHomeOrder(pulled, pinned, rows, r.managed)
	r.pulledHomeOrder = pulled
	return nothingToRevert, err
}

// nothingToRevert is the revert of a step that wrote nothing.
func nothingToRevert(context.Context) error { return nil }

// pushAddons is the addons step (Server.pushAddons).
func (r *pushRun) pushAddons() (pushRevert, error) {
	pulled, err := r.s.pushAddons(r.ctx, r.accessToken, r.slot, r.manifestURL)
	if err != nil {
		return nil, fmt.Errorf("addons push: %w", err)
	}
	return func(ctx context.Context) error { return r.s.nuvio.PushAddons(ctx, r.accessToken, r.slot, pulled) }, nil
}

// pushCollections is the collections step (Server.pushCollections).
func (r *pushRun) pushCollections() (pushRevert, error) {
	pulled, err := r.s.pushCollections(r.ctx, r.accessToken, r.slot, r.managed, r.record.Collections)
	if err != nil {
		return nil, fmt.Errorf("collections push: %w", err)
	}
	return func(ctx context.Context) error {
		return r.s.nuvio.PushCollections(ctx, r.accessToken, r.slot, pulled)
	}, nil
}

// pushHomeOrder is the home-order step: the list prepareHomeOrder built. Its
// revert pushes the list as it was pulled, or {} when there was none, which
// every Nuvio app reads as no saved order.
func (r *pushRun) pushHomeOrder() (pushRevert, error) {
	if err := r.s.nuvio.PushHomeOrder(r.ctx, r.accessToken, r.slot, r.homeOrder); err != nil {
		return nil, fmt.Errorf("home order push: %w", err)
	}
	return func(ctx context.Context) error {
		return r.s.nuvio.PushHomeOrder(ctx, r.accessToken, r.slot, r.pulledHomeOrder)
	}, nil
}

// failedPush answers a push whose step failed with err, after running
// reverts, what the steps before it wrote, within pushSettleBudget of ctx. A
// home-order list push couldn't read is a refusal the SPA has words for: it
// fails the first step, before anything reached Nuvio.
func failedPush(ctx context.Context, err error, reverts ...pushRevert) (int, pushResult) {
	log.Printf("push: %v", err)
	settleCtx, cancel := context.WithTimeout(ctx, pushSettleBudget)
	defer cancel()
	status, result := undoPush(settleCtx, nuvioErrorStatus(err), reverts...)
	if errors.Is(err, errHomeOrderUnreadable) {
		result.Refused = refusedHomeOrderUnreadable
	}
	return status, result
}

// undoPush runs every revert of a failed push and answers it with status. A
// revert that fails too â€” two independent failures back to back â€” leaves
// Nuvio holding part of the push, which the answer says with a 500 and
// UndoFailed.
func undoPush(ctx context.Context, status int, reverts ...pushRevert) (int, pushResult) {
	undone := true
	for _, revert := range reverts {
		if err := revert(ctx); err != nil {
			log.Printf("push: compensating revert also failed: %v", err)
			undone = false
		}
	}
	if !undone {
		return http.StatusInternalServerError, pushResult{UndoFailed: true}
	}
	return status, pushResult{}
}

// pushRecord builds what a push of body puts in Nuvio, once: its collections
// are the bytes sent, and the whole record is what the local write stores. The
// build refuses an id profileID may not put on Home (vault.ErrInvalidInput),
// which is load-bearing, not a fail-fast nicety: with the write moved to the
// end, it is the only check standing between the request body and a
// third-party API call.
func (s *Server) pushRecord(ctx context.Context, profileID uuid.UUID, body pushRequest) (vault.PushRecord, error) {
	home, err := body.selection()
	if err != nil {
		return vault.PushRecord{}, err
	}
	return s.vault.BuildPushRecord(ctx, profileID, home)
}

// pushAddons runs the addons read-modify-write cycle: pull the profile's
// current addons, merge Uno's own entry into that list (mergeAddon), and push
// the complete merged list back â€” omitting any existing addon would delete
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
//  1. Pull the current blob as raw JSON per element â€” never decoded into a
//     generic map, which would round-trip numbers through float64 and
//     silently corrupt any collection Uno doesn't own.
//  2. Drop every pulled entry in managed: owned by this profile, or sent by
//     its last push (managedCollectionIDs). Everything on Home is owned, so
//     the owned set covers it; the push record covers a collection deleted
//     since the last push, whatever it held. A collection is never dropped
//     for what its folders hold: Nuvio's own collection editors build
//     sources from Uno's catalogs too.
//  3. Append fresh, the profile's pending selection as the record holds it:
//     in Home order, each pinned to the top of home as its selection entry
//     says, as the exact bytes the record keeps.
//
// Returns the pulled blob on success, which a failed push puts back.
func (s *Server) pushCollections(ctx context.Context, accessToken string, nuvioProfileIndex int, managed map[string]bool, fresh []json.RawMessage) ([]json.RawMessage, error) {
	pulled, err := s.nuvio.PullCollections(ctx, accessToken, nuvioProfileIndex)
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
