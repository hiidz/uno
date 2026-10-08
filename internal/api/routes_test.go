package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"

	"github.com/hiidz/uno/internal/httpx"
	"github.com/hiidz/uno/internal/nuvio"
	"github.com/hiidz/uno/internal/provider"
	"github.com/hiidz/uno/internal/vault"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// fakeTMDB answers every request sent through http.DefaultTransport with
// handler until the test ends, and counts them. The TMDB client's
// http.Client has no Transport of its own, so its requests go there; tests
// assert the count, so a client that stops sending through it fails here
// instead of reaching the real TMDB.
func fakeTMDB(t *testing.T, handler http.HandlerFunc) *atomic.Int32 {
	t.Helper()
	var hits atomic.Int32
	orig := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		hits.Add(1)
		rec := httptest.NewRecorder()
		handler(rec, r)
		return rec.Result(), nil
	})
	t.Cleanup(func() { http.DefaultTransport = orig })
	return &hits
}

// tmdbUp serves one discover page holding one film and a two-genre list.
func tmdbUp(w http.ResponseWriter, r *http.Request) {
	switch {
	case strings.HasPrefix(r.URL.Path, "/3/discover/"):
		fmt.Fprint(w, `{"results":[{"id":155,"title":"The Dark Knight","release_date":"2008-07-16"}],"total_results":1,"total_pages":1}`)
	case strings.HasPrefix(r.URL.Path, "/3/genre/"):
		fmt.Fprint(w, `{"genres":[{"id":28,"name":"Action"},{"id":80,"name":"Crime"}]}`)
	default:
		http.NotFound(w, r)
	}
}

// tmdbDown fails every request.
func tmdbDown(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, "unavailable", http.StatusServiceUnavailable)
}

// routeStep is one request through the real router and the answer it must
// get: its status, and a fragment of its body.
type routeStep struct {
	name, method, path, body string
	noAuth                   bool
	wantStatus               int
	wantBody                 string
}

// serve sends one request through s's router, as the authenticated caller
// unless noAuth.
func serve(t *testing.T, s *Server, method, path, body string, noAuth bool) *httptest.ResponseRecorder {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req := httptest.NewRequestWithContext(t.Context(), method, path, r)
	if !noAuth {
		req.Header.Set("Authorization", "Bearer token")
	}
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)
	return w
}

// answerText is the words of w's error answer: the error field of an ErrorBody,
// else the whole body, trimmed.
func answerText(w *httptest.ResponseRecorder) string {
	var body httpx.ErrorBody
	if json.Unmarshal(w.Body.Bytes(), &body) == nil && body.Error != "" {
		return body.Error
	}
	return strings.TrimSpace(w.Body.String())
}

// runSteps runs steps in order as subtests, since a step's answer can
// depend on what the steps before it changed.
func runSteps(t *testing.T, s *Server, steps []routeStep) {
	t.Helper()
	for _, step := range steps {
		t.Run(step.name, func(t *testing.T) {
			w := serve(t, s, step.method, step.path, step.body, step.noAuth)
			if w.Code != step.wantStatus {
				t.Fatalf("status = %d, want %d (body %q)", w.Code, step.wantStatus, w.Body.String())
			}
			if !strings.Contains(answerText(w), step.wantBody) {
				t.Fatalf("body = %q, want it to contain %q", answerText(w), step.wantBody)
			}
		})
	}
}

// routeFixture is the caller's library — catalog mine and collection
// mineColl, whose folder references mine — beside another profile's private
// catalog theirs and collection theirColl.
type routeFixture struct {
	db                  *vault.DB
	s                   *Server
	caller              vault.Profile
	mine, theirs        vault.Catalog
	mineColl, theirColl vault.CollectionWithFolders
}

const popular = `{"sort_by":"popularity.desc"}`

func newRouteFixture(t *testing.T) routeFixture {
	t.Helper()
	ctx := t.Context()
	f := routeFixture{db: newTestVaultDB(t)}
	f.s = newProfileTestServer(t, f.db)

	var err error
	f.caller, err = f.db.ResolveOrCreateProfile(ctx, "test-sub", 1, "nuvio-profile-caller")
	if err != nil {
		t.Fatalf("ResolveOrCreateProfile (caller): %v", err)
	}
	owner, err := f.db.ResolveOrCreateProfile(ctx, "owner", 1, "nuvio-profile-owner")
	if err != nil {
		t.Fatalf("ResolveOrCreateProfile (owner): %v", err)
	}
	library := func(p vault.Profile, name string) (vault.Catalog, vault.CollectionWithFolders) {
		c, err := f.db.CreateUserCatalog(ctx, p.ID, vault.CatalogForm{
			Type: "movie", Name: name, Provider: "tmdb", Params: popular,
		})
		if err != nil {
			t.Fatalf("create catalog %s: %v", name, err)
		}
		coll, err := f.db.CreateUserCollection(ctx, p.ID, vault.CollectionForm{
			Title: name, ViewMode: "ROWS",
			Folders: []vault.FolderData{{FolderArt: vault.FolderArt{TileShape: "POSTER"}, Title: "Folder", Catalogs: vault.CatalogRefs(c.ID)}},
		})
		if err != nil {
			t.Fatalf("create collection %s: %v", name, err)
		}
		return c, coll
	}
	f.mine, f.mineColl = library(f.caller, "Mine")
	f.theirs, f.theirColl = library(owner, "Theirs")
	return f
}

// The library carries none of the row columns the builder never reads: a
// row's owner, which is always the caller, its created and updated times, and
// a folder's collection and place, which its collection and its order already
// give.
func TestLibraryLeavesOutUnreadColumns(t *testing.T) {
	f := newRouteFixture(t)
	keysOf := func(t *testing.T, raw json.RawMessage) map[string]json.RawMessage {
		t.Helper()
		var keys map[string]json.RawMessage
		if err := json.Unmarshal(raw, &keys); err != nil {
			t.Fatal(err)
		}
		return keys
	}
	var lib struct{ Catalogs, Collections, Pending []json.RawMessage }
	w := serve(t, f.s, http.MethodGet, "/api/p/1/library", "", false)
	if err := json.Unmarshal(w.Body.Bytes(), &lib); err != nil || len(lib.Catalogs) != 1 || len(lib.Collections) != 1 || lib.Pending == nil {
		t.Fatalf("GET library = %s (%v), want one catalog, one collection and a pending list", w.Body.String(), err)
	}
	catalogs, collections := lib.Catalogs, lib.Collections
	collection := keysOf(t, collections[0])
	var folders []json.RawMessage
	if err := json.Unmarshal(collection["folders"], &folders); err != nil || len(folders) != 1 {
		t.Fatalf("folders = %s (%v), want one", collection["folders"], err)
	}
	for name, row := range map[string]map[string]json.RawMessage{
		"catalog": keysOf(t, catalogs[0]), "collection": collection, "folder": keysOf(t, folders[0]),
	} {
		for _, key := range []string{"owner_id", "sort_order", "created_at", "updated_at", "publisher_unpublished"} {
			if _, ok := row[key]; ok {
				t.Errorf("%s carries %q", name, key)
			}
		}
	}
	if _, ok := keysOf(t, folders[0])["collection_id"]; ok {
		t.Error(`folder carries "collection_id"`)
	}
}

// noTMDB fails the test on any TMDB request: every recipe these routes are
// sent is judged without one.
func noTMDB(t *testing.T) {
	t.Helper()
	fakeTMDB(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected TMDB request %s", r.URL)
		http.Error(w, "unexpected", http.StatusTeapot)
	})
}

// TestCatalogRoutes drives the catalog CRUD routes: a create stores the
// recipe in canonical form, a bad body or recipe is a 400, and another
// profile's catalog answers exactly like one that doesn't exist.
func TestCatalogRoutes(t *testing.T) {
	f := newRouteFixture(t)
	noTMDB(t)

	t.Run("create stores the canonical recipe", func(t *testing.T) {
		body := `{"type":"movie","name":"New","provider":"tmdb","params":"{\"vote_count_gte\":0,\"sort_by\":\"vote_average.desc\"}"}`
		var created vault.Catalog
		w := serve(t, f.s, http.MethodPost, "/api/p/1/catalogs", body, false)
		if w.Code != http.StatusCreated {
			t.Fatalf("status = %d, want %d (body %q)", w.Code, http.StatusCreated, w.Body.String())
		}
		if err := json.NewDecoder(w.Body).Decode(&created); err != nil {
			t.Fatalf("decoding: %v", err)
		}
		rows, err := f.db.GetCatalogsByIDs(t.Context(), []uuid.UUID{created.ID})
		if err != nil || len(rows) != 1 {
			t.Fatalf("GetCatalogsByIDs = %d rows (%v)", len(rows), err)
		}
		const canonical = `{"sort_by":"vote_average.desc"}`
		if created.Params != canonical || rows[0].Params != canonical {
			t.Fatalf("params answered %s and stored %s, want %s", created.Params, rows[0].Params, canonical)
		}
		if want := recipeHashOf(t, canonical); rows[0].RecipeHash != want {
			t.Fatalf("stored recipe hash = %q, want %q", rows[0].RecipeHash, want)
		}
	})

	mine := "/api/p/1/catalogs/" + f.mine.ID.String()
	theirs := "/api/p/1/catalogs/" + f.theirs.ID.String()
	valid := `{"type":"movie","name":"Renamed","provider":"tmdb","params":"{\"sort_by\":\"popularity.desc\"}"}`
	runSteps(t, f.s, []routeStep{
		{name: "unauthenticated", method: http.MethodGet, path: "/api/p/1/library", noAuth: true, wantStatus: http.StatusUnauthorized},
		{name: "unprovisioned profile slot", method: http.MethodGet, path: "/api/p/2/library", wantStatus: http.StatusNotFound, wantBody: "profile not found"},
		{name: "list", method: http.MethodGet, path: "/api/p/1/library", wantStatus: http.StatusOK, wantBody: f.mine.ID.String()},
		{name: "list includes the created catalog", method: http.MethodGet, path: "/api/p/1/library", wantStatus: http.StatusOK, wantBody: `"name":"New"`},
		{name: "community leaves out unpublished catalogs", method: http.MethodGet, path: "/api/p/1/community?kind=catalog&sort=newest", wantStatus: http.StatusOK, wantBody: `{"items":[],"next_cursor":null}`},
		{name: "create with a malformed body", method: http.MethodPost, path: "/api/p/1/catalogs", body: `{`, wantStatus: http.StatusBadRequest, wantBody: "invalid request body"},
		{name: "create for another provider", method: http.MethodPost, path: "/api/p/1/catalogs", body: `{"type":"movie","name":"X","provider":"mdblist","params":"{}"}`, wantStatus: http.StatusBadRequest, wantBody: `no recipes for provider "mdblist"`},
		{name: "create with a broken recipe", method: http.MethodPost, path: "/api/p/1/catalogs", body: `{"type":"movie","name":"X","provider":"tmdb","params":"{\"sort_by\":\"bogus.desc\"}"}`, wantStatus: http.StatusBadRequest},
		{name: "create with no name", method: http.MethodPost, path: "/api/p/1/catalogs", body: `{"type":"movie","name":"","provider":"tmdb","params":"{}"}`, wantStatus: http.StatusBadRequest},
		{name: "update", method: http.MethodPut, path: mine, body: `{"type":"movie","name":"Renamed","provider":"tmdb","params":"{\"sort_by\":\"popularity.desc\"}","revision":1}`, wantStatus: http.StatusOK, wantBody: `"name":"Renamed"`},
		{name: "update with a path id that isn't a uuid", method: http.MethodPut, path: "/api/p/1/catalogs/nope", body: valid, wantStatus: http.StatusBadRequest, wantBody: "invalid catalog id"},
		{name: "update with a broken recipe", method: http.MethodPut, path: mine, body: `{"type":"movie","name":"X","provider":"tmdb","params":"{"}`, wantStatus: http.StatusBadRequest},
		{name: "update another profile's catalog", method: http.MethodPut, path: theirs, body: valid, wantStatus: http.StatusNotFound, wantBody: "catalog not found"},
		{name: "duplicate", method: http.MethodPost, path: mine + "/duplicate", wantStatus: http.StatusCreated, wantBody: `"name":"Renamed (copy)"`},
		{name: "duplicate with a path id that isn't a uuid", method: http.MethodPost, path: "/api/p/1/catalogs/nope/duplicate", wantStatus: http.StatusBadRequest, wantBody: "invalid catalog id"},
		{name: "duplicate another profile's catalog", method: http.MethodPost, path: theirs + "/duplicate", wantStatus: http.StatusNotFound, wantBody: "catalog not found"},
		{name: "delete another profile's catalog", method: http.MethodDelete, path: theirs, wantStatus: http.StatusNotFound, wantBody: "catalog not found"},
		{name: "delete", method: http.MethodDelete, path: mine, wantStatus: http.StatusNoContent},
		{name: "delete again", method: http.MethodDelete, path: mine, wantStatus: http.StatusNotFound, wantBody: "catalog not found"},
	})

	if _, err := f.db.GetCatalogsByIDs(t.Context(), []uuid.UUID{f.theirs.ID}); err != nil {
		t.Fatalf("another profile's catalog after the steps: %v", err)
	}
}

// TestCollectionRoutes drives the collection CRUD and duplicate routes: a
// new scoped catalog's recipe is checked like a standalone one, a folder
// can't reference another profile's private catalog, and another profile's
// collection answers exactly like one that doesn't exist.
func TestCollectionRoutes(t *testing.T) {
	f := newRouteFixture(t)
	noTMDB(t)

	mine := "/api/p/1/collections/" + f.mineColl.ID.String()
	theirs := "/api/p/1/collections/" + f.theirColl.ID.String()
	withNew := func(params string) string {
		return string(mustJSON(t, vault.CollectionForm{
			Title: "Scoped", ViewMode: "ROWS",
			Folders: []vault.FolderData{{FolderArt: vault.FolderArt{TileShape: "POSTER"}, Title: "F", Catalogs: []vault.FolderCatalogRef{{New: &vault.NewScopedCatalog{
				Key: "s", Type: "movie", Name: "S", Provider: "tmdb", Params: params,
			}}}}},
		}))
	}
	refs := func(id uuid.UUID) vault.CollectionForm {
		return vault.CollectionForm{
			Title: "Refs", ViewMode: "ROWS",
			Folders: []vault.FolderData{{FolderArt: vault.FolderArt{TileShape: "POSTER"}, Title: "F", Catalogs: vault.CatalogRefs(id)}},
		}
	}
	referencing := func(id uuid.UUID) string { return string(mustJSON(t, refs(id))) }

	runSteps(t, f.s, []routeStep{
		{name: "list", method: http.MethodGet, path: "/api/p/1/library", wantStatus: http.StatusOK, wantBody: f.mineColl.ID.String()},
		{name: "community leaves out unpublished collections", method: http.MethodGet, path: "/api/p/1/community?kind=collection&sort=newest", wantStatus: http.StatusOK, wantBody: `{"items":[],"next_cursor":null}`},
		{name: "create with a new scoped catalog", method: http.MethodPost, path: "/api/p/1/collections", body: withNew(popular), wantStatus: http.StatusCreated, wantBody: `"title":"Scoped"`},
		{name: "create with a broken scoped recipe", method: http.MethodPost, path: "/api/p/1/collections", body: withNew(`{"sort_by":"bogus.desc"}`), wantStatus: http.StatusBadRequest},
		{name: "create referencing another profile's private catalog", method: http.MethodPost, path: "/api/p/1/collections", body: referencing(f.theirs.ID), wantStatus: http.StatusBadRequest},
		{name: "create with a malformed body", method: http.MethodPost, path: "/api/p/1/collections", body: `[`, wantStatus: http.StatusBadRequest, wantBody: "invalid request body"},
		{name: "update", method: http.MethodPut, path: mine, body: string(mustJSON(t, collectionSave{CollectionForm: refs(f.mine.ID), Revision: f.mineColl.Revision})), wantStatus: http.StatusOK, wantBody: `"title":"Refs"`},
		{name: "update with a path id that isn't a uuid", method: http.MethodPut, path: "/api/p/1/collections/nope", body: referencing(f.mine.ID), wantStatus: http.StatusBadRequest, wantBody: "invalid collection id"},
		{name: "update another profile's collection", method: http.MethodPut, path: theirs, body: referencing(f.mine.ID), wantStatus: http.StatusNotFound, wantBody: "collection not found"},
		{name: "duplicate", method: http.MethodPost, path: mine + "/duplicate", wantStatus: http.StatusCreated},
		{name: "duplicate another profile's collection", method: http.MethodPost, path: theirs + "/duplicate", wantStatus: http.StatusNotFound, wantBody: "collection not found"},
		{name: "delete another profile's collection", method: http.MethodDelete, path: theirs, wantStatus: http.StatusNotFound, wantBody: "collection not found"},
		{name: "delete", method: http.MethodDelete, path: mine, wantStatus: http.StatusNoContent},
		{name: "delete again", method: http.MethodDelete, path: mine, wantStatus: http.StatusNotFound, wantBody: "collection not found"},
	})
}

// TestDeleteRoutesAllowWhatNuvioHolds deletes a catalog and a collection that
// are on Home: each answers 204 and goes, and the pending push lists both as
// removed, by the names the last push left, until a push takes them off.
func TestDeleteRoutesAllowWhatNuvioHolds(t *testing.T) {
	f := newRouteFixture(t)
	record, err := f.db.BuildPushRecord(t.Context(), f.caller.ID,
		vault.PushedHome{Catalogs: []vault.SelectedCatalogInput{{CatalogID: f.mine.ID, ShowInHome: true}}, Collections: []vault.SelectedCollectionInput{{CollectionID: f.mineColl.ID}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.SavePush(t.Context(), f.caller.ID, record); err != nil {
		t.Fatal(err)
	}
	runSteps(t, f.s, []routeStep{
		{name: "nothing is waiting after the push", method: http.MethodGet, path: "/api/p/1/library", wantStatus: http.StatusOK, wantBody: "[]"},
		{name: "delete a catalog on Home", method: http.MethodDelete, path: "/api/p/1/catalogs/" + f.mine.ID.String(), wantStatus: http.StatusNoContent},
		{name: "delete a collection on Home", method: http.MethodDelete, path: "/api/p/1/collections/" + f.mineColl.ID.String(), wantStatus: http.StatusNoContent},
		{name: "the catalog's removal waits for a push", method: http.MethodGet, path: "/api/p/1/library", wantStatus: http.StatusOK,
			wantBody: fmt.Sprintf(`{"kind":"catalog","id":%q,"name":"Mine","change":"removed"}`, f.mine.ID)},
		{name: "the collection's removal waits for a push", method: http.MethodGet, path: "/api/p/1/library", wantStatus: http.StatusOK,
			wantBody: fmt.Sprintf(`{"kind":"collection","id":%q,"name":"Mine","change":"removed"}`, f.mineColl.ID)},
	})
}

// TestRecipeRoutesReachTMDB covers the routes whose answer comes from TMDB:
// preview and genre options answer with what it returns and 502 when it is
// down, and a catalog whose recipe can't be checked against it is a 502
// rather than a 400, since nothing has found fault with the recipe.
func TestRecipeRoutesReachTMDB(t *testing.T) {
	f := newRouteFixture(t)
	withGenres := `{"type":"movie","name":"Action","provider":"tmdb","params":"{\"with_genres\":\"28\"}"}`

	tests := []struct {
		tmdb http.HandlerFunc
		routeStep
	}{
		{tmdbUp, routeStep{name: "preview", method: http.MethodPost, path: "/api/catalogs/preview", body: `{"type":"movie","params":"{}"}`, wantStatus: http.StatusOK, wantBody: `"total_results":1`}},
		{tmdbDown, routeStep{name: "preview with TMDB down", method: http.MethodPost, path: "/api/catalogs/preview", body: `{"type":"movie","params":"{}"}`, wantStatus: http.StatusBadGateway, wantBody: "failed to reach TMDB"}},
		{tmdbUp, routeStep{name: "genre options", method: http.MethodPost, path: "/api/catalogs/genre-options", body: `{"type":"movie","params":"{\"with_genres\":\"28\"}"}`, wantStatus: http.StatusOK, wantBody: `"name":"Crime"`}},
		{tmdbDown, routeStep{name: "genre options with TMDB down", method: http.MethodPost, path: "/api/catalogs/genre-options", body: `{"type":"movie","params":"{}"}`, wantStatus: http.StatusBadGateway, wantBody: "failed to fetch genres"}},
		{tmdbDown, routeStep{name: "create with TMDB down", method: http.MethodPost, path: "/api/p/1/catalogs", body: withGenres, wantStatus: http.StatusBadGateway, wantBody: "failed to reach TMDB"}},
		{tmdbUp, routeStep{name: "create checked against TMDB", method: http.MethodPost, path: "/api/p/1/catalogs", body: withGenres, wantStatus: http.StatusCreated}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			hits := fakeTMDB(t, tc.tmdb)
			// A fresh client for each case, so no case answers from a list
			// another one cached.
			f.s.provider = provider.NewTMDBClient("key")
			w := serve(t, f.s, tc.method, tc.path, tc.body, false)
			if w.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d (body %q)", w.Code, tc.wantStatus, w.Body.String())
			}
			if !strings.Contains(answerText(w), tc.wantBody) {
				t.Fatalf("body = %q, want it to contain %q", answerText(w), tc.wantBody)
			}
			if hits.Load() == 0 {
				t.Fatal("TMDB was never called")
			}
		})
	}
}

// TestRecipeRoutesRefuseWithoutTMDB covers preview's and genre options'
// refusals, which never reach TMDB.
func TestRecipeRoutesRefuseWithoutTMDB(t *testing.T) {
	f := newRouteFixture(t)
	noTMDB(t)

	var steps []routeStep
	for _, path := range []string{"/api/catalogs/preview", "/api/catalogs/genre-options"} {
		steps = append(steps,
			routeStep{name: path + " unauthenticated", method: http.MethodPost, path: path, body: `{"type":"movie","params":"{}"}`, noAuth: true, wantStatus: http.StatusUnauthorized},
			routeStep{name: path + " malformed body", method: http.MethodPost, path: path, body: `{`, wantStatus: http.StatusBadRequest, wantBody: "invalid request body"},
			routeStep{name: path + " unknown type", method: http.MethodPost, path: path, body: `{"type":"anime","params":"{}"}`, wantStatus: http.StatusBadRequest, wantBody: "unknown catalog type"},
			routeStep{name: path + " broken recipe", method: http.MethodPost, path: path, body: `{"type":"movie","params":"{\"sort_by\":\"bogus.desc\"}"}`, wantStatus: http.StatusBadRequest},
		)
	}
	runSteps(t, f.s, steps)
}

// TestProfileRoutes covers listing the caller's Nuvio profiles and selecting
// one: selection binds a vault profile only for an index the caller's own
// account holds, and a Nuvio failure is a 502 where anything else is a 500.
func TestProfileRoutes(t *testing.T) {
	profiles := []nuvio.NuvioProfile{
		{ID: "nuvio-1", UserID: "test-sub", ProfileIndex: 1, Name: "Main"},
		{ID: "nuvio-2", UserID: "test-sub", ProfileIndex: 2, Name: "Kids"},
		{ID: "nuvio-3", UserID: "someone-else", ProfileIndex: 3, Name: "Not yours"},
	}
	nuvioDown := fmt.Errorf("%w: status 503", nuvio.ErrNuvioRequestFailed)

	tests := []struct {
		fake *fakeNuvio
		routeStep
		wantSlot int // a profile slot the call must leave provisioned, or 0
	}{
		{fake: &fakeNuvio{profiles: profiles}, routeStep: routeStep{name: "list", method: http.MethodGet, path: "/api/profiles", wantStatus: http.StatusOK, wantBody: `"name":"Kids"`}},
		{fake: &fakeNuvio{profiles: []nuvio.NuvioProfile{}}, routeStep: routeStep{name: "list with none", method: http.MethodGet, path: "/api/profiles", wantStatus: http.StatusOK, wantBody: "[]"}},
		{fake: &fakeNuvio{listProfilesErr: nuvioDown}, routeStep: routeStep{name: "list with Nuvio down", method: http.MethodGet, path: "/api/profiles", wantStatus: http.StatusBadGateway, wantBody: "nuvio unavailable"}},
		{fake: &fakeNuvio{listProfilesErr: errors.New("boom")}, routeStep: routeStep{name: "list failing otherwise", method: http.MethodGet, path: "/api/profiles", wantStatus: http.StatusInternalServerError, wantBody: "failed to list profiles"}},
		{fake: &fakeNuvio{profiles: profiles}, routeStep: routeStep{name: "list unauthenticated", method: http.MethodGet, path: "/api/profiles", noAuth: true, wantStatus: http.StatusUnauthorized}},
		{fake: &fakeNuvio{profiles: profiles}, routeStep: routeStep{name: "select", method: http.MethodPost, path: "/api/profiles/select", body: `{"profile_index":2}`, wantStatus: http.StatusOK, wantBody: `{"manifest_url":"http://example.com/u/`}, wantSlot: 2},
		{fake: &fakeNuvio{profiles: profiles}, routeStep: routeStep{name: "select an index the account doesn't have", method: http.MethodPost, path: "/api/profiles/select", body: `{"profile_index":5}`, wantStatus: http.StatusBadRequest, wantBody: "profile index not found"}},
		{fake: &fakeNuvio{profiles: profiles}, routeStep: routeStep{name: "select another account's profile", method: http.MethodPost, path: "/api/profiles/select", body: `{"profile_index":3}`, wantStatus: http.StatusBadRequest, wantBody: "profile index not found on this account"}},
		{fake: &fakeNuvio{profiles: profiles}, routeStep: routeStep{name: "select with a malformed body", method: http.MethodPost, path: "/api/profiles/select", body: `{`, wantStatus: http.StatusBadRequest, wantBody: "invalid request body"}},
		{fake: &fakeNuvio{listProfilesErr: nuvioDown}, routeStep: routeStep{name: "select with Nuvio down", method: http.MethodPost, path: "/api/profiles/select", body: `{"profile_index":1}`, wantStatus: http.StatusBadGateway, wantBody: "nuvio unavailable"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db := newTestVaultDB(t)
			s := newTestServer(t, db, tc.fake)
			w := serve(t, s, tc.method, tc.path, tc.body, tc.noAuth)
			if w.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d (body %q)", w.Code, tc.wantStatus, w.Body.String())
			}
			if !strings.Contains(answerText(w), tc.wantBody) {
				t.Fatalf("body = %q, want it to contain %q", answerText(w), tc.wantBody)
			}
			for slot := 1; slot <= 3; slot++ {
				_, err := db.GetProfileBySlot(t.Context(), "test-sub", slot)
				if provisioned := err == nil; provisioned != (slot == tc.wantSlot) {
					t.Fatalf("slot %d provisioned = %v (%v), want %v", slot, provisioned, err, slot == tc.wantSlot)
				}
			}
		})
	}
}

// GET /api/profiles names the picture each profile shows: its own upload,
// else its built-in avatar's image, else none. Nuvio's avatar list is asked
// for only when a profile uses a built-in avatar, and a failure to read it
// leaves every profile its colour rather than failing the list.
func TestProfileListCarriesAvatarImages(t *testing.T) {
	uploaded := nuvio.NuvioProfile{ID: "n1", UserID: "test-sub", ProfileIndex: 1, Name: "Up", AvatarURL: "https://img.example/up.png", AvatarID: "avatar_lalo"}
	builtIn := nuvio.NuvioProfile{ID: "n2", UserID: "test-sub", ProfileIndex: 2, Name: "Built", AvatarID: "avatar_lalo"}
	plain := nuvio.NuvioProfile{ID: "n3", UserID: "test-sub", ProfileIndex: 3, Name: "Plain", AvatarColorHex: "#1E88E5"}
	lalo := map[string]string{"avatar_lalo": "https://nuvio.example/storage/v1/object/public/avatars/animals/bram-v1.png"}

	tests := []struct {
		name      string
		fake      *fakeNuvio
		wantCalls int
		want      []string
	}{
		{"uploads and built-in avatars", &fakeNuvio{profiles: []nuvio.NuvioProfile{uploaded, builtIn, plain}, avatarImages: lalo}, 1,
			[]string{"https://img.example/up.png", lalo["avatar_lalo"], ""}},
		{"no built-in avatar in use", &fakeNuvio{profiles: []nuvio.NuvioProfile{plain}}, 0, []string{""}},
		{"the avatar list failing", &fakeNuvio{profiles: []nuvio.NuvioProfile{builtIn}, avatarImagesErr: errors.New("boom")}, 1, []string{""}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestServer(t, newTestVaultDB(t), tc.fake)
			w := serve(t, s, http.MethodGet, "/api/profiles", "", false)
			if w.Code != http.StatusOK {
				t.Fatalf("status = %d (%s)", w.Code, w.Body.String())
			}
			var got []pickerProfile
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			images := make([]string, len(got))
			for i, p := range got {
				images[i] = p.AvatarImageURL
			}
			if !slices.Equal(images, tc.want) || tc.fake.avatarCalls != tc.wantCalls {
				t.Errorf("images = %q, avatar list read %d times; want %q and %d", images, tc.fake.avatarCalls, tc.want, tc.wantCalls)
			}
		})
	}
}

// The health probe needs no auth and reads nothing.
func TestHealth(t *testing.T) {
	db := newTestVaultDB(t)
	s := newProfileTestServer(t, db)
	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	w := serve(t, s, http.MethodGet, "/api/health", "", true)
	if w.Code != http.StatusOK || w.Body.String() != "ok" {
		t.Fatalf("GET /api/health = %d %q, want 200 \"ok\"", w.Code, w.Body.String())
	}
}

// Every failure the builder API answers, whichever layer refuses it, is JSON
// with an error field: a path no route serves, an unauthenticated call, a
// malformed id, a missing row, a route in the wrong key mode.
func TestEveryAPIErrorIsJSON(t *testing.T) {
	f := newRouteFixture(t)
	for _, c := range []struct {
		name, method, path string
		noAuth             bool
		status             int
	}{
		{"unknown path", http.MethodGet, "/api/nope", false, http.StatusNotFound},
		{"unauthenticated", http.MethodGet, "/api/profiles", true, http.StatusUnauthorized},
		{"bad profile index", http.MethodGet, "/api/p/9/library", false, http.StatusBadRequest},
		{"malformed id", http.MethodDelete, "/api/p/1/catalogs/nope", false, http.StatusBadRequest},
		{"missing row", http.MethodDelete, "/api/p/1/catalogs/" + uuid.NewString(), false, http.StatusNotFound},
		{"TMDB key route in shared mode", http.MethodGet, "/api/account/tmdb-key", false, http.StatusNotFound},
		{"malformed TMDB id", http.MethodGet, "/api/companies/abc", false, http.StatusBadRequest},
	} {
		t.Run(c.name, func(t *testing.T) {
			w := serve(t, f.s, c.method, c.path, "", c.noAuth)
			var body httpx.ErrorBody
			if w.Code != c.status || w.Header().Get("Content-Type") != "application/json" ||
				json.Unmarshal(w.Body.Bytes(), &body) != nil || body.Error == "" {
				t.Errorf("%s %s = %d %q (%s), want %d with a JSON error", c.method, c.path, w.Code, w.Body.String(), w.Header().Get("Content-Type"), c.status)
			}
		})
	}
}

// A path under /api no route serves is a 404, whatever its method, never the
// SPA's index.html.
func TestUnknownAPIPathIsNotFound(t *testing.T) {
	s := newProfileTestServer(t, newTestVaultDB(t))
	for _, c := range []struct{ method, path string }{
		{http.MethodGet, "/api/nope"},
		{http.MethodDelete, "/api/health"},
		{http.MethodPost, "/api/p/1/library"},
	} {
		w := serve(t, s, c.method, c.path, "", true)
		if w.Code != http.StatusNotFound || strings.Contains(w.Body.String(), "<html") {
			t.Errorf("%s %s = %d %q, want 404 without the SPA", c.method, c.path, w.Code, w.Body.String())
		}
	}
}

// New refuses Deps missing any dependency a request would otherwise panic
// on, and an empty SiteBaseURL, which would push a relative manifest URL.
func TestNewRequiresEveryDependency(t *testing.T) {
	complete := func() Deps {
		return Deps{
			Vault:        newTestVaultDB(t),
			Provider:     provider.NewTMDBClient("key"),
			Verifier:     acceptAnyToken{},
			Nuvio:        &fakeNuvio{},
			SiteBaseURL:  "http://example.com",
			NuvioBaseURL: "https://nuvio.example.com",
		}
	}
	if _, err := New(complete()); err != nil {
		t.Fatalf("New(complete Deps): %v", err)
	}

	for name, strip := range map[string]func(*Deps){
		"Vault":       func(d *Deps) { d.Vault = nil },
		"Provider":    func(d *Deps) { d.Provider = nil },
		"Verifier":    func(d *Deps) { d.Verifier = nil },
		"Nuvio":       func(d *Deps) { d.Nuvio = nil },
		"SiteBaseURL": func(d *Deps) { d.SiteBaseURL = "" },
	} {
		d := complete()
		strip(&d)
		if _, err := New(d); err == nil || !strings.Contains(err.Error(), name) {
			t.Errorf("New without %s: err = %v, want one naming it", name, err)
		}
	}
}

// Every route that takes a body decodes it strictly: a key the request type
// doesn't have, at the top level or inside a folder, is a 400 naming it, so a
// misspelled field never saves as its zero value.
func TestRoutesRefuseAnUnknownField(t *testing.T) {
	f := newRouteFixture(t)
	noTMDB(t)
	mine := "/api/p/1/catalogs/" + f.mine.ID.String()
	collection := "/api/p/1/collections/" + f.mineColl.ID.String()
	steps := []routeStep{
		{"create a catalog", http.MethodPost, "/api/p/1/catalogs", `{"type":"movie","name":"X","provider":"tmdb","params":"{}","colour":"red"}`, false, 0, `unknown field "colour"`},
		{"update a catalog", http.MethodPut, mine, `{"type":"movie","name":"X","provider":"tmdb","params":"{}","colour":"red"}`, false, 0, `unknown field "colour"`},
		{"create a collection", http.MethodPost, "/api/p/1/collections", `{"title":"T","focus_glow_enable":true,"folders":[]}`, false, 0, `unknown field "focus_glow_enable"`},
		{"update a collection", http.MethodPut, collection, `{"title":"T","focus_glow_enable":true,"folders":[]}`, false, 0, `unknown field "focus_glow_enable"`},
		{"a folder of a collection", http.MethodPost, "/api/p/1/collections", `{"title":"T","folders":[{"title":"F","focus_gif_enable":true,"catalogs":[]}]}`, false, 0, `unknown field "focus_gif_enable"`},
		{"a catalog ref of a folder", http.MethodPost, "/api/p/1/collections", `{"title":"T","folders":[{"title":"F","catalogs":[{"catalog_id":"` + f.mine.ID.String() + `","genres":"Action"}]}]}`, false, 0, `unknown field "genres"`},
		{"select a profile", http.MethodPost, "/api/profiles/select", `{"profile_index":1,"index":1}`, false, 0, `unknown field "index"`},
		{"preview a recipe", http.MethodPost, "/api/catalogs/preview", `{"type":"movie","params":"{}","gnre":"x"}`, false, 0, `unknown field "gnre"`},
		{"genre options", http.MethodPost, "/api/catalogs/genre-options", `{"type":"movie","params":"{}","gnre":"x"}`, false, 0, `unknown field "gnre"`},
		{"export", http.MethodPost, "/api/p/1/export", `{"catalog_ids":[],"collection":[]}`, false, 0, `unknown field "collection"`},
		{"check an import", http.MethodPost, "/api/p/1/import/check", `{"bundle":{},"reuse":{}}`, false, 0, `unknown field "reuse"`},
		{"import", http.MethodPost, "/api/p/1/import", `{"bundle":{},"skip":[]}`, false, 0, `unknown field "skip"`},
		{"push", http.MethodPost, "/api/p/1/push", `{"rows":[],"row":[]}`, false, 0, `unknown field "row"`},
	}
	for i := range steps {
		steps[i].wantStatus = http.StatusBadRequest
	}
	runSteps(t, f.s, steps)
}

// A profile slot the caller never selected answers 404 with a JSON body whose
// code is profile_not_found, which is how the builder tells it from a route's
// own 404, an ordinary answer with no code.
func TestProfileNotFoundCarriesACode(t *testing.T) {
	f := newRouteFixture(t)

	w := serve(t, f.s, http.MethodGet, "/api/p/2/library", "", false)
	var body struct{ Error, Code string }
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("body %q is not JSON: %v", w.Body.String(), err)
	}
	if w.Code != http.StatusNotFound || body.Code != "profile_not_found" || body.Error != "profile not found" {
		t.Errorf("unselected slot = %d %+v, want 404 {profile not found, profile_not_found}", w.Code, body)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	w = serve(t, f.s, http.MethodDelete, "/api/p/1/catalogs/"+uuid.NewString(), "", false)
	if w.Code != http.StatusNotFound {
		t.Fatalf("deleting a missing catalog = %d, want 404", w.Code)
	}
	var own httpx.ErrorBody
	if err := json.Unmarshal(w.Body.Bytes(), &own); err != nil || own.Error != "catalog not found" || own.Code != "" {
		t.Errorf("a route's own 404 = %q (%v), want {catalog not found} with no code", w.Body.String(), err)
	}
}

// Selecting a profile that belongs to another account is answered exactly as
// selecting an index the account doesn't have, so the answer says nothing of
// the other account.
func TestSelectingAnotherAccountsProfileSaysNothingOfIt(t *testing.T) {
	s := newTestServer(t, newTestVaultDB(t), &fakeNuvio{profiles: []nuvio.NuvioProfile{
		{ID: "nuvio-3", UserID: "someone-else", ProfileIndex: 3, Name: "Not yours"},
	}})
	other := serve(t, s, http.MethodPost, "/api/profiles/select", `{"profile_index":3}`, false)
	missing := serve(t, s, http.MethodPost, "/api/profiles/select", `{"profile_index":5}`, false)
	if other.Code != http.StatusBadRequest || other.Body.String() != missing.Body.String() {
		t.Errorf("another account's profile = %d %q, an absent index = %d %q; want the same answer",
			other.Code, other.Body.String(), missing.Code, missing.Body.String())
	}
}

// TestRoutesNameTheirTMDBCaller pins that a signed-in route pacing its TMDB
// calls names the account as their caller.
func TestRoutesNameTheirTMDBCaller(t *testing.T) {
	f := newRouteFixture(t)
	var (
		mu      sync.Mutex
		callers []string
	)
	fakeTMDB(t, func(w http.ResponseWriter, r *http.Request) {
		caller, _ := provider.CallerFrom(r.Context())
		mu.Lock()
		callers = append(callers, caller)
		mu.Unlock()
		tmdbUp(w, r)
	})
	f.s.provider = provider.NewTMDBClient("key")

	w := serve(t, f.s, http.MethodPost, "/api/catalogs/preview", `{"type":"movie","params":"{}"}`, false)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body %q)", w.Code, http.StatusOK, w.Body.String())
	}
	mu.Lock()
	defer mu.Unlock()
	if len(callers) == 0 {
		t.Fatal("TMDB was never called")
	}
	for i, got := range callers {
		if got != "account:test-sub" {
			t.Errorf("TMDB request %d made for caller %q, want %q", i, got, "account:test-sub")
		}
	}
}
