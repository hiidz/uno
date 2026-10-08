package vault

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

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
	published, err := db.PublishCatalog(context.Background(), owner, c.ID)
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
	published, err := db.PublishCollection(context.Background(), owner, c.ID)
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

// subscribeCollection publishes owner's collection sourceID and subscribes
// subscriber to it, returning subscriber's copy.
func subscribeCollection(t *testing.T, db *DB, owner, subscriber, sourceID uuid.UUID) CollectionWithFolders {
	t.Helper()
	published, err := db.PublishCollection(context.Background(), owner, sourceID)
	if err != nil {
		t.Fatalf("PublishCollection: %v", err)
	}
	return *subscribe(t, db, subscriber, published.Publication.ID).Collection
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
// collections on Home in the order and with the pins entries give, with no
// catalog rows, and stores the push record of that Home as it is now.
func pushSelection(t *testing.T, db *DB, profileID uuid.UUID, entries ...SelectedCollectionInput) {
	t.Helper()
	savePush(t, db, profileID, PushedHome{Collections: entries})
}

// savePush stands in for push's local write of home: it builds its push
// record now and stores it.
func savePush(t *testing.T, db *DB, profileID uuid.UUID, home PushedHome) {
	t.Helper()
	record, err := db.BuildPushRecord(context.Background(), profileID, home)
	if err != nil {
		t.Fatalf("BuildPushRecord: %v", err)
	}
	if _, err := db.SavePush(context.Background(), profileID, record); err != nil {
		t.Fatalf("SavePush: %v", err)
	}
}
