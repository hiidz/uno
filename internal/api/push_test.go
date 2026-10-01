package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/google/uuid"

	"github.com/hiidz/uno/internal/addon"
	"github.com/hiidz/uno/internal/nuvio"
	"github.com/hiidz/uno/internal/vault"
)

func newTestVaultDB(t *testing.T) *vault.DB {
	t.Helper()
	db, err := vault.InitDB(filepath.Join(t.TempDir(), "vault.db"))
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// fakeNuvio is a NuvioClient test double: each Nuvio call it records, and
// each failure-prone call can be scripted to fail (or, for PushCollections,
// to fail on a specific invocation — the initial push versus a compensating
// revert are both routed through the same method).
type fakeNuvio struct {
	profiles        []nuvio.NuvioProfile
	listProfilesErr error

	// addons is the profile's addon list as Nuvio holds it before a push.
	addons        []nuvio.NuvioAddon
	listAddonsErr error

	pushAddonsErr   error
	pushAddonsCalls [][]nuvio.NuvioAddon

	pullCollections    []json.RawMessage
	pullCollectionsErr error

	// pushCollectionsErrs supplies a per-call error override, indexed by
	// call number (0 = the push itself, 1 = a compensating revert, if one
	// happens). A missing index means that call succeeds.
	pushCollectionsErrs  []error
	pushCollectionsCalls [][]json.RawMessage
	// onPushCollections, if set, runs before pushCollectionsErrs is
	// consulted — lets a test mutate the vault mid-push to simulate state
	// changing underneath a push already in flight.
	onPushCollections func(call int, collections []json.RawMessage)
}

var _ NuvioClient = (*fakeNuvio)(nil)

func (f *fakeNuvio) ListProfiles(ctx context.Context, accessToken string) ([]nuvio.NuvioProfile, error) {
	if f.listProfilesErr != nil {
		return nil, f.listProfilesErr
	}
	return f.profiles, nil
}

func (f *fakeNuvio) ListAddons(ctx context.Context, accessToken string, profileID int) ([]nuvio.NuvioAddon, error) {
	if f.listAddonsErr != nil {
		return nil, f.listAddonsErr
	}
	return f.addons, nil
}

func (f *fakeNuvio) PushAddons(ctx context.Context, accessToken string, profileID int, addons []nuvio.NuvioAddon) error {
	f.pushAddonsCalls = append(f.pushAddonsCalls, addons)
	return f.pushAddonsErr
}

func (f *fakeNuvio) PullCollections(ctx context.Context, accessToken string, profileID int) ([]json.RawMessage, error) {
	if f.pullCollectionsErr != nil {
		return nil, f.pullCollectionsErr
	}
	return f.pullCollections, nil
}

func (f *fakeNuvio) PushCollections(ctx context.Context, accessToken string, profileID int, collections []json.RawMessage) error {
	call := len(f.pushCollectionsCalls)
	f.pushCollectionsCalls = append(f.pushCollectionsCalls, collections)
	if f.onPushCollections != nil {
		f.onPushCollections(call, collections)
	}
	if call < len(f.pushCollectionsErrs) {
		return f.pushCollectionsErrs[call]
	}
	return nil
}

func newPushRequest(t *testing.T, ctx context.Context, body pushRequest) *http.Request {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshaling request body: %v", err)
	}
	return httptest.NewRequest(http.MethodPost, "/api/p/1/push", bytes.NewReader(raw)).WithContext(ctx)
}

func decodePushResult(t *testing.T, w *httptest.ResponseRecorder) pushResult {
	t.Helper()
	var result pushResult
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decoding push response: %v", err)
	}
	return result
}

// A Nuvio outage on the very first RPC push makes (ListAddons, the read
// half of pushAddons' read-modify-write) must fail the whole push before
// anything else is attempted or written.
func TestPush_NuvioUnreachable(t *testing.T) {
	db := newTestVaultDB(t)
	ctx := context.Background()

	profile, err := db.ResolveOrCreateProfile(ctx, "user-unreachable", 1, "nuvio-uuid-unreachable")
	if err != nil {
		t.Fatalf("creating profile: %v", err)
	}

	fake := &fakeNuvio{listAddonsErr: fmt.Errorf("%w: dial tcp: connection refused", nuvio.ErrNuvioRequestFailed)}
	s := &Server{vault: db, nuvio: fake, siteBaseURL: "http://example.com"}

	reqCtx := withNuvioToken(withProfileID(ctx, profile.ID), "token")
	req := newPushRequest(t, reqCtx, pushRequest{})
	w := httptest.NewRecorder()

	s.push(w, req)

	if w.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadGateway)
	}
	if result := decodePushResult(t, w); result.Success {
		t.Fatalf("Success = true, want false")
	}
	if len(fake.pushAddonsCalls) != 0 {
		t.Fatalf("PushAddons was called despite ListAddons failing")
	}
	if len(fake.pushCollectionsCalls) != 0 {
		t.Fatalf("collections push was attempted despite addons push never completing")
	}

	sel, err := db.GetCurrentCollectionSelection(ctx, profile.ID)
	if err != nil {
		t.Fatalf("GetCurrentCollectionSelection: %v", err)
	}
	if len(sel) != 0 {
		t.Fatalf("collection selection was written despite Nuvio being unreachable: %v", sel)
	}
}

// Either half of the two-push sequence being rejected must fail the whole
// push, and (per the addons-before-collections ordering) a rejected addons
// push must stop before collections is ever attempted.
func TestPush_OnePushRejected(t *testing.T) {
	rejected := fmt.Errorf("%w: status 400", nuvio.ErrNuvioRequestFailed)

	for _, tc := range []struct {
		name                 string
		fake                 *fakeNuvio
		wantAddonsCalls      int
		wantCollectionsCalls int
	}{
		{
			name:                 "addons push rejected",
			fake:                 &fakeNuvio{pushAddonsErr: rejected},
			wantAddonsCalls:      1,
			wantCollectionsCalls: 0,
		},
		{
			// No compensating action exists for this case: pushAddons
			// discards the addon list it read before merging in Uno's own
			// entry, so there is nothing to revert Nuvio's already-accepted
			// addons push to. The manifest addon stays installed in Nuvio
			// even though this push reports failure and writes nothing
			// locally.
			name:                 "collections push rejected",
			fake:                 &fakeNuvio{pushCollectionsErrs: []error{rejected}},
			wantAddonsCalls:      1,
			wantCollectionsCalls: 1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := newTestVaultDB(t)
			ctx := context.Background()

			profile, err := db.ResolveOrCreateProfile(ctx, "user-"+tc.name, 1, "nuvio-uuid-"+tc.name)
			if err != nil {
				t.Fatalf("creating profile: %v", err)
			}

			s := &Server{vault: db, nuvio: tc.fake, siteBaseURL: "http://example.com"}

			reqCtx := withNuvioToken(withProfileID(ctx, profile.ID), "token")
			req := newPushRequest(t, reqCtx, pushRequest{})
			w := httptest.NewRecorder()

			s.push(w, req)

			if w.Code != http.StatusBadGateway {
				t.Fatalf("status = %d, want %d", w.Code, http.StatusBadGateway)
			}
			if result := decodePushResult(t, w); result.Success {
				t.Fatalf("Success = true, want false")
			}
			if len(tc.fake.pushAddonsCalls) != tc.wantAddonsCalls {
				t.Fatalf("PushAddons calls = %d, want %d", len(tc.fake.pushAddonsCalls), tc.wantAddonsCalls)
			}
			if len(tc.fake.pushCollectionsCalls) != tc.wantCollectionsCalls {
				t.Fatalf("PushCollections calls = %d, want %d", len(tc.fake.pushCollectionsCalls), tc.wantCollectionsCalls)
			}

			sel, err := db.GetCurrentCollectionSelection(ctx, profile.ID)
			if err != nil {
				t.Fatalf("GetCurrentCollectionSelection: %v", err)
			}
			if len(sel) != 0 {
				t.Fatalf("local selection was written despite a rejected push: %v", sel)
			}
		})
	}
}

// When both Nuvio pushes succeed but the local commit that follows fails,
// push must attempt a compensating revert of the collections push — and
// report whether that revert itself succeeded, via UndoFailed.
func TestPush_CompensatingRevert(t *testing.T) {
	for _, tc := range []struct {
		name           string
		revertErr      error
		wantUndoFailed bool
	}{
		{name: "revert succeeds", revertErr: nil, wantUndoFailed: false},
		{name: "revert also fails", revertErr: fmt.Errorf("%w: status 500", nuvio.ErrNuvioRequestFailed), wantUndoFailed: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := newTestVaultDB(t)
			ctx := context.Background()

			profile, err := db.ResolveOrCreateProfile(ctx, "user-revert", 1, "nuvio-uuid-revert")
			if err != nil {
				t.Fatalf("creating profile: %v", err)
			}
			coll, err := db.CreateUserCollection(ctx, profile.ID, vault.CollectionForm{Title: "Mine"})
			if err != nil {
				t.Fatalf("creating collection: %v", err)
			}

			// A foreign entry already sitting in Nuvio's blob before this
			// push, unrelated to the collection push is about to add — this
			// is what the revert must restore, not just any prior state.
			priorRaw, err := json.Marshal(map[string]string{"id": uuid.New().String(), "title": "Prior State"})
			if err != nil {
				t.Fatalf("marshaling prior raw entry: %v", err)
			}

			fake := &fakeNuvio{
				pullCollections:     []json.RawMessage{priorRaw},
				pushCollectionsErrs: []error{nil, tc.revertErr},
				// Simulate the collection being deleted (e.g. by a concurrent
				// request) in the window between Nuvio accepting the
				// collections push and Uno's local commit — the one case
				// push's own ordering doesn't guard against, since the local
				// write is genuinely the last step.
				onPushCollections: func(call int, _ []json.RawMessage) {
					if call == 0 {
						if err := db.DeleteUserCollection(ctx, profile.ID, coll.ID); err != nil {
							t.Fatalf("deleting collection mid-push: %v", err)
						}
					}
				},
			}
			s := &Server{vault: db, nuvio: fake, siteBaseURL: "http://example.com"}

			reqCtx := withNuvioToken(withProfileID(ctx, profile.ID), "token")
			req := newPushRequest(t, reqCtx, pushRequest{
				Collections: vault.CollectionSelectionForm{Collections: []vault.SelectedCollectionInput{{CollectionID: coll.ID}}},
			})
			w := httptest.NewRecorder()

			s.push(w, req)

			if w.Code != http.StatusInternalServerError {
				t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
			}
			result := decodePushResult(t, w)
			if result.Success {
				t.Fatalf("Success = true, want false")
			}
			if result.UndoFailed != tc.wantUndoFailed {
				t.Fatalf("UndoFailed = %v, want %v", result.UndoFailed, tc.wantUndoFailed)
			}
			if len(fake.pushCollectionsCalls) != 2 {
				t.Fatalf("PushCollections calls = %d, want 2 (the push, then a compensating revert)", len(fake.pushCollectionsCalls))
			}

			// The revert must restore exactly what pushCollections pulled
			// before this push touched anything — not the just-pushed
			// blob (which would make the revert a no-op) and not some
			// other value.
			pushedBlob, revertBlob := fake.pushCollectionsCalls[0], fake.pushCollectionsCalls[1]
			if !reflect.DeepEqual(fake.pullCollections, revertBlob) {
				t.Fatalf("revert blob = %s, want the originally pulled blob %s", revertBlob, fake.pullCollections)
			}
			if reflect.DeepEqual(pushedBlob, revertBlob) {
				t.Fatalf("revert pushed the same blob as the original push instead of restoring prior state")
			}
		})
	}
}

// A pulled collection this profile doesn't own, but whose every source
// carries this addon's id, is dropped from the pushed blob — the signal that
// Uno once pushed it but has since forgotten it (hard-deleted locally, or a
// recreated database). Tested under both "sources" (real pulled data's key)
// and "catalogSources" (the key Uno's own push writes under), since which
// key a previously-Uno-pushed collection round-trips under isn't confirmed.
// A pulled collection with a foreign addon id, or no sources at all, must
// survive untouched.
func TestPush_DropsForgottenUnoManagedCollections(t *testing.T) {
	for _, sourcesKey := range []string{"sources", "catalogSources"} {
		t.Run(sourcesKey, func(t *testing.T) {
			db := newTestVaultDB(t)
			ctx := context.Background()

			profile, err := db.ResolveOrCreateProfile(ctx, "user-forgotten-"+sourcesKey, 1, "nuvio-uuid-forgotten-"+sourcesKey)
			if err != nil {
				t.Fatalf("creating profile: %v", err)
			}

			marshalCollection := func(id string, folders []map[string]any) json.RawMessage {
				raw, err := json.Marshal(map[string]any{"id": id, "folders": folders})
				if err != nil {
					t.Fatalf("marshaling collection: %v", err)
				}
				return raw
			}

			forgottenRaw := marshalCollection(uuid.New().String(), []map[string]any{
				{sourcesKey: []map[string]string{{"addonId": addon.ID}, {"addonId": addon.ID}}},
			})
			foreignRaw := marshalCollection(uuid.New().String(), []map[string]any{
				{sourcesKey: []map[string]string{{"addonId": "some.other.addon"}}},
			})
			emptyRaw := marshalCollection(uuid.New().String(), []map[string]any{})

			fake := &fakeNuvio{pullCollections: []json.RawMessage{forgottenRaw, foreignRaw, emptyRaw}}
			s := &Server{vault: db, nuvio: fake, siteBaseURL: "http://example.com"}

			reqCtx := withNuvioToken(withProfileID(ctx, profile.ID), "token")
			req := newPushRequest(t, reqCtx, pushRequest{})
			w := httptest.NewRecorder()

			s.push(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
			}
			if len(fake.pushCollectionsCalls) != 1 {
				t.Fatalf("PushCollections calls = %d, want 1", len(fake.pushCollectionsCalls))
			}
			pushed := fake.pushCollectionsCalls[0]
			if len(pushed) != 2 {
				t.Fatalf("pushed blob has %d entries, want 2 (foreign + empty kept, forgotten dropped): %s", len(pushed), pushed)
			}
			for _, raw := range pushed {
				if string(raw) == string(forgottenRaw) {
					t.Fatalf("forgotten Uno-managed collection survived in the pushed blob: %s", raw)
				}
			}
		})
	}
}

// The addons push is a full replace, so push sends back every addon the
// profile already has, unchanged and in order, with Uno's own entry upserted
// by manifest URL: appended when absent, refreshed in place when present,
// never duplicated. An addon with Uno's name at another URL is a different
// addon.
func TestPush_MergesUnoIntoExistingAddons(t *testing.T) {
	other := nuvio.NuvioAddon{URL: "https://other.example/manifest.json", Name: "Other", Enabled: true, SortOrder: 0}
	disabled := nuvio.NuvioAddon{URL: "https://off.example/manifest.json", Name: "Off", Enabled: false, SortOrder: 1}
	lookalike := nuvio.NuvioAddon{URL: "https://uno.example/u/another-token/manifest.json", Name: addon.Name, Enabled: true, SortOrder: 2}

	tests := []struct {
		name     string
		existing func(manifestURL string) []nuvio.NuvioAddon
		want     func(manifestURL string) []nuvio.NuvioAddon
	}{
		{
			name:     "no addons yet",
			existing: func(string) []nuvio.NuvioAddon { return nil },
			want: func(manifestURL string) []nuvio.NuvioAddon {
				return []nuvio.NuvioAddon{{URL: manifestURL, Name: addon.Name, Enabled: true, SortOrder: 0}}
			},
		},
		{
			name: "appended after the profile's other addons",
			existing: func(string) []nuvio.NuvioAddon {
				return []nuvio.NuvioAddon{other, disabled, lookalike}
			},
			want: func(manifestURL string) []nuvio.NuvioAddon {
				return []nuvio.NuvioAddon{other, disabled, lookalike, {URL: manifestURL, Name: addon.Name, Enabled: true, SortOrder: 3}}
			},
		},
		{
			name: "refreshed in place where it already is",
			existing: func(manifestURL string) []nuvio.NuvioAddon {
				return []nuvio.NuvioAddon{other, {URL: manifestURL, Name: "Renamed by the user", Enabled: false, SortOrder: 1}, disabled}
			},
			want: func(manifestURL string) []nuvio.NuvioAddon {
				return []nuvio.NuvioAddon{other, {URL: manifestURL, Name: addon.Name, Enabled: true, SortOrder: 1}, disabled}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db := newTestVaultDB(t)
			ctx := t.Context()
			profile, err := db.ResolveOrCreateProfile(ctx, "user-addons", 1, "nuvio-uuid-addons")
			if err != nil {
				t.Fatalf("creating profile: %v", err)
			}
			manifestURL := "https://uno.example" + addon.ManifestPath(profile.Token)

			fake := &fakeNuvio{addons: tc.existing(manifestURL)}
			s := &Server{vault: db, nuvio: fake, siteBaseURL: "https://uno.example"}
			w := httptest.NewRecorder()
			s.push(w, newPushRequest(t, withNuvioToken(withProfileID(ctx, profile.ID), "token"), pushRequest{}))

			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d (body %q)", w.Code, http.StatusOK, w.Body.String())
			}
			if result := decodePushResult(t, w); result.ManifestURL != manifestURL {
				t.Fatalf("manifest_url = %q, want %q", result.ManifestURL, manifestURL)
			}
			if len(fake.pushAddonsCalls) != 1 {
				t.Fatalf("PushAddons calls = %d, want 1", len(fake.pushAddonsCalls))
			}
			if got, want := fake.pushAddonsCalls[0], tc.want(manifestURL); !reflect.DeepEqual(got, want) {
				t.Fatalf("pushed addons = %+v\nwant           %+v", got, want)
			}
		})
	}
}

// The collections push is a full replace too. A pulled collection this
// profile owns is dropped whether or not it is still selected, since the
// fresh copy of each selected one is appended after everything kept; one it
// doesn't own goes back byte-for-byte, including a number too large for a
// float64 and formatting a re-encode would change.
func TestPush_MergesCollectionsIntoPulledBlob(t *testing.T) {
	db := newTestVaultDB(t)
	ctx := t.Context()
	profile, err := db.ResolveOrCreateProfile(ctx, "user-merge", 1, "nuvio-uuid-merge")
	if err != nil {
		t.Fatalf("creating profile: %v", err)
	}
	selected, err := db.CreateUserCollection(ctx, profile.ID, vault.CollectionForm{Title: "Selected"})
	if err != nil {
		t.Fatalf("creating selected collection: %v", err)
	}
	deselected, err := db.CreateUserCollection(ctx, profile.ID, vault.CollectionForm{Title: "Deselected"})
	if err != nil {
		t.Fatalf("creating deselected collection: %v", err)
	}

	foreign := json.RawMessage(`{"id":"foreign-1",  "title":"Theirs","n":12345678901234567890,"folders":[{"sources":[{"addonId":"some.other.addon"}]}]}`)
	// Owned entries are dropped by id: their sources name another addon, so
	// the Uno-managed heuristic alone would keep them.
	staleSelected := json.RawMessage(`{"id":"` + selected.ID.String() + `","title":"Stale","folders":[{"sources":[{"addonId":"some.other.addon"}]}]}`)
	staleDeselected := json.RawMessage(`{"id":"` + deselected.ID.String() + `","title":"Deselected","folders":[{"sources":[{"addonId":"some.other.addon"}]}]}`)

	fake := &fakeNuvio{pullCollections: []json.RawMessage{staleSelected, foreign, staleDeselected}}
	s := &Server{vault: db, nuvio: fake, siteBaseURL: "https://uno.example"}
	w := httptest.NewRecorder()
	s.push(w, newPushRequest(t, withNuvioToken(withProfileID(ctx, profile.ID), "token"), pushRequest{
		Collections: vault.CollectionSelectionForm{Collections: []vault.SelectedCollectionInput{{CollectionID: selected.ID}}},
	}))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body %q)", w.Code, http.StatusOK, w.Body.String())
	}
	if len(fake.pushCollectionsCalls) != 1 {
		t.Fatalf("PushCollections calls = %d, want 1", len(fake.pushCollectionsCalls))
	}
	pushed := fake.pushCollectionsCalls[0]
	if len(pushed) != 2 {
		t.Fatalf("pushed %d collections, want 2 (the foreign one, then the selected one): %s", len(pushed), pushed)
	}
	if string(pushed[0]) != string(foreign) {
		t.Fatalf("foreign collection changed in transit:\n got %s\nwant %s", pushed[0], foreign)
	}
	var fresh vault.PushCollection
	if err := json.Unmarshal(pushed[1], &fresh); err != nil {
		t.Fatalf("decoding pushed collection: %v", err)
	}
	if fresh.ID != selected.ID.String() || fresh.Title != "Selected" {
		t.Fatalf("pushed collection = %s, want a fresh build of %q", pushed[1], "Selected")
	}

	sel, err := db.GetCurrentCollectionSelection(ctx, profile.ID)
	if err != nil {
		t.Fatalf("GetCurrentCollectionSelection: %v", err)
	}
	if len(sel) != 1 {
		t.Fatalf("saved selection = %v, want the one selected collection", sel)
	}
	if sel[0].NeedsPush {
		t.Errorf("needs_push right after the push = true, want false: push stores the record of what it sent")
	}
	if _, err := db.UpdateUserCollection(ctx, profile.ID, selected.ID, vault.CollectionForm{Title: "Renamed"}); err != nil {
		t.Fatal(err)
	}
	if sel, err = db.GetCurrentCollectionSelection(ctx, profile.ID); err != nil || !sel[0].NeedsPush {
		t.Errorf("needs_push after a rename = %v (%v), want true", sel, err)
	}
}

// A push body with a field the request doesn't have is a 400, before
// anything reaches Nuvio or the vault. The case that matters is a tab loaded
// before the selection gained its pins: it still sends collection_ids, which
// a lenient read would take as an empty selection, clearing every Uno
// collection from Nuvio and every collection from Home.
func TestPush_RefusesABodyInAnotherShape(t *testing.T) {
	db := newTestVaultDB(t)
	profile, err := db.ResolveOrCreateProfile(t.Context(), "user-shape", 1, "nuvio-uuid-shape")
	if err != nil {
		t.Fatalf("creating profile: %v", err)
	}
	ctx := withNuvioToken(withProfileID(t.Context(), profile.ID), "token")
	onHome, err := db.CreateUserCollection(ctx, profile.ID, vault.CollectionForm{Title: "On Home"})
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{vault: db, nuvio: &fakeNuvio{}, siteBaseURL: "https://uno.example"}
	w := httptest.NewRecorder()
	s.push(w, newPushRequest(t, ctx, pushRequest{Collections: vault.CollectionSelectionForm{
		Collections: []vault.SelectedCollectionInput{{CollectionID: onHome.ID}},
	}}))
	if w.Code != http.StatusOK {
		t.Fatalf("first push = %d (%s), want 200", w.Code, w.Body.String())
	}

	fake := &fakeNuvio{}
	s.nuvio = fake
	old := `{"catalogs":{"catalogs":[]},"collections":{"collection_ids":["` + onHome.ID.String() + `"]}}`
	w = httptest.NewRecorder()
	s.push(w, httptest.NewRequest(http.MethodPost, "/api/p/1/push", bytes.NewReader([]byte(old))).WithContext(ctx))
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d (%s), want 400", w.Code, w.Body.String())
	}
	if len(fake.pushAddonsCalls) != 0 || len(fake.pushCollectionsCalls) != 0 {
		t.Errorf("Nuvio calls = %d addons, %d collections; want none", len(fake.pushAddonsCalls), len(fake.pushCollectionsCalls))
	}
	if sel, err := db.GetCurrentCollectionSelection(ctx, profile.ID); err != nil || len(sel) != 1 || sel[0].ID != onHome.ID {
		t.Errorf("selection after the refused push = %v (%v), want On Home still on it", sel, err)
	}
}

// Push sends each selected collection's pin as its selection entry has it,
// not as the row last stored it, and stores that pin once Nuvio has taken
// the push. A collection push leaves off Home keeps the pin it had.
func TestPush_SendsAndStoresTheSelectionsPin(t *testing.T) {
	db := newTestVaultDB(t)
	ctx := t.Context()
	profile, err := db.ResolveOrCreateProfile(ctx, "user-pin", 1, "nuvio-uuid-pin")
	if err != nil {
		t.Fatalf("creating profile: %v", err)
	}
	first, err := db.CreateUserCollection(ctx, profile.ID, vault.CollectionForm{Title: "First"})
	if err != nil {
		t.Fatal(err)
	}
	later, err := db.CreateUserCollection(ctx, profile.ID, vault.CollectionForm{Title: "Later"})
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{vault: db, siteBaseURL: "https://uno.example"}
	push := func(entries ...vault.SelectedCollectionInput) []json.RawMessage {
		t.Helper()
		fake := &fakeNuvio{}
		s.nuvio = fake
		w := httptest.NewRecorder()
		s.push(w, newPushRequest(t, withNuvioToken(withProfileID(ctx, profile.ID), "token"), pushRequest{
			Collections: vault.CollectionSelectionForm{Collections: entries},
		}))
		if w.Code != http.StatusOK || len(fake.pushCollectionsCalls) != 1 {
			t.Fatalf("status = %d, PushCollections calls = %d; want 200 and 1 (body %q)", w.Code, len(fake.pushCollectionsCalls), w.Body.String())
		}
		return fake.pushCollectionsCalls[0]
	}
	pinnedOf := func(raw json.RawMessage) bool {
		t.Helper()
		var c vault.PushCollection
		if err := json.Unmarshal(raw, &c); err != nil {
			t.Fatal(err)
		}
		return c.PinToTop
	}
	stored := func(id uuid.UUID) bool {
		t.Helper()
		all, err := db.GetCollectionsByIDs(ctx, []uuid.UUID{id})
		if err != nil || len(all) != 1 {
			t.Fatalf("GetCollectionsByIDs = %v, %v", all, err)
		}
		return all[0].PinToTop
	}
	// What push recorded carries the selection's pin; what a read builds
	// carries the stored one. Once push has stored it, the two agree.
	pushedClean := func(when string) {
		t.Helper()
		sel, err := db.GetCurrentCollectionSelection(ctx, profile.ID)
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range sel {
			if c.NeedsPush {
				t.Errorf("%s: %q needs a push, want none right after pushing its pin", when, c.Title)
			}
		}
	}

	sent := push(vault.SelectedCollectionInput{CollectionID: first.ID, PinToTop: true}, vault.SelectedCollectionInput{CollectionID: later.ID})
	if !pinnedOf(sent[0]) || pinnedOf(sent[1]) || !stored(first.ID) || stored(later.ID) {
		t.Fatalf("first push: sent pins %v, %v; stored %v, %v; want First pinned and Later not, sent and stored",
			pinnedOf(sent[0]), pinnedOf(sent[1]), stored(first.ID), stored(later.ID))
	}
	pushedClean("first push")

	sent = push(vault.SelectedCollectionInput{CollectionID: later.ID, PinToTop: true})
	if len(sent) != 1 || !pinnedOf(sent[0]) || !stored(later.ID) || !stored(first.ID) {
		t.Errorf("second push: want Later sent and stored pinned, and First, now off Home, still pinned")
	}
	pushedClean("second push, Later's pin flipped")
}
