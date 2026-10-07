package vault

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// readTestBundle decodes testdata/bundle_v1.json: two top-level catalogs
// (c1, c2) and one collection whose own catalog c3 its two folders
// reference, one of them beside c1.
func readTestBundle(t *testing.T) Bundle {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "bundle_v1.json"))
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	var b Bundle
	if err := json.Unmarshal(data, &b); err != nil {
		t.Fatalf("decoding fixture: %v", err)
	}
	return b
}

func requireInvalid(t *testing.T, err error, fragment string) {
	t.Helper()
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err = %v, want it to wrap ErrInvalidInput", err)
	}
	if !strings.Contains(err.Error(), fragment) {
		t.Fatalf("err = %q, want it to contain %q", err, fragment)
	}
}

// The fixture passes, and decoding compacts its params: c1's are spread over
// several lines in the file.
func TestBundleValidateAcceptsFixture(t *testing.T) {
	b := readTestBundle(t)
	if err := b.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if got, want := string(b.Catalogs[0].Params), `{"sort_by":"popularity.desc"}`; got != want {
		t.Fatalf("c1 params = %s, want %s", got, want)
	}
}

// Each file-level rule is its own problem, and every problem found is
// listed in one ErrInvalidInput.
func TestBundleValidateRules(t *testing.T) {
	longKey := strings.Repeat("k", maxBundleKeyLen+1)
	for _, tc := range []struct {
		name   string
		mutate func(b *Bundle)
		want   string
	}{
		{"format", func(b *Bundle) { b.Format = "stremio" }, `format must be "uno"`},
		{"version", func(b *Bundle) { b.Version = 2 }, "version must be 1"},
		{"empty key", func(b *Bundle) { b.Catalogs[1].Key = "" }, "catalog 1: key is required"},
		{"long key", func(b *Bundle) { b.Catalogs[1].Key = longKey }, "catalog 1: key is longer than 64 characters"},
		{"key repeated across lists", func(b *Bundle) { b.Collections[0].Catalogs[0].Key = "c1" },
			`collection 0: catalog 0: key "c1" is used by another catalog`},
		{"null params", func(b *Bundle) { b.Catalogs[0].Params = json.RawMessage("null") }, "catalog 0: params must be a JSON object"},
		{"array params", func(b *Bundle) { b.Catalogs[0].Params = json.RawMessage("[]") }, "catalog 0: params must be a JSON object"},
		{"missing params", func(b *Bundle) { b.Catalogs[0].Params = nil }, "catalog 0: params must be a JSON object"},
		{"unknown ref", func(b *Bundle) { b.Collections[0].Folders[1].Refs[0].Catalog = "c9" },
			`collection 0: folder 1: ref 0: catalog "c9" is neither`},
		{"another collection's key", func(b *Bundle) {
			second := b.Collections[0]
			second.Catalogs = []BundleCatalog{}
			b.Collections = append(b.Collections, second)
		}, `collection 1: folder 0: ref 1: catalog "c3" is neither`},
		{"too many catalogs", func(b *Bundle) {
			for i := range maxBundleCatalogs {
				b.Catalogs = append(b.Catalogs, BundleCatalog{Key: fmt.Sprintf("x%d", i), Params: json.RawMessage("{}")})
			}
		}, "a bundle holds at most 200 catalogs"},
		{"too many collections", func(b *Bundle) {
			for range maxBundleCollections {
				b.Collections = append(b.Collections, BundleCollection{Title: "More"})
			}
		}, "a bundle holds at most 50 collections"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := readTestBundle(t)
			tc.mutate(&b)
			requireInvalid(t, b.Validate(), "bundle: "+tc.want)
		})
	}

	t.Run("every problem at once", func(t *testing.T) {
		b := readTestBundle(t)
		b.Format, b.Catalogs[0].Key = "", ""
		err := b.Validate()
		requireInvalid(t, err, `format must be "uno"`)
		requireInvalid(t, err, "catalog 0: key is required")
	})
}

// exportFixture is one profile's library for the export tests: listed
// catalogs A, B and C, and collection X, whose folder references its own
// scoped catalog S and the listed C and A. Y is a second collection.
type exportFixture struct {
	db         *DB
	owner      uuid.UUID
	a, b, c    Catalog
	x, y       CollectionWithFolders
	scopedName string
}

func newExportFixture(t *testing.T) exportFixture {
	t.Helper()
	ctx := context.Background()
	db := newTestDB(t)
	f := exportFixture{db: db, owner: newTestProfile(t, db, "exporter"), scopedName: "S"}
	f.a = createListed(t, db, f.owner, "A")
	f.b = createListed(t, db, f.owner, "B")
	f.c = createListed(t, db, f.owner, "C")
	x, err := db.CreateUserCollection(ctx, f.owner, CollectionForm{
		Title: "X", ViewMode: "ROWS",
		Folders: []FolderData{{Title: "F", Catalogs: []FolderCatalogRef{
			{New: &NewScopedCatalog{Key: "s", Type: "movie", Name: f.scopedName, Provider: "tmdb", Params: `{"sort_by":"s"}`}},
			{CatalogID: &f.c.ID, Genre: "Drama"},
			{CatalogID: &f.a.ID},
		}}},
	})
	if err != nil {
		t.Fatalf("create X: %v", err)
	}
	f.x = x
	f.y, err = db.CreateUserCollection(ctx, f.owner, CollectionForm{Title: "Y"})
	if err != nil {
		t.Fatalf("create Y: %v", err)
	}
	return f
}

// createListed creates a listed movie catalog named name.
func createListed(t *testing.T, db *DB, owner uuid.UUID, name string) Catalog {
	t.Helper()
	params := `{"sort_by":"` + name + `"}`
	c, err := db.CreateUserCatalog(context.Background(), owner, CatalogForm{
		Type: "movie", Name: name, Provider: "tmdb", Params: params,
	})
	if err != nil {
		t.Fatalf("create %s: %v", name, err)
	}
	return c
}

// reusedFrom is the catalog of b under key, top-level or a collection's own.
func reusedFrom(b Bundle, key string) BundleCatalog {
	for _, c := range append(slices.Clone(b.Catalogs), b.Collections[0].Catalogs...) {
		if c.Key == key {
			return c
		}
	}
	panic("no bundle catalog " + key)
}

// createLike creates a listed catalog named name holding the recipe of like.
func createLike(t *testing.T, db *DB, owner uuid.UUID, name string, like BundleCatalog) Catalog {
	t.Helper()
	c, err := db.CreateUserCatalog(context.Background(), owner, CatalogForm{
		Type: like.Type, Name: name, Provider: like.Provider, Params: string(like.Params),
	})
	if err != nil {
		t.Fatalf("create %s: %v", name, err)
	}
	return c
}

// Selected catalogs come first in library order whatever order they were
// asked for in, then each listed catalog a collection references, once; a
// scoped catalog stays in its collection's own list.
func TestExportBundlePlacement(t *testing.T) {
	f := newExportFixture(t)

	b, err := f.db.ExportBundle(context.Background(), f.owner, []uuid.UUID{f.b.ID, f.a.ID, f.b.ID}, []uuid.UUID{f.x.ID})
	if err != nil {
		t.Fatalf("ExportBundle: %v", err)
	}
	assertStrings(t, "top-level catalogs", bundleCatalogKeys(b.Catalogs), []string{"c1=A", "c2=B", "c4=C"})
	if len(b.Collections) != 1 || b.Collections[0].Title != "X" {
		t.Fatalf("collections = %+v, want X alone", b.Collections)
	}
	x := b.Collections[0]
	assertStrings(t, "X's own catalogs", bundleCatalogKeys(x.Catalogs), []string{"c3=S"})
	assertStrings(t, "X's refs", bundleFolderRefs(x.Folders[0]), []string{"c3/", "c4/Drama", "c1/"})
	if err := b.Validate(); err != nil {
		t.Fatalf("the export fails its own Validate: %v", err)
	}
}

// The wire form carries only the format's own keys, and an empty list is
// written as [].
func TestExportBundleWireForm(t *testing.T) {
	f := newExportFixture(t)
	ctx := context.Background()

	withX, err := f.db.ExportBundle(ctx, f.owner, nil, []uuid.UUID{f.x.ID, f.y.ID})
	if err != nil {
		t.Fatalf("ExportBundle: %v", err)
	}
	var wire struct {
		Catalogs    []map[string]json.RawMessage `json:"catalogs"`
		Collections []map[string]json.RawMessage `json:"collections"`
	}
	raw, err := json.Marshal(withX)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	var folders []map[string]json.RawMessage
	if err := json.Unmarshal(wire.Collections[0]["folders"], &folders); err != nil {
		t.Fatalf("unmarshal folders: %v", err)
	}
	assertStrings(t, "catalog keys", sortedKeys(wire.Catalogs[0]), []string{"key", "name", "params", "provider", "type"})
	assertStrings(t, "collection keys", sortedKeys(wire.Collections[0]), []string{
		"backdrop_image_url", "catalogs", "focus_glow_enabled", "folders", "show_all_tab", "title", "view_mode"})
	assertStrings(t, "folder keys", sortedKeys(folders[0]), []string{
		"cover_emoji", "cover_image_url", "focus_gif_enabled", "focus_gif_url", "hero_backdrop_url", "hero_video_url",
		"hide_title", "refs", "tile_shape", "title", "title_logo_url"})
	if got := string(wire.Collections[1]["catalogs"]) + string(wire.Collections[1]["folders"]); got != "[][]" {
		t.Errorf("Y's catalogs and folders = %s, want [][]", got)
	}

	onlyA, err := f.db.ExportBundle(ctx, f.owner, []uuid.UUID{f.a.ID}, nil)
	if err != nil {
		t.Fatalf("ExportBundle: %v", err)
	}
	raw, err = json.Marshal(onlyA)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(raw), `"collections":[]`) {
		t.Errorf("export = %s, want an empty collections list", raw)
	}
}

func sortedKeys(m map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// Export takes only the caller's own listed catalogs and own collections,
// and a selection of something.
func TestExportBundleRejectsIDs(t *testing.T) {
	f := newExportFixture(t)
	ctx := context.Background()
	other := newTestProfile(t, f.db, "other")
	theirs := createListed(t, f.db, other, "Theirs")
	scoped := f.x.Catalogs[slices.IndexFunc(f.x.Catalogs, func(c Catalog) bool { return c.Name == f.scopedName })]
	stranger := uuid.New()

	_, err := f.db.ExportBundle(ctx, f.owner, nil, []uuid.UUID{})
	requireInvalid(t, err, "select at least one catalog or collection")

	for _, tc := range []struct {
		name                     string
		catalogIDs, collectionID []uuid.UUID
		want                     uuid.UUID
	}{
		{"another profile's catalog", []uuid.UUID{f.a.ID, theirs.ID}, nil, theirs.ID},
		{"a scoped catalog", []uuid.UUID{scoped.ID}, nil, scoped.ID},
		{"an unknown collection", nil, []uuid.UUID{stranger}, stranger},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := f.db.ExportBundle(ctx, f.owner, tc.catalogIDs, tc.collectionID)
			requireInvalid(t, err, tc.want.String())
		})
	}
}

// Import writes the fixture as new rows: private, listed or scoped as the
// file places them, never linked, off the home screen, version 1 and never
// pushed, with the titles it was given, compacted params, and their recipes.
func TestImportBundleWritesNewRows(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	importer := newTestProfile(t, db, "importer")

	catalogs, collections, err := db.ImportBundle(ctx, importer, readTestBundle(t), nil)
	if err != nil {
		t.Fatalf("ImportBundle: %v", err)
	}

	if names := catalogNames(catalogs); !slices.Equal(names, []string{"80s Horror", "Top Rated"}) {
		t.Fatalf("imported catalogs = %v, want the two top-level ones in order", names)
	}
	for _, c := range catalogs {
		if c.OwnerID != importer || c.Publication != nil || c.CollectionID != nil || c.Subscription != nil || c.SubKey != "" || c.HomeSortOrder != nil {
			t.Errorf("listed %q = %+v, want an unpublished, unsubscribed, listed row off Home", c.Name, c)
		}
		if want := RecipeHash(c.Type, c.Provider, c.Params); c.RecipeHash != want {
			t.Errorf("listed %q recipe hash = %q, want %q", c.Name, c.RecipeHash, want)
		}
	}
	if got := catalogs[0].Params; got != `{"sort_by":"popularity.desc"}` {
		t.Errorf("c1 stored params = %s, want them compacted", got)
	}

	if len(collections) != 1 {
		t.Fatalf("imported %d collections, want 1", len(collections))
	}
	halloween := collections[0]
	if halloween.Title != "Halloween" || halloween.Publication != nil ||
		halloween.HomeSortOrder != nil || halloween.Subscription != nil || halloween.PinToTop || halloween.BackdropImageURL == "" {
		t.Errorf("collection = %+v, want the file's fields on an unpublished, unsubscribed row off Home", halloween.Collection)
	}
	slashers := halloween.Catalogs[slices.IndexFunc(halloween.Catalogs, func(c Catalog) bool { return c.Name == "Slashers" })]
	if slashers.CollectionID == nil || *slashers.CollectionID != halloween.ID || slashers.SubKey != "" {
		t.Errorf("Slashers = %+v, want it scoped to the new collection, with no sub_key", slashers)
	}
	if want := RecipeHash("movie", "tmdb", `{"sort_by":"revenue.desc"}`); slashers.RecipeHash != want {
		t.Errorf("Slashers recipe hash = %q, want %q", slashers.RecipeHash, want)
	}
	requireRefs(t, halloween, 0, []FolderRef{{catalogs[0].ID, ""}, {slashers.ID, "Horror"}})
	requireRefs(t, halloween, 1, []FolderRef{{slashers.ID, ""}})

	listed, err := db.GetUserCatalogs(ctx, importer)
	if err != nil || len(listed) != 2 {
		t.Fatalf("GetUserCatalogs = %d rows (%v), want the 2 imported listed catalogs", len(listed), err)
	}
}

func catalogNames(catalogs []Catalog) []string {
	names := make([]string, len(catalogs))
	for i, c := range catalogs {
		names[i] = c.Name
	}
	return names
}

func requireRefs(t *testing.T, tree CollectionWithFolders, folder int, want []FolderRef) {
	t.Helper()
	if got := tree.Folders[folder].Refs; !slices.Equal(got, want) {
		t.Errorf("folder %d refs = %v, want %v", folder, got, want)
	}
}

// Importing one file twice gives two independent sets of rows.
func TestImportBundleTwiceGivesTwoSets(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	importer := newTestProfile(t, db, "importer")
	b := readTestBundle(t)

	firstCatalogs, firstCollections, err := db.ImportBundle(ctx, importer, b, nil)
	if err != nil {
		t.Fatalf("first ImportBundle: %v", err)
	}
	secondCatalogs, secondCollections, err := db.ImportBundle(ctx, importer, b, nil)
	if err != nil {
		t.Fatalf("second ImportBundle: %v", err)
	}
	if firstCatalogs[0].ID == secondCatalogs[0].ID || firstCollections[0].ID == secondCollections[0].ID {
		t.Fatal("the second import reused a row of the first")
	}
	if secondCollections[0].Folders[0].Refs[0].CatalogID != secondCatalogs[0].ID {
		t.Error("the second set's collection references the first set's catalog")
	}
	listed, err := db.GetUserCatalogs(ctx, importer)
	if err != nil || len(listed) != 4 {
		t.Fatalf("GetUserCatalogs = %d rows (%v), want 4", len(listed), err)
	}
}

// reuse points a key's refs at one of the importer's listed catalogs: a
// reused top-level key gets no new row, a reused own key no scoped copy, and
// two keys reused onto one row under the same genre in a folder collapse to
// one ref.
func TestImportBundleReuse(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name       string
		reuse      []string
		sameGenre  bool
		wantListed []string
		wantScoped int
		wantFirst  func(mine, c1, c3 uuid.UUID) []FolderRef
	}{
		{"a top-level key", []string{"c1"}, false, []string{"Top Rated"}, 1,
			func(mine, _, c3 uuid.UUID) []FolderRef { return []FolderRef{{mine, ""}, {c3, "Horror"}} }},
		{"a collection's own key", []string{"c3"}, false, []string{"80s Horror", "Top Rated"}, 0,
			func(mine, c1, _ uuid.UUID) []FolderRef { return []FolderRef{{c1, ""}, {mine, "Horror"}} }},
		{"two keys onto one row", []string{"c1", "c3"}, true, []string{"Top Rated"}, 0,
			func(mine, _, _ uuid.UUID) []FolderRef { return []FolderRef{{mine, ""}} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := newTestDB(t)
			importer := newTestProfile(t, db, "importer")
			b := readTestBundle(t)
			if tc.sameGenre {
				b.Collections[0].Folders[0].Refs[1].Genre = ""
				b.Collections[0].Catalogs[0].Params = b.Catalogs[0].Params
			}
			mine := createLike(t, db, importer, "Mine", reusedFrom(b, tc.reuse[0]))
			reuse := map[string]uuid.UUID{}
			for _, key := range tc.reuse {
				reuse[key] = mine.ID
			}

			catalogs, collections, err := db.ImportBundle(ctx, importer, b, reuse)
			if err != nil {
				t.Fatalf("ImportBundle: %v", err)
			}
			if names := catalogNames(catalogs); !slices.Equal(names, tc.wantListed) {
				t.Fatalf("new listed catalogs = %v, want %v", names, tc.wantListed)
			}
			tree := collections[0]
			var c1, c3 uuid.UUID
			for _, c := range append(catalogs, tree.Catalogs...) {
				switch c.Name {
				case "80s Horror":
					c1 = c.ID
				case "Slashers":
					c3 = c.ID
				}
			}
			scoped := 0
			for _, c := range tree.Catalogs {
				if c.CollectionID != nil {
					scoped++
				}
			}
			if scoped != tc.wantScoped {
				t.Errorf("scoped catalogs = %d, want %d", scoped, tc.wantScoped)
			}
			requireRefs(t, tree, 0, tc.wantFirst(mine.ID, c1, c3))
		})
	}
}

// Each import failure writes nothing: a reuse target that isn't one of the
// importer's listed catalogs, a reuse key the bundle doesn't have, a bad
// listed catalog, a bad second collection, a bundle failing Validate.
func TestImportBundleFailsWhole(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T, db *DB, importer uuid.UUID) (Bundle, map[string]uuid.UUID)
		want  string
	}{
		{"another profile's catalog", func(t *testing.T, db *DB, _ uuid.UUID) (Bundle, map[string]uuid.UUID) {
			theirs := createListed(t, db, newTestProfile(t, db, "other"), "Theirs")
			return readTestBundle(t), map[string]uuid.UUID{"c3": theirs.ID}
		}, "is not usable in this collection's folders"},
		{"one of the importer's scoped catalogs", func(t *testing.T, db *DB, importer uuid.UUID) (Bundle, map[string]uuid.UUID) {
			scoped, _ := scopedCatalogInFolder(t, db, importer, newTestCollection(t, db, importer, "Own"), "Scoped")
			return readTestBundle(t), map[string]uuid.UUID{"c1": scoped.ID}
		}, "is not usable in this collection's folders"},
		{"a listed catalog of another type", func(t *testing.T, db *DB, importer uuid.UUID) (Bundle, map[string]uuid.UUID) {
			b := readTestBundle(t)
			series := reusedFrom(b, "c1")
			series.Type = "series"
			return b, map[string]uuid.UUID{"c1": createLike(t, db, importer, "Series", series).ID}
		}, `reuse catalog key "c1" points at a catalog with a different recipe`},
		{"a listed catalog with other filters", func(t *testing.T, db *DB, importer uuid.UUID) (Bundle, map[string]uuid.UUID) {
			return readTestBundle(t), map[string]uuid.UUID{"c3": createListed(t, db, importer, "Other").ID}
		}, `reuse catalog key "c3" points at a catalog with a different recipe`},
		{"a key the bundle doesn't have", func(t *testing.T, _ *DB, _ uuid.UUID) (Bundle, map[string]uuid.UUID) {
			return readTestBundle(t), map[string]uuid.UUID{"c9": uuid.New()}
		}, `reuse names catalog key "c9"`},
		{"a bad listed catalog", func(t *testing.T, _ *DB, _ uuid.UUID) (Bundle, map[string]uuid.UUID) {
			b := readTestBundle(t)
			b.Catalogs[1].Name = " "
			return b, nil
		}, "catalog 1: name is required"},
		{"a bad second collection", func(t *testing.T, _ *DB, _ uuid.UUID) (Bundle, map[string]uuid.UUID) {
			b := readTestBundle(t)
			second := b.Collections[0]
			second.Catalogs = []BundleCatalog{}
			second.Folders = []BundleFolder{{Title: "F", Refs: []BundleRef{{Catalog: "c2"}}}}
			second.ViewMode = "CAROUSEL"
			b.Collections = append(b.Collections, second)
			return b, nil
		}, "collection 1: invalid input: view mode must be"},
		{"a bundle failing Validate", func(t *testing.T, _ *DB, _ uuid.UUID) (Bundle, map[string]uuid.UUID) {
			b := readTestBundle(t)
			b.Version = 0
			return b, nil
		}, "bundle: version must be 1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := newTestDB(t)
			importer := newTestProfile(t, db, "importer")
			b, reuse := tc.setup(t, db, importer)
			before, err := db.queryCatalogs(ctx, "c.owner_id = ?", importer.String())
			if err != nil {
				t.Fatalf("queryCatalogs: %v", err)
			}
			beforeCollections, err := db.GetUserCollections(ctx, importer)
			if err != nil {
				t.Fatalf("GetUserCollections: %v", err)
			}

			_, _, err = db.ImportBundle(ctx, importer, b, reuse)
			requireInvalid(t, err, tc.want)

			after, err := db.queryCatalogs(ctx, "c.owner_id = ?", importer.String())
			if err != nil {
				t.Fatalf("queryCatalogs: %v", err)
			}
			afterCollections, err := db.GetUserCollections(ctx, importer)
			if err != nil {
				t.Fatalf("GetUserCollections: %v", err)
			}
			if len(after) != len(before) || len(afterCollections) != len(beforeCollections) {
				t.Fatalf("the failed import wrote rows: catalogs %d → %d, collections %d → %d",
					len(before), len(after), len(beforeCollections), len(afterCollections))
			}
		})
	}
}

// An export imported into another profile gives collections that hash the
// same as their sources, through the JSON the file is written as.
func TestExportImportRoundTrip(t *testing.T) {
	f := newExportFixture(t)
	ctx := context.Background()

	exported, err := f.db.ExportBundle(ctx, f.owner, []uuid.UUID{f.b.ID}, []uuid.UUID{f.x.ID, f.y.ID})
	if err != nil {
		t.Fatalf("ExportBundle: %v", err)
	}
	file, err := json.MarshalIndent(exported, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded Bundle
	if err := json.Unmarshal(file, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	importer := newTestProfile(t, f.db, "importer")
	_, collections, err := f.db.ImportBundle(ctx, importer, decoded, nil)
	if err != nil {
		t.Fatalf("ImportBundle: %v", err)
	}
	for i, source := range []CollectionWithFolders{f.x, f.y} {
		stored, err := f.db.GetCollectionsByIDs(ctx, []uuid.UUID{source.ID})
		if err != nil {
			t.Fatalf("reload source: %v", err)
		}
		if got, want := contentOf(t, collections[i]), contentOf(t, stored[0]); got != want {
			t.Errorf("collection %q: imported %s, source %s", source.Title, got, want)
		}
	}
	requireOwnedCounts(t, f.db, importer, 3, 2)
}

// requireOwnedCounts fails unless profileID owns exactly listed
// listed catalogs and collections collections.
func requireOwnedCounts(t *testing.T, db *DB, profileID uuid.UUID, listed, collections int) {
	t.Helper()
	ctx := context.Background()
	gotListed, err := db.GetUserCatalogs(ctx, profileID)
	if err != nil {
		t.Fatalf("GetUserCatalogs: %v", err)
	}
	gotCollections, err := db.GetUserCollections(ctx, profileID)
	if err != nil {
		t.Fatalf("GetUserCollections: %v", err)
	}
	if len(gotListed) != listed || len(gotCollections) != collections {
		t.Fatalf("profile owns %d listed catalogs and %d collections, want %d and %d", len(gotListed), len(gotCollections), listed, collections)
	}
}

// A failure after rows are written inside the transaction rolls back every
// one of them: the listed catalogs, the first collection and its scoped
// catalog are inserted before a trigger aborts the second collection.
func TestImportBundleRollsBackWrittenRows(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	importer := newTestProfile(t, db, "importer")
	if _, err := db.conn.ExecContext(ctx, `CREATE TRIGGER boom BEFORE INSERT ON collections
		WHEN NEW.title = 'Boom' BEGIN SELECT RAISE(ABORT, 'boom'); END`); err != nil {
		t.Fatalf("creating trigger: %v", err)
	}
	b := readTestBundle(t)
	b.Collections = append(b.Collections, BundleCollection{
		Title: "Boom", Catalogs: []BundleCatalog{},
		Folders: []BundleFolder{{Title: "F", Refs: []BundleRef{{Catalog: "c2"}}}},
	})

	if _, _, err := db.ImportBundle(ctx, importer, b, nil); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("err = %v, want the trigger's abort", err)
	}
	all, err := db.queryCatalogs(ctx, "c.owner_id = ?", importer.String())
	if err != nil {
		t.Fatalf("queryCatalogs: %v", err)
	}
	if len(all) != 0 {
		t.Fatalf("the failed import left %d catalogs", len(all))
	}
	requireOwnedCounts(t, db, importer, 0, 0)
}

// contentOf is tree's content: the JSON of its bundle form, every catalog it
// references in its own list, which leaves ids out.
func contentOf(t *testing.T, tree CollectionWithFolders) string {
	t.Helper()
	b, err := json.Marshal(extractBundle(nil, []CollectionWithFolders{tree}, true).Collections[0])
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(b)
}
