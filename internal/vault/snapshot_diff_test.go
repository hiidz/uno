package vault

import (
	"encoding/json"
	"reflect"
	"testing"
)

// label is a change as one line a test can compare: "removed catalog Retro
// @80s [Horror]".
func label(c SnapshotChange) string {
	s := c.Op + " " + c.Kind
	if c.Aspect != "" {
		s += "/" + c.Aspect
	}
	if c.Name != "" {
		s += " " + c.Name
	}
	if c.Was != "" {
		s += " (was " + c.Was + ")"
	}
	if c.Folder != "" {
		s += " @" + c.Folder
	}
	if c.Genre != "" {
		s += " [" + c.Genre + "]"
	}
	if c.Catalog != nil {
		s += " +recipe"
	}
	return s
}

func labels(changes []SnapshotChange) []string {
	out := []string{}
	for _, c := range changes {
		out = append(out, label(c))
	}
	return out
}

func testCatalog(key, name, params string) BundleCatalog {
	return BundleCatalog{Key: key, Name: name, Type: "movie", Provider: "tmdb", Params: json.RawMessage(params)}
}

// testFolder is a folder under key whose refs are the catalogs named, each as
// "key" or "key/genre".
func testFolder(key, title string, refs ...BundleRef) SnapshotFolder {
	return SnapshotFolder{Key: key, BundleFolder: BundleFolder{Title: title, TileShape: "POSTER", Refs: refs}}
}

func testCollectionSnapshot(title string, folders []SnapshotFolder, catalogs ...BundleCatalog) Snapshot {
	return Snapshot{
		Format: SnapshotFormat, Version: SnapshotVersion, Catalogs: catalogs,
		Collection: &SnapshotCollection{Title: title, ViewMode: "TABBED_GRID", Folders: folders},
	}
}

func ref(catalog, genre string) BundleRef { return BundleRef{Catalog: catalog, Genre: genre} }

func assertChanges(t *testing.T, from, to Snapshot, want ...string) {
	t.Helper()
	got := diffSnapshots(from, to)
	if got == nil {
		t.Fatal("diffSnapshots = nil, want a list, empty or not")
	}
	if want == nil {
		want = []string{}
	}
	if gotLabels := labels(got); !reflect.DeepEqual(gotLabels, want) {
		t.Errorf("changes =\n  %q\nwant\n  %q", gotLabels, want)
	}
}

// Removals come first, then additions, then changes, each in folder order: a
// folder lost whole is one item followed by the catalogs it held that nothing
// else holds, a folder kept names the catalogs it lost, and a folder gained
// is the same read the other way.
func TestDiffListsRemovalsThenAdditionsThenChanges(t *testing.T) {
	from := testCollectionSnapshot("Weekend",
		[]SnapshotFolder{
			testFolder("fa", "80s", ref("c1", ""), ref("c2", "")),
			testFolder("fb", "Kids", ref("c3", ""), ref("c1", "")),
		},
		testCatalog("c1", "Alien", `{"a":1}`), testCatalog("c2", "Retro", `{"a":2}`), testCatalog("c3", "Cars", `{"a":3}`))
	to := testCollectionSnapshot("Weekend",
		[]SnapshotFolder{
			testFolder("fa", "80s", ref("c1", "")),
			testFolder("fc", "Classics", ref("c4", ""), ref("c1", "")),
		},
		testCatalog("c1", "Alien", `{"a":9}`), testCatalog("c4", "Heat", `{"a":4}`))

	assertChanges(t, from, to,
		"removed catalog Retro @80s",
		"removed folder Kids",
		"removed catalog Cars @Kids",
		"added folder Classics",
		"added catalog Heat @Classics",
		"changed catalog/recipe Alien +recipe")
}

// A catalog moved from one folder to another, both kept, is removed from the
// one and added to the other, and is not a catalog gained or lost.
func TestDiffMovedCatalog(t *testing.T) {
	c := testCatalog("c1", "Alien", "{}")
	from := testCollectionSnapshot("W", []SnapshotFolder{testFolder("fa", "A", ref("c1", "")), testFolder("fb", "B")}, c)
	to := testCollectionSnapshot("W", []SnapshotFolder{testFolder("fa", "A"), testFolder("fb", "B", ref("c1", ""))}, c)
	assertChanges(t, from, to, "removed catalog Alien @A", "added catalog Alien @B")
}

// A folder may reference one catalog twice, narrowed to different genres: a
// ref is matched by catalog and genre, so narrowing one reads as a removal
// and an addition.
func TestDiffMatchesRefsByCatalogAndGenre(t *testing.T) {
	c := testCatalog("c1", "Alien", "{}")
	from := testCollectionSnapshot("W", []SnapshotFolder{testFolder("fa", "A", ref("c1", ""), ref("c1", "War"))}, c)
	to := testCollectionSnapshot("W", []SnapshotFolder{testFolder("fa", "A", ref("c1", ""), ref("c1", "Horror"))}, c)
	assertChanges(t, from, to, "removed catalog Alien @A [War]", "added catalog Alien @A [Horror]")
}

// A catalog used in two folders and changed is one item, naming the catalog as
// it is now and as it was with its recipe, and its earlier name when that changed too; a
// name alone carries no recipe.
func TestDiffChangedCatalogIsOneItem(t *testing.T) {
	folders := []SnapshotFolder{testFolder("fa", "A", ref("c1", ""), ref("c2", "")), testFolder("fb", "B", ref("c1", ""))}
	from := testCollectionSnapshot("W", folders, testCatalog("c1", "Alien", `{"a":1}`), testCatalog("c2", "Heat", "{}"))
	to := testCollectionSnapshot("W", folders, testCatalog("c1", "Aliens", `{"a":2}`), testCatalog("c2", "Heats", "{}"))

	assertChanges(t, from, to, "changed catalog/recipe Aliens (was Alien) +recipe", "changed catalog/name Heats (was Heat)")
	changes := diffSnapshots(from, to)
	got := changes[0]
	if c := got.Catalog; c == nil || string(c.Params) != `{"a":2}` {
		t.Errorf("recipe change carries %+v, want the catalog as it is now", c)
	}
	if c := got.WasCatalog; c == nil || c.Name != "Alien" || string(c.Params) != `{"a":1}` {
		t.Errorf("recipe change was %+v, want the catalog as it was", c)
	}
	if c := changes[1]; c.Catalog != nil || c.WasCatalog != nil {
		t.Errorf("name-only change carries %+v and %+v, want no recipes", c.Catalog, c.WasCatalog)
	}
}

// What the collection and its folders say of themselves: names, settings,
// art, the order of a folder's catalogs and the order of the folders.
func TestDiffChangedCollectionAndFolders(t *testing.T) {
	c1, c2 := testCatalog("c1", "A", "{}"), testCatalog("c2", "B", "{}")
	from := testCollectionSnapshot("Old", []SnapshotFolder{
		testFolder("fa", "Kids", ref("c1", ""), ref("c2", "")),
		testFolder("fb", "Adults", ref("c1", "")),
	}, c1, c2)
	to := testCollectionSnapshot("New", []SnapshotFolder{
		testFolder("fb", "Adults", ref("c1", "")),
		testFolder("fa", "Family", ref("c2", ""), ref("c1", "")),
	}, c1, c2)
	to.Collection.ShowAllTab = true
	to.Collection.Folders[0].CoverEmoji = "🎃"

	assertChanges(t, from, to,
		"changed collection/name New (was Old)",
		"changed collection/settings",
		"changed folder/art Adults",
		"changed folder/name Family (was Kids)",
		"changed folder/catalog_order Family",
		"changed collection/order")
}

// A catalog publication holds one catalog and no folders.
func TestDiffCatalogSnapshot(t *testing.T) {
	snap := func(key, name, params string) Snapshot {
		return Snapshot{Format: SnapshotFormat, Version: SnapshotVersion, Catalogs: []BundleCatalog{testCatalog(key, name, params)}}
	}
	assertChanges(t, snap("k", "Popular", `{"a":1}`), snap("k", "Popular", `{"a":2}`), "changed catalog/recipe Popular +recipe")
	assertChanges(t, snap("k", "Popular", "{}"), snap("k", "Popular", "{}"))
	assertChanges(t, snap("k", "Popular", "{}"), snap("j", "Other", "{}"), "removed catalog Popular", "added catalog Other")
}

// The list is empty exactly when the two snapshots have the same content hash:
// a row flagged Publish changes or Update available always has something to
// show, and an edit followed by its undo has nothing. Each mutation changes one
// field of a collection tree a snapshot holds.
func TestDiffIsEmptyExactlyWhenTheContentHashMatches(t *testing.T) {
	mutations := map[string]func(*CollectionWithFolders){
		"none":              func(*CollectionWithFolders) {},
		"title":             func(t *CollectionWithFolders) { t.Title += "!" },
		"view mode":         func(t *CollectionWithFolders) { t.ViewMode = "ROWS_2" },
		"show all tab":      func(t *CollectionWithFolders) { t.ShowAllTab = !t.ShowAllTab },
		"backdrop":          func(t *CollectionWithFolders) { t.BackdropImageURL += "x" },
		"focus glow":        func(t *CollectionWithFolders) { t.FocusGlowEnabled = !t.FocusGlowEnabled },
		"folder title":      func(t *CollectionWithFolders) { t.Folders[0].Title += "!" },
		"tile shape":        func(t *CollectionWithFolders) { t.Folders[0].TileShape = "WIDE" },
		"hide title":        func(t *CollectionWithFolders) { t.Folders[0].HideTitle = true },
		"cover emoji":       func(t *CollectionWithFolders) { t.Folders[0].CoverEmoji = "x" },
		"cover image":       func(t *CollectionWithFolders) { t.Folders[0].CoverImageURL += "x" },
		"focus gif":         func(t *CollectionWithFolders) { t.Folders[0].FocusGIFURL += "x" },
		"focus gif enabled": func(t *CollectionWithFolders) { t.Folders[0].FocusGIFEnabled = false },
		"hero backdrop":     func(t *CollectionWithFolders) { t.Folders[0].HeroBackdropURL += "x" },
		"hero video":        func(t *CollectionWithFolders) { t.Folders[0].HeroVideoURL += "x" },
		"title logo":        func(t *CollectionWithFolders) { t.Folders[0].TitleLogoURL += "x" },
		"ref genre":         func(t *CollectionWithFolders) { t.Folders[0].Refs[1].Genre = "War" },
		"ref order":         func(t *CollectionWithFolders) { r := t.Folders[0].Refs; r[0], r[1] = r[1], r[0] },
		"ref removed":       func(t *CollectionWithFolders) { t.Folders[0].Refs = t.Folders[0].Refs[:1] },
		"ref added": func(t *CollectionWithFolders) {
			t.Folders[1].Refs = append(t.Folders[1].Refs, FolderRef{CatalogID: t.Catalogs[1].ID})
		},
		"folder order":   func(t *CollectionWithFolders) { f := t.Folders; f[0], f[1] = f[1], f[0] },
		"folder removed": func(t *CollectionWithFolders) { t.Folders = t.Folders[:1] },
		"catalog name":   func(t *CollectionWithFolders) { t.Catalogs[0].Name += "!" },
		"catalog params": func(t *CollectionWithFolders) { t.Catalogs[0].Params = `{"sort_by":"vote_average.desc"}` },
		"catalog type":   func(t *CollectionWithFolders) { t.Catalogs[0].Type = "series" },
		"catalog provider": func(t *CollectionWithFolders) {
			t.Catalogs[0].Provider = "other"
		},
	}
	base := collectionSnapshot(fixedID(9), fixedTree())
	_, baseHash, err := base.encode()
	if err != nil {
		t.Fatal(err)
	}
	for name, mutate := range mutations {
		tree := fixedTree()
		mutate(&tree)
		s := collectionSnapshot(fixedID(9), tree)
		_, hash, err := s.encode()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if changes := diffSnapshots(base, s); (len(changes) == 0) != (hash == baseHash) {
			t.Errorf("%s: %d changes %q, content hash equal = %v", name, len(changes), labels(changes), hash == baseHash)
		}
	}
}

// Params compare as the snapshot stores them, so a recipe with the same keys
// in the same order is the same however it was spaced when it was read.
func TestDiffComparesParamsAsEncoded(t *testing.T) {
	a := testCatalog("k", "C", `{"with_keywords": "1",  "sort_by":"popularity.desc"}`)
	b := testCatalog("k", "C", `{"with_keywords":"1","sort_by":"popularity.desc"}`)
	assertChanges(t,
		Snapshot{Catalogs: []BundleCatalog{a}},
		Snapshot{Catalogs: []BundleCatalog{b}})
}
