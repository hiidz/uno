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
	"github.com/hiidz/uno/internal/provider"
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

// A catalog only reachable through a folder of an on-TV collection is
// published with the required genre extra (Stremio's mechanism for keeping
// it out of home's automatic rows).
func TestBuildManifestFolderOnlyCatalogGetsGenreExtra(t *testing.T) {
	ctx := context.Background()
	db := newTestVault(t)

	owner, err := db.ResolveOrCreateProfile(ctx, "owner", 1, "nuvio-profile-owner")
	if err != nil {
		t.Fatalf("creating profile: %v", err)
	}

	folderOnly, err := db.CreateUserCatalog(ctx, owner.ID, listedCatalogForm("Folder only"))
	if err != nil {
		t.Fatalf("create catalog: %v", err)
	}

	collection, err := db.CreateUserCollection(ctx, owner.ID, vault.CollectionForm{Title: "On TV", ViewMode: "TABBED_GRID"})
	if err != nil {
		t.Fatalf("create collection: %v", err)
	}
	if _, err := db.UpdateUserCollection(ctx, owner.ID, collection.ID, collection.Revision, vault.CollectionForm{
		Title: "On TV", ViewMode: "TABBED_GRID",
		Folders: []vault.FolderData{{FolderArt: vault.FolderArt{TileShape: "POSTER"}, Title: "Folder", Catalogs: vault.CatalogRefs(folderOnly.ID)}},
	}); err != nil {
		t.Fatalf("saving collection: %v", err)
	}

	savePush(t, db, owner.ID, vault.PushedHome{Collections: []vault.SelectedCollectionInput{{CollectionID: collection.ID}}})

	selection, err := db.GetPublishedCatalogs(ctx, owner.ID)
	if err != nil {
		t.Fatalf("GetPublishedCatalogs: %v", err)
	}

	m := buildManifest(selection, actionComedy)
	var mc *manifestCatalog
	for i := range m.Catalogs {
		if m.Catalogs[i].ID == vault.ManifestID(folderOnly) {
			mc = &m.Catalogs[i]
		}
	}
	if mc == nil {
		t.Fatalf("manifest catalogs = %+v, want an entry for the folder-only catalog", m.Catalogs)
	}
	var hasRequiredGenre bool
	for _, e := range mc.Extra {
		if e.Name == "genre" && e.IsRequired {
			hasRequiredGenre = true
		}
	}
	if !hasRequiredGenre {
		t.Fatalf("folder-only catalog's extras = %+v, want a required genre extra", mc.Extra)
	}
}

// A catalog both on the home screen and inside a folder of an on-TV
// collection appears exactly once in the manifest, carrying its home
// ShowInHome (no genre extra) rather than the folder-derived false.
func TestBuildManifestHomeAndFolderCatalogAppearsOnceWithHomeShowInHome(t *testing.T) {
	ctx := context.Background()
	db := newTestVault(t)

	owner, err := db.ResolveOrCreateProfile(ctx, "owner", 1, "nuvio-profile-owner")
	if err != nil {
		t.Fatalf("creating profile: %v", err)
	}

	catalog, err := db.CreateUserCatalog(ctx, owner.ID, listedCatalogForm("Home and folder"))
	if err != nil {
		t.Fatalf("create catalog: %v", err)
	}

	collection, err := db.CreateUserCollection(ctx, owner.ID, vault.CollectionForm{Title: "On TV", ViewMode: "TABBED_GRID"})
	if err != nil {
		t.Fatalf("create collection: %v", err)
	}
	if _, err := db.UpdateUserCollection(ctx, owner.ID, collection.ID, collection.Revision, vault.CollectionForm{
		Title: "On TV", ViewMode: "TABBED_GRID",
		Folders: []vault.FolderData{{FolderArt: vault.FolderArt{TileShape: "POSTER"}, Title: "Folder", Catalogs: vault.CatalogRefs(catalog.ID)}},
	}); err != nil {
		t.Fatalf("saving collection: %v", err)
	}

	savePush(t, db, owner.ID, vault.PushedHome{Catalogs: []vault.SelectedCatalogInput{{CatalogID: catalog.ID, ShowInHome: true}}, Collections: []vault.SelectedCollectionInput{{CollectionID: collection.ID}}})

	selection, err := db.GetPublishedCatalogs(ctx, owner.ID)
	if err != nil {
		t.Fatalf("GetPublishedCatalogs: %v", err)
	}

	m := buildManifest(selection, actionComedy)
	var count int
	var mc manifestCatalog
	for _, c := range m.Catalogs {
		if c.ID == vault.ManifestID(catalog) {
			count++
			mc = c
		}
	}
	if count != 1 {
		t.Fatalf("catalog on home and in a folder appeared %d times in the manifest, want 1", count)
	}
	if !mc.ShowInHome {
		t.Fatalf("catalog on home has showInHome false, want true — its home ShowInHome must win")
	}
	for _, e := range mc.Extra {
		if e.Name == "genre" && e.IsRequired {
			t.Fatalf("catalog on home got a required genre extra %+v, want an optional one — its home ShowInHome must win", mc.Extra)
		}
	}
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

// A catalog reachable only through a folder of a collection that is not on
// the TV does not appear in the manifest.
func TestBuildManifestFolderCatalogOfOffTVCollectionDoesNotAppear(t *testing.T) {
	ctx := context.Background()
	db := newTestVault(t)

	owner, err := db.ResolveOrCreateProfile(ctx, "owner", 1, "nuvio-profile-owner")
	if err != nil {
		t.Fatalf("creating profile: %v", err)
	}

	catalog, err := db.CreateUserCatalog(ctx, owner.ID, listedCatalogForm("Off TV"))
	if err != nil {
		t.Fatalf("create catalog: %v", err)
	}

	collection, err := db.CreateUserCollection(ctx, owner.ID, vault.CollectionForm{Title: "Off TV", ViewMode: "TABBED_GRID"})
	if err != nil {
		t.Fatalf("create collection: %v", err)
	}
	if _, err := db.UpdateUserCollection(ctx, owner.ID, collection.ID, collection.Revision, vault.CollectionForm{
		Title: "Off TV", ViewMode: "TABBED_GRID",
		Folders: []vault.FolderData{{FolderArt: vault.FolderArt{TileShape: "POSTER"}, Title: "Folder", Catalogs: vault.CatalogRefs(catalog.ID)}},
	}); err != nil {
		t.Fatalf("saving collection: %v", err)
	}
	// collection is deliberately never pushed onto the home screen.

	selection, err := db.GetPublishedCatalogs(ctx, owner.ID)
	if err != nil {
		t.Fatalf("GetPublishedCatalogs: %v", err)
	}

	m := buildManifest(selection, actionComedy)
	for _, c := range m.Catalogs {
		if c.ID == vault.ManifestID(catalog) {
			t.Fatalf("catalog in a folder of an off-TV collection appeared in the manifest: %+v", c)
		}
	}
}

// A skip at the depth a catalog row ends is answered with an empty page and
// no TMDB call: the provider here is built with a throwaway key, so any call
// at all would surface as a 502.
func TestCatalogHandlerSkipAtServedDepthServesAnEmptyPage(t *testing.T) {
	ctx := context.Background()
	db := newTestVault(t)

	owner, err := db.ResolveOrCreateProfile(ctx, "owner", 1, "nuvio-profile-owner")
	if err != nil {
		t.Fatalf("creating profile: %v", err)
	}

	catalog, err := db.CreateUserCatalog(ctx, owner.ID, listedCatalogForm("On home"))
	if err != nil {
		t.Fatalf("create catalog: %v", err)
	}

	savePush(t, db, owner.ID, vault.PushedHome{Catalogs: []vault.SelectedCatalogInput{
		{CatalogID: catalog.ID, ShowInHome: true},
	}})

	s, err := New(db, provider.NewTMDBClient("test-key"), nil, "https://uno.example")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /u/{token}/catalog/{type}/{rest...}", s.CatalogHandler)

	// Spelled out rather than derived from maxServedTitles, so a change to
	// how deep the public route walks fails here.
	const skipAtServedDepth = 500
	path := "/u/" + owner.Token + "/catalog/movie/" + vault.ManifestID(catalog) +
		"/skip=" + strconv.Itoa(skipAtServedDepth) + ".json"
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body %q", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got catalogResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decoding response %q: %v", rec.Body.String(), err)
	}
	if len(got.Metas) != 0 {
		t.Errorf("metas = %+v, want empty", got.Metas)
	}
	if !strings.Contains(rec.Body.String(), `"metas":[]`) {
		t.Errorf("body = %s, want an empty metas array rather than null", rec.Body.String())
	}
}
