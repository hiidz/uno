package vault

import (
	"context"
	"database/sql"
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

// createScopedCatalog writes form as profileID's catalog scoped to
// collectionID: the row a New entry in that collection's save writes, without
// the save.
func createScopedCatalog(t *testing.T, db *DB, profileID, collectionID uuid.UUID, form CatalogForm) Catalog {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	var created Catalog
	err := db.inTx(ctx, func(tx *sql.Tx) error {
		var err error
		created, err = insertCatalog(ctx, tx, Catalog{
			ID: uuid.New(), Type: form.Type, Name: form.Name, Provider: form.Provider, Params: form.Params,
			OwnerID: profileID, CollectionID: &collectionID, CreatedAt: now, UpdatedAt: now,
		})
		return err
	})
	if err != nil {
		t.Fatalf("creating catalog %q scoped to %s: %v", form.Name, collectionID, err)
	}
	return created
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

	createScopedCatalog(t, db, owner, collectionID, listedCatalogForm("Scoped"))

	got, err := db.GetUserCatalogs(ctx, owner)
	if err != nil {
		t.Fatalf("GetUserCatalogs: %v", err)
	}
	if len(got) != 1 || got[0].ID != listed.ID {
		t.Fatalf("GetUserCatalogs = %+v, want exactly the listed catalog %s", got, listed.ID)
	}
}

// A catalog inside a collection is written only through that collection's
// save: PUT and DELETE on it are refused as invalid input, not as a missing
// row, while an id that isn't the caller's at all is still not found.
func TestUpdateAndDeleteUserCatalogRefuseScoped(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)

	owner := newTestProfile(t, db, "owner")
	collectionID := newTestCollection(t, db, owner, "My Collection")
	scoped := createScopedCatalog(t, db, owner, collectionID, listedCatalogForm("Scoped"))

	if _, err := db.UpdateUserCatalog(ctx, owner, scoped.ID, listedCatalogForm("Scoped")); !errors.Is(err, ErrInvalidInput) {
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

	scoped := createScopedCatalog(t, db, owner, collectionID, listedCatalogForm("Scoped"))

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
