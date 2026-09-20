package vault

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// Every length and count bound in validation.go rejects the form that
// oversteps it, and accepts the one that sits exactly on it.
func TestCatalogFormLengthBounds(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner := newTestProfile(t, db, "owner")

	atCeiling := listedCatalogForm(strings.Repeat("n", maxNameLen))
	atCeiling.Params = `{"with_genres":"` + strings.Repeat("1", maxParamsLen-len(`{"with_genres":""}`)) + `"}`
	if _, err := db.CreateUserCatalog(ctx, owner, atCeiling); err != nil {
		t.Fatalf("create with name and params exactly at their bounds: %v", err)
	}

	longName := listedCatalogForm(strings.Repeat("n", maxNameLen+1))
	if _, err := db.CreateUserCatalog(ctx, owner, longName); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("create with an overlong name = %v, want ErrInvalidInput", err)
	}

	longParams := listedCatalogForm("Fine")
	longParams.Params = strings.Repeat("p", maxParamsLen+1)
	if _, err := db.CreateUserCatalog(ctx, owner, longParams); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("create with overlong params = %v, want ErrInvalidInput", err)
	}
}

func TestCollectionFormLengthAndCountBounds(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner := newTestProfile(t, db, "owner")

	catalog, err := db.CreateUserCatalog(ctx, owner, listedCatalogForm("Popular"))
	if err != nil {
		t.Fatalf("create catalog: %v", err)
	}

	tooManyFolders := make([]FolderData, maxFoldersPerCollection+1)
	for i := range tooManyFolders {
		tooManyFolders[i] = FolderData{Title: "F"}
	}
	tooManyRefs := make([]FolderCatalogRef, maxRefsPerFolder+1)
	for i := range tooManyRefs {
		tooManyRefs[i] = FolderCatalogRef{CatalogID: &catalog.ID, Genre: strings.Repeat("g", i+1)}
	}

	for _, tt := range []struct {
		name string
		form CollectionForm
	}{
		{"overlong title", CollectionForm{Title: strings.Repeat("t", maxNameLen+1)}},
		{"overlong folder title", CollectionForm{Title: "Fine", Folders: []FolderData{
			{Title: strings.Repeat("t", maxNameLen+1)},
		}}},
		{"overlong cover emoji", CollectionForm{Title: "Fine", Folders: []FolderData{
			{Title: "F", CoverEmoji: strings.Repeat("x", maxCoverEmojiLen+1)},
		}}},
		{"too many folders", CollectionForm{Title: "Fine", Folders: tooManyFolders}},
		{"too many refs", CollectionForm{Title: "Fine", Folders: []FolderData{
			{Title: "F", Catalogs: tooManyRefs},
		}}},
		{"overlong new catalog name", CollectionForm{Title: "Fine", Folders: []FolderData{
			{Title: "F", Catalogs: []FolderCatalogRef{{New: &NewScopedCatalog{
				Key: "draft:a", Type: "movie", Name: strings.Repeat("n", maxNameLen+1), Provider: "tmdb", Params: "{}",
			}}}},
		}}},
		{"overlong new catalog params", CollectionForm{Title: "Fine", Folders: []FolderData{
			{Title: "F", Catalogs: []FolderCatalogRef{{New: &NewScopedCatalog{
				Key: "draft:a", Type: "movie", Name: "Staged", Provider: "tmdb", Params: strings.Repeat("p", maxParamsLen+1),
			}}}},
		}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := db.CreateUserCollection(ctx, owner, tt.form); !errors.Is(err, ErrInvalidInput) {
				t.Errorf("create = %v, want ErrInvalidInput", err)
			}
		})
	}

	// The bounds are ceilings, not off-by-one rejections: a collection sitting
	// exactly on every one of them saves.
	folders := make([]FolderData, maxFoldersPerCollection)
	for i := range folders {
		folders[i] = FolderData{Title: strings.Repeat("t", maxNameLen), CoverEmoji: strings.Repeat("x", maxCoverEmojiLen)}
	}
	folders[0].Catalogs = []FolderCatalogRef{{CatalogID: &catalog.ID, Genre: strings.Repeat("g", maxGenreLen)}}
	if _, err := db.CreateUserCollection(ctx, owner, CollectionForm{
		Title:   strings.Repeat("t", maxNameLen),
		Folders: folders,
	}); err != nil {
		t.Fatalf("create with every field exactly at its bound: %v", err)
	}
}

// A stored collection that never passed today's checks — an unrecognized
// view mode, an overlong folder title — is not copyable, by Take or by
// Duplicate: the copy path re-checks the source rather than trusting it.
func TestCollectionCopyRejectsStaleSourceRows(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner := newTestProfile(t, db, "owner")
	taker := newTestProfile(t, db, "taker")

	source, err := db.CreateUserCollection(ctx, owner, CollectionForm{
		Title:    "Source",
		IsPublic: true,
		Folders:  []FolderData{{Title: "Folder 1"}},
	})
	if err != nil {
		t.Fatalf("create source collection: %v", err)
	}

	for _, tt := range []struct {
		name  string
		query string
		arg   string
	}{
		{"unrecognized view mode", `UPDATE collections SET view_mode = ? WHERE id = ?`, "CAROUSEL"},
		{"overlong folder title", `UPDATE folders SET title = ? WHERE collection_id = ?`, strings.Repeat("t", maxNameLen+1)},
		{"javascript backdrop", `UPDATE collections SET backdrop_image_url = ? WHERE id = ?`, "javascript:alert(1)"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := db.conn.ExecContext(ctx, tt.query, tt.arg, source.ID.String()); err != nil {
				t.Fatalf("writing the stale row: %v", err)
			}
			// Restored before the next case runs, so each one tests its field alone.
			t.Cleanup(func() {
				if _, err := db.conn.ExecContext(ctx, `
					UPDATE collections SET view_mode = 'TABBED_GRID', backdrop_image_url = '' WHERE id = ?
				`, source.ID.String()); err != nil {
					t.Errorf("restoring the collection row: %v", err)
				}
				if _, err := db.conn.ExecContext(ctx, `UPDATE folders SET title = 'Folder 1' WHERE collection_id = ?`,
					source.ID.String()); err != nil {
					t.Errorf("restoring the folder row: %v", err)
				}
			})

			if _, err := db.TakeCollection(ctx, taker, source.ID, allowAnyCatalogParams); !errors.Is(err, ErrInvalidInput) {
				t.Errorf("TakeCollection = %v, want ErrInvalidInput", err)
			}
			if _, err := db.DuplicateCollection(ctx, owner, source.ID); !errors.Is(err, ErrInvalidInput) {
				t.Errorf("DuplicateCollection = %v, want ErrInvalidInput", err)
			}
		})
	}
}

// A stored public catalog past today's length bounds is not takeable on its
// own either: TakeCatalog re-checks the source row the same way the
// collection copy path re-checks every catalog in a tree, so the one listed
// row is bounded whichever door it is taken through.
func TestTakeCatalogRejectsStaleSourceRows(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner := newTestProfile(t, db, "owner")
	taker := newTestProfile(t, db, "taker")

	source, err := db.CreateUserCatalog(ctx, owner, publicCatalogForm("Source"))
	if err != nil {
		t.Fatalf("create source catalog: %v", err)
	}

	for _, tt := range []struct {
		name  string
		query string
		arg   string
	}{
		{"overlong name", `UPDATE catalogs SET name = ? WHERE id = ?`, strings.Repeat("n", maxNameLen+1)},
		{"overlong params", `UPDATE catalogs SET params = ? WHERE id = ?`, strings.Repeat("p", maxParamsLen+1)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := db.conn.ExecContext(ctx, tt.query, tt.arg, source.ID.String()); err != nil {
				t.Fatalf("writing the stale row: %v", err)
			}
			// Restored before the next case runs, so each one tests its field alone.
			t.Cleanup(func() {
				if _, err := db.conn.ExecContext(ctx, `UPDATE catalogs SET name = 'Source', params = '{}' WHERE id = ?`,
					source.ID.String()); err != nil {
					t.Errorf("restoring the catalog row: %v", err)
				}
			})

			if _, err := db.TakeCatalog(ctx, taker, source.ID, allowAnyCatalogParams); !errors.Is(err, ErrInvalidInput) {
				t.Errorf("TakeCatalog = %v, want ErrInvalidInput", err)
			}
		})
	}

	// The restored row is takeable, so the bound is what refused it.
	if _, err := db.TakeCatalog(ctx, taker, source.ID, allowAnyCatalogParams); err != nil {
		t.Errorf("TakeCatalog on the restored row: %v", err)
	}
}

// DuplicateCollection's "(copy)" suffix is part of the title it writes, so a
// title with room for it duplicates and the result saves again, while one
// already at maxNameLen is refused rather than stored past the bound.
func TestDuplicateCollectionBoundsTheSuffixedTitle(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner := newTestProfile(t, db, "owner")

	const suffix = " (copy)"
	roomy, err := db.CreateUserCollection(ctx, owner, CollectionForm{
		Title:   strings.Repeat("t", maxNameLen-len(suffix)),
		Folders: []FolderData{{Title: "Folder 1"}},
	})
	if err != nil {
		t.Fatalf("create collection: %v", err)
	}
	copied, err := db.DuplicateCollection(ctx, owner, roomy.ID)
	if err != nil {
		t.Fatalf("duplicate a title with room for the suffix: %v", err)
	}
	if _, err := db.UpdateUserCollection(ctx, owner, copied.ID, CollectionForm{
		Title:   copied.Title,
		Folders: []FolderData{{Title: "Folder 1"}},
	}); err != nil {
		t.Fatalf("saving the duplicate's own title back: %v", err)
	}

	full, err := db.CreateUserCollection(ctx, owner, CollectionForm{Title: strings.Repeat("t", maxNameLen)})
	if err != nil {
		t.Fatalf("create collection at the title bound: %v", err)
	}
	if _, err := db.DuplicateCollection(ctx, owner, full.ID); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("duplicate a title with no room for the suffix = %v, want ErrInvalidInput", err)
	}
}

// updateFolder's WHERE names the collection as well as the folder, and a
// statement that matches nothing is an error rather than a Folder the caller
// can return as saved.
func TestUpdateFolderRejectsAForeignFolder(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner := newTestProfile(t, db, "owner")
	collectionID := newTestCollection(t, db, owner, "Mine")

	tx, err := db.conn.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback() }()

	_, err = updateFolder(ctx, tx, uuid.New(), collectionID, 0, FolderData{Title: "Nowhere"})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("updateFolder on a folder of no collection = %v, want ErrInvalidInput", err)
	}
}
