package vault

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

// DuplicateCollection keeps every folder ref: a listed source catalog stays
// a reference (same id), while each distinct scoped source catalog becomes
// one fresh scoped copy in the new collection, referenced everywhere the
// source referenced it — the same one-copy-per-distinct-source-catalog rule
// TakeCollection uses. taken_from stays nil throughout: this is a copy of
// the caller's own data, not a take.
func TestDuplicateCollection(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)

	owner := newTestProfile(t, db, "owner")

	listed, err := db.CreateUserCatalog(ctx, owner, listedCatalogForm("Listed"))
	if err != nil {
		t.Fatalf("create listed catalog: %v", err)
	}

	source, err := db.CreateUserCollection(ctx, owner, CollectionForm{Title: "Source"})
	if err != nil {
		t.Fatalf("create source collection: %v", err)
	}
	scopedForm := listedCatalogForm("Scoped")
	scopedForm.CollectionID = &source.ID
	scoped, err := db.CreateUserCatalog(ctx, owner, scopedForm)
	if err != nil {
		t.Fatalf("create scoped catalog: %v", err)
	}

	source, err = db.UpdateUserCollection(ctx, owner, source.ID, CollectionForm{
		Title: "Source",
		Folders: []FolderData{
			{Title: "Folder 1", Catalogs: CatalogRefs(listed.ID, scoped.ID)},
			{Title: "Folder 2", Catalogs: CatalogRefs(scoped.ID)},
		},
	})
	if err != nil {
		t.Fatalf("saving source collection: %v", err)
	}

	dup, err := db.DuplicateCollection(ctx, owner, source.ID)
	if err != nil {
		t.Fatalf("DuplicateCollection: %v", err)
	}

	if dup.ID == source.ID {
		t.Fatalf("duplicate reused the source's id")
	}
	if dup.OwnerID != owner {
		t.Fatalf("duplicate owner = %s, want %s", dup.OwnerID, owner)
	}
	if dup.IsPublic {
		t.Fatalf("duplicate is_public = true, want false")
	}
	if dup.TakenFrom != nil {
		t.Fatalf("duplicate taken_from = %v, want nil", dup.TakenFrom)
	}
	if dup.Title != "Source (copy)" {
		t.Fatalf("duplicate title = %q, want %q", dup.Title, "Source (copy)")
	}
	if len(dup.Folders) != 2 {
		t.Fatalf("duplicate has %d folders, want 2", len(dup.Folders))
	}

	if len(dup.Folders[0].CatalogIDs()) != 2 || len(dup.Folders[1].CatalogIDs()) != 1 {
		t.Fatalf("duplicate folder catalog counts = %d, %d, want 2, 1",
			len(dup.Folders[0].CatalogIDs()), len(dup.Folders[1].CatalogIDs()))
	}

	// The listed catalog stayed a reference: same id.
	if dup.Folders[0].CatalogIDs()[0] != listed.ID {
		t.Fatalf("duplicate folder 1's first catalog = %s, want the same listed catalog %s", dup.Folders[0].CatalogIDs()[0], listed.ID)
	}

	// The scoped catalog became one new scoped copy, referenced by both folders.
	newScopedID := dup.Folders[1].CatalogIDs()[0]
	if newScopedID == scoped.ID {
		t.Fatalf("duplicate's scoped catalog reused the source's id")
	}
	if dup.Folders[0].CatalogIDs()[1] != newScopedID {
		t.Fatalf("duplicate folder 1's second catalog %s != folder 2's catalog %s, want the same new scoped copy",
			dup.Folders[0].CatalogIDs()[1], newScopedID)
	}

	var newScoped Catalog
	for _, c := range dup.Catalogs {
		if c.ID == newScopedID {
			newScoped = c
		}
	}
	if newScoped.ID == uuid.Nil {
		t.Fatalf("duplicate.Catalogs missing the new scoped copy %s", newScopedID)
	}
	if newScoped.TakenFrom != nil {
		t.Fatalf("duplicate's scoped catalog taken_from = %v, want nil", newScoped.TakenFrom)
	}
	if newScoped.CollectionID == nil || *newScoped.CollectionID != dup.ID {
		t.Fatalf("duplicate's scoped catalog collection_id = %v, want %s", newScoped.CollectionID, dup.ID)
	}

	// The source is untouched by duplicating it.
	sourceAfter, err := db.GetCollectionsByIDs(ctx, []uuid.UUID{source.ID})
	if err != nil {
		t.Fatalf("reloading source collection: %v", err)
	}
	if len(sourceAfter) != 1 || len(sourceAfter[0].Catalogs) != 2 {
		t.Fatalf("source collection changed by duplicating it: %+v", sourceAfter)
	}
}

// Duplicating a collection you don't own is ErrCollectionNotFound — the
// caller shouldn't be told whether the id exists at all.
func TestDuplicateCollectionNotFound(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)

	owner := newTestProfile(t, db, "owner")
	other := newTestProfile(t, db, "other")

	notMine, err := db.CreateUserCollection(ctx, other, CollectionForm{Title: "Not mine"})
	if err != nil {
		t.Fatalf("create other's collection: %v", err)
	}
	if _, err := db.DuplicateCollection(ctx, owner, notMine.ID); !errors.Is(err, ErrCollectionNotFound) {
		t.Fatalf("duplicate other's collection: got %v, want ErrCollectionNotFound", err)
	}
}
