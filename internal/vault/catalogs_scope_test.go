package vault

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

func newTestProfile(t *testing.T, db *DB, nuvioUserID string) uuid.UUID {
	t.Helper()
	p, err := db.ResolveOrCreateProfile(context.Background(), nuvioUserID, 1, "nuvio-profile-"+nuvioUserID)
	if err != nil {
		t.Fatalf("creating profile %s: %v", nuvioUserID, err)
	}
	return p.ID
}

func newTestCollection(t *testing.T, db *DB, ownerID uuid.UUID, title string) uuid.UUID {
	t.Helper()
	c, err := db.CreateUserCollection(context.Background(), ownerID, CollectionForm{Title: title})
	if err != nil {
		t.Fatalf("creating collection %q: %v", title, err)
	}
	return c.ID
}

func listedCatalogForm(name string) CatalogForm {
	return CatalogForm{Type: "movie", Name: name, Provider: "tmdb", Params: "{}"}
}

// A catalog created with collection_id set must belong to a collection the
// caller owns.
func TestCreateScopedCatalogRequiresOwnedCollection(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)

	owner := newTestProfile(t, db, "owner")
	other := newTestProfile(t, db, "other")
	collectionID := newTestCollection(t, db, owner, "My Collection")

	// Scoping to a collection you don't own is rejected.
	form := listedCatalogForm("Scoped")
	form.CollectionID = &collectionID
	if _, err := db.CreateUserCatalog(ctx, other, form); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("create scoped to another owner's collection: got %v, want ErrInvalidInput", err)
	}

	// Scoping to your own collection, not public, succeeds.
	form = listedCatalogForm("Scoped")
	form.CollectionID = &collectionID
	c, err := db.CreateUserCatalog(ctx, owner, form)
	if err != nil {
		t.Fatalf("create scoped catalog: %v", err)
	}
	if c.CollectionID == nil || *c.CollectionID != collectionID {
		t.Fatalf("created catalog's collection_id = %v, want %s", c.CollectionID, collectionID)
	}
}

// GetUserCatalogs (the library) must not surface catalogs scoped to a
// collection — they're reached through the collection's own tree.
func TestGetUserCatalogsExcludesScoped(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)

	owner := newTestProfile(t, db, "owner")
	collectionID := newTestCollection(t, db, owner, "My Collection")

	listed, err := db.CreateUserCatalog(ctx, owner, listedCatalogForm("Listed"))
	if err != nil {
		t.Fatalf("create listed catalog: %v", err)
	}

	scopedForm := listedCatalogForm("Scoped")
	scopedForm.CollectionID = &collectionID
	if _, err := db.CreateUserCatalog(ctx, owner, scopedForm); err != nil {
		t.Fatalf("create scoped catalog: %v", err)
	}

	got, err := db.GetUserCatalogs(ctx, owner)
	if err != nil {
		t.Fatalf("GetUserCatalogs: %v", err)
	}
	if len(got) != 1 || got[0].ID != listed.ID {
		t.Fatalf("GetUserCatalogs = %+v, want exactly the listed catalog %s", got, listed.ID)
	}
}

// A collection_id sent with an update of a listed catalog changes nothing: the
// catalog stays listed and published, and a subscribed copy the id names is
// not detached.
func TestUpdateUserCatalogIgnoresCollectionID(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)

	owner := newTestProfile(t, db, "owner")
	taker := newTestProfile(t, db, "taker")
	source := newTestCollection(t, db, owner, "Source")
	copied := takeCollection(t, db, owner, taker, source)
	if copied.Subscription == nil {
		t.Fatal("the taken collection has no subscription")
	}

	catalog := publishCatalog(t, db, taker, "Listed", `{"sort_by":"vote_average.desc"}`)

	form := listedCatalogForm("Renamed")
	form.Params = catalog.Params
	form.CollectionID = &copied.ID
	updated, err := db.UpdateUserCatalog(ctx, taker, catalog.ID, form)
	if err != nil {
		t.Fatalf("update with a collection_id: %v", err)
	}
	if updated.Name != "Renamed" || updated.CollectionID != nil {
		t.Fatalf("updated catalog = name %q, collection_id %v, want %q and listed", updated.Name, updated.CollectionID, "Renamed")
	}
	if reloaded := reloadCatalog(t, db, catalog.ID); reloaded.Publication == nil || reloaded.Publication.Status != "live" {
		t.Errorf("publication after the update = %+v, want live", reloaded.Publication)
	}
	if after := mustOwnCollection(t, db, taker, copied.ID); after.Subscription == nil {
		t.Error("the collection named in collection_id was detached")
	}
}

// moveToLibraryEdit is the catalog edit "Move to library" saves: c's own
// name and recipe, unchanged, with MoveToLibrary set.
func moveToLibraryEdit(c Catalog) ScopedCatalogEdit {
	return ScopedCatalogEdit{ID: c.ID, Type: c.Type, Provider: c.Provider, Name: c.Name, Params: c.Params, MoveToLibrary: true}
}

// A catalog inside a collection is written only through that collection's
// save: PUT and DELETE on it are refused as invalid input, not as a missing
// row, while an id that isn't the caller's at all is still not found.
func TestUpdateAndDeleteUserCatalogRefuseScoped(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)

	owner := newTestProfile(t, db, "owner")
	collectionID := newTestCollection(t, db, owner, "My Collection")
	scopedForm := listedCatalogForm("Scoped")
	scopedForm.CollectionID = &collectionID
	scoped, err := db.CreateUserCatalog(ctx, owner, scopedForm)
	if err != nil {
		t.Fatalf("create scoped catalog: %v", err)
	}

	if _, err := db.UpdateUserCatalog(ctx, owner, scoped.ID, scopedForm); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("PUT on a scoped catalog = %v, want ErrInvalidInput", err)
	}
	if err := db.DeleteUserCatalog(ctx, owner, scoped.ID); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("DELETE on a scoped catalog = %v, want ErrInvalidInput", err)
	}
	if err := db.DeleteUserCatalog(ctx, owner, uuid.New()); !errors.Is(err, ErrCatalogNotFound) {
		t.Errorf("DELETE on an unknown catalog = %v, want ErrCatalogNotFound", err)
	}

	remaining, err := db.queryCatalogs(ctx, "c.id = ?", scoped.ID.String())
	if err != nil {
		t.Fatalf("querying for scoped catalog: %v", err)
	}
	if len(remaining) != 1 || remaining[0].Name != "Scoped" {
		t.Fatalf("scoped catalog after refused writes = %+v, want it untouched", remaining)
	}
}

// A catalog's type can't be changed by UpdateUserCatalog — the pushed
// collections blob carries each folder source's catalog type, so a change
// here would alter what Nuvio should have without bumping any collection's
// version.
func TestUpdateUserCatalogRejectsTypeChange(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)

	owner := newTestProfile(t, db, "owner")
	c, err := db.CreateUserCatalog(ctx, owner, listedCatalogForm("Catalog"))
	if err != nil {
		t.Fatalf("create catalog: %v", err)
	}

	form := listedCatalogForm("Catalog")
	form.Type = "series"
	if _, err := db.UpdateUserCatalog(ctx, owner, c.ID, form); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("update with changed type: got %v, want ErrInvalidInput", err)
	}

	unchanged, err := db.UpdateUserCatalog(ctx, owner, c.ID, listedCatalogForm("Catalog"))
	if err != nil {
		t.Fatalf("update with unchanged type: %v", err)
	}
	if unchanged.Type != "movie" {
		t.Fatalf("catalog type = %q after a same-type update, want %q", unchanged.Type, "movie")
	}
}

// Deleting a collection cascades to the scoped catalogs that lived inside
// it.
func TestDeleteCollectionCascadesScopedCatalogs(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)

	owner := newTestProfile(t, db, "owner")
	collectionID := newTestCollection(t, db, owner, "My Collection")

	scopedForm := listedCatalogForm("Scoped")
	scopedForm.CollectionID = &collectionID
	scoped, err := db.CreateUserCatalog(ctx, owner, scopedForm)
	if err != nil {
		t.Fatalf("create scoped catalog: %v", err)
	}

	if err := db.DeleteUserCollection(ctx, owner, collectionID); err != nil {
		t.Fatalf("deleting collection: %v", err)
	}

	remaining, err := db.queryCatalogs(ctx, "c.id = ?", scoped.ID.String())
	if err != nil {
		t.Fatalf("querying for scoped catalog after collection delete: %v", err)
	}
	if len(remaining) != 0 {
		t.Fatalf("scoped catalog %s survived its collection's deletion", scoped.ID)
	}
}
