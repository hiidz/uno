package addon

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/hiidz/uno/internal/vault"
)

func newTestVault(t *testing.T) *vault.DB {
	t.Helper()
	db, err := vault.InitDB(filepath.Join(t.TempDir(), "vault.db"))
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// savePush stands in for push's local write of home: it builds its push
// record now and stores it.
func savePush(t *testing.T, db *vault.DB, profileID uuid.UUID, home vault.PushedHome) {
	t.Helper()
	record, err := db.BuildPushRecord(context.Background(), profileID, home)
	if err != nil {
		t.Fatalf("BuildPushRecord: %v", err)
	}
	if _, err := db.SavePush(context.Background(), profileID, record); err != nil {
		t.Fatalf("SavePush: %v", err)
	}
}

func listedCatalogForm(name string) vault.CatalogForm {
	return vault.CatalogForm{Type: "movie", Name: name, Provider: "tmdb", Params: "{}"}
}

// actionComedy stands in for provider.GenreExtraOptions' names, so manifest
// tests don't reach TMDB.
func actionComedy(vault.Catalog) []string { return []string{"Action", "Comedy"} }

func selectedWithParams(params string, showInHome bool) vault.Catalog {
	return vault.Catalog{ID: uuid.New(), Type: "movie", Provider: "tmdb", Params: params, ShowInHome: showInHome}
}

// The genre extra carries the off-home required flag and the genre names in
// one entry, with "All" leading only when required.
func TestGenreExtra(t *testing.T) {
	names := []string{"Action", "Comedy"}

	tests := []struct {
		name       string
		names      []string
		showInHome bool
		want       *manifestExtra
	}{
		{"on home, no names", nil, true, nil},
		{"on home, names", names, true,
			&manifestExtra{Name: "genre", Options: []string{"Action", "Comedy"}, OptionsLimit: 1}},
		{"off home, no names", nil, false,
			&manifestExtra{Name: "genre", IsRequired: true, Options: []string{"All"}, OptionsLimit: 1}},
		{"off home, names", names, false,
			&manifestExtra{Name: "genre", IsRequired: true, Options: []string{"All", "Action", "Comedy"}, OptionsLimit: 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := genreExtra(tt.showInHome, tt.names)
			if tt.want == nil {
				if ok {
					t.Fatalf("genreExtra = %+v, want none", got)
				}
				return
			}
			if !ok || !reflect.DeepEqual(got, *tt.want) {
				t.Fatalf("genreExtra = %+v (ok=%v), want %+v", got, ok, *tt.want)
			}
		})
	}
}

// Extra props are read from the escaped path, so a percent-encoded "&" inside
// a genre name stays part of the name instead of splitting the query.
func TestParseCatalogPath(t *testing.T) {
	tests := []struct {
		path      string
		wantID    string
		wantSkip  int
		wantGenre string
	}{
		{"/u/t/catalog/movie/tmdb-x.json", "tmdb-x", 0, ""},
		{"/u/t/catalog/movie/tmdb-x/skip=40.json", "tmdb-x", 40, ""},
		{"/u/t/catalog/movie/tmdb-x/genre=Action.json", "tmdb-x", 0, "Action"},
		{"/u/t/catalog/series/tmdb-x/genre=Sci-Fi%20%26%20Fantasy&skip=20.json", "tmdb-x", 20, "Sci-Fi & Fantasy"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			var id, genre string
			var skip int
			mux := http.NewServeMux()
			mux.HandleFunc("GET /u/{token}/catalog/{type}/{rest...}", func(_ http.ResponseWriter, r *http.Request) {
				id, skip, genre = parseCatalogPath(r)
			})
			mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, tt.path, nil))
			if id != tt.wantID || skip != tt.wantSkip || genre != tt.wantGenre {
				t.Fatalf("parseCatalogPath = (%q, %d, %q), want (%q, %d, %q)", id, skip, genre, tt.wantID, tt.wantSkip, tt.wantGenre)
			}
		})
	}
}

// showInHome is on the wire even when false: Nuvio TV reads an absent field
// as "show on home".
func TestManifestCatalogShowInHomeIsAlwaysExplicit(t *testing.T) {
	m := buildManifest([]vault.Catalog{selectedWithParams(`{}`, false)}, actionComedy)
	raw, err := json.Marshal(m.Catalogs[0])
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(raw), `"showInHome":false`) {
		t.Fatalf("manifest catalog = %s, want an explicit \"showInHome\":false", raw)
	}
}

// A catalog the push put on Home keeps its home row and an optional genre
// extra; one only a folder uses carries the required genre extra, Stremio's
// way of keeping it out of Home's automatic rows.
func TestBuildManifestKeepsFolderOnlyCatalogsOffHome(t *testing.T) {
	onHome, folderOnly := selectedWithParams(`{}`, true), selectedWithParams(`{}`, false)
	m := buildManifest([]vault.Catalog{onHome, folderOnly}, actionComedy)
	for i, want := range []struct {
		id                     string
		showInHome, isRequired bool
	}{
		{vault.ManifestID(onHome), true, false},
		{vault.ManifestID(folderOnly), false, true},
	} {
		got := m.Catalogs[i]
		required := false
		for _, e := range got.Extra {
			required = required || (e.Name == "genre" && e.IsRequired)
		}
		if got.ID != want.id || got.ShowInHome != want.showInHome || required != want.isRequired {
			t.Errorf("catalog %d = %s showInHome %v, required genre %v; want %s %v %v",
				i, got.ID, got.ShowInHome, required, want.id, want.showInHome, want.isRequired)
		}
	}
}

// A skip at the depth a catalog row ends is answered with an empty page and
// no TMDB call.
func TestCatalogHandlerSkipAtServedDepthServesAnEmptyPage(t *testing.T) {
	f := newHandlerFixture(t)
	hits := fakeTMDB(t, tmdbUp)

	// Spelled out rather than derived from maxServedTitles, so a change to
	// how deep the public route walks fails here.
	const skipAtServedDepth = 500
	w := f.get(t, "/u/"+f.owner.Token+"/catalog/movie/"+vault.ManifestID(f.onHome)+
		"/skip="+strconv.Itoa(skipAtServedDepth)+".json")

	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"metas":[]`) {
		t.Fatalf("answer = %d %s, want 200 with an empty metas array", w.Code, w.Body.String())
	}
	if n := hits.Load(); n != 0 {
		t.Errorf("TMDB called %d times, want none", n)
	}
}
