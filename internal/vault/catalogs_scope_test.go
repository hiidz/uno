package vault

import (
	"context"
	"errors"
	"testing"
	"time"

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
// caller owns, and cannot be public.
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

	// Scoping to your own collection while also public is rejected.
	form = listedCatalogForm("Scoped and public")
	form.CollectionID = &collectionID
	form.IsPublic = true
	if _, err := db.CreateUserCatalog(ctx, owner, form); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("create scoped+public: got %v, want ErrInvalidInput", err)
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

// Demoting a listed catalog into a collection is refused while it's on the
// home screen.
func TestUpdateUserCatalogDemoteRefusedWhileOnHome(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)

	owner := newTestProfile(t, db, "owner")
	collectionID := newTestCollection(t, db, owner, "My Collection")

	catalog, err := db.CreateUserCatalog(ctx, owner, listedCatalogForm("Listed"))
	if err != nil {
		t.Fatalf("create listed catalog: %v", err)
	}

	if _, err := db.conn.ExecContext(ctx, `
		UPDATE catalogs SET home_sort_order = 0, show_in_home = 1 WHERE id = ?
	`, catalog.ID.String()); err != nil {
		t.Fatalf("seeding home selection: %v", err)
	}

	form := listedCatalogForm("Listed")
	form.CollectionID = &collectionID
	if _, err := db.UpdateUserCatalog(ctx, owner, catalog.ID, form); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("demote while on home: got %v, want ErrInvalidInput", err)
	}
}

// Demoting a catalog into a collection is refused if a folder outside that
// collection still references it.
func TestUpdateUserCatalogDemoteRefusedWhenReferencedOutsideCollection(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)

	owner := newTestProfile(t, db, "owner")
	targetCollection := newTestCollection(t, db, owner, "Target")
	otherCollection := newTestCollection(t, db, owner, "Other")

	catalog, err := db.CreateUserCatalog(ctx, owner, listedCatalogForm("Listed"))
	if err != nil {
		t.Fatalf("create listed catalog: %v", err)
	}

	// Reference the catalog from a folder in otherCollection.
	if _, err := db.UpdateUserCollection(ctx, owner, otherCollection, CollectionForm{
		Title:   "Other",
		Folders: []FolderData{{Title: "Folder", Catalogs: CatalogRefs(catalog.ID)}},
	}); err != nil {
		t.Fatalf("adding folder ref in other collection: %v", err)
	}

	form := listedCatalogForm("Listed")
	form.CollectionID = &targetCollection
	if _, err := db.UpdateUserCatalog(ctx, owner, catalog.ID, form); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("demote while referenced elsewhere: got %v, want ErrInvalidInput", err)
	}
}

// Demoting succeeds when the catalog is off the home screen and every
// existing folder ref to it is already inside the target collection;
// promoting it back to listed, through the collection's save, always
// succeeds.
func TestUpdateUserCatalogDemoteThenPromote(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)

	owner := newTestProfile(t, db, "owner")
	collectionID := newTestCollection(t, db, owner, "My Collection")

	catalog, err := db.CreateUserCatalog(ctx, owner, listedCatalogForm("Listed"))
	if err != nil {
		t.Fatalf("create listed catalog: %v", err)
	}

	demoteForm := listedCatalogForm("Listed")
	demoteForm.CollectionID = &collectionID
	demoted, err := db.UpdateUserCatalog(ctx, owner, catalog.ID, demoteForm)
	if err != nil {
		t.Fatalf("demote: %v", err)
	}
	if demoted.CollectionID == nil || *demoted.CollectionID != collectionID {
		t.Fatalf("demoted catalog's collection_id = %v, want %s", demoted.CollectionID, collectionID)
	}
	// CreatedAt round-trips through an RFC3339 TEXT column (second
	// precision), so compare at that precision rather than the in-memory
	// value's sub-second one.
	if !demoted.CreatedAt.Equal(catalog.CreatedAt.Truncate(time.Second)) {
		t.Fatalf("demote changed created_at: got %v, want %v", demoted.CreatedAt, catalog.CreatedAt)
	}

	if _, err := db.UpdateUserCollection(ctx, owner, collectionID, CollectionForm{
		Title:        "My Collection",
		Folders:      []FolderData{{Title: "Folder", Catalogs: CatalogRefs(catalog.ID)}},
		CatalogEdits: []ScopedCatalogEdit{moveToLibraryEdit(demoted)},
	}); err != nil {
		t.Fatalf("promote: %v", err)
	}
	library, err := db.GetUserCatalogs(ctx, owner)
	if err != nil {
		t.Fatalf("GetUserCatalogs: %v", err)
	}
	if len(library) != 1 || library[0].ID != catalog.ID || library[0].CollectionID != nil {
		t.Fatalf("library after promote = %+v, want the catalog listed again", library)
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

	remaining, err := db.queryCatalogs(ctx, "id = ?", scoped.ID.String())
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

	remaining, err := db.queryCatalogs(ctx, "id = ?", scoped.ID.String())
	if err != nil {
		t.Fatalf("querying for scoped catalog after collection delete: %v", err)
	}
	if len(remaining) != 0 {
		t.Fatalf("scoped catalog %s survived its collection's deletion", scoped.ID)
	}
}
