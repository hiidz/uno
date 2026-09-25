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
				Collections: vault.CollectionSelectionForm{CollectionIDs: []uuid.UUID{coll.ID}},
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
		Collections: vault.CollectionSelectionForm{CollectionIDs: []uuid.UUID{selected.ID}},
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
	var fresh nuvio.PushCollection
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
}

// A folder ref's genre goes out as its catalogSources entry's "genre"; an
// unfiltered ref has no "genre" key at all rather than an empty one. One
// catalog referenced under two genres becomes two sources.
func TestBuildPushCollection_CarriesRefGenre(t *testing.T) {
	filtered := vault.Catalog{ID: uuid.New(), Type: "movie", Provider: "tmdb"}
	plain := vault.Catalog{ID: uuid.New(), Type: "series", Provider: "tmdb"}
	c := vault.CollectionWithFolders{
		Collection: vault.Collection{ID: uuid.New(), Title: "C"},
		Folders: []vault.FolderWithCatalogs{{
			Folder: vault.Folder{ID: uuid.New(), Title: "F"},
			Refs: []vault.FolderRef{
				{CatalogID: filtered.ID, Genre: "Western"},
				{CatalogID: plain.ID},
				{CatalogID: filtered.ID, Genre: "War"},
			},
		}},
	}

	pushed := buildPushCollection(c, map[uuid.UUID]vault.Catalog{filtered.ID: filtered, plain.ID: plain})
	raw, err := json.Marshal(pushed.Folders[0].CatalogSources)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var sources []map[string]any
	if err := json.Unmarshal(raw, &sources); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(sources) != 3 {
		t.Fatalf("got %d sources, want 3: %s", len(sources), raw)
	}
	if sources[0]["genre"] != "Western" || sources[2]["genre"] != "War" {
		t.Errorf("filtered source genres = %v, %v, want Western, War: %s", sources[0]["genre"], sources[2]["genre"], raw)
	}
	if sources[0]["catalogId"] != sources[2]["catalogId"] {
		t.Errorf("the two genres of one catalog carry different catalogIds: %s", raw)
	}
	if _, ok := sources[1]["genre"]; ok {
		t.Errorf("unfiltered source has a genre key: %s", raw)
	}
}

// Nuvio reads an absent focusGlowEnabled/focusGifEnabled as true, so both go
// out as an explicit false when off. The appearance URLs are omitted when
// empty, like coverImageUrl.
func TestBuildPushCollection_AppearanceFields(t *testing.T) {
	c := vault.CollectionWithFolders{
		Collection: vault.Collection{ID: uuid.New(), Title: "C"},
		Folders: []vault.FolderWithCatalogs{
			{Folder: vault.Folder{ID: uuid.New(), Title: "Off"}},
			{Folder: vault.Folder{
				ID: uuid.New(), Title: "On",
				FocusGIFURL: "https://example.com/f.gif", FocusGIFEnabled: true,
				HeroBackdropURL: "https://example.com/b.jpg", HeroVideoURL: "https://example.com/v.mp4",
				TitleLogoURL: "https://example.com/l.png",
			}},
		},
	}

	raw, err := json.Marshal(buildPushCollection(c, nil))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var pushed struct {
		FocusGlowEnabled *bool            `json:"focusGlowEnabled"`
		Folders          []map[string]any `json:"folders"`
	}
	if err := json.Unmarshal(raw, &pushed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if pushed.FocusGlowEnabled == nil || *pushed.FocusGlowEnabled {
		t.Errorf("focusGlowEnabled = %v, want explicit false: %s", pushed.FocusGlowEnabled, raw)
	}
	off, on := pushed.Folders[0], pushed.Folders[1]
	if v, ok := off["focusGifEnabled"]; !ok || v != false {
		t.Errorf("off folder focusGifEnabled = %v (present %v), want explicit false: %s", v, ok, raw)
	}
	for _, key := range []string{"focusGifUrl", "heroBackdropUrl", "heroVideoUrl", "titleLogoUrl"} {
		if _, ok := off[key]; ok {
			t.Errorf("off folder carries empty %s: %s", key, raw)
		}
	}
	want := map[string]any{
		"focusGifEnabled": true,
		"focusGifUrl":     "https://example.com/f.gif",
		"heroBackdropUrl": "https://example.com/b.jpg",
		"heroVideoUrl":    "https://example.com/v.mp4",
		"titleLogoUrl":    "https://example.com/l.png",
	}
	for key, v := range want {
		if on[key] != v {
			t.Errorf("on folder %s = %v, want %v: %s", key, on[key], v, raw)
		}
	}
}
