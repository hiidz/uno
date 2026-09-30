package vault

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

// scopedCatalogInFolder creates a catalog scoped to collectionID and saves the
// collection with one folder referencing it, returning the catalog and the
// saved collection.
func scopedCatalogInFolder(t *testing.T, db *DB, owner, collectionID uuid.UUID, name string) (Catalog, CollectionWithFolders) {
	t.Helper()
	ctx := context.Background()
	form := listedCatalogForm(name)
	form.CollectionID = &collectionID
	catalog, err := db.CreateUserCatalog(ctx, owner, form)
	if err != nil {
		t.Fatalf("create scoped catalog %q: %v", name, err)
	}
	saved, err := db.UpdateUserCollection(ctx, owner, collectionID, CollectionForm{
		Title:   "My Collection",
		Folders: []FolderData{{Title: "Folder", Catalogs: CatalogRefs(catalog.ID)}},
	})
	if err != nil {
		t.Fatalf("referencing scoped catalog %q: %v", name, err)
	}
	return catalog, saved
}

// editOf is an edit that leaves c exactly as it is — the starting point each
// test changes one field of.
func editOf(c Catalog) ScopedCatalogEdit {
	return ScopedCatalogEdit{ID: c.ID, Type: c.Type, Provider: c.Provider, Name: c.Name, Params: c.Params}
}

// sameFolders is a save form that keeps saved's folders and refs as they are.
func sameFolders(saved CollectionWithFolders) []FolderData {
	folders := make([]FolderData, len(saved.Folders))
	for i, f := range saved.Folders {
		folders[i] = FolderData{ID: &f.ID, Title: f.Title, Catalogs: CatalogRefs(f.CatalogIDs()...)}
	}
	return folders
}

// A catalog edit lands with the rest of the collection's save, in the same
// response.
func TestUpdateUserCollectionAppliesCatalogEdits(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner := newTestProfile(t, db, "owner")
	collectionID := newTestCollection(t, db, owner, "My Collection")
	catalog, saved := scopedCatalogInFolder(t, db, owner, collectionID, "Before")

	edit := editOf(catalog)
	edit.Name = "After"
	edit.Params = `{"sort_by":"popularity.desc"}`
	got, err := db.UpdateUserCollection(ctx, owner, collectionID, CollectionForm{
		Title:        "Renamed",
		Folders:      sameFolders(saved),
		CatalogEdits: []ScopedCatalogEdit{edit},
	})
	if err != nil {
		t.Fatalf("save with a catalog edit: %v", err)
	}
	if got.Title != "Renamed" {
		t.Errorf("title = %q, want %q", got.Title, "Renamed")
	}
	if len(got.Catalogs) != 1 || got.Catalogs[0].Name != "After" || got.Catalogs[0].Params != edit.Params {
		t.Fatalf("response catalogs = %+v, want the edited catalog", got.Catalogs)
	}

	stored, err := db.queryCatalogs(ctx, "c.id = ?", catalog.ID.String())
	if err != nil {
		t.Fatalf("querying edited catalog: %v", err)
	}
	if stored[0].RecipeHash != edit.recipeHash() || stored[0].CollectionID == nil {
		t.Fatalf("stored catalog = %+v, want the new recipe and still scoped", stored[0])
	}
}

// One refused edit fails the whole save: nothing in it is written, not the
// collection's own fields and not the other, valid edit.
func TestUpdateUserCollectionRefusedCatalogEditWritesNothing(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner := newTestProfile(t, db, "owner")
	other := newTestProfile(t, db, "other")

	collectionID := newTestCollection(t, db, owner, "My Collection")
	catalog, saved := scopedCatalogInFolder(t, db, owner, collectionID, "Mine")

	elsewhereID := newTestCollection(t, db, owner, "Elsewhere")
	elsewhere, _ := scopedCatalogInFolder(t, db, owner, elsewhereID, "Elsewhere")
	listed, err := db.CreateUserCatalog(ctx, owner, listedCatalogForm("Listed"))
	if err != nil {
		t.Fatalf("create listed catalog: %v", err)
	}
	theirs, err := db.CreateUserCatalog(ctx, other, listedCatalogForm("Theirs"))
	if err != nil {
		t.Fatalf("create other owner's catalog: %v", err)
	}
	retyped := editOf(catalog)
	retyped.Type = "series"

	for _, tt := range []struct {
		name string
		bad  ScopedCatalogEdit
	}{
		{"scoped to another collection", editOf(elsewhere)},
		{"listed", editOf(listed)},
		{"another owner's", editOf(theirs)},
		{"unknown", ScopedCatalogEdit{ID: uuid.New(), Type: "movie", Provider: "tmdb", Name: "Ghost", Params: "{}"}},
		{"type change", retyped},
	} {
		t.Run(tt.name, func(t *testing.T) {
			valid := editOf(catalog)
			valid.Name = "Renamed catalog"
			edits := []ScopedCatalogEdit{valid, tt.bad}
			if tt.bad.ID == catalog.ID {
				edits = []ScopedCatalogEdit{tt.bad}
			}
			_, err := db.UpdateUserCollection(ctx, owner, collectionID, CollectionForm{
				Title:        "Renamed",
				Folders:      sameFolders(saved),
				CatalogEdits: edits,
			})
			if !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("save = %v, want ErrInvalidInput", err)
			}

			trees, err := db.GetCollectionsByIDs(ctx, []uuid.UUID{collectionID})
			if err != nil {
				t.Fatalf("reloading collection: %v", err)
			}
			if trees[0].Title != "My Collection" || !trees[0].UpdatedAt.Equal(saved.UpdatedAt) {
				t.Errorf("collection after refused save = %q updated %s, want %q updated %s",
					trees[0].Title, trees[0].UpdatedAt, "My Collection", saved.UpdatedAt)
			}
			if len(trees[0].Catalogs) != 1 || trees[0].Catalogs[0].Name != "Mine" || trees[0].Catalogs[0].Type != "movie" {
				t.Errorf("catalogs after refused save = %+v, want %q untouched", trees[0].Catalogs, "Mine")
			}
		})
	}
}

// Move to library makes the catalog listed, and a listed catalog survives
// the save's orphan cleanup even when no folder here references it any more.
func TestUpdateUserCollectionMovesCatalogToLibrary(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner := newTestProfile(t, db, "owner")
	collectionID := newTestCollection(t, db, owner, "My Collection")
	catalog, saved := scopedCatalogInFolder(t, db, owner, collectionID, "Moving")

	if _, err := db.UpdateUserCollection(ctx, owner, collectionID, CollectionForm{
		Title:        "My Collection",
		Folders:      []FolderData{{ID: &saved.Folders[0].ID, Title: "Folder"}},
		CatalogEdits: []ScopedCatalogEdit{moveToLibraryEdit(catalog)},
	}); err != nil {
		t.Fatalf("save with move to library: %v", err)
	}

	library, err := db.GetUserCatalogs(ctx, owner)
	if err != nil {
		t.Fatalf("GetUserCatalogs: %v", err)
	}
	if len(library) != 1 || library[0].ID != catalog.ID {
		t.Fatalf("library = %+v, want the moved catalog %s", library, catalog.ID)
	}
}

// An edit that changes nothing is skipped, so the row's updated_at stays put;
// an edit that changes only the recipe still writes.
func TestUpdateUserCollectionSkipsNoOpCatalogEdit(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner := newTestProfile(t, db, "owner")
	collectionID := newTestCollection(t, db, owner, "My Collection")
	catalog, saved := scopedCatalogInFolder(t, db, owner, collectionID, "Steady")

	const past = "2020-01-01T00:00:00Z"
	if _, err := db.conn.ExecContext(ctx, `UPDATE catalogs SET updated_at = ? WHERE id = ?`, past, catalog.ID.String()); err != nil {
		t.Fatalf("backdating catalog: %v", err)
	}
	updatedAt := func() string {
		t.Helper()
		var s string
		if err := db.conn.QueryRowContext(ctx, `SELECT updated_at FROM catalogs WHERE id = ?`, catalog.ID.String()).Scan(&s); err != nil {
			t.Fatalf("reading updated_at: %v", err)
		}
		return s
	}
	save := func(edit ScopedCatalogEdit) {
		t.Helper()
		if _, err := db.UpdateUserCollection(ctx, owner, collectionID, CollectionForm{
			Title:        "My Collection",
			Folders:      sameFolders(saved),
			CatalogEdits: []ScopedCatalogEdit{edit},
		}); err != nil {
			t.Fatalf("save: %v", err)
		}
	}

	save(editOf(catalog))
	if got := updatedAt(); got != past {
		t.Fatalf("updated_at after a no-op edit = %s, want %s", got, past)
	}

	recipeOnly := editOf(catalog)
	recipeOnly.Params = `{"sort_by":"revenue.desc"}`
	save(recipeOnly)
	if got := updatedAt(); got == past {
		t.Fatalf("updated_at after a recipe-only edit = %s, want it rewritten", got)
	}
}

// A collection being created has no scoped catalogs, so an edit in its first
// save is refused rather than ignored.
func TestCreateUserCollectionRefusesCatalogEdits(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner := newTestProfile(t, db, "owner")

	_, err := db.CreateUserCollection(ctx, owner, CollectionForm{
		Title:        "New",
		CatalogEdits: []ScopedCatalogEdit{{ID: uuid.New(), Type: "movie", Provider: "tmdb", Name: "X", Params: "{}"}},
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("create with a catalog edit = %v, want ErrInvalidInput", err)
	}
}
