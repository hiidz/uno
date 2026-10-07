package vault

import (
	"context"
	"errors"
	"slices"
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
// view mode, an overlong folder title — is neither publishable nor copyable
// by Duplicate: both re-check the source rather than trusting it.
func TestCollectionCopyRejectsStaleSourceRows(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner := newTestProfile(t, db, "owner")

	source, err := db.CreateUserCollection(ctx, owner, CollectionForm{
		Title:   "Source",
		Folders: []FolderData{{Title: "Folder 1"}},
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

			if _, err := db.PublishCollection(ctx, owner, source.ID); !errors.Is(err, ErrInvalidInput) {
				t.Errorf("PublishCollection = %v, want ErrInvalidInput", err)
			}
			if _, err := db.DuplicateCollection(ctx, owner, source.ID); !errors.Is(err, ErrInvalidInput) {
				t.Errorf("DuplicateCollection = %v, want ErrInvalidInput", err)
			}
		})
	}
}

// A stored catalog past today's length bounds is not publishable either: a
// publish re-checks the source row the same way it re-checks every catalog
// in a collection, so the one listed row is bounded whichever way it is
// shared.
func TestPublishCatalogRejectsStaleSourceRows(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner := newTestProfile(t, db, "owner")

	source, err := db.CreateUserCatalog(ctx, owner, listedCatalogForm("Source"))
	if err != nil {
		t.Fatalf("create source catalog: %v", err)
	}

	for _, tt := range []struct {
		name  string
		query string
		arg   string
	}{
		{"overlong name", `UPDATE catalogs SET name = ? WHERE id = ?`, strings.Repeat("n", maxNameLen+1)},
		{"overlong params", `UPDATE recipes SET params = ? WHERE hash = (SELECT recipe_hash FROM catalogs WHERE id = ?)`, strings.Repeat("p", maxParamsLen+1)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := db.conn.ExecContext(ctx, tt.query, tt.arg, source.ID.String()); err != nil {
				t.Fatalf("writing the stale row: %v", err)
			}
			// Restored before the next case runs, so each one tests its field alone.
			t.Cleanup(func() {
				if _, err := db.conn.ExecContext(ctx, `UPDATE catalogs SET name = 'Source' WHERE id = ?`, source.ID.String()); err != nil {
					t.Errorf("restoring the catalog row: %v", err)
				}
				if _, err := db.conn.ExecContext(ctx, `UPDATE recipes SET params = '{}' WHERE hash = ?`, source.RecipeHash); err != nil {
					t.Errorf("restoring the recipe row: %v", err)
				}
			})

			if _, err := db.PublishCatalog(ctx, owner, source.ID); !errors.Is(err, ErrInvalidInput) {
				t.Errorf("PublishCatalog = %v, want ErrInvalidInput", err)
			}
		})
	}

	// The restored row publishes, so the bound is what refused it.
	if _, err := db.PublishCatalog(ctx, owner, source.ID); err != nil {
		t.Errorf("PublishCatalog on the restored row: %v", err)
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

// A catalog edit is held to a catalog save's own rules, and one catalog can't
// be edited twice in one save. Each problem is reported with the edit's
// position.
func TestCollectionFormCatalogEditProblems(t *testing.T) {
	id := uuid.New()
	valid := ScopedCatalogEdit{ID: id, Type: "movie", Provider: "tmdb", Name: "Fine", Params: "{}"}
	with := func(change func(*ScopedCatalogEdit)) ScopedCatalogEdit {
		e := valid
		change(&e)
		return e
	}

	for _, tt := range []struct {
		name  string
		edits []ScopedCatalogEdit
		want  string
	}{
		{"blank name", []ScopedCatalogEdit{with(func(e *ScopedCatalogEdit) { e.Name = " " })}, "catalog edit 0: name is required"},
		{"overlong name", []ScopedCatalogEdit{with(func(e *ScopedCatalogEdit) { e.Name = strings.Repeat("n", maxNameLen+1) })}, "catalog edit 0: name is longer"},
		{"overlong params", []ScopedCatalogEdit{with(func(e *ScopedCatalogEdit) { e.Params = strings.Repeat("p", maxParamsLen+1) })}, "catalog edit 0: params is longer"},
		{"unknown provider", []ScopedCatalogEdit{with(func(e *ScopedCatalogEdit) { e.Provider = "mdblist" })}, `catalog edit 0: provider must be "tmdb"`},
		{"unknown type", []ScopedCatalogEdit{with(func(e *ScopedCatalogEdit) { e.Type = "anime" })}, `catalog edit 0: type must be "movie" or "series"`},
		{"same catalog twice", []ScopedCatalogEdit{valid, valid}, "catalog edit 1: repeats a catalog"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := CollectionForm{Title: "C", CatalogEdits: tt.edits}.Validate()
			if !errors.Is(err, ErrInvalidInput) || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate = %v, want ErrInvalidInput mentioning %q", err, tt.want)
			}
		})
	}

	if err := (CollectionForm{Title: "C", CatalogEdits: []ScopedCatalogEdit{valid}}).Validate(); err != nil {
		t.Fatalf("Validate with one valid edit = %v, want nil", err)
	}
}

// normalizedWant is what the normalization tests store for their padded,
// empty-valued forms.
var normalizedWant = []string{"C", "TABBED_GRID", "https://example.com/b.jpg", "Folder", "POSTER", "🎃", "https://example.com/cover.jpg"}

// paddedForm is a collection form with every trimmed field padded and an
// empty view mode and tile shape, its one folder holding refs.
func paddedForm(refs ...FolderCatalogRef) CollectionForm {
	return CollectionForm{
		Title: " C ", BackdropImageURL: " https://example.com/b.jpg ",
		Folders: []FolderData{{
			Title: " Folder ", CoverEmoji: " 🎃 ", CoverImageURL: " https://example.com/cover.jpg ",
			Catalogs: refs,
		}},
	}
}

// requireNormalizedTree fails unless tree holds normalizedWant and its one
// scoped catalog is named scopedName.
func requireNormalizedTree(t *testing.T, label string, tree CollectionWithFolders, scopedName string) {
	t.Helper()
	f := tree.Folders[0]
	got := []string{tree.Title, tree.ViewMode, tree.BackdropImageURL, f.Title, f.TileShape, f.CoverEmoji, f.CoverImageURL}
	if !slices.Equal(got, normalizedWant) {
		t.Errorf("%s = %q, want %q", label, got, normalizedWant)
	}
	for _, c := range tree.Catalogs {
		if c.CollectionID != nil && c.Name != scopedName {
			t.Errorf("%s scoped catalog name = %q, want %q", label, c.Name, scopedName)
		}
	}
}

// A builder write stores its form normalized — text trimmed, new and edited
// catalog names included, an empty view mode or tile shape as its default —
// and responds with what it stored. A padded URL is trimmed before it is
// checked, so it is accepted.
func TestBuilderWritesStoreNormalizedValues(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner := newTestProfile(t, db, "owner")

	listed, err := db.CreateUserCatalog(ctx, owner, listedCatalogForm("  Listed  "))
	if err != nil {
		t.Fatalf("CreateUserCatalog: %v", err)
	}
	if listed.Name != "Listed" || reloadCatalog(t, db, listed.ID).Name != "Listed" {
		t.Errorf("created catalog name = %q, want it trimmed", listed.Name)
	}
	renamed, err := db.UpdateUserCatalog(ctx, owner, listed.ID, listedCatalogForm(" Renamed "))
	if err != nil {
		t.Fatalf("UpdateUserCatalog: %v", err)
	}
	if renamed.Name != "Renamed" || reloadCatalog(t, db, listed.ID).Name != "Renamed" {
		t.Errorf("updated catalog name = %q, want it trimmed", renamed.Name)
	}

	scoped := &NewScopedCatalog{Key: "k", Type: "movie", Name: " Scoped ", Provider: "tmdb", Params: "{}"}
	created, err := db.CreateUserCollection(ctx, owner, paddedForm(FolderCatalogRef{CatalogID: &listed.ID}, FolderCatalogRef{New: scoped}))
	if err != nil {
		t.Fatalf("CreateUserCollection: %v", err)
	}
	requireNormalizedTree(t, "created", created, "Scoped")
	stored, err := ownCollection(ctx, db.conn, owner, created.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	requireNormalizedTree(t, "stored after create", stored, "Scoped")

	i := slices.IndexFunc(stored.Catalogs, func(c Catalog) bool { return c.CollectionID != nil })
	edited := stored.Catalogs[i]
	form := paddedForm(FolderCatalogRef{CatalogID: &listed.ID}, FolderCatalogRef{CatalogID: &edited.ID})
	form.Folders[0].ID = &stored.Folders[0].ID
	form.CatalogEdits = []ScopedCatalogEdit{{ID: edited.ID, Type: edited.Type, Provider: edited.Provider, Name: " Scoped 2 ", Params: edited.Params}}
	updated, err := db.UpdateUserCollection(ctx, owner, created.ID, form)
	if err != nil {
		t.Fatalf("UpdateUserCollection: %v", err)
	}
	requireNormalizedTree(t, "updated", updated, "Scoped 2")
	if stored, err = ownCollection(ctx, db.conn, owner, created.ID); err != nil {
		t.Fatalf("reload: %v", err)
	}
	requireNormalizedTree(t, "stored after update", stored, "Scoped 2")
}

// An import stores the bundle normalized the way a builder write does.
func TestImportBundleStoresNormalizedValues(t *testing.T) {
	db := newTestDB(t)
	b := readTestBundle(t)
	b.Catalogs[0].Name = " 80s Horror "
	bc := &b.Collections[0]
	bc.Title, bc.ViewMode, bc.BackdropImageURL = " C ", "", " https://example.com/b.jpg "
	bc.Catalogs[0].Name = " Slashers "
	f := &bc.Folders[0]
	f.Title, f.TileShape, f.CoverEmoji, f.CoverImageURL = " Folder ", "", " 🎃 ", " https://example.com/cover.jpg "

	catalogs, collections, err := db.ImportBundle(context.Background(), newTestProfile(t, db, "importer"), b, nil)
	if err != nil {
		t.Fatalf("ImportBundle: %v", err)
	}
	if catalogs[0].Name != "80s Horror" {
		t.Errorf("listed catalog name = %q, want it trimmed", catalogs[0].Name)
	}
	requireNormalizedTree(t, "imported", collections[0], "Slashers")
}

// A publish snapshots what its source stores as it is, values a builder
// write would normalize included, and a subscribe writes the snapshot as it
// is, so the copy matches its snapshot: no update is available.
func TestSubscribeCopiesStoredValuesAsTheyAre(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner, subscriber := newTestProfile(t, db, "owner"), newTestProfile(t, db, "subscriber")
	scoped := &NewScopedCatalog{Key: "k", Type: "movie", Name: "Scoped", Provider: "tmdb", Params: "{}"}
	source, err := db.CreateUserCollection(ctx, owner, CollectionForm{
		Title: "Source", Folders: []FolderData{{Title: "F", Catalogs: []FolderCatalogRef{{New: scoped}}}},
	})
	if err != nil {
		t.Fatalf("CreateUserCollection: %v", err)
	}
	for _, stmt := range []string{
		`UPDATE collections SET title = ' Source ', view_mode = '' WHERE id = ?`,
		`UPDATE folders SET tile_shape = '' WHERE collection_id = ?`,
	} {
		if _, err := db.conn.ExecContext(ctx, stmt, source.ID.String()); err != nil {
			t.Fatalf("storing unnormalized values: %v", err)
		}
	}

	subscribed := subscribeCollection(t, db, owner, subscriber, source.ID)
	if subscribed.Title != " Source " || subscribed.ViewMode != "" || subscribed.Folders[0].TileShape != "" {
		t.Errorf("subscribed title, view mode, tile shape = %q, %q, %q, want the source's as stored", subscribed.Title, subscribed.ViewMode, subscribed.Folders[0].TileShape)
	}
	if subscribed.Subscription == nil || subscribed.Subscription.UpdateAvailable {
		t.Errorf("subscription = %+v, want one with no update available", subscribed.Subscription)
	}
	if copyTreeSnapshot(subscribed).contentHash() != collectionSnapshot(subscribed.Subscription.PublicationID, mustOwnCollection(t, db, owner, source.ID)).contentHash() {
		t.Error("the copy does not snapshot to what its publication holds")
	}
}
