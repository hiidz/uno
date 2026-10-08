package vault

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

// A folder ref to another owner's catalog is rejected even if that catalog
// is public — the closed graph has no cross-owner reference in a write path.
func TestUpdateUserCollectionRejectsFolderRefToAnotherOwnersPublicCatalog(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)

	owner := newTestProfile(t, db, "owner")
	other := newTestProfile(t, db, "other")
	collectionID := newTestCollection(t, db, owner, "My Collection")

	othersCatalog, err := db.CreateUserCatalog(ctx, other, listedCatalogForm("Other's catalog"))
	if err != nil {
		t.Fatalf("create other's public catalog: %v", err)
	}

	_, err = db.UpdateUserCollection(ctx, owner, collectionID, collectionRevision(t, db, collectionID), CollectionForm{
		Title: "My Collection", ViewMode: "TABBED_GRID",
		Folders: []FolderData{{FolderArt: FolderArt{TileShape: "POSTER"}, Title: "Folder", Catalogs: CatalogRefs(othersCatalog.ID)}},
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("folder ref to another owner's public catalog: got %v, want ErrInvalidInput", err)
	}
}

// A folder ref to a catalog scoped to a different collection is rejected,
// even when both collections are owned by the same profile.
func TestUpdateUserCollectionRejectsFolderRefToCatalogScopedElsewhere(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)

	owner := newTestProfile(t, db, "owner")
	collectionA := newTestCollection(t, db, owner, "A")
	collectionB := newTestCollection(t, db, owner, "B")

	scoped := createScopedCatalog(t, db, owner, collectionA, listedCatalogForm("Scoped to A"))

	_, err := db.UpdateUserCollection(ctx, owner, collectionB, collectionRevision(t, db, collectionB), CollectionForm{
		Title: "B", ViewMode: "TABBED_GRID",
		Folders: []FolderData{{FolderArt: FolderArt{TileShape: "POSTER"}, Title: "Folder", Catalogs: CatalogRefs(scoped.ID)}},
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("folder ref in B to catalog scoped to A: got %v, want ErrInvalidInput", err)
	}
}

// CreateUserCollection has no collection id yet, so a folder ref to a
// catalog scoped to any (other) collection is rejected — only listed
// catalogs qualify.
func TestCreateUserCollectionRejectsFolderRefToScopedCatalog(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)

	owner := newTestProfile(t, db, "owner")
	otherCollection := newTestCollection(t, db, owner, "Other")

	scoped := createScopedCatalog(t, db, owner, otherCollection, listedCatalogForm("Scoped"))

	_, err := db.CreateUserCollection(ctx, owner, CollectionForm{
		Title: "New", ViewMode: "TABBED_GRID",
		Folders: []FolderData{{FolderArt: FolderArt{TileShape: "POSTER"}, Title: "Folder", Catalogs: CatalogRefs(scoped.ID)}},
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("folder ref to scoped catalog on create: got %v, want ErrInvalidInput", err)
	}
}

// A scoped catalog is deleted on Save once no folder in its collection still
// references it, but survives while another folder in the same collection
// still does.
func TestUpdateUserCollectionGCsOrphanedScopedCatalogs(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)

	owner := newTestProfile(t, db, "owner")
	collectionID := newTestCollection(t, db, owner, "My Collection")

	scoped := createScopedCatalog(t, db, owner, collectionID, listedCatalogForm("Scoped"))

	// Referenced by two folders in the same collection.
	saved, err := db.UpdateUserCollection(ctx, owner, collectionID, collectionRevision(t, db, collectionID), CollectionForm{
		Title: "My Collection", ViewMode: "TABBED_GRID",
		Folders: []FolderData{
			{FolderArt: FolderArt{TileShape: "POSTER"}, Title: "Folder 1", Catalogs: CatalogRefs(scoped.ID)},
			{FolderArt: FolderArt{TileShape: "POSTER"}, Title: "Folder 2", Catalogs: CatalogRefs(scoped.ID)},
		},
	})
	if err != nil {
		t.Fatalf("saving with two refs: %v", err)
	}
	folder1ID := saved.Folders[0].ID

	// Drop the ref from folder 2, keep folder 1 — the catalog must survive.
	_, err = db.UpdateUserCollection(ctx, owner, collectionID, collectionRevision(t, db, collectionID), CollectionForm{
		Title: "My Collection", ViewMode: "TABBED_GRID",
		Folders: []FolderData{
			{FolderArt: FolderArt{TileShape: "POSTER"}, ID: &folder1ID, Title: "Folder 1", Catalogs: CatalogRefs(scoped.ID)},
		},
	})
	if err != nil {
		t.Fatalf("saving with one ref remaining: %v", err)
	}
	remaining, err := db.queryCatalogs(ctx, "c.id = ?", scoped.ID.String())
	if err != nil {
		t.Fatalf("querying for scoped catalog: %v", err)
	}
	if len(remaining) != 1 {
		t.Fatalf("scoped catalog %s was deleted while still referenced by folder 1", scoped.ID)
	}

	// Drop the last ref — the catalog must now be GC'd.
	_, err = db.UpdateUserCollection(ctx, owner, collectionID, collectionRevision(t, db, collectionID), CollectionForm{
		Title: "My Collection", ViewMode: "TABBED_GRID",
		Folders: []FolderData{{FolderArt: FolderArt{TileShape: "POSTER"}, ID: &folder1ID, Title: "Folder 1", Catalogs: nil}},
	})
	if err != nil {
		t.Fatalf("saving with no refs: %v", err)
	}
	remaining, err = db.queryCatalogs(ctx, "c.id = ?", scoped.ID.String())
	if err != nil {
		t.Fatalf("querying for scoped catalog after last ref dropped: %v", err)
	}
	if len(remaining) != 0 {
		t.Fatalf("scoped catalog %s survived losing its last folder reference", scoped.ID)
	}
}

// A folder ref's New entry creates a fresh catalog scoped to this
// collection, in the same save as the folder write that references it.
func TestUpdateUserCollectionCreatesScopedCatalogFromNewRef(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)

	owner := newTestProfile(t, db, "owner")
	collectionID := newTestCollection(t, db, owner, "My Collection")

	saved, err := db.UpdateUserCollection(ctx, owner, collectionID, collectionRevision(t, db, collectionID), CollectionForm{
		Title: "My Collection", ViewMode: "TABBED_GRID",
		Folders: []FolderData{
			{FolderArt: FolderArt{TileShape: "POSTER"}, Title: "Folder", Catalogs: []FolderCatalogRef{
				{New: &NewScopedCatalog{Key: "draft:a", Type: "movie", Name: "New Scoped", Provider: "tmdb", Params: `{"a":1}`}},
			}},
		},
	})
	if err != nil {
		t.Fatalf("saving collection with a New ref: %v", err)
	}

	if len(saved.Folders) != 1 || len(saved.Folders[0].CatalogIDs()) != 1 {
		t.Fatalf("saved.Folders = %+v, want one folder with one catalog id", saved.Folders)
	}
	newID := saved.Folders[0].CatalogIDs()[0]

	if len(saved.Catalogs) != 1 || saved.Catalogs[0].ID != newID {
		t.Fatalf("saved.Catalogs = %+v, want exactly the new catalog %s", saved.Catalogs, newID)
	}
	got := saved.Catalogs[0]
	if got.Name != "New Scoped" || got.CollectionID == nil || *got.CollectionID != collectionID {
		t.Fatalf("created catalog = %+v, want name=%q, collection_id=%s",
			got, "New Scoped", collectionID)
	}
}

// A New entry works on CreateUserCollection too, not only Update — the
// general endpoint should handle a payload carrying drafts regardless of
// which call created the collection row.
func TestCreateUserCollectionCreatesScopedCatalogFromNewRef(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)

	owner := newTestProfile(t, db, "owner")

	saved, err := db.CreateUserCollection(ctx, owner, CollectionForm{
		Title: "New Collection", ViewMode: "TABBED_GRID",
		Folders: []FolderData{
			{FolderArt: FolderArt{TileShape: "POSTER"}, Title: "Folder", Catalogs: []FolderCatalogRef{
				{New: &NewScopedCatalog{Key: "draft:a", Type: "series", Name: "New Scoped", Provider: "tmdb", Params: "{}"}},
			}},
		},
	})
	if err != nil {
		t.Fatalf("creating collection with a New ref: %v", err)
	}
	if len(saved.Catalogs) != 1 || saved.Catalogs[0].CollectionID == nil || *saved.Catalogs[0].CollectionID != saved.ID {
		t.Fatalf("saved.Catalogs = %+v, want exactly one catalog scoped to %s", saved.Catalogs, saved.ID)
	}
}

// A New catalog is only ever committed atomically with the rest of the
// save: if a later folder in the same payload fails (here, a duplicate
// catalog_id within one folder — a PRIMARY KEY violation, per FolderPayload's
// doc comment), the whole transaction rolls back, and an earlier folder's
// New catalog is rolled back with it rather than left as an orphan.
func TestUpdateUserCollectionRollsBackNewCatalogOnLaterFolderFailure(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)

	owner := newTestProfile(t, db, "owner")
	collectionID := newTestCollection(t, db, owner, "My Collection")

	listed, err := db.CreateUserCatalog(ctx, owner, listedCatalogForm("Listed"))
	if err != nil {
		t.Fatalf("create listed catalog: %v", err)
	}

	_, err = db.UpdateUserCollection(ctx, owner, collectionID, collectionRevision(t, db, collectionID), CollectionForm{
		Title: "My Collection", ViewMode: "TABBED_GRID",
		Folders: []FolderData{
			{FolderArt: FolderArt{TileShape: "POSTER"}, Title: "Folder 1", Catalogs: []FolderCatalogRef{
				{New: &NewScopedCatalog{Key: "draft:a", Type: "movie", Name: "New Scoped", Provider: "tmdb", Params: "{}"}},
			}},
			{FolderArt: FolderArt{TileShape: "POSTER"}, Title: "Folder 2", Catalogs: CatalogRefs(listed.ID, listed.ID)},
		},
	})
	if err == nil {
		t.Fatalf("expected the duplicate ref in folder 2 to fail the save")
	}

	remaining, err := db.queryCatalogs(ctx, "c.name = ?", "New Scoped")
	if err != nil {
		t.Fatalf("querying for the new catalog: %v", err)
	}
	if len(remaining) != 0 {
		t.Fatalf("New catalog from folder 1 survived a save that failed on folder 2 — transaction did not roll back")
	}
}

// A saved collection's response carries every catalog its folders
// reference, listed or scoped, so the editor never needs the library to
// render a folder.
func TestUpdateUserCollectionResponseIncludesScopedCatalogs(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)

	owner := newTestProfile(t, db, "owner")
	collectionID := newTestCollection(t, db, owner, "My Collection")

	listed, err := db.CreateUserCatalog(ctx, owner, listedCatalogForm("Listed"))
	if err != nil {
		t.Fatalf("create listed catalog: %v", err)
	}

	scoped := createScopedCatalog(t, db, owner, collectionID, listedCatalogForm("Scoped"))

	saved, err := db.UpdateUserCollection(ctx, owner, collectionID, collectionRevision(t, db, collectionID), CollectionForm{
		Title: "My Collection", ViewMode: "TABBED_GRID",
		Folders: []FolderData{{FolderArt: FolderArt{TileShape: "POSTER"}, Title: "Folder", Catalogs: CatalogRefs(listed.ID, scoped.ID)}},
	})
	if err != nil {
		t.Fatalf("saving collection: %v", err)
	}

	gotIDs := make(map[uuid.UUID]bool, len(saved.Catalogs))
	for _, c := range saved.Catalogs {
		gotIDs[c.ID] = true
	}
	if !gotIDs[listed.ID] || !gotIDs[scoped.ID] {
		t.Fatalf("saved.Catalogs = %+v, want both %s and %s", saved.Catalogs, listed.ID, scoped.ID)
	}

	// GetUserCollections (assembleCollectionTree) must agree.
	all, err := db.GetUserCollections(ctx, owner)
	if err != nil {
		t.Fatalf("GetUserCollections: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("GetUserCollections returned %d collections, want 1", len(all))
	}
	gotIDs = make(map[uuid.UUID]bool, len(all[0].Catalogs))
	for _, c := range all[0].Catalogs {
		gotIDs[c.ID] = true
	}
	if !gotIDs[listed.ID] || !gotIDs[scoped.ID] {
		t.Fatalf("GetUserCollections catalogs = %+v, want both %s and %s", all[0].Catalogs, listed.ID, scoped.ID)
	}
}

// New entries sharing a Key are one staged catalog: two genre refs to it in
// one folder, and a ref to it from a second folder, all resolve to the single
// catalog the save creates. Distinct Keys with identical specs stay distinct
// catalogs, since two copies of one catalog are a deliberate choice.
func TestUpdateUserCollectionResolvesSharedNewKeyToOneCatalog(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)

	owner := newTestProfile(t, db, "owner")
	collectionID := newTestCollection(t, db, owner, "My Collection")

	shared := NewScopedCatalog{Key: "draft:shared", Type: "movie", Name: "Shared", Provider: "tmdb", Params: "{}"}
	twinA := NewScopedCatalog{Key: "draft:twin-a", Type: "movie", Name: "Twin", Provider: "tmdb", Params: "{}"}
	twinB := twinA
	twinB.Key = "draft:twin-b"
	ref := func(spec NewScopedCatalog, genre string) FolderCatalogRef {
		return FolderCatalogRef{New: &spec, Genre: genre}
	}

	saved, err := db.UpdateUserCollection(ctx, owner, collectionID, collectionRevision(t, db, collectionID), CollectionForm{
		Title: "My Collection", ViewMode: "TABBED_GRID",
		Folders: []FolderData{
			{FolderArt: FolderArt{TileShape: "POSTER"}, Title: "Folder 1", Catalogs: []FolderCatalogRef{
				ref(shared, "Action"), ref(shared, "Comedy"), ref(twinA, ""), ref(twinB, ""),
			}},
			{FolderArt: FolderArt{TileShape: "POSTER"}, Title: "Folder 2", Catalogs: []FolderCatalogRef{ref(shared, "")}},
		},
	})
	if err != nil {
		t.Fatalf("saving collection with shared New keys: %v", err)
	}

	first, second := saved.Folders[0].CatalogIDs(), saved.Folders[1].CatalogIDs()
	if first[0] != first[1] || first[0] != second[0] {
		t.Fatalf("refs to key %q resolved to %s, %s, %s; want one catalog", shared.Key, first[0], first[1], second[0])
	}
	if first[2] == first[3] {
		t.Fatalf("distinct keys %q and %q resolved to one catalog %s", twinA.Key, twinB.Key, first[2])
	}

	for name, want := range map[string]int{"Shared": 1, "Twin": 2} {
		got, err := db.queryCatalogs(ctx, "c.name = ?", name)
		if err != nil {
			t.Fatalf("querying catalogs named %q: %v", name, err)
		}
		if len(got) != want {
			t.Fatalf("%d catalogs named %q, want %d", len(got), name, want)
		}
	}
}
