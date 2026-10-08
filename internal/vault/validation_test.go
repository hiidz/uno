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
		tooManyFolders[i] = FolderData{FolderArt: FolderArt{TileShape: "POSTER"}, Title: "F"}
	}
	tooManyRefs := make([]FolderCatalogRef, maxRefsPerFolder+1)
	for i := range tooManyRefs {
		tooManyRefs[i] = FolderCatalogRef{CatalogID: &catalog.ID, Genre: strings.Repeat("g", i+1)}
	}

	for _, tt := range []struct {
		name string
		form CollectionForm
	}{
		{"overlong title", CollectionForm{Title: strings.Repeat("t", maxNameLen+1), ViewMode: "TABBED_GRID"}},
		{"overlong folder title", CollectionForm{Title: "Fine", ViewMode: "TABBED_GRID", Folders: []FolderData{
			{FolderArt: FolderArt{TileShape: "POSTER"}, Title: strings.Repeat("t", maxNameLen+1)},
		}}},
		{"overlong cover emoji", CollectionForm{Title: "Fine", ViewMode: "TABBED_GRID", Folders: []FolderData{
			{Title: "F", FolderArt: FolderArt{TileShape: "POSTER", CoverEmoji: strings.Repeat("x", maxCoverEmojiLen+1)}},
		}}},
		{"too many folders", CollectionForm{Title: "Fine", ViewMode: "TABBED_GRID", Folders: tooManyFolders}},
		{"too many refs", CollectionForm{Title: "Fine", ViewMode: "TABBED_GRID", Folders: []FolderData{
			{FolderArt: FolderArt{TileShape: "POSTER"}, Title: "F", Catalogs: tooManyRefs},
		}}},
		{"overlong new catalog name", CollectionForm{Title: "Fine", ViewMode: "TABBED_GRID", Folders: []FolderData{
			{FolderArt: FolderArt{TileShape: "POSTER"}, Title: "F", Catalogs: []FolderCatalogRef{{New: &NewScopedCatalog{
				Key: "draft:a", Type: "movie", Name: strings.Repeat("n", maxNameLen+1), Provider: "tmdb", Params: "{}",
			}}}},
		}}},
		{"overlong new catalog params", CollectionForm{Title: "Fine", ViewMode: "TABBED_GRID", Folders: []FolderData{
			{FolderArt: FolderArt{TileShape: "POSTER"}, Title: "F", Catalogs: []FolderCatalogRef{{New: &NewScopedCatalog{
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
		folders[i] = FolderData{Title: strings.Repeat("t", maxNameLen), FolderArt: FolderArt{TileShape: "POSTER", CoverEmoji: strings.Repeat("x", maxCoverEmojiLen)}}
	}
	folders[0].Catalogs = []FolderCatalogRef{{CatalogID: &catalog.ID, Genre: strings.Repeat("g", maxGenreLen)}}
	if _, err := db.CreateUserCollection(ctx, owner, CollectionForm{
		Title: strings.Repeat("t", maxNameLen), ViewMode: "TABBED_GRID",
		Folders: folders,
	}); err != nil {
		t.Fatalf("create with every field exactly at its bound: %v", err)
	}
}

// A stored collection that never passed today's checks — an unrecognized,
// FOLLOW_LAYOUT or empty view mode, an empty tile shape, an overlong folder
// title — is neither publishable nor copyable by Duplicate: both re-check
// the source rather than trusting it.
func TestCollectionCopyRejectsStaleSourceRows(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner := newTestProfile(t, db, "owner")

	source, err := db.CreateUserCollection(ctx, owner, CollectionForm{
		Title: "Source", ViewMode: "TABBED_GRID",
		Folders: []FolderData{{FolderArt: FolderArt{TileShape: "POSTER"}, Title: "Folder 1"}},
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
		{"follow layout", `UPDATE collections SET view_mode = ? WHERE id = ?`, "FOLLOW_LAYOUT"},
		{"empty view mode", `UPDATE collections SET view_mode = ? WHERE id = ?`, ""},
		{"empty tile shape", `UPDATE folders SET tile_shape = ? WHERE collection_id = ?`, ""},
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
				if _, err := db.conn.ExecContext(ctx, `UPDATE folders SET title = 'Folder 1', tile_shape = 'POSTER' WHERE collection_id = ?`,
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
		want  string
	}{
		{"overlong name", `UPDATE catalogs SET name = ? WHERE id = ?`, strings.Repeat("n", maxNameLen+1), "name is longer"},
		{"overlong params", `UPDATE catalogs SET params = ? WHERE id = ?`, `{"with_genres":"` + strings.Repeat("1", maxParamsLen) + `"}`, "params is longer"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := db.conn.ExecContext(ctx, tt.query, tt.arg, source.ID.String()); err != nil {
				t.Fatalf("writing the stale row: %v", err)
			}
			// Restored before the next case runs, so each one tests its field alone.
			t.Cleanup(func() {
				if _, err := db.conn.ExecContext(ctx, `UPDATE catalogs SET name = 'Source', params = ? WHERE id = ?`, source.Params, source.ID.String()); err != nil {
					t.Errorf("restoring the catalog row: %v", err)
				}
			})

			if _, err := db.PublishCatalog(ctx, owner, source.ID); !errors.Is(err, ErrInvalidInput) || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("PublishCatalog = %v, want ErrInvalidInput naming %q", err, tt.want)
			}
		})
	}

	// The restored row publishes, so the bound is what refused it.
	if _, err := db.PublishCatalog(ctx, owner, source.ID); err != nil {
		t.Errorf("PublishCatalog on the restored row: %v", err)
	}
}

// copyName gives every Duplicate's name its suffix and cuts the name, in
// characters, to leave room for it.
func TestCopyName(t *testing.T) {
	const suffix = " (copy)"
	room := maxNameLen - len(suffix)
	for name, tc := range map[string]struct{ in, want string }{
		"short":                 {"Source", "Source (copy)"},
		"exactly fits":          {strings.Repeat("n", room), strings.Repeat("n", room) + suffix},
		"one over":              {strings.Repeat("n", room+1), strings.Repeat("n", room) + suffix},
		"at the bound":          {strings.Repeat("n", maxNameLen), strings.Repeat("n", room) + suffix},
		"multi-byte, cut whole": {strings.Repeat("字", maxNameLen), strings.Repeat("字", room) + suffix},
	} {
		if got := copyName(tc.in); got != tc.want || tooLong(got, maxNameLen) {
			t.Errorf("%s: copyName = %d characters, want %d", name, len([]rune(got)), len([]rune(tc.want)))
		}
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

	_, err = updateFolder(ctx, tx, uuid.New(), collectionID, 0, FolderData{FolderArt: FolderArt{TileShape: "POSTER"}, Title: "Nowhere"})
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
			err := CollectionForm{Title: "C", ViewMode: "TABBED_GRID", CatalogEdits: tt.edits}.Validate()
			if !errors.Is(err, ErrInvalidInput) || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate = %v, want ErrInvalidInput mentioning %q", err, tt.want)
			}
		})
	}

	if err := (CollectionForm{Title: "C", ViewMode: "TABBED_GRID", CatalogEdits: []ScopedCatalogEdit{valid}}).Validate(); err != nil {
		t.Fatalf("Validate with one valid edit = %v, want nil", err)
	}
}

// normalizedWant is what the normalization tests store for their padded
// forms.
var normalizedWant = []string{"C", "TABBED_GRID", "https://example.com/b.jpg", "Folder", "POSTER", "🎃", "https://example.com/cover.jpg"}

// paddedForm is a collection form with every trimmed field padded, its one
// folder holding refs.
func paddedForm(refs ...FolderCatalogRef) CollectionForm {
	return CollectionForm{
		Title: " C ", ViewMode: "TABBED_GRID", BackdropImageURL: " https://example.com/b.jpg ",
		Folders: []FolderData{{
			Title:     " Folder ",
			FolderArt: FolderArt{TileShape: "POSTER", CoverEmoji: " 🎃 ", CoverImageURL: " https://example.com/cover.jpg "},
			Catalogs:  refs,
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
// catalog names included — and responds with what it stored. A padded URL is trimmed before it is
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
	renamed, err := db.UpdateUserCatalog(ctx, owner, listed.ID, catalogRevision(t, db, listed.ID), listedCatalogForm(" Renamed "))
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
	updated, err := db.UpdateUserCollection(ctx, owner, created.ID, collectionRevision(t, db, created.ID), form)
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
	bc.Title, bc.BackdropImageURL = " C ", " https://example.com/b.jpg "
	bc.Catalogs[0].Name = " Slashers "
	f := &bc.Folders[0]
	f.Title, f.TileShape, f.CoverEmoji, f.CoverImageURL = " Folder ", "POSTER", " 🎃 ", " https://example.com/cover.jpg "

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
		Title: "Source", ViewMode: "TABBED_GRID", Folders: []FolderData{{FolderArt: FolderArt{TileShape: "POSTER"}, Title: "F", Catalogs: []FolderCatalogRef{{New: scoped}}}},
	})
	if err != nil {
		t.Fatalf("CreateUserCollection: %v", err)
	}
	if _, err := db.conn.ExecContext(ctx, `UPDATE collections SET title = ' Source ' WHERE id = ?`, source.ID.String()); err != nil {
		t.Fatalf("storing an unnormalized value: %v", err)
	}

	subscribed := subscribeCollection(t, db, owner, subscriber, source.ID)
	if subscribed.Title != " Source " {
		t.Errorf("subscribed title = %q, want the source's as stored", subscribed.Title)
	}
	if subscribed.Subscription == nil || subscribed.Subscription.UpdateAvailable {
		t.Errorf("subscription = %+v, want one with no update available", subscribed.Subscription)
	}
	if copyTreeSnapshot(subscribed).contentHash() != collectionSnapshot(subscribed.Subscription.PublicationID, mustOwnCollection(t, db, owner, source.ID)).contentHash() {
		t.Error("the copy does not snapshot to what its publication holds")
	}
}

// Lengths count characters, not bytes: a name made only of three-byte
// characters fits at maxNameLen of them and not at one more, a media URL and a
// cover emoji count the same way, and the message says characters because they
// are.
func TestLengthBoundsCountCharacters(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner := newTestProfile(t, db, "owner")

	atCeiling := strings.Repeat("字", maxNameLen) // 600 bytes
	if len(atCeiling) <= maxNameLen {
		t.Fatalf("the test name is %d bytes, want more than %d", len(atCeiling), maxNameLen)
	}
	if _, err := db.CreateUserCatalog(ctx, owner, listedCatalogForm(atCeiling)); err != nil {
		t.Fatalf("create with a name of %d characters in a script of three-byte characters: %v", maxNameLen, err)
	}
	_, err := db.CreateUserCollection(ctx, owner, CollectionForm{Title: atCeiling, ViewMode: "TABBED_GRID", Folders: []FolderData{{FolderArt: FolderArt{TileShape: "POSTER"}, Title: atCeiling}}})
	if err != nil {
		t.Fatalf("create a collection and folder titled with %d such characters: %v", maxNameLen, err)
	}

	over := strings.Repeat("字", maxNameLen+1)
	_, err = db.CreateUserCatalog(ctx, owner, listedCatalogForm(over))
	if !errors.Is(err, ErrInvalidInput) || !strings.Contains(err.Error(), "name is longer than 200 characters") {
		t.Errorf("create with %d characters = %v, want ErrInvalidInput naming 200 characters", maxNameLen+1, err)
	}

	for _, tc := range []struct {
		name string
		form CollectionForm
		want string
	}{
		{"a cover emoji", CollectionForm{Title: "C", ViewMode: "TABBED_GRID", Folders: []FolderData{{Title: "F", FolderArt: FolderArt{TileShape: "POSTER", CoverEmoji: strings.Repeat("字", maxCoverEmojiLen+1)}}}}, "cover emoji is longer than 32 characters"},
		{"a genre", CollectionForm{Title: "C", ViewMode: "TABBED_GRID", Folders: []FolderData{{FolderArt: FolderArt{TileShape: "POSTER"}, Title: "F", Catalogs: []FolderCatalogRef{{CatalogID: &uuid.UUID{1}, Genre: strings.Repeat("字", maxGenreLen+1)}}}}}, "genre is longer than 64 characters"},
		{"a media URL", CollectionForm{Title: "C", ViewMode: "TABBED_GRID", BackdropImageURL: "https://example.com/" + strings.Repeat("字", maxMediaURLLen)}, "backdrop image url is longer than 2048 characters"},
	} {
		if err := tc.form.Validate(); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: Validate = %v, want it to say %q", tc.name, err, tc.want)
		}
	}

	// Within the bound in characters, though past it in bytes.
	fits := CollectionForm{Title: "C", ViewMode: "TABBED_GRID", BackdropImageURL: "https://example.com/" + strings.Repeat("字", 1000)}
	if len(fits.BackdropImageURL) <= maxMediaURLLen {
		t.Fatalf("the test URL is %d bytes, want more than %d", len(fits.BackdropImageURL), maxMediaURLLen)
	}
	if err := fits.Validate(); err != nil {
		t.Errorf("a URL of %d characters: Validate = %v, want nil", len([]rune(fits.BackdropImageURL)), err)
	}
}

// A save naming one existing folder twice is refused rather than writing
// both entries to one row; new folders, which carry no id, never collide.
func TestCollectionFormRefusesARepeatedFolderID(t *testing.T) {
	folder := func(id *uuid.UUID) FolderData {
		return FolderData{ID: id, Title: "F", FolderArt: FolderArt{TileShape: "POSTER"}}
	}
	id, other := uuid.New(), uuid.New()
	repeated := CollectionForm{Title: "C", ViewMode: "TABBED_GRID", Folders: []FolderData{folder(&id), folder(&other), folder(&id)}}
	err := repeated.Validate()
	if !errors.Is(err, ErrInvalidInput) || !strings.Contains(err.Error(), "folder 2 repeats an earlier folder's id") {
		t.Errorf("Validate = %v, want ErrInvalidInput naming folder 2", err)
	}
	distinct := CollectionForm{Title: "C", ViewMode: "TABBED_GRID", Folders: []FolderData{folder(&id), folder(nil), folder(nil)}}
	if err := distinct.Validate(); err != nil {
		t.Errorf("distinct ids and new folders: Validate = %v, want nil", err)
	}
}

// A bundle catalog key counts characters too, like every other length.
func TestBundleKeyLengthCountsCharacters(t *testing.T) {
	if problem := bundleKeyProblem("", strings.Repeat("字", maxBundleKeyLen), map[string]bool{}); problem != "" {
		t.Errorf("a key of %d characters: %q, want none", maxBundleKeyLen, problem)
	}
	if problem := bundleKeyProblem("", strings.Repeat("字", maxBundleKeyLen+1), map[string]bool{}); !strings.Contains(problem, "longer than 64 characters") {
		t.Errorf("a key of %d characters: %q, want it too long", maxBundleKeyLen+1, problem)
	}
}

// A view mode and a tile shape are required: a builder write or an import
// with either empty, or with the FOLLOW_LAYOUT view mode, is refused rather
// than given a default.
func TestViewModeAndTileShapeAreRequired(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner := newTestProfile(t, db, "owner")
	valid := func() CollectionForm {
		return CollectionForm{Title: "C", ViewMode: "ROWS", Folders: []FolderData{{Title: "F", FolderArt: FolderArt{TileShape: "SQUARE"}}}}
	}
	for _, tt := range []struct {
		name   string
		change func(*CollectionForm)
		want   string
	}{
		{"empty view mode", func(f *CollectionForm) { f.ViewMode = "" }, `view mode must be "TABBED_GRID" or "ROWS"`},
		{"follow layout", func(f *CollectionForm) { f.ViewMode = "FOLLOW_LAYOUT" }, `view mode must be "TABBED_GRID" or "ROWS"`},
		{"empty tile shape", func(f *CollectionForm) { f.Folders[0].TileShape = "" }, `folder 0: tile shape must be`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			form := valid()
			tt.change(&form)
			if _, err := db.CreateUserCollection(ctx, owner, form); !errors.Is(err, ErrInvalidInput) || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("create = %v, want ErrInvalidInput naming %q", err, tt.want)
			}
		})
	}

	b := readTestBundle(t)
	b.Collections[0].ViewMode = "FOLLOW_LAYOUT"
	b.Collections[0].Folders[1].TileShape = ""
	_, _, err := db.ImportBundle(ctx, owner, b, nil)
	if !errors.Is(err, ErrInvalidInput) || !strings.Contains(err.Error(), "view mode must be") || !strings.Contains(err.Error(), "folder 1: tile shape must be") {
		t.Errorf("import = %v, want both refused", err)
	}

	created, err := db.CreateUserCollection(ctx, owner, valid())
	if err != nil {
		t.Fatalf("create the valid form: %v", err)
	}
	if created.ViewMode != "ROWS" || created.Folders[0].TileShape != "SQUARE" {
		t.Errorf("stored view mode, tile shape = %q, %q; want the form's", created.ViewMode, created.Folders[0].TileShape)
	}
}

// A collection stored past the folder or ref caps stays readable and goes on
// Home, but its save is refused until it is trimmed back under them.
func TestOverCapCollectionStaysReadable(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner := newTestProfile(t, db, "owner")
	catalog, err := db.CreateUserCatalog(ctx, owner, listedCatalogForm("Popular"))
	if err != nil {
		t.Fatal(err)
	}
	refs := make([]FolderCatalogRef, maxRefsPerFolder)
	for i := range refs {
		refs[i] = FolderCatalogRef{CatalogID: &catalog.ID, Genre: strings.Repeat("g", i+1)}
	}
	form := CollectionForm{Title: "Big", ViewMode: "TABBED_GRID", Folders: []FolderData{{Title: "F", FolderArt: FolderArt{TileShape: "POSTER"}, Catalogs: refs}}}
	created, err := db.CreateUserCollection(ctx, owner, form)
	if err != nil {
		t.Fatalf("create at the caps: %v", err)
	}
	if _, err := db.conn.ExecContext(ctx, `
		INSERT INTO folder_catalogs (folder_id, catalog_id, sort_order, genre) VALUES (?, ?, ?, 'one too many')
	`, created.Folders[0].ID.String(), catalog.ID.String(), maxRefsPerFolder); err != nil {
		t.Fatal(err)
	}

	stored := mustOwnCollection(t, db, owner, created.ID)
	if got := len(stored.Folders[0].Refs); got != maxRefsPerFolder+1 {
		t.Fatalf("stored refs = %d, want %d", got, maxRefsPerFolder+1)
	}
	if _, err := db.BuildPushRecord(ctx, owner, PushedHome{Collections: []SelectedCollectionInput{{CollectionID: created.ID}}}); err != nil {
		t.Errorf("a push record with the over-cap collection = %v, want it built", err)
	}
	saved := saveFormOf(stored)
	if _, err := db.UpdateUserCollection(ctx, owner, created.ID, collectionRevision(t, db, created.ID), saved); !errors.Is(err, ErrInvalidInput) || !strings.Contains(err.Error(), "holds more than 20 catalogs") {
		t.Errorf("save of the over-cap collection = %v, want refused", err)
	}
	saved.Folders[0].Catalogs = saved.Folders[0].Catalogs[:maxRefsPerFolder]
	if _, err := db.UpdateUserCollection(ctx, owner, created.ID, collectionRevision(t, db, created.ID), saved); err != nil {
		t.Errorf("save once trimmed = %v, want it saved", err)
	}
}
