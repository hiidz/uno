package vault

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/google/uuid"
)

// catalogRevision is the revision catalog id is at now: what a test save built
// from the row as it stands carries.
func catalogRevision(t *testing.T, db *DB, id uuid.UUID) int64 {
	t.Helper()
	return rowRevision(t, db, `SELECT revision FROM catalogs WHERE id = ?`, id)
}

// collectionRevision is the revision collection id is at now.
func collectionRevision(t *testing.T, db *DB, id uuid.UUID) int64 {
	t.Helper()
	return rowRevision(t, db, `SELECT revision FROM collections WHERE id = ?`, id)
}

// rowRevision is the revision query reads for id, 0 when there is no such
// row.
func rowRevision(t *testing.T, db *DB, query string, id uuid.UUID) int64 {
	t.Helper()
	var revision int64
	if err := db.conn.QueryRow(query, id.String()).Scan(&revision); err != nil && !errors.Is(err, sql.ErrNoRows) {
		t.Fatal(err)
	}
	return revision
}

// A catalog save raises the catalog's revision; one from an earlier revision,
// or carrying none, is ErrStale and writes nothing. One for a catalog that
// isn't the caller's is not found, whatever its revision.
func TestCatalogSaveRaisesAndChecksRevision(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner := newTestProfile(t, db, "owner")
	other := newTestProfile(t, db, "other")
	c, err := db.CreateUserCatalog(ctx, owner, listedCatalogForm("First"))
	if err != nil || c.Revision != 1 || catalogRevision(t, db, c.ID) != 1 {
		t.Fatalf("CreateUserCatalog = %+v, %v; want revision 1, stored and answered", c, err)
	}

	saved, err := db.UpdateUserCatalog(ctx, owner, c.ID, 1, listedCatalogForm("Second"))
	if err != nil || saved.Revision != 2 || saved.Name != "Second" {
		t.Fatalf("save at 1 = %+v, %v; want Second at revision 2", saved, err)
	}
	if _, err := db.UpdateUserCatalog(ctx, owner, c.ID, 2, listedCatalogForm("Second")); err != nil {
		t.Fatalf("an unchanged save at 2: %v", err)
	}
	if got := catalogRevision(t, db, c.ID); got != 3 {
		t.Errorf("after an unchanged save the catalog is at %d, want 3: every save is a write", got)
	}

	for _, stale := range []int64{2, 0, 4} {
		if _, err := db.UpdateUserCatalog(ctx, owner, c.ID, stale, listedCatalogForm("Stale")); !errors.Is(err, ErrStale) {
			t.Errorf("save at %d = %v, want ErrStale", stale, err)
		}
	}
	if got := reloadCatalog(t, db, c.ID); got.Name != "Second" || got.Revision != 3 {
		t.Errorf("after the stale saves = %q at %d, want Second at 3", got.Name, got.Revision)
	}
	if _, err := db.UpdateUserCatalog(ctx, other, c.ID, 0, listedCatalogForm("Theirs")); !errors.Is(err, ErrCatalogNotFound) {
		t.Errorf("another profile's save = %v, want ErrCatalogNotFound", err)
	}
}

// A collection save raises the collection's revision, and the revision of
// each scoped catalog its edits change, not of one an edit leaves as it is.
// A save from an earlier revision, or carrying none, is ErrStale and writes
// nothing, its catalog edits included. One for a collection that isn't the
// caller's is not found.
func TestCollectionSaveRaisesAndChecksRevision(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner := newTestProfile(t, db, "owner")
	other := newTestProfile(t, db, "other")
	created, err := db.CreateUserCollection(ctx, owner, CollectionForm{Title: "Studios", ViewMode: "TABBED_GRID", Folders: []FolderData{
		{Title: "Pixar", FolderArt: FolderArt{TileShape: "POSTER"}, Catalogs: []FolderCatalogRef{newScoped("k", "Pixar", "{}")}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	scoped := created.Catalogs[0]
	if created.Revision != 1 || scoped.Revision != 1 {
		t.Fatalf("a new collection = %d with its catalog at %d, want both at 1", created.Revision, scoped.Revision)
	}

	form := saveFormOf(created)
	form.CatalogEdits = []ScopedCatalogEdit{{ID: scoped.ID, Type: "movie", Provider: "tmdb", Name: "Pixar films", Params: "{}"}}
	saved, err := db.UpdateUserCollection(ctx, owner, created.ID, 1, form)
	if err != nil || saved.Revision != 2 {
		t.Fatalf("save at 1 = %v; want revision 2, got %d", err, saved.Revision)
	}
	if got := catalogRevision(t, db, scoped.ID); got != 2 {
		t.Errorf("the edited catalog is at %d, want 2", got)
	}
	if _, err := db.UpdateUserCollection(ctx, owner, created.ID, 2, form); err != nil {
		t.Fatalf("save at 2 with the same edit: %v", err)
	}
	if got, want := [2]int64{collectionRevision(t, db, created.ID), catalogRevision(t, db, scoped.ID)}, [2]int64{3, 2}; got != want {
		t.Errorf("after an edit that changes nothing, collection and catalog = %v, want %v", got, want)
	}

	stale := saveFormOf(created)
	stale.Title = "Stale"
	stale.CatalogEdits = []ScopedCatalogEdit{{ID: scoped.ID, Type: "movie", Provider: "tmdb", Name: "Stale", Params: "{}"}}
	for _, revision := range []int64{2, 0} {
		if _, err := db.UpdateUserCollection(ctx, owner, created.ID, revision, stale); !errors.Is(err, ErrStale) {
			t.Errorf("save at %d = %v, want ErrStale", revision, err)
		}
	}
	if got := mustOwnCollection(t, db, owner, created.ID); got.Title != "Studios" || got.Revision != 3 || reloadCatalog(t, db, scoped.ID).Name != "Pixar films" {
		t.Errorf("after the stale saves = %q at %d, catalog %q; want them unwritten", got.Title, got.Revision, reloadCatalog(t, db, scoped.ID).Name)
	}
	if _, err := db.UpdateUserCollection(ctx, other, created.ID, 0, stale); !errors.Is(err, ErrCollectionNotFound) {
		t.Errorf("another profile's save = %v, want ErrCollectionNotFound", err)
	}
}

// A push raises the profile's home_revision, which the library reads, and no
// row's revision; neither do publishing, unpublishing, picking the profile
// again, or a catalog delete cascading out of a collection's folder.
func TestRevisionsUnmovedByHomeSharingAndDelete(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner := newTestProfile(t, db, "owner")
	listed, err := db.CreateUserCatalog(ctx, owner, listedCatalogForm("Listed"))
	if err != nil {
		t.Fatal(err)
	}
	doomed, err := db.CreateUserCatalog(ctx, owner, listedCatalogForm("Doomed"))
	if err != nil {
		t.Fatal(err)
	}
	ids := []uuid.UUID{listed.ID, doomed.ID}
	collection, err := db.CreateUserCollection(ctx, owner, CollectionForm{Title: "Mixed", ViewMode: "TABBED_GRID", Folders: []FolderData{
		{Title: "Both", FolderArt: FolderArt{TileShape: "POSTER"}, Catalogs: []FolderCatalogRef{{CatalogID: &ids[0]}, {CatalogID: &ids[1]}}},
	}})
	if err != nil {
		t.Fatal(err)
	}

	record, err := db.BuildPushRecord(ctx, owner, PushedHome{
		Catalogs:    []SelectedCatalogInput{{CatalogID: listed.ID, ShowInHome: true, Position: 0}},
		Collections: []SelectedCollectionInput{{CollectionID: collection.ID, PinToTop: true, Position: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if revision, err := db.SavePush(ctx, owner, record); err != nil || revision != 2 {
		t.Fatalf("SavePush = %d, %v; want home revision 2", revision, err)
	}
	if lib, err := db.GetLibrary(ctx, owner); err != nil || lib.HomeRevision != 2 {
		t.Errorf("GetLibrary home_revision = %d, %v; want 2", lib.HomeRevision, err)
	}
	if revision, err := db.SavePush(ctx, owner, record); err != nil || revision != 3 {
		t.Errorf("a second SavePush = %d, %v; want 3", revision, err)
	}

	if _, err := db.PublishCatalog(ctx, owner, listed.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.PublishCollection(ctx, owner, collection.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.UnpublishCatalog(ctx, owner, listed.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.UnpublishCollection(ctx, owner, collection.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ResolveOrCreateProfile(ctx, "owner", 1, "nuvio-profile-owner"); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteUserCatalog(ctx, owner, doomed.ID); err != nil {
		t.Fatal(err)
	}

	if got := mustOwnCollection(t, db, owner, collection.ID); got.Revision != 1 || len(got.Folders[0].Refs) != 1 {
		t.Errorf("collection = revision %d with %d refs, want 1 with the deleted catalog's ref gone", got.Revision, len(got.Folders[0].Refs))
	}
	if got := catalogRevision(t, db, listed.ID); got != 1 {
		t.Errorf("listed catalog = revision %d, want 1", got)
	}
	if got, err := db.HomeRevision(ctx, owner); err != nil || got != 3 {
		t.Errorf("HomeRevision = %d, %v; want 3", got, err)
	}
	if _, err := db.HomeRevision(ctx, uuid.New()); !errors.Is(err, ErrProfileNotFound) {
		t.Errorf("HomeRevision of no profile = %v, want ErrProfileNotFound", err)
	}
}

// Update writes a subscribed copy with no revision to check, and raises it:
// a catalog copy's and a collection copy's alike.
func TestUpdateRaisesCopyRevisionUnchecked(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner := newTestProfile(t, db, "owner")
	subscriber := newTestProfile(t, db, "subscriber")

	source := publishCatalog(t, db, owner, "Popular", "{}")
	catalogCopy := subscribe(t, db, subscriber, source.Publication.ID).Catalog
	tree := publishCollection(t, db, owner, CollectionForm{Title: "Studios", ViewMode: "TABBED_GRID", Folders: []FolderData{
		{Title: "Pixar", FolderArt: FolderArt{TileShape: "POSTER"}, Catalogs: []FolderCatalogRef{newScoped("k", "Pixar", "{}")}},
	}})
	collectionCopy := subscribe(t, db, subscriber, tree.Publication.ID).Collection

	if _, err := db.UpdateUserCatalog(ctx, owner, source.ID, 1, listedCatalogForm("Popular now")); err != nil {
		t.Fatal(err)
	}
	if _, err := db.PublishCatalog(ctx, owner, source.ID); err != nil {
		t.Fatal(err)
	}
	form := saveFormOf(tree)
	form.Title = "Studios now"
	if _, err := db.UpdateUserCollection(ctx, owner, tree.ID, 1, form); err != nil {
		t.Fatal(err)
	}
	if _, err := db.PublishCollection(ctx, owner, tree.ID); err != nil {
		t.Fatal(err)
	}

	if _, err := db.UpdateSubscription(ctx, subscriber, source.Publication.ID); err != nil {
		t.Fatalf("Update of the catalog copy: %v", err)
	}
	if _, err := db.UpdateSubscription(ctx, subscriber, tree.Publication.ID); err != nil {
		t.Fatalf("Update of the collection copy: %v", err)
	}
	if got := catalogRevision(t, db, catalogCopy.ID); got != 2 {
		t.Errorf("catalog copy after Update = revision %d, want 2", got)
	}
	if got := collectionRevision(t, db, collectionCopy.ID); got != 2 {
		t.Errorf("collection copy after Update = revision %d, want 2", got)
	}
}
