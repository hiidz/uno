package vault

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

// fillCatalogs gives profileID n listed catalogs.
func fillCatalogs(t *testing.T, db *DB, profileID uuid.UUID, n int) {
	t.Helper()
	for range n {
		if _, err := db.CreateUserCatalog(context.Background(), profileID, listedCatalogForm("Filler")); err != nil {
			t.Fatalf("filling catalogs: %v", err)
		}
	}
}

// fillCollections gives profileID n collections.
func fillCollections(t *testing.T, db *DB, profileID uuid.UUID, n int) {
	t.Helper()
	for range n {
		if _, err := db.CreateUserCollection(context.Background(), profileID, CollectionForm{Title: "Filler", ViewMode: "ROWS"}); err != nil {
			t.Fatalf("filling collections: %v", err)
		}
	}
}

// A profile at its catalog cap can add no more by any path: create, duplicate,
// subscribe, Community duplicate, or an import, which is refused whole.
func TestListedCatalogCapRefusesEveryAddingPath(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	publisher, profile := newTestProfile(t, db, "publisher"), newTestProfile(t, db, "profile")
	published := publishCatalog(t, db, publisher, "Popular", "{}")

	fillCatalogs(t, db, profile, maxListedCatalogsPerProfile)
	own, err := db.GetUserCatalogs(ctx, profile)
	if err != nil || len(own) != maxListedCatalogsPerProfile {
		t.Fatalf("GetUserCatalogs = %d rows, %v; want the cap", len(own), err)
	}

	for name, err := range map[string]error{
		"create":              second(db.CreateUserCatalog(ctx, profile, listedCatalogForm("One more"))),
		"duplicate":           second(db.DuplicateCatalog(ctx, profile, own[0].ID)),
		"subscribe":           second(db.Subscribe(ctx, profile, published.Publication.ID)),
		"Community duplicate": second(db.DuplicatePublication(ctx, profile, published.Publication.ID)),
		"import":              third(db.ImportBundle(ctx, profile, readTestBundle(t), nil)),
	} {
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("%s at the cap = %v, want ErrInvalidInput", name, err)
		}
	}
	if got, _ := db.GetUserCatalogs(ctx, profile); len(got) != maxListedCatalogsPerProfile {
		t.Errorf("catalogs after the refusals = %d, want %d", len(got), maxListedCatalogsPerProfile)
	}
}

// An import that would carry a profile past the cap writes nothing, not the
// rows that fit.
func TestImportOverTheCatalogCapWritesNothing(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	profile := newTestProfile(t, db, "profile")

	fillCatalogs(t, db, profile, maxListedCatalogsPerProfile-1)
	collections, err := db.GetUserCollections(ctx, profile)
	if err != nil {
		t.Fatal(err)
	}
	if err := third(db.ImportBundle(ctx, profile, readTestBundle(t), nil)); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("import over the cap = %v, want ErrInvalidInput", err)
	}
	if got, _ := db.GetUserCatalogs(ctx, profile); len(got) != maxListedCatalogsPerProfile-1 {
		t.Errorf("catalogs after the refused import = %d, want %d", len(got), maxListedCatalogsPerProfile-1)
	}
	if after, _ := db.GetUserCollections(ctx, profile); len(after) != len(collections) {
		t.Errorf("collections after the refused import = %d, want %d", len(after), len(collections))
	}
}

// A profile at its collection cap can add no more by any path.
func TestCollectionCapRefusesEveryAddingPath(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	publisher, profile := newTestProfile(t, db, "publisher"), newTestProfile(t, db, "profile")
	published := publishCollection(t, db, publisher, CollectionForm{Title: "Shared", ViewMode: "ROWS"})

	fillCollections(t, db, profile, maxCollectionsPerProfile)
	own, err := db.GetUserCollections(ctx, profile)
	if err != nil || len(own) != maxCollectionsPerProfile {
		t.Fatalf("GetUserCollections = %d rows, %v; want the cap", len(own), err)
	}

	for name, err := range map[string]error{
		"create":              second(db.CreateUserCollection(ctx, profile, CollectionForm{Title: "One more", ViewMode: "ROWS"})),
		"duplicate":           second(db.DuplicateCollection(ctx, profile, own[0].ID)),
		"subscribe":           second(db.Subscribe(ctx, profile, published.Publication.ID)),
		"Community duplicate": second(db.DuplicatePublication(ctx, profile, published.Publication.ID)),
		"import":              third(db.ImportBundle(ctx, profile, readTestBundle(t), nil)),
	} {
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("%s at the cap = %v, want ErrInvalidInput", name, err)
		}
	}
}

// Saving a row, and a collection's own scoped catalogs, are not adds: a profile
// at both caps still edits what it has.
func TestCapsLeaveSavesAndScopedCatalogsAlone(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	profile := newTestProfile(t, db, "profile")

	fillCatalogs(t, db, profile, maxListedCatalogsPerProfile-1)
	collection, err := db.CreateUserCollection(ctx, profile, CollectionForm{
		Title: "Scoped", ViewMode: "ROWS",
		Folders: []FolderData{{FolderArt: FolderArt{TileShape: "POSTER"}, Title: "F", Catalogs: []FolderCatalogRef{newScoped("k", "S", "{}")}}},
	})
	if err != nil {
		t.Fatalf("a collection with a scoped catalog next to %d listed ones: %v", maxListedCatalogsPerProfile-1, err)
	}
	fillCatalogs(t, db, profile, 1)

	own, _ := db.GetUserCatalogs(ctx, profile)
	if _, err := db.UpdateUserCatalog(ctx, profile, own[0].ID, listedCatalogForm("Renamed")); err != nil {
		t.Errorf("save of a catalog at the cap = %v, want it saved", err)
	}
	if _, err := db.UpdateUserCollection(ctx, profile, collection.ID, saveFormOf(mustOwnCollection(t, db, profile, collection.ID))); err != nil {
		t.Errorf("save of a collection with a scoped catalog at the cap = %v, want it saved", err)
	}
}

// third is the error of a three-value call.
func third[A, B any](_ A, _ B, err error) error { return err }
