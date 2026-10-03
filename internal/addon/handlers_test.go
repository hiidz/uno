package addon

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

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

// tmdbUp serves one discover page holding one film, its IMDB id, and a
// two-genre list. A later page is empty, as TMDB answers past the last.
func tmdbUp(w http.ResponseWriter, r *http.Request) {
	switch {
	case strings.HasPrefix(r.URL.Path, "/3/discover/") && r.URL.Query().Get("page") != "1":
		fmt.Fprint(w, `{"results":[],"total_results":1,"total_pages":1}`)
	case strings.HasPrefix(r.URL.Path, "/3/discover/"):
		fmt.Fprint(w, `{"results":[{"id":155,"title":"The Dark Knight","genre_ids":[80],"release_date":"2008-07-16"}],"total_results":1,"total_pages":1}`)
	case strings.HasSuffix(r.URL.Path, "/external_ids"):
		fmt.Fprint(w, `{"imdb_id":"tt0468569"}`)
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

// handlerFixture is two profiles' published libraries: the owner's home
// catalog, an unpublished one and a deleted one, the other profile's home
// catalog and a deleted one, and a third profile that publishes nothing.
type handlerFixture struct {
	db                          *vault.DB
	owner, other, empty         vault.Profile
	onHome, unpublished, theirs vault.Catalog
	deleted, theirsDeleted      vault.Catalog
}

func newHandlerFixture(t *testing.T) handlerFixture {
	t.Helper()
	ctx := t.Context()
	f := handlerFixture{db: newTestVault(t)}
	profile := func(sub string) vault.Profile {
		p, err := f.db.ResolveOrCreateProfile(ctx, sub, 1, "nuvio-profile-"+sub)
		if err != nil {
			t.Fatalf("creating profile %s: %v", sub, err)
		}
		return p
	}
	catalog := func(p vault.Profile, name string) vault.Catalog {
		c, err := f.db.CreateUserCatalog(ctx, p.ID, listedCatalogForm(name))
		if err != nil {
			t.Fatalf("creating catalog %s: %v", name, err)
		}
		return c
	}
	publish := func(p vault.Profile, c vault.Catalog) {
		savePush(t, f.db, p.ID, vault.CatalogSelectionForm{Catalogs: []vault.SelectedCatalogInput{{CatalogID: c.ID, ShowInHome: true}}}, vault.CollectionSelectionForm{})
	}

	f.owner, f.other, f.empty = profile("owner"), profile("other"), profile("empty")
	f.onHome, f.unpublished, f.theirs = catalog(f.owner, "On home"), catalog(f.owner, "Unpublished"), catalog(f.other, "Theirs")
	publish(f.owner, f.onHome)
	publish(f.other, f.theirs)
	f.deleted, f.theirsDeleted = catalog(f.owner, "Deleted"), catalog(f.other, "Theirs deleted")
	for _, d := range []struct {
		p vault.Profile
		c vault.Catalog
	}{{f.owner, f.deleted}, {f.other, f.theirsDeleted}} {
		if err := f.db.DeleteUserCatalog(ctx, d.p.ID, d.c.ID); err != nil {
			t.Fatalf("deleting %s: %v", d.c.Name, err)
		}
	}
	return f
}

// get serves path through the addon routes as internal/api registers them,
// Public wrapper included, with a fresh TMDB client so no test inherits
// another's cached lists.
func (f handlerFixture) get(t *testing.T, path string) *httptest.ResponseRecorder {
	t.Helper()
	s, err := New(f.db, provider.NewTMDBClient("test-key"), nil, "https://uno.example")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+ManifestPathPattern, s.Public(s.ManifestHandler))
	mux.HandleFunc("GET /u/{token}/catalog/{type}/{rest...}", s.Public(s.CatalogHandler))
	mux.HandleFunc("GET "+ConfigurePathPattern, s.Public(s.ConfigureHandler))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil))
	return w
}

// TestManifestHandler covers the manifest route: an unknown token is a 404
// rather than an empty manifest, a profile's published catalogs are listed
// with the genre names TMDB offers, a TMDB outage drops the names but not the
// manifest, and a profile with nothing published gets an empty catalog list
// with no TMDB call.
func TestManifestHandler(t *testing.T) {
	f := newHandlerFixture(t)

	tests := []struct {
		name       string
		token      string
		tmdb       http.HandlerFunc
		wantStatus int
		wantTMDB   bool
		wantGenres []string // the one listed catalog's genre options, when one is
	}{
		{name: "unknown token", token: "no-such-token", tmdb: tmdbUp, wantStatus: http.StatusNotFound},
		{name: "published catalog", token: f.owner.Token, tmdb: tmdbUp, wantStatus: http.StatusOK, wantTMDB: true, wantGenres: []string{"Action", "Crime"}},
		{name: "TMDB down", token: f.owner.Token, tmdb: tmdbDown, wantStatus: http.StatusOK, wantTMDB: true},
		{name: "nothing published", token: f.empty.Token, tmdb: tmdbUp, wantStatus: http.StatusOK},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			hits := fakeTMDB(t, tc.tmdb)
			w := f.get(t, ManifestPath(tc.token))

			if w.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d (body %q)", w.Code, tc.wantStatus, w.Body.String())
			}
			if got := hits.Load() > 0; got != tc.wantTMDB {
				t.Fatalf("TMDB called = %v, want %v", got, tc.wantTMDB)
			}
			if w.Code != http.StatusOK {
				return
			}
			var m manifest
			if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
				t.Fatalf("decoding manifest %q: %v", w.Body.String(), err)
			}
			if m.ID != ID {
				t.Fatalf("manifest id = %q, want %q", m.ID, ID)
			}
			if m.Logo != "https://uno.example/logo.png" || !m.BehaviorHints.Configurable {
				t.Fatalf("logo = %q, configurable = %v; want the site's logo.png and true", m.Logo, m.BehaviorHints.Configurable)
			}
			if tc.token == f.empty.Token {
				if len(m.Catalogs) != 0 || !strings.Contains(w.Body.String(), `"catalogs":[]`) {
					t.Fatalf("body = %s, want an empty catalogs array", w.Body.String())
				}
				return
			}
			if len(m.Catalogs) != 1 || m.Catalogs[0].ID != ManifestID(f.onHome) || !m.Catalogs[0].ShowInHome {
				t.Fatalf("catalogs = %+v, want only %q on home", m.Catalogs, ManifestID(f.onHome))
			}
			var genres []string
			for _, e := range m.Catalogs[0].Extra {
				if e.Name == "genre" {
					genres = e.Options
				}
			}
			if strings.Join(genres, ",") != strings.Join(tc.wantGenres, ",") {
				t.Fatalf("genre options = %v, want %v", genres, tc.wantGenres)
			}
		})
	}
}

// TestConfigureHandler: the addon's Configure route, whatever the token,
// sends the browser to the builder's profile picker.
func TestConfigureHandler(t *testing.T) {
	f := newHandlerFixture(t)
	for _, token := range []string{f.owner.Token, "no-such-token"} {
		w := f.get(t, "/u/"+token+"/configure")
		if w.Code != http.StatusFound || w.Header().Get("Location") != "/profiles" {
			t.Errorf("token %q: status %d, Location %q; want 302 to /profiles", token, w.Code, w.Header().Get("Location"))
		}
	}
}

// TestCatalogHandler covers the catalog route. It serves a catalog the
// token's profile has on the TV. Its own catalog off the TV or deleted,
// another profile's catalog, live or deleted, a type or provider the catalog
// doesn't have, and an id ManifestID can't have written are the same 404 as
// one that doesn't exist, and are never fetched. A TMDB failure is a 502.
// tmdbSparse serves three discover pages of twenty films, a fifth of them
// without an IMDB id, so each page leaves sixteen titles once those are
// dropped; a later page is empty.
func tmdbSparse(w http.ResponseWriter, r *http.Request) {
	switch {
	case strings.HasPrefix(r.URL.Path, "/3/discover/"):
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		var results []string
		for i := 0; page <= 3 && i < 20; i++ {
			results = append(results, fmt.Sprintf(`{"id":%d,"title":"Film %d"}`, page*100+i, page*100+i))
		}
		fmt.Fprintf(w, `{"results":[%s],"total_results":60,"total_pages":3}`, strings.Join(results, ","))
	case strings.HasSuffix(r.URL.Path, "/external_ids"):
		id, _ := strconv.Atoi(strings.Split(r.URL.Path, "/")[3])
		if id%5 == 0 {
			fmt.Fprint(w, `{"imdb_id":null}`)
			return
		}
		fmt.Fprintf(w, `{"imdb_id":"tt%d"}`, id)
	case strings.HasPrefix(r.URL.Path, "/3/genre/"):
		fmt.Fprint(w, `{"genres":[]}`)
	default:
		http.NotFound(w, r)
	}
}

// A client pages through a catalog by the number of titles it holds, and
// TMDB pages run short once titles without an IMDB id are dropped. Each skip
// gets the next twenty titles, every one exactly once and in order, until
// the catalog runs out: 16 + 16 + 16 titles over three TMDB pages come back
// as 20, 20, 8, then nothing.
func TestCatalogHandlerServesTheTitlesAfterSkip(t *testing.T) {
	f := newHandlerFixture(t)
	fakeTMDB(t, tmdbSparse)

	var all []string
	for _, tc := range []struct{ skip, want int }{{0, 20}, {20, 20}, {40, 8}, {48, 0}} {
		w := f.get(t, "/u/"+f.owner.Token+"/catalog/movie/"+ManifestID(f.onHome)+"/skip="+strconv.Itoa(tc.skip)+".json")
		if w.Code != http.StatusOK {
			t.Fatalf("skip=%d: status %d (%s)", tc.skip, w.Code, w.Body.String())
		}
		var got catalogResponse
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if len(got.Metas) != tc.want {
			t.Fatalf("skip=%d: %d titles, want %d", tc.skip, len(got.Metas), tc.want)
		}
		for _, m := range got.Metas {
			all = append(all, m.ID)
		}
	}

	var want []string
	for page := 1; page <= 3; page++ {
		for i := 0; i < 20; i++ {
			if id := page*100 + i; id%5 != 0 {
				want = append(want, "tt"+strconv.Itoa(id))
			}
		}
	}
	if strings.Join(all, ",") != strings.Join(want, ",") {
		t.Errorf("titles across the pages =\n%v\nwant every kept title once, in order:\n%v", all, want)
	}
}

// titlesFrom is a page from a title on: all of it from before its start,
// none from past its end.
func TestTitlesFrom(t *testing.T) {
	page := []provider.Meta{{ID: "a"}, {ID: "b"}, {ID: "c"}}
	for from, want := range map[int]int{-5: 3, 0: 3, 2: 1, 3: 0, 9: 0} {
		if got := titlesFrom(page, from); len(got) != want {
			t.Errorf("titlesFrom(page, %d) has %d titles, want %d", from, len(got), want)
		}
	}
}

// A randomized recipe picks its page at random, so it has no order to walk.
func TestRandomizedRecipe(t *testing.T) {
	for params, want := range map[string]bool{`{"randomized":true}`: true, `{}`: false, `not json`: false} {
		if got := randomizedRecipe("movie", params); got != want {
			t.Errorf("randomizedRecipe(%s) = %v, want %v", params, got, want)
		}
	}
}

func TestCatalogHandler(t *testing.T) {
	f := newHandlerFixture(t)
	path := func(token, catalogType string, c vault.Catalog) string {
		return "/u/" + token + "/catalog/" + catalogType + "/" + ManifestID(c) + ".json"
	}

	tests := []struct {
		name       string
		path       string
		tmdb       http.HandlerFunc
		wantStatus int
		wantTMDB   bool
	}{
		{name: "published catalog", path: path(f.owner.Token, "movie", f.onHome), tmdb: tmdbUp, wantStatus: http.StatusOK, wantTMDB: true},
		{name: "unknown token", path: path("no-such-token", "movie", f.onHome), tmdb: tmdbUp, wantStatus: http.StatusNotFound},
		{name: "another profile's catalog", path: path(f.owner.Token, "movie", f.theirs), tmdb: tmdbUp, wantStatus: http.StatusNotFound},
		{name: "own catalog off the TV", path: path(f.owner.Token, "movie", f.unpublished), tmdb: tmdbUp, wantStatus: http.StatusNotFound},
		{name: "own deleted catalog", path: path(f.owner.Token, "movie", f.deleted), tmdb: tmdbUp, wantStatus: http.StatusNotFound},
		{name: "another profile's deleted catalog", path: path(f.owner.Token, "movie", f.theirsDeleted), tmdb: tmdbUp, wantStatus: http.StatusNotFound},
		{name: "another provider", path: "/u/" + f.owner.Token + "/catalog/movie/other-" + f.onHome.ID.String() + ".json", tmdb: tmdbUp, wantStatus: http.StatusNotFound},
		{name: "an id in another form", path: "/u/" + f.owner.Token + "/catalog/movie/tmdb-" + strings.ToUpper(f.onHome.ID.String()) + ".json", tmdb: tmdbUp, wantStatus: http.StatusNotFound},
		{name: "no provider", path: "/u/" + f.owner.Token + "/catalog/movie/" + strings.ReplaceAll(f.onHome.ID.String(), "-", "") + ".json", tmdb: tmdbUp, wantStatus: http.StatusNotFound},
		{name: "published catalog under the wrong type", path: path(f.owner.Token, "series", f.onHome), tmdb: tmdbUp, wantStatus: http.StatusNotFound},
		{name: "TMDB down", path: path(f.owner.Token, "movie", f.onHome), tmdb: tmdbDown, wantStatus: http.StatusBadGateway, wantTMDB: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			hits := fakeTMDB(t, tc.tmdb)
			w := f.get(t, tc.path)

			if w.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d (body %q)", w.Code, tc.wantStatus, w.Body.String())
			}
			if got := hits.Load() > 0; got != tc.wantTMDB {
				t.Fatalf("TMDB called = %v, want %v", got, tc.wantTMDB)
			}
			if w.Code != http.StatusOK {
				return
			}
			var got catalogResponse
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
				t.Fatalf("decoding %q: %v", w.Body.String(), err)
			}
			if len(got.Metas) != 1 || got.Metas[0].ID != "tt0468569" || got.Metas[0].Name != "The Dark Knight" {
				t.Fatalf("metas = %+v, want The Dark Knight as tt0468569", got.Metas)
			}
			if got.CacheMaxAge != catalogCacheMaxAge || got.StaleRevalidate != catalogStaleRevalidate {
				t.Fatalf("cache hints = %d/%d, want %d/%d", got.CacheMaxAge, got.StaleRevalidate, catalogCacheMaxAge, catalogStaleRevalidate)
			}
		})
	}
}

// A vault failure on either route is a 500, not the unknown token's 404.
func TestHandlersVaultFailure(t *testing.T) {
	f := newHandlerFixture(t)
	fakeTMDB(t, tmdbUp)
	if err := f.db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	for _, path := range []string{
		ManifestPath(f.owner.Token),
		"/u/" + f.owner.Token + "/catalog/movie/" + ManifestID(f.onHome) + ".json",
	} {
		if w := f.get(t, path); w.Code != http.StatusInternalServerError {
			t.Errorf("GET %s: status = %d, want %d", path, w.Code, http.StatusInternalServerError)
		}
	}
}

// Public opens CORS on every answer, since Stremio clients fetch from any
// origin, and turns a handler panic into a 500 — nothing else on this
// unauthenticated path would recover it.
func TestPublic(t *testing.T) {
	s, err := New(newTestVault(t), provider.NewTMDBClient("test-key"), nil, "https://uno.example")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	for _, tc := range []struct {
		name       string
		handler    http.HandlerFunc
		wantStatus int
	}{
		{"answer", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }, http.StatusNoContent},
		{"panic", func(http.ResponseWriter, *http.Request) { panic("boom") }, http.StatusInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			s.Public(tc.handler)(w, httptest.NewRequest(http.MethodGet, "/u/token/manifest.json", nil))
			if w.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", w.Code, tc.wantStatus)
			}
			if got := w.Header().Get("Access-Control-Allow-Origin"); got != "*" {
				t.Fatalf("Access-Control-Allow-Origin = %q, want *", got)
			}
		})
	}
}

func TestNewRequiresBothDependencies(t *testing.T) {
	if _, err := New(nil, provider.NewTMDBClient("test-key"), nil, ""); err == nil {
		t.Error("New(nil vault) = nil error, want one")
	}
	if _, err := New(newTestVault(t), nil, nil, ""); err == nil {
		t.Error("New(nil provider) = nil error, want one")
	}
}

// The addon serves what the owner's last push put in Nuvio: after the
// catalog on Home is edited and then deleted, its manifest entry and catalog
// route stay as pushed until the next push takes it off.
func TestAddonServesWhatTheLastPushLeft(t *testing.T) {
	ctx := t.Context()
	f := newHandlerFixture(t)
	var discoverQueries []string
	fakeTMDB(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/3/discover/") {
			discoverQueries = append(discoverQueries, r.URL.RawQuery)
		}
		tmdbUp(w, r)
	})
	manifest := "/u/" + f.owner.Token + "/manifest.json"
	route := "/u/" + f.owner.Token + "/catalog/movie/" + ManifestID(f.onHome) + ".json"
	wantServed := func(step string, listed bool, status int) {
		t.Helper()
		if body := f.get(t, manifest).Body.String(); strings.Contains(body, ManifestID(f.onHome)) != listed {
			t.Errorf("%s: manifest lists the catalog = %v, want %v (%s)", step, !listed, listed, body)
		}
		if w := f.get(t, route); w.Code != status {
			t.Errorf("%s: catalog route = %d, want %d", step, w.Code, status)
		}
	}

	edited := listedCatalogForm("Edited")
	edited.Params = `{"sort_by":"vote_average.desc"}`
	if _, err := f.db.UpdateUserCatalog(ctx, f.owner.ID, f.onHome.ID, edited); err != nil {
		t.Fatal(err)
	}
	wantServed("after an edit", true, http.StatusOK)
	if body := f.get(t, manifest).Body.String(); !strings.Contains(body, `"name":"On home"`) || strings.Contains(body, "Edited") {
		t.Errorf("manifest after an edit = %s, want the name as pushed", body)
	}
	for _, q := range discoverQueries {
		if strings.Contains(q, "vote_average") {
			t.Errorf("discover query %q uses the edited recipe before a push", q)
		}
	}

	if err := f.db.DeleteUserCatalog(ctx, f.owner.ID, f.onHome.ID); err != nil {
		t.Fatalf("deleting a catalog on Home: %v", err)
	}
	wantServed("after a delete", true, http.StatusOK)

	savePush(t, f.db, f.owner.ID, vault.CatalogSelectionForm{}, vault.CollectionSelectionForm{})
	wantServed("after the next push", false, http.StatusNotFound)
}
