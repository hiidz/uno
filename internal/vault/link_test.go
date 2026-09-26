package vault

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

// linkFixture is a public collection of owner's, shown first on Home, and
// taker's linked copy of it, which is not. The source holds a listed catalog in both of its folders and a scoped
// catalog under a genre in the first.
type linkFixture struct {
	db             *DB
	owner, taker   uuid.UUID
	source, taken  CollectionWithFolders
	listed, scoped Catalog
}

func newLinkFixture(t *testing.T) linkFixture {
	t.Helper()
	ctx := context.Background()
	db := newTestDB(t)
	f := linkFixture{db: db, owner: newTestProfile(t, db, "owner"), taker: newTestProfile(t, db, "taker")}

	listedForm := listedCatalogForm("Listed")
	listedForm.Fingerprint = "fp-listed"
	listed, err := db.CreateUserCatalog(ctx, f.owner, listedForm)
	if err != nil {
		t.Fatalf("create listed catalog: %v", err)
	}
	scoped := &NewScopedCatalog{Key: "scoped", Type: "movie", Name: "Scoped", Provider: "tmdb", Params: "{}", Fingerprint: "fp-scoped"}
	f.source, err = db.CreateUserCollection(ctx, f.owner, CollectionForm{
		Title: "Source", IsPublic: true, PinToTop: true, ViewMode: "TABBED_GRID",
		Folders: []FolderData{
			{Title: "Folder 1", Catalogs: []FolderCatalogRef{{CatalogID: &listed.ID}, {New: scoped, Genre: "Drama"}}},
			{Title: "Folder 2", Catalogs: CatalogRefs(listed.ID)},
		},
	})
	if err != nil {
		t.Fatalf("create source collection: %v", err)
	}
	for _, c := range f.source.Catalogs {
		if c.CollectionID == nil {
			f.listed = c
		} else {
			f.scoped = c
		}
	}
	f.taken, err = db.TakeCollection(ctx, f.taker, f.source.ID, allowAnyCatalogParams)
	if err != nil {
		t.Fatalf("TakeCollection: %v", err)
	}
	if f.taken.PinToTop {
		t.Fatal("taken copy pin_to_top = true, want false: Show first is the taker's own")
	}
	return f
}

// saveFormOf is the form an editor save of tree sends when nothing was
// changed: every field and folder as stored, each ref by id and genre.
func saveFormOf(tree CollectionWithFolders) CollectionForm {
	form := CollectionForm{
		Title: tree.Title, IsPublic: tree.IsPublic, PinToTop: tree.PinToTop, ViewMode: tree.ViewMode,
		ShowAllTab: tree.ShowAllTab, BackdropImageURL: tree.BackdropImageURL, FocusGlowEnabled: tree.FocusGlowEnabled,
	}
	for _, f := range tree.Folders {
		fd := FolderData{
			ID: &f.ID, Title: f.Title, TileShape: f.TileShape, HideTitle: f.HideTitle, CoverEmoji: f.CoverEmoji,
			CoverImageURL: f.CoverImageURL, FocusGIFURL: f.FocusGIFURL, FocusGIFEnabled: f.FocusGIFEnabled,
			HeroBackdropURL: f.HeroBackdropURL, HeroVideoURL: f.HeroVideoURL, TitleLogoURL: f.TitleLogoURL,
		}
		for _, ref := range f.Refs {
			fd.Catalogs = append(fd.Catalogs, FolderCatalogRef{CatalogID: &ref.CatalogID, Genre: ref.Genre})
		}
		form.Folders = append(form.Folders, fd)
	}
	return form
}

// saveOwnerTitle saves the fixture's source with a new title: an owner edit
// that changes the collection's hash.
func (f linkFixture) saveOwnerTitle(t *testing.T, title string) {
	t.Helper()
	form := saveFormOf(f.source)
	form.Title = title
	if _, err := f.db.UpdateUserCollection(context.Background(), f.owner, f.source.ID, form); err != nil {
		t.Fatalf("owner edit: %v", err)
	}
}

// requireCommunityCollection fails unless the source's community row for
// profileID reads taken and updateAvailable.
func (f linkFixture) requireCommunityCollection(t *testing.T, taken, updateAvailable bool) {
	t.Helper()
	row := communityCollectionRow(t, f.db, f.taker, f.source.ID)
	if row.Taken != taken || row.UpdateAvailable != updateAvailable {
		t.Fatalf("community row taken = %v, update_available = %v, want %v, %v", row.Taken, row.UpdateAvailable, taken, updateAvailable)
	}
}

// reloadCatalog reads catalog id back, listed or scoped.
func reloadCatalog(t *testing.T, db *DB, id uuid.UUID) Catalog {
	t.Helper()
	rows, err := db.queryCatalogs(context.Background(), "id = ?", id.String())
	if err != nil || len(rows) != 1 {
		t.Fatalf("loading catalog %s = %+v, %v, want one row", id, rows, err)
	}
	return rows[0]
}

// Community reads none, then taken, then update available once the owner
// edits, and taken with no update again once the taker updates.
func TestCommunityCollectionOffersUpdateAfterOwnerEdit(t *testing.T) {
	ctx := context.Background()
	f := newLinkFixture(t)
	other := newTestProfile(t, f.db, "other")

	if row := communityCollectionRow(t, f.db, other, f.source.ID); row.Taken || row.UpdateAvailable {
		t.Fatalf("community row before any take = %+v, want neither taken nor update_available", row)
	}
	f.requireCommunityCollection(t, true, false)

	f.saveOwnerTitle(t, "Source, renamed")
	f.requireCommunityCollection(t, true, true)

	updated, err := f.db.UpdateTakenCollection(ctx, f.taker, f.source.ID, allowAnyCatalogParams)
	if err != nil {
		t.Fatalf("UpdateTakenCollection: %v", err)
	}
	if updated.Title != "Source, renamed" || !updated.Linked {
		t.Fatalf("updated copy title = %q, linked = %v, want the new title, still linked", updated.Title, updated.Linked)
	}
	f.requireCommunityCollection(t, true, false)
}

// Update rewrites the copy with the original's content while keeping what is
// the taker's own: the collection's id, is_public and pin_to_top, its home placement and
// pushed_version, the ids of folders by position and of catalogs by source.
// Version goes up by one. A catalog the original dropped is removed, and one
// it added becomes a new scoped catalog linked to its source.
func TestUpdateTakenCollectionRewritesCopy(t *testing.T) {
	ctx := context.Background()
	f := newLinkFixture(t)

	// The taker's own settings: public, and on Home as pushed.
	form := saveFormOf(f.taken)
	form.IsPublic = true
	copyBefore, err := f.db.UpdateUserCollection(ctx, f.taker, f.taken.ID, form)
	if err != nil || !copyBefore.Linked {
		t.Fatalf("making the copy public = %+v, %v, want it saved and still linked", copyBefore.Linked, err)
	}
	if err := f.db.SaveSelectionsForPush(ctx, f.taker, CatalogSelectionForm{}, CollectionSelectionForm{CollectionIDs: []uuid.UUID{f.taken.ID}},
		map[uuid.UUID]int{f.taken.ID: copyBefore.Version}); err != nil {
		t.Fatalf("SaveSelectionsForPush: %v", err)
	}

	// The owner renames the scoped catalog, drops the listed one and the
	// second folder, and adds two folders holding a new catalog.
	added, err := f.db.CreateUserCatalog(ctx, f.owner, listedCatalogForm("Added"))
	if err != nil {
		t.Fatalf("create added catalog: %v", err)
	}
	edit := editOf(f.scoped)
	edit.Name, edit.Fingerprint = "Scoped, renamed", "fp-scoped-2"
	if _, err := f.db.UpdateUserCollection(ctx, f.owner, f.source.ID, CollectionForm{
		Title: "Source", IsPublic: true, ViewMode: "TABBED_GRID",
		Folders: []FolderData{
			{ID: &f.source.Folders[0].ID, Title: "Folder 1", Catalogs: []FolderCatalogRef{{CatalogID: &f.scoped.ID, Genre: "Drama"}}},
			{Title: "Folder 3", Catalogs: CatalogRefs(added.ID)},
			{Title: "Folder 4", Catalogs: []FolderCatalogRef{{CatalogID: &added.ID, Genre: "Horror"}}},
		},
		CatalogEdits: []ScopedCatalogEdit{edit},
	}); err != nil {
		t.Fatalf("owner edit: %v", err)
	}

	updated, err := f.db.UpdateTakenCollection(ctx, f.taker, f.source.ID, allowAnyCatalogParams)
	if err != nil {
		t.Fatalf("UpdateTakenCollection: %v", err)
	}

	if updated.ID != f.taken.ID || !updated.IsPublic || updated.PinToTop || !updated.Linked {
		t.Fatalf("updated copy id = %s, public = %v, pinned = %v, linked = %v, want %s, true, false, true",
			updated.ID, updated.IsPublic, updated.PinToTop, updated.Linked, f.taken.ID)
	}
	if updated.Version != copyBefore.Version+1 || updated.PushedVersion == nil || *updated.PushedVersion != copyBefore.Version {
		t.Fatalf("updated copy version = %d, pushed_version = %v, want %d, %d", updated.Version, updated.PushedVersion, copyBefore.Version+1, copyBefore.Version)
	}
	onHome, err := f.db.GetCurrentCollectionSelection(ctx, f.taker)
	if err != nil || len(onHome) != 1 || onHome[0].ID != f.taken.ID {
		t.Fatalf("taker's home selection = %+v, %v, want the copy still on it", onHome, err)
	}

	if len(updated.Folders) != 3 || updated.Folders[0].ID != f.taken.Folders[0].ID || updated.Folders[1].ID != f.taken.Folders[1].ID {
		t.Fatalf("updated folders = %+v, want three, the first two keeping the copy's folder ids", updated.Folders)
	}
	if updated.Folders[2].Title != "Folder 4" {
		t.Fatalf("third folder = %q, want Folder 4", updated.Folders[2].Title)
	}

	var scopedCopy, listedCopy uuid.UUID
	for _, c := range f.taken.Catalogs {
		switch *c.TakenFrom {
		case f.scoped.ID:
			scopedCopy = c.ID
		case f.listed.ID:
			listedCopy = c.ID
		}
	}
	byID := map[uuid.UUID]Catalog{}
	for _, c := range updated.Catalogs {
		byID[c.ID] = c
	}
	if len(updated.Catalogs) != 2 || byID[scopedCopy].Name != "Scoped, renamed" || byID[scopedCopy].Fingerprint != "fp-scoped-2" {
		t.Fatalf("updated catalogs = %+v, want the scoped copy %s renamed in place plus one new catalog", updated.Catalogs, scopedCopy)
	}
	if _, kept := byID[listedCopy]; kept {
		t.Fatalf("the copy of the dropped listed catalog %s is still referenced", listedCopy)
	}
	if rows, _ := f.db.queryCatalogs(ctx, "id = ?", listedCopy.String()); len(rows) != 0 {
		t.Fatalf("the copy of the dropped listed catalog %s still exists", listedCopy)
	}
	for id, c := range byID {
		if id != scopedCopy && (c.TakenFrom == nil || *c.TakenFrom != added.ID || c.CollectionID == nil) {
			t.Fatalf("new catalog %+v, want it scoped to the copy and linked to %s", c, added.ID)
		}
	}

	f.requireCommunityCollection(t, true, false)
}

// A save of a linked collection keeps the link while the content is
// unchanged — an unchanged save, a Public and Show first toggle, a Home
// change — and a real
// edit unlinks it, after which the source can be taken again.
func TestCollectionSaveKeepsOrClearsLink(t *testing.T) {
	ctx := context.Background()
	f := newLinkFixture(t)

	unchanged, err := f.db.UpdateUserCollection(ctx, f.taker, f.taken.ID, saveFormOf(f.taken))
	if err != nil || !unchanged.Linked {
		t.Fatalf("unchanged save linked = %v, %v, want still linked", unchanged.Linked, err)
	}
	public := saveFormOf(f.taken)
	public.IsPublic, public.PinToTop = true, true
	toggled, err := f.db.UpdateUserCollection(ctx, f.taker, f.taken.ID, public)
	if err != nil || !toggled.Linked {
		t.Fatalf("Public and Show first toggle linked = %v, %v, want still linked", toggled.Linked, err)
	}
	if err := f.db.SaveSelectionsForPush(ctx, f.taker, CatalogSelectionForm{}, CollectionSelectionForm{CollectionIDs: []uuid.UUID{f.taken.ID}},
		map[uuid.UUID]int{f.taken.ID: toggled.Version}); err != nil {
		t.Fatalf("SaveSelectionsForPush: %v", err)
	}
	if onHome, err := f.db.reloadCollection(ctx, f.taken.ID); err != nil || !onHome.Linked {
		t.Fatalf("after a Home change linked = %v, %v, want still linked", onHome.Linked, err)
	}
	f.requireCommunityCollection(t, true, false)

	edited := saveFormOf(f.taken)
	edited.Folders[1].Title = "Renamed by the taker"
	saved, err := f.db.UpdateUserCollection(ctx, f.taker, f.taken.ID, edited)
	if err != nil || saved.Linked {
		t.Fatalf("real edit linked = %v, %v, want unlinked", saved.Linked, err)
	}
	if reloaded, err := f.db.reloadCollection(ctx, f.taken.ID); err != nil || reloaded.Linked || reloaded.TakenHash != "" {
		t.Fatalf("stored after a real edit: linked = %v, taken_hash = %q, %v, want both cleared", reloaded.Linked, reloaded.TakenHash, err)
	}
	f.requireCommunityCollection(t, false, false)

	if _, err := f.db.TakeCollection(ctx, f.taker, f.source.ID, allowAnyCatalogParams); err != nil {
		t.Fatalf("Take after unlinking: %v", err)
	}
}

// Moving a catalog of a linked collection to the library unlinks the
// collection, and the moved catalog carries no taken_from, so it never reads
// as a Take of its own.
func TestMoveToLibraryUnlinksCollection(t *testing.T) {
	ctx := context.Background()
	f := newLinkFixture(t)

	moved := f.taken.Catalogs[0]
	form := saveFormOf(f.taken)
	form.CatalogEdits = []ScopedCatalogEdit{moveToLibraryEdit(moved)}
	saved, err := f.db.UpdateUserCollection(ctx, f.taker, f.taken.ID, form)
	if err != nil || saved.Linked {
		t.Fatalf("Move to library linked = %v, %v, want the collection unlinked", saved.Linked, err)
	}
	if c := reloadCatalog(t, f.db, moved.ID); c.CollectionID != nil || c.TakenFrom != nil || c.Linked {
		t.Fatalf("moved catalog = %+v, want it listed with no taken_from", c)
	}
}

// Community reads taken, then update available once the owner edits; Update
// brings the copy's name and recipe up to date and keeps it linked.
func TestCommunityCatalogOffersUpdateAfterOwnerEdit(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner := newTestProfile(t, db, "owner")
	taker := newTestProfile(t, db, "taker")

	form := publicCatalogForm("Source")
	form.Fingerprint = "fp-source"
	source, err := db.CreateUserCatalog(ctx, owner, form)
	if err != nil {
		t.Fatalf("create source: %v", err)
	}
	if _, err := db.TakeCatalog(ctx, taker, source.ID, allowAnyCatalogParams); err != nil {
		t.Fatalf("TakeCatalog: %v", err)
	}

	form.Name, form.Params, form.Fingerprint = "Source, renamed", `{"sort_by":"popularity.desc"}`, "fp-source-2"
	if _, err := db.UpdateUserCatalog(ctx, owner, source.ID, form); err != nil {
		t.Fatalf("owner edit: %v", err)
	}
	if row := communityCatalogRow(t, db, taker, source.ID); !row.Taken || !row.UpdateAvailable {
		t.Fatalf("community row after the owner's edit = %+v, want taken with an update available", row)
	}

	updated, err := db.UpdateTakenCatalog(ctx, taker, source.ID, allowAnyCatalogParams)
	if err != nil {
		t.Fatalf("UpdateTakenCatalog: %v", err)
	}
	if updated.Name != form.Name || updated.Params != form.Params || updated.Fingerprint != form.Fingerprint || !updated.Linked {
		t.Fatalf("updated copy = %+v, want the owner's name, params and fingerprint, still linked", updated)
	}
	if row := communityCatalogRow(t, db, taker, source.ID); !row.Taken || row.UpdateAvailable {
		t.Fatalf("community row after Update = %+v, want taken with no update", row)
	}
}

// A save of a linked catalog keeps the link while its name and recipe are
// unchanged; a real edit, or a move into a collection, unlinks it, after
// which the source can be taken again.
func TestCatalogSaveKeepsOrClearsLink(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner := newTestProfile(t, db, "owner")
	taker := newTestProfile(t, db, "taker")

	sourceForm := publicCatalogForm("Source")
	sourceForm.Fingerprint = "fp-source"
	source, err := db.CreateUserCatalog(ctx, owner, sourceForm)
	if err != nil {
		t.Fatalf("create source: %v", err)
	}
	takeCopy := func() Catalog {
		t.Helper()
		c, err := db.TakeCatalog(ctx, taker, source.ID, allowAnyCatalogParams)
		if err != nil {
			t.Fatalf("TakeCatalog: %v", err)
		}
		return c
	}

	linked := takeCopy()
	public := CatalogForm{Type: linked.Type, Name: linked.Name, Provider: linked.Provider, Params: linked.Params, IsPublic: true, Fingerprint: linked.Fingerprint}
	if saved, err := db.UpdateUserCatalog(ctx, taker, linked.ID, public); err != nil || !saved.Linked {
		t.Fatalf("Public toggle linked = %v, %v, want still linked", saved.Linked, err)
	}
	if c := reloadCatalog(t, db, linked.ID); !c.Linked || c.TakenHash != linked.TakenHash {
		t.Fatalf("stored after a Public toggle: linked = %v, taken_hash = %q, want the link unchanged", c.Linked, c.TakenHash)
	}

	renamed := public
	renamed.Name = "Renamed by the taker"
	if saved, err := db.UpdateUserCatalog(ctx, taker, linked.ID, renamed); err != nil || saved.Linked {
		t.Fatalf("real edit linked = %v, %v, want unlinked", saved.Linked, err)
	}
	if c := reloadCatalog(t, db, linked.ID); c.TakenFrom != nil || c.TakenHash != "" {
		t.Fatalf("stored after a real edit: taken_from = %v, taken_hash = %q, want both cleared", c.TakenFrom, c.TakenHash)
	}

	again := takeCopy()
	collectionID := newTestCollection(t, db, taker, "Taker's")
	scoped := CatalogForm{Type: again.Type, Name: again.Name, Provider: again.Provider, Params: again.Params, CollectionID: &collectionID, Fingerprint: again.Fingerprint}
	if saved, err := db.UpdateUserCatalog(ctx, taker, again.ID, scoped); err != nil || saved.Linked {
		t.Fatalf("move into a collection linked = %v, %v, want unlinked", saved.Linked, err)
	}
	if c := reloadCatalog(t, db, again.ID); c.TakenFrom != nil {
		t.Fatalf("stored after a move into a collection: taken_from = %v, want cleared", c.TakenFrom)
	}
	takeCopy()
}

// A copy whose stored taken_hash predates the current hash rule, and which
// still matches its original, is offered an Update that changes nothing but
// taken_hash.
func TestUpdateTakenCollectionSettlesStaleHash(t *testing.T) {
	ctx := context.Background()
	f := newLinkFixture(t)
	if _, err := f.db.conn.ExecContext(ctx, `UPDATE collections SET taken_hash = 'stale' WHERE id = ?`, f.taken.ID.String()); err != nil {
		t.Fatalf("staling taken_hash: %v", err)
	}
	f.requireCommunityCollection(t, true, true)

	updated, err := f.db.UpdateTakenCollection(ctx, f.taker, f.source.ID, allowAnyCatalogParams)
	if err != nil {
		t.Fatalf("UpdateTakenCollection: %v", err)
	}
	requireSameTree(t, updated, f.taken)
	f.requireCommunityCollection(t, true, false)
}

// A linked copy changed some way a save didn't unlink no longer matches its
// taken_hash: Update refuses to overwrite it with ErrConflict, unlinks it,
// and leaves it as it was, after which the source can be taken again.
func TestUpdateTakenCollectionConflictUnlinks(t *testing.T) {
	ctx := context.Background()
	f := newLinkFixture(t)
	if _, err := f.db.conn.ExecContext(ctx, `UPDATE collections SET title = 'Changed behind the save' WHERE id = ?`, f.taken.ID.String()); err != nil {
		t.Fatalf("changing the copy: %v", err)
	}
	f.saveOwnerTitle(t, "Source, renamed")

	if _, err := f.db.UpdateTakenCollection(ctx, f.taker, f.source.ID, allowAnyCatalogParams); !errors.Is(err, ErrConflict) {
		t.Fatalf("UpdateTakenCollection err = %v, want ErrConflict", err)
	}
	reloaded, err := f.db.reloadCollection(ctx, f.taken.ID)
	if err != nil || reloaded.Linked || reloaded.Title != "Changed behind the save" || reloaded.Version != f.taken.Version {
		t.Fatalf("copy after the conflict = linked %v, %q v%d, %v, want it unlinked and untouched", reloaded.Linked, reloaded.Title, reloaded.Version, err)
	}
	if _, err := f.db.TakeCollection(ctx, f.taker, f.source.ID, allowAnyCatalogParams); err != nil {
		t.Fatalf("Take after the conflict: %v", err)
	}
}

// The same conflict for a catalog: ErrConflict, the copy unlinked and left
// as it was, and the source takeable again.
func TestUpdateTakenCatalogConflictUnlinks(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner := newTestProfile(t, db, "owner")
	taker := newTestProfile(t, db, "taker")

	form := publicCatalogForm("Source")
	form.Fingerprint = "fp-source"
	source, err := db.CreateUserCatalog(ctx, owner, form)
	if err != nil {
		t.Fatalf("create source: %v", err)
	}
	taken, err := db.TakeCatalog(ctx, taker, source.ID, allowAnyCatalogParams)
	if err != nil {
		t.Fatalf("TakeCatalog: %v", err)
	}
	if _, err := db.conn.ExecContext(ctx, `UPDATE catalogs SET name = 'Changed behind the save' WHERE id = ?`, taken.ID.String()); err != nil {
		t.Fatalf("changing the copy: %v", err)
	}

	if _, err := db.UpdateTakenCatalog(ctx, taker, source.ID, allowAnyCatalogParams); !errors.Is(err, ErrConflict) {
		t.Fatalf("UpdateTakenCatalog err = %v, want ErrConflict", err)
	}
	if c := reloadCatalog(t, db, taken.ID); c.Linked || c.Name != "Changed behind the save" {
		t.Fatalf("copy after the conflict = %+v, want it unlinked and untouched", c)
	}
	if _, err := db.TakeCatalog(ctx, taker, source.ID, allowAnyCatalogParams); err != nil {
		t.Fatalf("Take after the conflict: %v", err)
	}
}

// An original made private can't be updated from (not found) and leaves the
// copy linked; a deleted original unlinks its copies.
func TestUpdateAfterOriginalPrivateOrDeleted(t *testing.T) {
	ctx := context.Background()
	f := newLinkFixture(t)

	private := saveFormOf(f.source)
	private.IsPublic = false
	if _, err := f.db.UpdateUserCollection(ctx, f.owner, f.source.ID, private); err != nil {
		t.Fatalf("making the source private: %v", err)
	}
	if _, err := f.db.UpdateTakenCollection(ctx, f.taker, f.source.ID, allowAnyCatalogParams); !errors.Is(err, ErrCollectionNotFound) {
		t.Fatalf("Update from a private original err = %v, want ErrCollectionNotFound", err)
	}
	if c, err := f.db.reloadCollection(ctx, f.taken.ID); err != nil || !c.Linked {
		t.Fatalf("copy of a private original linked = %v, %v, want still linked", c.Linked, err)
	}

	if err := f.db.DeleteUserCollection(ctx, f.owner, f.source.ID); err != nil {
		t.Fatalf("deleting the source: %v", err)
	}
	if c, err := f.db.reloadCollection(ctx, f.taken.ID); err != nil || c.Linked {
		t.Fatalf("copy of a deleted original linked = %v, %v, want unlinked", c.Linked, err)
	}

	listedForm := publicCatalogForm("Listed source")
	source, err := f.db.CreateUserCatalog(ctx, f.owner, listedForm)
	if err != nil {
		t.Fatalf("create catalog source: %v", err)
	}
	taken, err := f.db.TakeCatalog(ctx, f.taker, source.ID, allowAnyCatalogParams)
	if err != nil {
		t.Fatalf("TakeCatalog: %v", err)
	}
	listedForm.IsPublic = false
	if _, err := f.db.UpdateUserCatalog(ctx, f.owner, source.ID, listedForm); err != nil {
		t.Fatalf("making the catalog source private: %v", err)
	}
	if _, err := f.db.UpdateTakenCatalog(ctx, f.taker, source.ID, allowAnyCatalogParams); !errors.Is(err, ErrCatalogNotFound) {
		t.Fatalf("Update from a private catalog err = %v, want ErrCatalogNotFound", err)
	}
	if err := f.db.DeleteUserCatalog(ctx, f.owner, source.ID); err != nil {
		t.Fatalf("deleting the catalog source: %v", err)
	}
	if c := reloadCatalog(t, f.db, taken.ID); c.Linked {
		t.Fatalf("copy of a deleted catalog = %+v, want unlinked", c)
	}
}

// Update needs a linked copy: before any Take, or once the copy is
// unlinked, it is not found.
func TestUpdateWithoutLinkedCopyIsNotFound(t *testing.T) {
	ctx := context.Background()
	f := newLinkFixture(t)
	other := newTestProfile(t, f.db, "other")

	if _, err := f.db.UpdateTakenCollection(ctx, other, f.source.ID, allowAnyCatalogParams); !errors.Is(err, ErrCollectionNotFound) {
		t.Fatalf("UpdateTakenCollection with no copy err = %v, want ErrCollectionNotFound", err)
	}
	if _, err := f.db.UpdateTakenCatalog(ctx, other, f.listed.ID, allowAnyCatalogParams); !errors.Is(err, ErrCatalogNotFound) {
		t.Fatalf("UpdateTakenCatalog of a private catalog err = %v, want ErrCatalogNotFound", err)
	}
	source, err := f.db.CreateUserCatalog(ctx, f.owner, publicCatalogForm("Public"))
	if err != nil {
		t.Fatalf("create catalog: %v", err)
	}
	if _, err := f.db.UpdateTakenCatalog(ctx, other, source.ID, allowAnyCatalogParams); !errors.Is(err, ErrCatalogNotFound) {
		t.Fatalf("UpdateTakenCatalog with no copy err = %v, want ErrCatalogNotFound", err)
	}
}

// Taking a collection doesn't mark its owner's listed catalogs taken: the
// scoped copies inside it carry taken_from, but only a listed copy is a
// catalog's link.
func TestTakingACollectionLeavesItsCatalogsUntaken(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner := newTestProfile(t, db, "owner")
	taker := newTestProfile(t, db, "taker")

	catalog, err := db.CreateUserCatalog(ctx, owner, publicCatalogForm("Public and in the collection"))
	if err != nil {
		t.Fatalf("create catalog: %v", err)
	}
	source, err := db.CreateUserCollection(ctx, owner, CollectionForm{
		Title: "Source", IsPublic: true, Folders: []FolderData{{Title: "Folder", Catalogs: CatalogRefs(catalog.ID)}},
	})
	if err != nil {
		t.Fatalf("create collection: %v", err)
	}
	if _, err := db.TakeCollection(ctx, taker, source.ID, allowAnyCatalogParams); err != nil {
		t.Fatalf("TakeCollection: %v", err)
	}
	if row := communityCatalogRow(t, db, taker, catalog.ID); row.Taken {
		t.Fatalf("community catalog row = %+v, want it not taken", row)
	}
}

// A second Take of one source is ErrConflict, for a catalog and a
// collection.
func TestSecondTakeConflicts(t *testing.T) {
	ctx := context.Background()
	f := newLinkFixture(t)

	if _, err := f.db.TakeCollection(ctx, f.taker, f.source.ID, allowAnyCatalogParams); !errors.Is(err, ErrConflict) {
		t.Fatalf("second TakeCollection err = %v, want ErrConflict", err)
	}
	source, err := f.db.CreateUserCatalog(ctx, f.owner, publicCatalogForm("Source"))
	if err != nil {
		t.Fatalf("create catalog: %v", err)
	}
	if _, err := f.db.TakeCatalog(ctx, f.taker, source.ID, allowAnyCatalogParams); err != nil {
		t.Fatalf("TakeCatalog: %v", err)
	}
	if _, err := f.db.TakeCatalog(ctx, f.taker, source.ID, allowAnyCatalogParams); !errors.Is(err, ErrConflict) {
		t.Fatalf("second TakeCatalog err = %v, want ErrConflict", err)
	}
}

// A Community Duplicate is never linked — nor are the catalogs copied inside
// a duplicated collection — keeps the original's title, and sits beside a
// Take and further Duplicates.
func TestCommunityDuplicateIsNotLinked(t *testing.T) {
	ctx := context.Background()
	f := newLinkFixture(t)

	for range 2 {
		dup, err := f.db.DuplicateCommunityCollection(ctx, f.taker, f.source.ID, allowAnyCatalogParams)
		if err != nil {
			t.Fatalf("DuplicateCommunityCollection: %v", err)
		}
		if dup.Linked || dup.TakenFrom != nil || dup.Title != f.source.Title {
			t.Fatalf("duplicate = linked %v, taken_from %v, title %q, want unlinked with the original title", dup.Linked, dup.TakenFrom, dup.Title)
		}
		for _, c := range dup.Catalogs {
			if c.TakenFrom != nil {
				t.Fatalf("duplicated catalog %s taken_from = %v, want nil", c.ID, c.TakenFrom)
			}
		}
	}
	f.requireCommunityCollection(t, true, false)

	source, err := f.db.CreateUserCatalog(ctx, f.owner, publicCatalogForm("Source"))
	if err != nil {
		t.Fatalf("create catalog: %v", err)
	}
	for range 2 {
		dup, err := f.db.DuplicateCommunityCatalog(ctx, f.taker, source.ID, allowAnyCatalogParams)
		if err != nil {
			t.Fatalf("DuplicateCommunityCatalog: %v", err)
		}
		if dup.Linked || dup.TakenFrom != nil {
			t.Fatalf("duplicate catalog = %+v, want unlinked", dup)
		}
	}
	if row := communityCatalogRow(t, f.db, f.taker, source.ID); row.Taken {
		t.Fatalf("community row after duplicating = %+v, want not taken", row)
	}
	if _, err := f.db.TakeCatalog(ctx, f.taker, source.ID, allowAnyCatalogParams); err != nil {
		t.Fatalf("Take beside duplicates: %v", err)
	}
}
