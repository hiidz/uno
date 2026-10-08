package api

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/hiidz/uno/internal/addon"
	"github.com/hiidz/uno/internal/nuvio"
	"github.com/hiidz/uno/internal/vault"
)

// homeCollections is profileID's collections on Home, in Home order: what
// push's local write stored.
func homeCollections(t *testing.T, db *vault.DB, profileID uuid.UUID) []vault.CollectionWithFolders {
	t.Helper()
	all, err := db.GetUserCollections(t.Context(), profileID)
	if err != nil {
		t.Fatalf("GetUserCollections: %v", err)
	}
	onHome := slices.DeleteFunc(all, func(c vault.CollectionWithFolders) bool { return c.HomeSortOrder == nil })
	slices.SortFunc(onHome, func(a, b vault.CollectionWithFolders) int { return cmp.Compare(*a.HomeSortOrder, *b.HomeSortOrder) })
	return onHome
}

// homeCatalogs is profileID's catalogs on Home, in Home order.
func homeCatalogs(t *testing.T, db *vault.DB, profileID uuid.UUID) []vault.Catalog {
	t.Helper()
	all, err := db.GetUserCatalogs(t.Context(), profileID)
	if err != nil {
		t.Fatalf("GetUserCatalogs: %v", err)
	}
	onHome := slices.DeleteFunc(all, func(c vault.Catalog) bool { return c.HomeSortOrder == nil })
	slices.SortFunc(onHome, func(a, b vault.Catalog) int { return cmp.Compare(*a.HomeSortOrder, *b.HomeSortOrder) })
	return onHome
}

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

	avatarImages    map[string]string
	avatarImagesErr error
	avatarCalls     int

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

	// homeOrder is the profile's home-order list as Nuvio holds it before a
	// push; nil is a profile with none.
	homeOrder        json.RawMessage
	pullHomeOrderErr error

	// pushHomeOrderErrs supplies a per-call error override, as
	// pushCollectionsErrs does.
	pushHomeOrderErrs  []error
	pushHomeOrderCalls []json.RawMessage
}

var _ NuvioClient = (*fakeNuvio)(nil)

func (f *fakeNuvio) ListProfiles(ctx context.Context, accessToken string) ([]nuvio.NuvioProfile, error) {
	if f.listProfilesErr != nil {
		return nil, f.listProfilesErr
	}
	return f.profiles, nil
}

// AvatarImages answers avatarImages, or fails with avatarImagesErr, and
// counts its calls.
func (f *fakeNuvio) AvatarImages(ctx context.Context, accessToken string) (map[string]string, error) {
	f.avatarCalls++
	return f.avatarImages, f.avatarImagesErr
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

func (f *fakeNuvio) PullHomeOrder(ctx context.Context, accessToken string, profileID int) (json.RawMessage, error) {
	return f.homeOrder, f.pullHomeOrderErr
}

func (f *fakeNuvio) PushHomeOrder(ctx context.Context, accessToken string, profileID int, settings json.RawMessage) error {
	call := len(f.pushHomeOrderCalls)
	f.pushHomeOrderCalls = append(f.pushHomeOrderCalls, settings)
	if call < len(f.pushHomeOrderErrs) {
		return f.pushHomeOrderErrs[call]
	}
	return nil
}

// liveAs is the profile list Nuvio answers with when profile's slot holds the
// Nuvio profile it was selected as: what push checks before it pushes.
func liveAs(profile vault.Profile) []nuvio.NuvioProfile {
	return []nuvio.NuvioProfile{{ID: profile.NuvioProfileUUID, UserID: profile.NuvioUserID, ProfileIndex: profile.NuvioProfileIndex}}
}

// createPushableCollection creates a collection with one folder, which push
// needs every collection on Home to have.
func createPushableCollection(t *testing.T, ctx context.Context, db *vault.DB, profileID uuid.UUID, title string) vault.CollectionWithFolders {
	t.Helper()
	coll, err := db.CreateUserCollection(ctx, profileID, vault.CollectionForm{Title: title, ViewMode: "TABBED_GRID", Folders: []vault.FolderData{{FolderArt: vault.FolderArt{TileShape: "POSTER"}, Title: "Folder"}}})
	if err != nil {
		t.Fatalf("creating collection %q: %v", title, err)
	}
	return coll
}

// newPushRequest is a push of body built from the Home as it stands: its
// home_revision is the one db holds now for the profile ctx carries.
func newPushRequest(t *testing.T, ctx context.Context, db *vault.DB, body pushRequest) *http.Request {
	t.Helper()
	profile, _ := profileFrom(ctx)
	revision, err := db.HomeRevision(ctx, profile.ID)
	if err != nil {
		t.Fatalf("reading the home revision: %v", err)
	}
	body.HomeRevision = revision
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

	fake := &fakeNuvio{profiles: liveAs(profile), listAddonsErr: fmt.Errorf("%w: dial tcp: connection refused", nuvio.ErrNuvioRequestFailed)}
	s := &Server{vault: db, nuvio: fake, siteBaseURL: "http://example.com"}

	reqCtx := withNuvioToken(withProfile(ctx, profile), "token")
	req := newPushRequest(t, reqCtx, db, pushRequest{})
	w := httptest.NewRecorder()

	s.push(w, req)

	if w.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadGateway)
	}
	if body := strings.TrimSpace(w.Body.String()); body != `{"success":false}` {
		t.Fatalf(`body = %s, want {"success":false}: the marker alone, an ordinary failure having nothing more to say`, body)
	}
	if len(fake.pushAddonsCalls) != 0 {
		t.Fatalf("PushAddons was called despite ListAddons failing")
	}
	if len(fake.pushCollectionsCalls) != 0 {
		t.Fatalf("collections push was attempted despite addons push never completing")
	}

	if sel := homeCollections(t, db, profile.ID); len(sel) != 0 {
		t.Fatalf("collection selection was written despite Nuvio being unreachable: %v", sel)
	}
}

// Either half of the two-push sequence being rejected must fail the whole
// push, and (per the addons-before-collections ordering) a rejected addons
// push must stop before collections is ever attempted. A rejected collections
// push puts the addon list back as it was pulled, so nothing changed.
func TestPush_OnePushRejected(t *testing.T) {
	rejected := fmt.Errorf("%w: status 400", nuvio.ErrNuvioRequestFailed)
	existing := []nuvio.NuvioAddon{{URL: "https://other.example/manifest.json", Name: "Other", Enabled: true}}

	for _, tc := range []struct {
		name                 string
		fake                 *fakeNuvio
		wantAddonsCalls      int
		wantCollectionsCalls int
	}{
		{
			name:                 "addons push rejected",
			fake:                 &fakeNuvio{addons: existing, pushAddonsErr: rejected},
			wantAddonsCalls:      1,
			wantCollectionsCalls: 0,
		},
		{
			name:                 "collections push rejected",
			fake:                 &fakeNuvio{addons: existing, pushCollectionsErrs: []error{rejected}},
			wantAddonsCalls:      2,
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

			tc.fake.profiles = liveAs(profile)
			s := &Server{vault: db, nuvio: tc.fake, siteBaseURL: "http://example.com"}

			reqCtx := withNuvioToken(withProfile(ctx, profile), "token")
			req := newPushRequest(t, reqCtx, db, pushRequest{})
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
			if last := tc.fake.pushAddonsCalls[len(tc.fake.pushAddonsCalls)-1]; tc.wantAddonsCalls > 1 && !reflect.DeepEqual(last, existing) {
				t.Fatalf("addons put back as %+v, want the pulled list %+v", last, existing)
			}

			if sel := homeCollections(t, db, profile.ID); len(sel) != 0 {
				t.Fatalf("local selection was written despite a rejected push: %v", sel)
			}
		})
	}
}

// When Nuvio takes every push but the local commit that follows fails, push
// reverts the collections, addons and home-order pushes to what it pulled, and
// reports whether that revert itself succeeded, via UndoFailed.
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
			coll := createPushableCollection(t, ctx, db, profile.ID, "Mine")

			// A foreign entry already sitting in Nuvio's blob before this
			// push, unrelated to the collection push is about to add — this
			// is what the revert must restore, not just any prior state.
			priorRaw, err := json.Marshal(map[string]string{"id": uuid.New().String(), "title": "Prior State"})
			if err != nil {
				t.Fatalf("marshaling prior raw entry: %v", err)
			}

			pulledHomeOrder := homeList(otherRow("top", 0))
			fake := &fakeNuvio{
				profiles:            liveAs(profile),
				pullCollections:     []json.RawMessage{priorRaw},
				homeOrder:           pulledHomeOrder,
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

			reqCtx := withNuvioToken(withProfile(ctx, profile), "token")
			req := newPushRequest(t, reqCtx, db, pushOf(vault.PushedHome{Collections: []vault.SelectedCollectionInput{{CollectionID: coll.ID}}}))
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
			if len(fake.pushAddonsCalls) != 2 || len(fake.pushAddonsCalls[1]) != 0 {
				t.Fatalf("PushAddons calls = %+v, want the push, then the pulled (empty) list put back", fake.pushAddonsCalls)
			}
			if len(fake.pushHomeOrderCalls) != 2 || !bytes.Equal(fake.pushHomeOrderCalls[1], pulledHomeOrder) {
				t.Fatalf("PushHomeOrder calls = %s, want the push, then %s put back as pulled", fake.pushHomeOrderCalls, pulledHomeOrder)
			}
		})
	}
}

// A pulled collection this profile neither owns nor last pushed survives the
// push untouched, even when every source in it carries this addon's id: Nuvio's
// own collection editors build collections from Uno's catalogs. Tested under
// both "sources" (the key Nuvio's apps write) and "catalogSources" (the key
// Uno's own push writes under).
func TestPush_KeepsCollectionsMadeInNuvioFromUnoCatalogs(t *testing.T) {
	for _, sourcesKey := range []string{"sources", "catalogSources"} {
		t.Run(sourcesKey, func(t *testing.T) {
			db := newTestVaultDB(t)
			ctx := context.Background()

			profile, err := db.ResolveOrCreateProfile(ctx, "user-made-in-nuvio-"+sourcesKey, 1, "nuvio-uuid-made-in-nuvio-"+sourcesKey)
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

			unoSourcedRaw := marshalCollection(uuid.New().String(), []map[string]any{
				{sourcesKey: []map[string]string{{"addonId": addon.ID}, {"addonId": addon.ID}}},
			})
			foreignRaw := marshalCollection(uuid.New().String(), []map[string]any{
				{sourcesKey: []map[string]string{{"addonId": "some.other.addon"}}},
			})
			emptyRaw := marshalCollection(uuid.New().String(), []map[string]any{})
			pulled := []json.RawMessage{unoSourcedRaw, foreignRaw, emptyRaw}

			fake := &fakeNuvio{profiles: liveAs(profile), pullCollections: pulled}
			s := &Server{vault: db, nuvio: fake, siteBaseURL: "http://example.com"}

			reqCtx := withNuvioToken(withProfile(ctx, profile), "token")
			req := newPushRequest(t, reqCtx, db, pushRequest{})
			w := httptest.NewRecorder()

			s.push(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
			}
			if len(fake.pushCollectionsCalls) != 1 {
				t.Fatalf("PushCollections calls = %d, want 1", len(fake.pushCollectionsCalls))
			}
			if pushed := fake.pushCollectionsCalls[0]; !reflect.DeepEqual(pushed, pulled) {
				t.Fatalf("pushed blob = %s, want every pulled collection back untouched: %s", pushed, pulled)
			}
		})
	}
}

// The addons push is a full replace, so push sends back every addon the
// profile already has, unchanged and in order, with Uno's own entry merged in
// once: appended when absent, switched on in place when present — under the
// URL and name it has, whether Nuvio TV saved it without /manifest.json or a
// user renamed it. A second entry for it is dropped, as is another token's
// Uno addon on this site. An addon at another site is a different addon,
// whatever it is called.
func TestPush_MergesUnoIntoExistingAddons(t *testing.T) {
	other := nuvio.NuvioAddon{URL: "https://other.example/manifest.json", Name: "Other", Enabled: true, SortOrder: 0}
	disabled := nuvio.NuvioAddon{URL: "https://off.example/manifest.json", Name: "Off", Enabled: false, SortOrder: 1}
	oldToken := nuvio.NuvioAddon{URL: "https://uno.example/u/another-token/manifest.json", Name: addon.Name, Enabled: true, SortOrder: 2}
	elsewhere := nuvio.NuvioAddon{URL: "https://uno.elsewhere/u/a-token/manifest.json", Name: addon.Name, Enabled: true, SortOrder: 3}

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
			name: "appended after the profile's other addons, another token's dropped",
			existing: func(string) []nuvio.NuvioAddon {
				return []nuvio.NuvioAddon{other, disabled, oldToken, elsewhere}
			},
			want: func(manifestURL string) []nuvio.NuvioAddon {
				return []nuvio.NuvioAddon{other, disabled, elsewhere, {URL: manifestURL, Name: addon.Name, Enabled: true, SortOrder: 4}}
			},
		},
		{
			name: "switched on in place, keeping the user's name",
			existing: func(manifestURL string) []nuvio.NuvioAddon {
				return []nuvio.NuvioAddon{other, {URL: manifestURL, Name: "Renamed by the user", Enabled: false, SortOrder: 1}, disabled}
			},
			want: func(manifestURL string) []nuvio.NuvioAddon {
				return []nuvio.NuvioAddon{other, {URL: manifestURL, Name: "Renamed by the user", Enabled: true, SortOrder: 1}, disabled}
			},
		},
		{
			name: "found as Nuvio TV saves it, its duplicate dropped",
			existing: func(manifestURL string) []nuvio.NuvioAddon {
				tvForm := strings.TrimSuffix(manifestURL, "/manifest.json")
				return []nuvio.NuvioAddon{
					{URL: tvForm, Enabled: false, SortOrder: 0},
					other,
					{URL: manifestURL, Name: addon.Name, Enabled: true, SortOrder: 2},
				}
			},
			want: func(manifestURL string) []nuvio.NuvioAddon {
				tvForm := strings.TrimSuffix(manifestURL, "/manifest.json")
				return []nuvio.NuvioAddon{{URL: tvForm, Enabled: true, SortOrder: 0}, other}
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

			fake := &fakeNuvio{profiles: liveAs(profile), addons: tc.existing(manifestURL)}
			s := &Server{vault: db, nuvio: fake, siteBaseURL: "https://uno.example"}
			w := httptest.NewRecorder()
			s.push(w, newPushRequest(t, withNuvioToken(withProfile(ctx, profile), "token"), db, pushRequest{}))

			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d (body %q)", w.Code, http.StatusOK, w.Body.String())
			}
			if body := strings.TrimSpace(w.Body.String()); body != `{"success":true,"home_revision":2}` {
				t.Fatalf("body = %s, want {\"success\":true,\"home_revision\":2}", body)
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

// addonKey compares addon URLs the way Nuvio TV's canonicalizeUrl does.
func TestAddonKey(t *testing.T) {
	for url, want := range map[string]string{
		"https://Uno.example/u/T/manifest.json":  "https://uno.example/u/t",
		" https://uno.example/u/t/ ":             "https://uno.example/u/t",
		"https://uno.example/u/t/MANIFEST.JSON/": "https://uno.example/u/t",
		"https://a.example/manifest.json?x=1":    "https://a.example?x=1",
	} {
		if got := addonKey(url); got != want {
			t.Errorf("addonKey(%q) = %q, want %q", url, got, want)
		}
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
	selected := createPushableCollection(t, ctx, db, profile.ID, "Selected")
	deselected, err := db.CreateUserCollection(ctx, profile.ID, vault.CollectionForm{Title: "Deselected", ViewMode: "TABBED_GRID"})
	if err != nil {
		t.Fatalf("creating deselected collection: %v", err)
	}

	foreign := json.RawMessage(`{"id":"foreign-1",  "title":"Theirs","n":12345678901234567890,"folders":[{"sources":[{"addonId":"some.other.addon"}]}]}`)
	// Owned entries are dropped by id, whatever their sources name.
	staleSelected := json.RawMessage(`{"id":"` + selected.ID.String() + `","title":"Stale","folders":[{"sources":[{"addonId":"some.other.addon"}]}]}`)
	staleDeselected := json.RawMessage(`{"id":"` + deselected.ID.String() + `","title":"Deselected","folders":[{"sources":[{"addonId":"some.other.addon"}]}]}`)

	fake := &fakeNuvio{profiles: liveAs(profile), pullCollections: []json.RawMessage{staleSelected, foreign, staleDeselected}}
	s := &Server{vault: db, nuvio: fake, siteBaseURL: "https://uno.example"}
	w := httptest.NewRecorder()
	s.push(w, newPushRequest(t, withNuvioToken(withProfile(ctx, profile), "token"), db, pushOf(vault.PushedHome{Collections: []vault.SelectedCollectionInput{{CollectionID: selected.ID}}})))

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

	if sel := homeCollections(t, db, profile.ID); len(sel) != 1 {
		t.Fatalf("saved selection = %v, want the one selected collection", sel)
	}
	if pending, err := db.PendingPush(ctx, profile.ID); err != nil || len(pending) != 0 {
		t.Errorf("pending right after the push = %+v, %v; want nothing: push stores the record of what it sent", pending, err)
	}
	if _, err := db.UpdateUserCollection(ctx, profile.ID, selected.ID, selected.Revision, vault.CollectionForm{Title: "Renamed", ViewMode: "TABBED_GRID"}); err != nil {
		t.Fatal(err)
	}
	if pending, err := db.PendingPush(ctx, profile.ID); err != nil || len(pending) != 1 || pending[0].ID != selected.ID || pending[0].Change != vault.PendingChanged {
		t.Errorf("pending after a rename = %+v, %v; want the collection changed", pending, err)
	}
}

// A push body with a field the request doesn't have is a 400, before
// anything reaches Nuvio or the vault. The cases that matter are tabs loaded
// before a deploy changed the body: one sending collection_ids, from before
// the selection gained its pins, and one sending catalogs and collections
// apart, from before Home took one ordered list of rows. A lenient read would
// take either as an empty selection, clearing every Uno collection from Nuvio
// and every collection from Home.
func TestPush_RefusesABodyInAnotherShape(t *testing.T) {
	db := newTestVaultDB(t)
	profile, err := db.ResolveOrCreateProfile(t.Context(), "user-shape", 1, "nuvio-uuid-shape")
	if err != nil {
		t.Fatalf("creating profile: %v", err)
	}
	ctx := withNuvioToken(withProfile(t.Context(), profile), "token")
	onHome := createPushableCollection(t, ctx, db, profile.ID, "On Home")
	s := &Server{vault: db, nuvio: &fakeNuvio{profiles: liveAs(profile)}, siteBaseURL: "https://uno.example"}
	w := httptest.NewRecorder()
	s.push(w, newPushRequest(t, ctx, db, pushOf(vault.PushedHome{Collections: []vault.SelectedCollectionInput{{CollectionID: onHome.ID}}})))
	if w.Code != http.StatusOK {
		t.Fatalf("first push = %d (%s), want 200", w.Code, w.Body.String())
	}

	fake := &fakeNuvio{}
	s.nuvio = fake
	for _, old := range []string{
		`{"catalogs":{"catalogs":[]},"collections":{"collection_ids":["` + onHome.ID.String() + `"]}}`,
		`{"catalogs":{"catalogs":[]},"collections":{"collections":[{"collection_id":"` + onHome.ID.String() + `","pin_to_top":false}]}}`,
	} {
		w = httptest.NewRecorder()
		s.push(w, httptest.NewRequest(http.MethodPost, "/api/p/1/push", bytes.NewReader([]byte(old))).WithContext(ctx))
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d (%s), want 400", old, w.Code, w.Body.String())
		}
	}
	if len(fake.pushAddonsCalls) != 0 || len(fake.pushCollectionsCalls) != 0 {
		t.Errorf("Nuvio calls = %d addons, %d collections; want none", len(fake.pushAddonsCalls), len(fake.pushCollectionsCalls))
	}
	if sel := homeCollections(t, db, profile.ID); len(sel) != 1 || sel[0].ID != onHome.ID {
		t.Errorf("selection after the refused push = %v, want On Home still on it", sel)
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
	first := createPushableCollection(t, ctx, db, profile.ID, "First")
	later := createPushableCollection(t, ctx, db, profile.ID, "Later")
	s := &Server{vault: db, siteBaseURL: "https://uno.example"}
	push := func(entries ...vault.SelectedCollectionInput) []json.RawMessage {
		t.Helper()
		fake := &fakeNuvio{profiles: liveAs(profile)}
		s.nuvio = fake
		w := httptest.NewRecorder()
		s.push(w, newPushRequest(t, withNuvioToken(withProfile(ctx, profile), "token"), db, pushOf(vault.PushedHome{Collections: entries})))
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
	// What push recorded carries the selection's pin; what Uno would push now
	// carries the stored one. Once push has stored it, the two agree.
	pushedClean := func(when string) {
		t.Helper()
		if pending, err := db.PendingPush(ctx, profile.ID); err != nil || len(pending) != 0 {
			t.Errorf("%s: pending = %+v, %v; want nothing waiting right after pushing its pin", when, pending, err)
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

// A collection the last push sent, deleted since, is dropped from Nuvio's blob
// by the next push: the push record still names it. A foreign collection with
// no folders stays.
func TestPush_DropsADeletedCollectionTheLastPushSent(t *testing.T) {
	db := newTestVaultDB(t)
	ctx := context.Background()
	profile, err := db.ResolveOrCreateProfile(ctx, "user-deleted", 1, "nuvio-uuid-deleted")
	if err != nil {
		t.Fatalf("creating profile: %v", err)
	}
	gone := createPushableCollection(t, ctx, db, profile.ID, "Gone")
	foreign := json.RawMessage(`{"id":"` + uuid.NewString() + `","folders":[]}`)
	fake := &fakeNuvio{profiles: liveAs(profile), pullCollections: []json.RawMessage{foreign}}
	s := &Server{vault: db, nuvio: fake, siteBaseURL: "http://example.com"}
	reqCtx := withNuvioToken(withProfile(ctx, profile), "token")

	first := vault.PushedHome{Collections: []vault.SelectedCollectionInput{{CollectionID: gone.ID}}}
	s.push(httptest.NewRecorder(), newPushRequest(t, reqCtx, db, pushOf(first)))
	if got := fake.pushCollectionsCalls[0]; len(got) != 2 {
		t.Fatalf("first push sent %d collections, want the foreign one and Gone", len(got))
	}

	if err := db.DeleteUserCollection(ctx, profile.ID, gone.ID); err != nil {
		t.Fatalf("deleting a collection on Home: %v", err)
	}
	fake.pullCollections = fake.pushCollectionsCalls[0]
	w := httptest.NewRecorder()
	s.push(w, newPushRequest(t, reqCtx, db, pushRequest{}))
	if w.Code != http.StatusOK {
		t.Fatalf("second push status = %d, want 200 (%s)", w.Code, w.Body.String())
	}
	if got := fake.pushCollectionsCalls[1]; len(got) != 1 || string(got[0]) != string(foreign) {
		t.Errorf("second push sent %s, want only the foreign collection", got)
	}
}

// A client that drops once Nuvio has taken the push doesn't cut the push in
// half: the local commit still lands, and the push answers success.
func TestPush_FinishesAfterTheClientDrops(t *testing.T) {
	db := newTestVaultDB(t)
	profile, err := db.ResolveOrCreateProfile(t.Context(), "user-drop", 1, "nuvio-uuid-drop")
	if err != nil {
		t.Fatalf("creating profile: %v", err)
	}
	coll := createPushableCollection(t, t.Context(), db, profile.ID, "Mine")
	reqCtx, drop := context.WithCancel(withNuvioToken(withProfile(t.Context(), profile), "token"))
	defer drop()
	fake := &fakeNuvio{
		profiles:          liveAs(profile),
		onPushCollections: func(int, []json.RawMessage) { drop() },
	}
	s := &Server{vault: db, nuvio: fake, siteBaseURL: "http://example.com"}

	w := httptest.NewRecorder()
	s.push(w, newPushRequest(t, reqCtx, db, pushOf(vault.PushedHome{Collections: []vault.SelectedCollectionInput{{CollectionID: coll.ID}}})))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", w.Code, w.Body.String())
	}
	if sel := homeCollections(t, db, profile.ID); len(sel) != 1 || sel[0].ID != coll.ID {
		t.Fatalf("selection = %v, want the pushed collection committed", sel)
	}
}

// One push runs at a time per profile: a second waits for the first's lock,
// and gives up when its request ends first. Another profile's push doesn't
// wait.
func TestLockPush(t *testing.T) {
	s := &Server{}
	mine, other := uuid.New(), uuid.New()
	unlock, err := s.lockPush(t.Context(), mine)
	if err != nil {
		t.Fatalf("first lock: %v", err)
	}

	gone, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := s.lockPush(gone, mine); !errors.Is(err, context.Canceled) {
		t.Fatalf("second lock while held = %v, want context.Canceled", err)
	}
	unlockOther, err := s.lockPush(t.Context(), other)
	if err != nil {
		t.Fatalf("another profile's lock while mine is held: %v", err)
	}
	unlockOther()

	unlock()
	unlock, err = s.lockPush(t.Context(), mine)
	if err != nil {
		t.Fatalf("lock after release: %v", err)
	}
	unlock()
}

// A selection naming a catalog the profile doesn't own is a 400, before Nuvio
// is contacted.
func TestPush_TurnsAwayACatalogItDoesNotOwn(t *testing.T) {
	db := newTestVaultDB(t)
	ctx := context.Background()
	profile, err := db.ResolveOrCreateProfile(ctx, "user-turned-away", 1, "nuvio-uuid-turned-away")
	if err != nil {
		t.Fatalf("creating profile: %v", err)
	}
	foreign := vault.PushedHome{Catalogs: []vault.SelectedCatalogInput{{CatalogID: uuid.New(), ShowInHome: true}}}

	fake := &fakeNuvio{}
	s := &Server{vault: db, nuvio: fake, siteBaseURL: "http://example.com"}
	w := httptest.NewRecorder()
	s.push(w, newPushRequest(t, withNuvioToken(withProfile(ctx, profile), "token"), db, pushOf(foreign)))
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 (%s)", w.Code, w.Body.String())
	}
	if len(fake.pushAddonsCalls) != 0 || len(fake.pushCollectionsCalls) != 0 {
		t.Error("Nuvio was contacted")
	}
}

// A push that can't go ahead as it stands is answered before any addon or
// collection reaches Nuvio: a collection on Home with no folders, a Nuvio
// profile slot that is empty or holds another Nuvio profile now, a Nuvio
// profile using profile 1's addons, and Nuvio failing to list the profiles.
func TestPush_RefusesBeforeNuvio(t *testing.T) {
	db := newTestVaultDB(t)
	ctx := context.Background()
	profile, err := db.ResolveOrCreateProfile(ctx, "user-refused", 2, "nuvio-uuid-refused")
	if err != nil {
		t.Fatalf("creating profile: %v", err)
	}
	empty, err := db.CreateUserCollection(ctx, profile.ID, vault.CollectionForm{Title: "Empty", ViewMode: "TABBED_GRID"})
	if err != nil {
		t.Fatal(err)
	}
	full := createPushableCollection(t, ctx, db, profile.ID, "Full")
	onHome := func(ids ...uuid.UUID) pushRequest {
		var home vault.PushedHome
		for _, id := range ids {
			home.Collections = append(home.Collections, vault.SelectedCollectionInput{CollectionID: id})
		}
		return pushOf(home)
	}
	sharing := liveAs(profile)
	sharing[0].UsesPrimaryAddons = true
	replaced := liveAs(profile)
	replaced[0].ID = "nuvio-uuid-someone-new"

	tests := []struct {
		name        string
		fake        *fakeNuvio
		body        pushRequest
		wantStatus  int
		wantRefused string
	}{
		{"a collection with no folders", &fakeNuvio{profiles: liveAs(profile)}, onHome(full.ID, empty.ID), http.StatusBadRequest, refusedEmptyCollection},
		{"a collection named twice", &fakeNuvio{profiles: liveAs(profile)}, onHome(full.ID, full.ID), http.StatusBadRequest, ""},
		{"the slot is empty", &fakeNuvio{}, onHome(full.ID), http.StatusConflict, refusedProfileChanged},
		{"the slot holds another Nuvio profile", &fakeNuvio{profiles: replaced}, onHome(full.ID), http.StatusConflict, refusedProfileChanged},
		{"the profile uses profile 1's addons", &fakeNuvio{profiles: sharing}, onHome(full.ID), http.StatusConflict, refusedSharesAddons},
		{"Nuvio can't list the profiles", &fakeNuvio{listProfilesErr: fmt.Errorf("%w: status 503", nuvio.ErrNuvioRequestFailed)}, onHome(full.ID), http.StatusBadGateway, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := &Server{vault: db, nuvio: tc.fake, siteBaseURL: "http://example.com"}
			w := httptest.NewRecorder()
			s.push(w, newPushRequest(t, withNuvioToken(withProfile(ctx, profile), "token"), db, tc.body))
			if w.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d (%s)", w.Code, tc.wantStatus, w.Body.String())
			}
			if result := decodePushResult(t, w); result.Success || result.Refused != tc.wantRefused {
				t.Errorf("result = %+v, want a failure refused as %q", result, tc.wantRefused)
			}
			if len(tc.fake.pushAddonsCalls) != 0 || len(tc.fake.pushCollectionsCalls) != 0 {
				t.Error("Nuvio was pushed to")
			}
		})
	}
}

// A record giving Nuvio more catalogs than maxPushedCatalogs is refused
// before Nuvio is contacted; one at the limit goes ahead.
func TestRecordRefusalCountsTheCatalogsForNuvio(t *testing.T) {
	record := vault.PushRecord{Catalogs: make([]vault.PushedCatalog, maxPushedCatalogs)}
	if got := recordRefusal(record); got != "" {
		t.Errorf("%d catalogs: refused %q, want none", len(record.Catalogs), got)
	}
	record.Catalogs = append(record.Catalogs, vault.PushedCatalog{})
	if got := recordRefusal(record); got != refusedTooManyCatalogs {
		t.Errorf("%d catalogs: refused %q, want %q", len(record.Catalogs), got, refusedTooManyCatalogs)
	}
}

// pushOf is a push body putting home's catalogs, then its collections, on
// Home, each in the order given.
func pushOf(home vault.PushedHome) pushRequest {
	var body pushRequest
	for _, c := range home.Catalogs {
		id := c.CatalogID
		body.Rows = append(body.Rows, pushRow{CatalogID: &id, ShowInHome: c.ShowInHome})
	}
	for _, c := range home.Collections {
		id := c.CollectionID
		body.Rows = append(body.Rows, pushRow{CollectionID: &id, PinToTop: c.PinToTop})
	}
	return body
}
