package vault

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// bundleTestCatalog is a stored catalog for the extractBundle tests: listed
// when collectionID is nil, scoped to it otherwise.
func bundleTestCatalog(name string, collectionID *uuid.UUID) Catalog {
	return Catalog{
		ID:           uuid.New(),
		Type:         "movie",
		Name:         name,
		Provider:     "tmdb",
		Params:       `{"sort_by":"` + name + `"}`,
		CollectionID: collectionID,
		RecipeHash:   "fp-" + name,
	}
}

// bundleTestTree is a stored collection tree for the extractBundle tests,
// with one folder per entry of folders.
func bundleTestTree(id uuid.UUID, title string, catalogs []Catalog, folders ...[]FolderRef) CollectionWithFolders {
	tree := CollectionWithFolders{Collection: Collection{ID: id, Title: title}, Catalogs: catalogs}
	for i, refs := range folders {
		tree.Folders = append(tree.Folders, FolderWithCatalogs{
			Folder: Folder{ID: uuid.New(), CollectionID: id, Title: fmt.Sprintf("Folder %d", i+1), SortOrder: i},
			Refs:   refs,
		})
	}
	return tree
}

func refTo(c Catalog, genre string) FolderRef {
	return FolderRef{CatalogID: c.ID, Genre: genre}
}

// bundleCatalogKeys lists "key=name" for each of catalogs, in order.
func bundleCatalogKeys(catalogs []BundleCatalog) []string {
	out := []string{}
	for _, c := range catalogs {
		out = append(out, c.Key+"="+c.Name)
	}
	return out
}

// bundleFolderRefs lists "key/genre" for each ref of f, in order.
func bundleFolderRefs(f BundleFolder) []string {
	out := []string{}
	for _, ref := range f.Refs {
		out = append(out, ref.Catalog+"/"+ref.Genre)
	}
	return out
}

func assertStrings(t *testing.T, label string, got, want []string) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s = %v, want %v", label, got, want)
	}
}

// Without scopeAll, listed catalogs go top-level — the ones passed as listed
// first, then each referenced one once across every collection — and scoped
// catalogs stay inside their collection.
func TestExtractBundleListsCatalogsByScope(t *testing.T) {
	aID, bID := uuid.New(), uuid.New()
	selected := bundleTestCatalog("Selected", nil)
	shared := bundleTestCatalog("Shared", nil)
	scoped := bundleTestCatalog("Scoped", &aID)

	b := extractBundle([]Catalog{selected}, []CollectionWithFolders{
		bundleTestTree(aID, "A", []Catalog{shared, scoped}, []FolderRef{refTo(scoped, ""), refTo(shared, "")}),
		bundleTestTree(bID, "B", []Catalog{shared}, []FolderRef{refTo(shared, "War")}),
	}, false)

	if b.Format != BundleFormat || b.Version != BundleVersion {
		t.Errorf("format, version = %q, %d, want %q, %d", b.Format, b.Version, BundleFormat, BundleVersion)
	}
	assertStrings(t, "top-level catalogs", bundleCatalogKeys(b.Catalogs), []string{"c1=Selected", "c3=Shared"})
	assertStrings(t, "A's catalogs", bundleCatalogKeys(b.Collections[0].Catalogs), []string{"c2=Scoped"})
	assertStrings(t, "A's refs", bundleFolderRefs(b.Collections[0].Folders[0]), []string{"c2/", "c3/"})
	assertStrings(t, "B's catalogs", bundleCatalogKeys(b.Collections[1].Catalogs), []string{})
	assertStrings(t, "B's refs", bundleFolderRefs(b.Collections[1].Folders[0]), []string{"c3/War"})
}

// With scopeAll, every catalog a collection references goes into its own
// list, so a listed catalog two collections share is emitted into each, and
// nothing is top-level. Each catalog remembers its row and recipe hash.
func TestExtractBundleScopeAllKeepsEveryCatalogInItsCollection(t *testing.T) {
	aID, bID := uuid.New(), uuid.New()
	shared := bundleTestCatalog("Shared", nil)
	scoped := bundleTestCatalog("Scoped", &aID)

	b := extractBundle(nil, []CollectionWithFolders{
		bundleTestTree(aID, "A", []Catalog{shared, scoped}, []FolderRef{refTo(scoped, ""), refTo(shared, "")}),
		bundleTestTree(bID, "B", []Catalog{shared}, []FolderRef{refTo(shared, "")}),
	}, true)

	assertStrings(t, "top-level catalogs", bundleCatalogKeys(b.Catalogs), []string{})
	assertStrings(t, "A's catalogs", bundleCatalogKeys(b.Collections[0].Catalogs), []string{"c1=Scoped", "c2=Shared"})
	assertStrings(t, "B's catalogs", bundleCatalogKeys(b.Collections[1].Catalogs), []string{"c3=Shared"})

	if got := b.Collections[0].SourceID; got == nil || *got != aID {
		t.Errorf("A's SourceID = %v, want %s", got, aID)
	}
	for _, c := range []BundleCatalog{b.Collections[0].Catalogs[1], b.Collections[1].Catalogs[0]} {
		if c.SourceID == nil || *c.SourceID != shared.ID {
			t.Errorf("%s SourceID = %v, want %s", c.Key, c.SourceID, shared.ID)
		}
		if c.RecipeHash != shared.RecipeHash || string(c.Params) != shared.Params {
			t.Errorf("%s recipe hash, params = %q, %s, want %q, %s", c.Key, c.RecipeHash, c.Params, shared.RecipeHash, shared.Params)
		}
	}
}

// Keys follow the order refs are walked in, folder by folder, not the order
// of the tree's Catalogs; genres and ref order survive.
func TestExtractBundleKeysFollowFirstSeenOrder(t *testing.T) {
	id := uuid.New()
	a, b, c := bundleTestCatalog("A", nil), bundleTestCatalog("B", nil), bundleTestCatalog("C", nil)

	got := extractBundle(nil, []CollectionWithFolders{bundleTestTree(id, "T", []Catalog{c, a, b},
		[]FolderRef{refTo(b, "Drama"), refTo(a, "")},
		[]FolderRef{refTo(a, "War"), refTo(c, ""), refTo(a, "Western")},
		[]FolderRef{refTo(b, "Drama")},
	)}, true).Collections[0]

	assertStrings(t, "catalogs", bundleCatalogKeys(got.Catalogs), []string{"c1=B", "c2=A", "c3=C"})
	assertStrings(t, "folder 1 refs", bundleFolderRefs(got.Folders[0]), []string{"c1/Drama", "c2/"})
	assertStrings(t, "folder 2 refs", bundleFolderRefs(got.Folders[1]), []string{"c2/War", "c3/", "c2/Western"})
	assertStrings(t, "folder 3 refs", bundleFolderRefs(got.Folders[2]), []string{"c1/Drama"})
}

// A ref whose catalog isn't among the tree's Catalogs is dropped, and uses
// up no key; a folder left with no refs is kept.
func TestExtractBundleDropsDanglingRefs(t *testing.T) {
	id := uuid.New()
	kept := bundleTestCatalog("Kept", nil)
	gone := bundleTestCatalog("Gone", nil)

	got := extractBundle(nil, []CollectionWithFolders{bundleTestTree(id, "T", []Catalog{kept},
		[]FolderRef{refTo(gone, ""), refTo(kept, "")},
		[]FolderRef{refTo(gone, "War")},
	)}, false)

	assertStrings(t, "top-level catalogs", bundleCatalogKeys(got.Catalogs), []string{"c1=Kept"})
	assertStrings(t, "folder 1 refs", bundleFolderRefs(got.Collections[0].Folders[0]), []string{"c1/"})
	assertStrings(t, "folder 2 refs", bundleFolderRefs(got.Collections[0].Folders[1]), []string{})
}

// writeBundleCollection writes bc through collectionFormFromBundle and
// createCollectionTx, the way a collection copy does, and reads it back.
func writeBundleCollection(t *testing.T, db *DB, profileID uuid.UUID, bc BundleCollection, topIDs map[string]uuid.UUID, keyed bool) CollectionWithFolders {
	t.Helper()
	ctx := context.Background()
	form := collectionFormFromBundle(bc, topIDs, keyed)
	if err := form.Validate(); err != nil {
		t.Fatalf("Validate the bundle's form: %v", err)
	}
	tx, err := db.conn.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback() }()
	created, _, err := createCollectionTx(ctx, tx, profileID, form)
	if err != nil {
		t.Fatalf("createCollectionTx: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
	trees, err := db.GetCollectionsByIDs(ctx, []uuid.UUID{created.ID})
	if err != nil || len(trees) != 1 {
		t.Fatalf("reload written collection: %v (%d rows)", err, len(trees))
	}
	return trees[0]
}

// withoutSourceIDs clears every SourceID in b, which a copy never keeps.
func withoutSourceIDs(b Bundle) Bundle {
	clearCatalogs := func(catalogs []BundleCatalog) {
		for i := range catalogs {
			catalogs[i].SourceID = nil
		}
	}
	clearCatalogs(b.Catalogs)
	for i := range b.Collections {
		b.Collections[i].SourceID = nil
		clearCatalogs(b.Collections[i].Catalogs)
	}
	return b
}

// Extracting a stored collection, writing the bundle form back as a new
// collection and extracting that gives the same bundle, apart from SourceID:
// with scopeAll and keys, as a subscribe writes it, and without, as a
// Duplicate does, with
// listed catalogs reused by their SourceID. Every field is set to a
// non-default value so a field lost on the way shows up here.
func TestBundleRoundTripsThroughCreate(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner := newTestProfile(t, db, "owner")
	subscriber := newTestProfile(t, db, "subscriber")

	listedForm := listedCatalogForm("Listed")
	listedForm.Params = `{"sort_by":"popularity.desc","with_genres":"27"}`
	listed, err := db.CreateUserCatalog(ctx, owner, listedForm)
	if err != nil {
		t.Fatalf("create listed catalog: %v", err)
	}
	scoped := &NewScopedCatalog{
		Key: "draft:scoped", Type: "series", Name: "Scoped", Provider: "tmdb",
		Params: `{"with_networks":"213"}`,
	}
	folder := func(title, shape string, refs ...FolderCatalogRef) FolderData {
		return FolderData{
			Title: title, TileShape: shape, HideTitle: true, CoverEmoji: "🎃",
			CoverImageURL:   "https://example.com/" + title + "/cover.png",
			FocusGIFURL:     "https://example.com/" + title + "/focus.gif",
			FocusGIFEnabled: true,
			HeroBackdropURL: "https://example.com/" + title + "/hero.jpg",
			HeroVideoURL:    "https://example.com/" + title + "/hero.mp4",
			TitleLogoURL:    "https://example.com/" + title + "/logo.png",
			Catalogs:        refs,
		}
	}
	source, err := db.CreateUserCollection(ctx, owner, CollectionForm{
		Title: "Halloween", ViewMode: "ROWS", ShowAllTab: true,
		BackdropImageURL: "https://example.com/backdrop.jpg", FocusGlowEnabled: true,
		Folders: []FolderData{
			folder("Classics", "LANDSCAPE",
				FolderCatalogRef{New: scoped, Genre: "Horror"},
				FolderCatalogRef{CatalogID: &listed.ID},
				FolderCatalogRef{CatalogID: &listed.ID, Genre: "War"}),
			folder("Modern", "SQUARE",
				FolderCatalogRef{New: scoped},
				FolderCatalogRef{CatalogID: &listed.ID, Genre: "Western"}),
		},
	})
	if err != nil {
		t.Fatalf("create source collection: %v", err)
	}
	trees, err := db.GetCollectionsByIDs(ctx, []uuid.UUID{source.ID})
	if err != nil {
		t.Fatalf("reload source collection: %v", err)
	}

	t.Run("scopeAll", func(t *testing.T) {
		want := extractBundle(nil, trees, true)
		written := writeBundleCollection(t, db, subscriber, want.Collections[0], nil, true)
		got := extractBundle(nil, []CollectionWithFolders{written}, true)
		if g, w := withoutSourceIDs(got), withoutSourceIDs(want); !reflect.DeepEqual(g, w) {
			t.Fatalf("round trip = %+v, want %+v", g, w)
		}
	})

	t.Run("listed top-level", func(t *testing.T) {
		want := extractBundle(nil, trees, false)
		topIDs := map[string]uuid.UUID{}
		for _, c := range want.Catalogs {
			topIDs[c.Key] = *c.SourceID
		}
		written := writeBundleCollection(t, db, owner, want.Collections[0], topIDs, false)
		if got := written.Folders[0].Refs[1].CatalogID; got != listed.ID {
			t.Errorf("listed ref points at %s, want the listed catalog %s", got, listed.ID)
		}
		got := extractBundle(nil, []CollectionWithFolders{written}, false)
		if g, w := withoutSourceIDs(got), withoutSourceIDs(want); !reflect.DeepEqual(g, w) {
			t.Fatalf("round trip = %+v, want %+v", g, w)
		}
	})
}

// A publish holds every catalog its snapshot shares to CatalogForm's rules,
// and a Duplicate every catalog it writes as a new row: a stored catalog
// with a blank name, an unknown type or provider, or an overlong name is
// refused by either. A Duplicate leaves a listed catalog a reference to the
// caller's own row and writes nothing from it, so it doesn't check one.
func TestCopiesHoldCopiedCatalogsToTheCatalogRules(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner := newTestProfile(t, db, "owner")

	listed, err := db.CreateUserCatalog(ctx, owner, listedCatalogForm("Listed"))
	if err != nil {
		t.Fatalf("create listed catalog: %v", err)
	}
	onlyListed, err := db.CreateUserCollection(ctx, owner, CollectionForm{
		Title:   "Only listed",
		Folders: []FolderData{{Title: "F", Catalogs: CatalogRefs(listed.ID)}},
	})
	if err != nil {
		t.Fatalf("create collection referencing the listed catalog: %v", err)
	}
	withScoped, err := db.CreateUserCollection(ctx, owner, CollectionForm{
		Title: "With scoped",
		Folders: []FolderData{{Title: "F", Catalogs: []FolderCatalogRef{{New: &NewScopedCatalog{
			Key: "draft:scoped", Type: "movie", Name: "Scoped", Provider: "tmdb", Params: "{}",
		}}}}},
	})
	if err != nil {
		t.Fatalf("create collection with a scoped catalog: %v", err)
	}
	scopedID := withScoped.Folders[0].Refs[0].CatalogID

	for _, tt := range []struct {
		name  string
		query string
		value string
	}{
		{"blank name", `UPDATE catalogs SET name = ? WHERE id = ?`, " "},
		{"overlong name", `UPDATE catalogs SET name = ? WHERE id = ?`, strings.Repeat("n", maxNameLen+1)},
		{"unknown type", `UPDATE recipes SET type = ? WHERE hash = (SELECT recipe_hash FROM catalogs WHERE id = ?)`, "anime"},
		{"unknown provider", `UPDATE recipes SET provider = ? WHERE hash = (SELECT recipe_hash FROM catalogs WHERE id = ?)`, "mdblist"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			corrupt := func(id uuid.UUID, name string) {
				t.Helper()
				if _, err := db.conn.ExecContext(ctx, tt.query, tt.value, id.String()); err != nil {
					t.Fatalf("writing the stale row: %v", err)
				}
				t.Cleanup(func() {
					if _, err := db.conn.ExecContext(ctx, `UPDATE catalogs SET name = ? WHERE id = ?`, name, id.String()); err != nil {
						t.Errorf("restoring the catalog row: %v", err)
					}
					if _, err := db.conn.ExecContext(ctx, `
						UPDATE recipes SET type = 'movie', provider = 'tmdb' WHERE hash = (SELECT recipe_hash FROM catalogs WHERE id = ?)
					`, id.String()); err != nil {
						t.Errorf("restoring the recipe row: %v", err)
					}
				})
			}

			t.Run("listed", func(t *testing.T) {
				corrupt(listed.ID, "Listed")
				if _, err := db.PublishCatalog(ctx, owner, listed.ID, allowAnyCatalogParams); !errors.Is(err, ErrInvalidInput) {
					t.Errorf("PublishCatalog = %v, want ErrInvalidInput", err)
				}
				if _, err := db.PublishCollection(ctx, owner, onlyListed.ID, allowAnyCatalogParams); !errors.Is(err, ErrInvalidInput) {
					t.Errorf("PublishCollection = %v, want ErrInvalidInput", err)
				}
				if _, err := db.DuplicateCollection(ctx, owner, onlyListed.ID); err != nil {
					t.Errorf("DuplicateCollection referencing the listed catalog = %v, want nil", err)
				}
			})

			t.Run("scoped", func(t *testing.T) {
				corrupt(scopedID, "Scoped")
				if _, err := db.PublishCollection(ctx, owner, withScoped.ID, allowAnyCatalogParams); !errors.Is(err, ErrInvalidInput) {
					t.Errorf("PublishCollection = %v, want ErrInvalidInput", err)
				}
				if _, err := db.DuplicateCollection(ctx, owner, withScoped.ID); !errors.Is(err, ErrInvalidInput) {
					t.Errorf("DuplicateCollection = %v, want ErrInvalidInput", err)
				}
			})
		})
	}
}

// New entries sharing a Key are one catalog, so they must agree on SubKey
// too: a subscribe writes each with its snapshot key.
func TestCollectionFormSharedKeyComparesSubKey(t *testing.T) {
	form := func(a, b string) CollectionForm {
		spec := func(subKey string) *NewScopedCatalog {
			return &NewScopedCatalog{Key: "c1", Type: "movie", Name: "Shared", Provider: "tmdb", Params: "{}", SubKey: subKey}
		}
		return CollectionForm{Title: "C", Folders: []FolderData{
			{Title: "F1", Catalogs: []FolderCatalogRef{{New: spec(a)}}},
			{Title: "F2", Catalogs: []FolderCatalogRef{{New: spec(b)}}},
		}}
	}
	if err := form("k", "k").Validate(); err != nil {
		t.Fatalf("Validate with one sub_key = %v, want nil", err)
	}
	err := form("k", "other").Validate()
	if !errors.Is(err, ErrInvalidInput) || !strings.Contains(err.Error(), "key is shared with a different catalog spec") {
		t.Errorf("Validate with two sub_keys = %v, want the shared-key problem", err)
	}
}
