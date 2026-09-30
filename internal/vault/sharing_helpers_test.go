package vault

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

// allowAnyCatalogParams is a CatalogParamsValidator that accepts every
// recipe, for tests exercising a publish rather than the recipe check. The
// real validator lives in internal/api, which this package cannot import.
func allowAnyCatalogParams(_, _, _ string) error { return nil }

// publishCatalog creates a listed catalog named name for owner and
// publishes it, returning the catalog with its publication.
func publishCatalog(t *testing.T, db *DB, owner uuid.UUID, name, params string) Catalog {
	t.Helper()
	form := listedCatalogForm(name)
	form.Params = params
	c, err := db.CreateUserCatalog(context.Background(), owner, form)
	if err != nil {
		t.Fatalf("create catalog %q: %v", name, err)
	}
	published, err := db.PublishCatalog(context.Background(), owner, c.ID, allowAnyCatalogParams)
	if err != nil {
		t.Fatalf("publish catalog %q: %v", name, err)
	}
	return published
}

// publishCollection creates form as owner's collection and publishes it,
// returning the collection with its publication.
func publishCollection(t *testing.T, db *DB, owner uuid.UUID, form CollectionForm) CollectionWithFolders {
	t.Helper()
	c, err := db.CreateUserCollection(context.Background(), owner, form)
	if err != nil {
		t.Fatalf("create collection %q: %v", form.Title, err)
	}
	published, err := db.PublishCollection(context.Background(), owner, c.ID, allowAnyCatalogParams)
	if err != nil {
		t.Fatalf("publish collection %q: %v", form.Title, err)
	}
	return published
}

// subscribe subscribes profileID to publicationID, failing the test on an
// error.
func subscribe(t *testing.T, db *DB, profileID, publicationID uuid.UUID) CommunityCopy {
	t.Helper()
	copied, err := db.Subscribe(context.Background(), profileID, publicationID)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	return copied
}

// newScoped is an inline New entry for a scoped catalog under key.
func newScoped(key, name, params string) FolderCatalogRef {
	return FolderCatalogRef{New: &NewScopedCatalog{Key: key, Type: "movie", Name: name, Provider: "tmdb", Params: params}}
}

// takeCollection publishes owner's collection sourceID and subscribes taker
// to it, returning taker's copy.
func takeCollection(t *testing.T, db *DB, owner, taker, sourceID uuid.UUID) CollectionWithFolders {
	t.Helper()
	published, err := db.PublishCollection(context.Background(), owner, sourceID, allowAnyCatalogParams)
	if err != nil {
		t.Fatalf("PublishCollection: %v", err)
	}
	return *subscribe(t, db, taker, published.Publication.ID).Collection
}

// reloadCatalog reads catalog id back, listed or scoped, whoever owns it,
// with its sharing state.
func reloadCatalog(t *testing.T, db *DB, id uuid.UUID) Catalog {
	t.Helper()
	catalogs, err := selectCatalogs(context.Background(), db.conn, "c.id = ?", id.String())
	if err != nil || len(catalogs) != 1 {
		t.Fatalf("reload catalog %s: %v (%d rows)", id, err, len(catalogs))
	}
	return catalogs[0]
}

// mustOwnCollection reads profileID's collection id, failing the test on an
// error.
func mustOwnCollection(t *testing.T, db *DB, profileID, id uuid.UUID) CollectionWithFolders {
	t.Helper()
	tree, err := ownCollection(context.Background(), db.conn, profileID, id)
	if err != nil {
		t.Fatalf("reload collection %s: %v", id, err)
	}
	return tree
}

// pushSelection stands in for push's local write: it puts profileID's
// collections on Home in the order and with the pins entries give, stamps
// each with the hash of what push would send for it now, and clears every
// other one's place.
func pushSelection(t *testing.T, db *DB, profileID uuid.UUID, entries ...SelectedCollectionInput) {
	t.Helper()
	form := CollectionSelectionForm{Collections: entries}
	trees, err := db.GetCollectionsByIDs(context.Background(), form.CollectionIDs())
	if err != nil {
		t.Fatalf("GetCollectionsByIDs: %v", err)
	}
	hashes := map[uuid.UUID]string{}
	for _, tree := range trees {
		tree.PinToTop = pinOf(entries, tree.ID)
		raw, err := tree.PushJSON()
		if err != nil {
			t.Fatal(err)
		}
		hashes[tree.ID] = PushHash(raw)
	}
	if err := db.SaveSelectionsForPush(context.Background(), profileID, CatalogSelectionForm{}, form, hashes); err != nil {
		t.Fatalf("SaveSelectionsForPush: %v", err)
	}
}

// pinOf is the pin entries give collection id.
func pinOf(entries []SelectedCollectionInput, id uuid.UUID) bool {
	for _, e := range entries {
		if e.CollectionID == id {
			return e.PinToTop
		}
	}
	return false
}
