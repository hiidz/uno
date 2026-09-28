package vault

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"
)

// staleCatalogParams is a recipe naming a TMDB genre id that no longer
// resolves — the shape of a stored row that passed validation when it was
// written and would fail it now.
const staleCatalogParams = `{"with_genres":"999"}`

// allowAnyCatalogParams satisfies TakeCatalog's required validator for tests
// that are exercising the copy itself, not the recipe check. The real
// validator lives in internal/api, which this package cannot import.
func allowAnyCatalogParams(_, _, _ string) error { return nil }

func publicCatalogForm(name string) CatalogForm {
	form := listedCatalogForm(name)
	form.IsPublic = true
	return form
}

// GetCommunityCatalogs excludes the caller's own rows, even when public.
func TestGetCommunityCatalogsExcludesOwn(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)

	owner := newTestProfile(t, db, "owner")
	other := newTestProfile(t, db, "other")

	if _, err := db.CreateUserCatalog(ctx, owner, publicCatalogForm("Mine")); err != nil {
		t.Fatalf("create own public catalog: %v", err)
	}
	othersCatalog, err := db.CreateUserCatalog(ctx, other, publicCatalogForm("Theirs"))
	if err != nil {
		t.Fatalf("create other's public catalog: %v", err)
	}

	got, err := db.GetCommunityCatalogs(ctx, owner)
	if err != nil {
		t.Fatalf("GetCommunityCatalogs: %v", err)
	}
	if len(got) != 1 || got[0].ID != othersCatalog.ID {
		t.Fatalf("GetCommunityCatalogs = %+v, want exactly the other profile's catalog %s", got, othersCatalog.ID)
	}
}

// A catalog scoped to a collection can never be public (the schema's own
// CHECK forbids it), so it can never appear in the community list either —
// this proves GetCommunityCatalogs doesn't need its own scope filter.
func TestGetCommunityCatalogsExcludesScoped(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)

	owner := newTestProfile(t, db, "owner")
	other := newTestProfile(t, db, "other")
	collectionID := newTestCollection(t, db, other, "Other's Collection")

	scopedForm := listedCatalogForm("Scoped")
	scopedForm.CollectionID = &collectionID
	if _, err := db.CreateUserCatalog(ctx, other, scopedForm); err != nil {
		t.Fatalf("create scoped catalog: %v", err)
	}

	got, err := db.GetCommunityCatalogs(ctx, owner)
	if err != nil {
		t.Fatalf("GetCommunityCatalogs: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("GetCommunityCatalogs = %+v, want none — a scoped catalog can never be public", got)
	}
}

// Two public catalogs sharing a recipe collapse to one row: the oldest
// by created_at.
func TestGetCommunityCatalogsCollapsesEqualRecipes(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)

	owner := newTestProfile(t, db, "owner")
	first := newTestProfile(t, db, "first")
	second := newTestProfile(t, db, "second")

	older, err := db.CreateUserCatalog(ctx, first, publicCatalogForm("Older"))
	if err != nil {
		t.Fatalf("create older catalog: %v", err)
	}
	newer, err := db.CreateUserCatalog(ctx, second, publicCatalogForm("Newer"))
	if err != nil {
		t.Fatalf("create newer catalog: %v", err)
	}
	if older.RecipeHash != newer.RecipeHash {
		t.Fatalf("test setup: expected identical recipes to share a hash, got %q and %q", older.RecipeHash, newer.RecipeHash)
	}

	// Force a deterministic created_at order — both rows landed in the same
	// second in real time, which the DB's second-precision TEXT timestamps
	// can't tell apart on their own.
	if _, err := db.conn.ExecContext(ctx, `UPDATE catalogs SET created_at = '2020-01-01T00:00:00Z' WHERE id = ?`, older.ID.String()); err != nil {
		t.Fatalf("backdating older catalog: %v", err)
	}
	if _, err := db.conn.ExecContext(ctx, `UPDATE catalogs SET created_at = '2020-01-02T00:00:00Z' WHERE id = ?`, newer.ID.String()); err != nil {
		t.Fatalf("backdating newer catalog: %v", err)
	}

	got, err := db.GetCommunityCatalogs(ctx, owner)
	if err != nil {
		t.Fatalf("GetCommunityCatalogs: %v", err)
	}
	if len(got) != 1 || got[0].ID != older.ID {
		t.Fatalf("GetCommunityCatalogs = %+v, want exactly the older row %s", got, older.ID)
	}
}

// The recipe collapse's survivor tie-break is oldest created_at, then
// smallest id — deterministic even when two rows share a created_at second,
// not left to the query's own row order.
func TestGetCommunityCatalogsCollapseTieBreaksByID(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)

	owner := newTestProfile(t, db, "owner")
	first := newTestProfile(t, db, "first")
	second := newTestProfile(t, db, "second")

	a, err := db.CreateUserCatalog(ctx, first, publicCatalogForm("Same"))
	if err != nil {
		t.Fatalf("create catalog a: %v", err)
	}
	b, err := db.CreateUserCatalog(ctx, second, publicCatalogForm("Same"))
	if err != nil {
		t.Fatalf("create catalog b: %v", err)
	}
	if a.RecipeHash != b.RecipeHash {
		t.Fatalf("test setup: expected identical recipes to share a hash, got %q and %q", a.RecipeHash, b.RecipeHash)
	}

	// Force an identical created_at so the tie-break falls entirely to id.
	if _, err := db.conn.ExecContext(ctx, `UPDATE catalogs SET created_at = '2020-01-01T00:00:00Z' WHERE id IN (?, ?)`,
		a.ID.String(), b.ID.String()); err != nil {
		t.Fatalf("forcing identical created_at: %v", err)
	}

	want := a.ID
	if b.ID.String() < a.ID.String() {
		want = b.ID
	}

	got, err := db.GetCommunityCatalogs(ctx, owner)
	if err != nil {
		t.Fatalf("GetCommunityCatalogs: %v", err)
	}
	if len(got) != 1 || got[0].ID != want {
		t.Fatalf("GetCommunityCatalogs = %+v, want exactly the smaller id %s", got, want)
	}
}

// A collapsed row shows the member the caller is linked to, not the oldest:
// taking the newer duplicate shows that one, flagged Taken, since it is the
// row the caller's Update follows.
func TestGetCommunityCatalogsTakenAppliesToWholeRecipeGroup(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)

	first := newTestProfile(t, db, "first")
	second := newTestProfile(t, db, "second")
	taker := newTestProfile(t, db, "taker")

	older, err := db.CreateUserCatalog(ctx, first, publicCatalogForm("Older"))
	if err != nil {
		t.Fatalf("create older catalog: %v", err)
	}
	newer, err := db.CreateUserCatalog(ctx, second, publicCatalogForm("Newer"))
	if err != nil {
		t.Fatalf("create newer catalog: %v", err)
	}
	if _, err := db.conn.ExecContext(ctx, `UPDATE catalogs SET created_at = '2020-01-01T00:00:00Z' WHERE id = ?`, older.ID.String()); err != nil {
		t.Fatalf("backdating older catalog: %v", err)
	}
	if _, err := db.conn.ExecContext(ctx, `UPDATE catalogs SET created_at = '2020-01-02T00:00:00Z' WHERE id = ?`, newer.ID.String()); err != nil {
		t.Fatalf("backdating newer catalog: %v", err)
	}

	// The taker takes the *newer* duplicate specifically, not the survivor.
	if _, err := db.TakeCatalog(ctx, taker, newer.ID, allowAnyCatalogParams); err != nil {
		t.Fatalf("TakeCatalog: %v", err)
	}

	got, err := db.GetCommunityCatalogs(ctx, taker)
	if err != nil {
		t.Fatalf("GetCommunityCatalogs: %v", err)
	}
	if len(got) != 1 || got[0].ID != newer.ID || !got[0].Taken || got[0].UpdateAvailable {
		t.Fatalf("GetCommunityCatalogs = %+v, want the linked row %s flagged Taken=true, UpdateAvailable=false", got, newer.ID)
	}
}

// TestGetCommunityCatalogsTakenFlag documents the full taken lifecycle: false
// before a take, true immediately after, and false again once the copy is
// deleted.
func TestGetCommunityCatalogsTakenFlag(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)

	owner := newTestProfile(t, db, "owner")
	other := newTestProfile(t, db, "other")

	source, err := db.CreateUserCatalog(ctx, other, publicCatalogForm("Theirs"))
	if err != nil {
		t.Fatalf("create source catalog: %v", err)
	}

	before, err := db.GetCommunityCatalogs(ctx, owner)
	if err != nil {
		t.Fatalf("GetCommunityCatalogs (before): %v", err)
	}
	if len(before) != 1 || before[0].Taken {
		t.Fatalf("GetCommunityCatalogs before take = %+v, want Taken=false", before)
	}

	takenCopy, err := db.TakeCatalog(ctx, owner, source.ID, allowAnyCatalogParams)
	if err != nil {
		t.Fatalf("TakeCatalog: %v", err)
	}

	after, err := db.GetCommunityCatalogs(ctx, owner)
	if err != nil {
		t.Fatalf("GetCommunityCatalogs (after): %v", err)
	}
	if len(after) != 1 || !after[0].Taken {
		t.Fatalf("GetCommunityCatalogs after take = %+v, want Taken=true", after)
	}

	if err := db.DeleteUserCatalog(ctx, owner, takenCopy.ID); err != nil {
		t.Fatalf("deleting taken copy: %v", err)
	}

	cleared, err := db.GetCommunityCatalogs(ctx, owner)
	if err != nil {
		t.Fatalf("GetCommunityCatalogs (cleared): %v", err)
	}
	if len(cleared) != 1 || cleared[0].Taken {
		t.Fatalf("GetCommunityCatalogs after deleting the copy = %+v, want Taken=false", cleared)
	}
}

// TakeCatalog copies a public source into a new, private, listed catalog
// owned by the taker, with fresh ids and taken_from set.
func TestTakeCatalog(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)

	owner := newTestProfile(t, db, "owner")
	taker := newTestProfile(t, db, "taker")

	source, err := db.CreateUserCatalog(ctx, owner, publicCatalogForm("Source"))
	if err != nil {
		t.Fatalf("create source catalog: %v", err)
	}

	takenCopy, err := db.TakeCatalog(ctx, taker, source.ID, allowAnyCatalogParams)
	if err != nil {
		t.Fatalf("TakeCatalog: %v", err)
	}

	if takenCopy.ID == source.ID {
		t.Fatalf("taken copy reused the source's id")
	}
	if takenCopy.OwnerID != taker {
		t.Fatalf("taken copy owner = %s, want %s", takenCopy.OwnerID, taker)
	}
	if takenCopy.IsPublic {
		t.Fatalf("taken copy is_public = true, want false")
	}
	if takenCopy.CollectionID != nil {
		t.Fatalf("taken copy collection_id = %v, want nil (listed)", takenCopy.CollectionID)
	}
	if takenCopy.TakenFrom == nil || *takenCopy.TakenFrom != source.ID {
		t.Fatalf("taken copy taken_from = %v, want %s", takenCopy.TakenFrom, source.ID)
	}
	if takenCopy.RecipeHash != source.RecipeHash {
		t.Fatalf("taken copy recipe hash = %q, want %q", takenCopy.RecipeHash, source.RecipeHash)
	}
}

// Taking a private or already-owned catalog is a 404-flavored
// ErrCatalogNotFound, not a 400 — the caller shouldn't be told whether the id
// exists at all.
func TestTakeCatalogNotFound(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)

	owner := newTestProfile(t, db, "owner")
	taker := newTestProfile(t, db, "taker")

	private, err := db.CreateUserCatalog(ctx, owner, listedCatalogForm("Private"))
	if err != nil {
		t.Fatalf("create private catalog: %v", err)
	}
	if _, err := db.TakeCatalog(ctx, taker, private.ID, allowAnyCatalogParams); !errors.Is(err, ErrCatalogNotFound) {
		t.Fatalf("take private catalog: got %v, want ErrCatalogNotFound", err)
	}

	own, err := db.CreateUserCatalog(ctx, taker, publicCatalogForm("Own"))
	if err != nil {
		t.Fatalf("create taker's own public catalog: %v", err)
	}
	if _, err := db.TakeCatalog(ctx, taker, own.ID, allowAnyCatalogParams); !errors.Is(err, ErrCatalogNotFound) {
		t.Fatalf("take own catalog: got %v, want ErrCatalogNotFound", err)
	}
}

// The taken copy survives the source's deletion — taken_from is nulled by
// ON DELETE SET NULL, the one write the closed graph makes across the owner
// boundary.
func TestTakeCatalogSurvivesSourceDeletion(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)

	owner := newTestProfile(t, db, "owner")
	taker := newTestProfile(t, db, "taker")

	source, err := db.CreateUserCatalog(ctx, owner, publicCatalogForm("Source"))
	if err != nil {
		t.Fatalf("create source catalog: %v", err)
	}
	takenCopy, err := db.TakeCatalog(ctx, taker, source.ID, allowAnyCatalogParams)
	if err != nil {
		t.Fatalf("TakeCatalog: %v", err)
	}

	if err := db.DeleteUserCatalog(ctx, owner, source.ID); err != nil {
		t.Fatalf("deleting source catalog: %v", err)
	}

	remaining, err := db.queryCatalogs(ctx, "c.id = ?", takenCopy.ID.String())
	if err != nil {
		t.Fatalf("querying for taken copy: %v", err)
	}
	if len(remaining) != 1 {
		t.Fatalf("taken copy %s did not survive its source's deletion", takenCopy.ID)
	}
	if remaining[0].TakenFrom != nil {
		t.Fatalf("taken copy's taken_from = %v after source deletion, want nil", remaining[0].TakenFrom)
	}
}

// GetCommunityCollections excludes the caller's own rows, even when public,
// and applies no recipe collapse.
func TestGetCommunityCollectionsExcludesOwn(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)

	owner := newTestProfile(t, db, "owner")
	other := newTestProfile(t, db, "other")

	if _, err := db.CreateUserCollection(ctx, owner, CollectionForm{Title: "Mine", IsPublic: true}); err != nil {
		t.Fatalf("create own public collection: %v", err)
	}
	othersCollection, err := db.CreateUserCollection(ctx, other, CollectionForm{Title: "Theirs", IsPublic: true})
	if err != nil {
		t.Fatalf("create other's public collection: %v", err)
	}

	got, err := db.GetCommunityCollections(ctx, owner)
	if err != nil {
		t.Fatalf("GetCommunityCollections: %v", err)
	}
	if len(got) != 1 || got[0].ID != othersCollection.ID {
		t.Fatalf("GetCommunityCollections = %+v, want exactly the other profile's collection %s", got, othersCollection.ID)
	}
}

// TakeCollection copies the source's cosmetics, folders (in order), and
// every distinct catalog its folders reference — a catalog used by two
// folders becomes one scoped copy referenced twice.
func TestTakeCollection(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)

	owner := newTestProfile(t, db, "owner")
	taker := newTestProfile(t, db, "taker")

	shared, err := db.CreateUserCatalog(ctx, owner, listedCatalogForm("Shared"))
	if err != nil {
		t.Fatalf("create shared catalog: %v", err)
	}
	only1, err := db.CreateUserCatalog(ctx, owner, listedCatalogForm("Only in folder 1"))
	if err != nil {
		t.Fatalf("create folder-1-only catalog: %v", err)
	}

	source, err := db.CreateUserCollection(ctx, owner, CollectionForm{
		Title:    "Source",
		IsPublic: true,
		Folders: []FolderData{
			{Title: "Folder 1", Catalogs: CatalogRefs(shared.ID, only1.ID)},
			{Title: "Folder 2", Catalogs: CatalogRefs(shared.ID)},
		},
	})
	if err != nil {
		t.Fatalf("create source collection: %v", err)
	}

	taken, err := db.TakeCollection(ctx, taker, source.ID, allowAnyCatalogParams)
	if err != nil {
		t.Fatalf("TakeCollection: %v", err)
	}

	if taken.ID == source.ID {
		t.Fatalf("taken collection reused the source's id")
	}
	if taken.OwnerID != taker {
		t.Fatalf("taken collection owner = %s, want %s", taken.OwnerID, taker)
	}
	if taken.IsPublic {
		t.Fatalf("taken collection is_public = true, want false")
	}
	if taken.TakenFrom == nil || *taken.TakenFrom != source.ID {
		t.Fatalf("taken collection taken_from = %v, want %s", taken.TakenFrom, source.ID)
	}
	if len(taken.Folders) != 2 {
		t.Fatalf("taken collection has %d folders, want 2", len(taken.Folders))
	}
	if taken.Folders[0].Title != "Folder 1" || taken.Folders[1].Title != "Folder 2" {
		t.Fatalf("taken collection folders = %+v, want Folder 1 then Folder 2", taken.Folders)
	}

	// Exactly one scoped copy for the shared catalog, referenced by both folders.
	if len(taken.Folders[0].CatalogIDs()) != 2 || len(taken.Folders[1].CatalogIDs()) != 1 {
		t.Fatalf("taken collection folder catalog counts = %d, %d, want 2, 1",
			len(taken.Folders[0].CatalogIDs()), len(taken.Folders[1].CatalogIDs()))
	}
	sharedCopyID := taken.Folders[1].CatalogIDs()[0]
	if taken.Folders[0].CatalogIDs()[0] != sharedCopyID {
		t.Fatalf("folder 1's first catalog id %s != folder 2's catalog id %s, want the same scoped copy", taken.Folders[0].CatalogIDs()[0], sharedCopyID)
	}
	if len(taken.Catalogs) != 2 {
		t.Fatalf("taken collection has %d distinct catalogs, want 2 (one scoped copy per distinct source catalog)", len(taken.Catalogs))
	}

	for _, c := range taken.Catalogs {
		if c.OwnerID != taker {
			t.Fatalf("taken scoped catalog %s owner = %s, want %s", c.ID, c.OwnerID, taker)
		}
		if c.CollectionID == nil || *c.CollectionID != taken.ID {
			t.Fatalf("taken scoped catalog %s collection_id = %v, want %s", c.ID, c.CollectionID, taken.ID)
		}
		if c.TakenFrom == nil {
			t.Fatalf("taken scoped catalog %s has no taken_from", c.ID)
		}
	}
}

// Taking a private or already-owned collection is ErrCollectionNotFound.
func TestTakeCollectionNotFound(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)

	owner := newTestProfile(t, db, "owner")
	taker := newTestProfile(t, db, "taker")

	private, err := db.CreateUserCollection(ctx, owner, CollectionForm{Title: "Private"})
	if err != nil {
		t.Fatalf("create private collection: %v", err)
	}
	if _, err := db.TakeCollection(ctx, taker, private.ID, allowAnyCatalogParams); !errors.Is(err, ErrCollectionNotFound) {
		t.Fatalf("take private collection: got %v, want ErrCollectionNotFound", err)
	}

	own, err := db.CreateUserCollection(ctx, taker, CollectionForm{Title: "Own", IsPublic: true})
	if err != nil {
		t.Fatalf("create taker's own public collection: %v", err)
	}
	if _, err := db.TakeCollection(ctx, taker, own.ID, allowAnyCatalogParams); !errors.Is(err, ErrCollectionNotFound) {
		t.Fatalf("take own collection: got %v, want ErrCollectionNotFound", err)
	}
}

// The taken collection copy survives the source's deletion — TakeCollection
// makes its own fully independent rows, so a cascade on the source's folders
// and scoped catalogs can't reach the takenCopy.
func TestTakeCollectionSurvivesSourceDeletion(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)

	owner := newTestProfile(t, db, "owner")
	taker := newTestProfile(t, db, "taker")

	catalog, err := db.CreateUserCatalog(ctx, owner, listedCatalogForm("Catalog"))
	if err != nil {
		t.Fatalf("create catalog: %v", err)
	}
	source, err := db.CreateUserCollection(ctx, owner, CollectionForm{
		Title:    "Source",
		IsPublic: true,
		Folders:  []FolderData{{Title: "Folder", Catalogs: CatalogRefs(catalog.ID)}},
	})
	if err != nil {
		t.Fatalf("create source collection: %v", err)
	}

	taken, err := db.TakeCollection(ctx, taker, source.ID, allowAnyCatalogParams)
	if err != nil {
		t.Fatalf("TakeCollection: %v", err)
	}

	if err := db.DeleteUserCollection(ctx, owner, source.ID); err != nil {
		t.Fatalf("deleting source collection: %v", err)
	}

	all, err := db.GetUserCollections(ctx, taker)
	if err != nil {
		t.Fatalf("GetUserCollections: %v", err)
	}
	if len(all) != 1 || all[0].ID != taken.ID {
		t.Fatalf("taken collection %s did not survive its source's deletion: %+v", taken.ID, all)
	}
	if len(all[0].Catalogs) != 1 {
		t.Fatalf("taken collection's scoped catalog did not survive: %+v", all[0].Catalogs)
	}
}

// TakeCollection re-validates every catalog recipe it would copy, the way
// TakeCatalog re-validates the single recipe it copies. One rejected recipe
// fails the whole take and writes nothing.
func TestTakeCollectionValidatesSourceCatalogParams(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)

	owner := newTestProfile(t, db, "owner")
	taker := newTestProfile(t, db, "taker")

	good, err := db.CreateUserCatalog(ctx, owner, listedCatalogForm("Good"))
	if err != nil {
		t.Fatalf("create good catalog: %v", err)
	}
	staleForm := listedCatalogForm("Stale")
	staleForm.Params = staleCatalogParams
	stale, err := db.CreateUserCatalog(ctx, owner, staleForm)
	if err != nil {
		t.Fatalf("create stale catalog: %v", err)
	}

	source, err := db.CreateUserCollection(ctx, owner, CollectionForm{
		Title:    "Source",
		IsPublic: true,
		Folders: []FolderData{
			{Title: "Folder 1", Catalogs: CatalogRefs(good.ID, stale.ID)},
		},
	})
	if err != nil {
		t.Fatalf("create source collection: %v", err)
	}

	// Stands in for api.validateCatalogParams rejecting a recipe whose TMDB
	// vocabulary no longer resolves — the case a stored row can reach
	// without ever having been re-checked.
	rejectStale := func(_, _, params string) error {
		if params == staleCatalogParams {
			return fmt.Errorf("%w: with_genres 999 is not a genre", ErrInvalidInput)
		}
		return nil
	}

	if _, err := db.TakeCollection(ctx, taker, source.ID, rejectStale); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("TakeCollection with a rejected recipe = %v, want ErrInvalidInput", err)
	}

	collections, err := db.GetUserCollections(ctx, taker)
	if err != nil {
		t.Fatalf("GetUserCollections: %v", err)
	}
	if len(collections) != 0 {
		t.Fatalf("taker has %d collections after a failed take, want 0", len(collections))
	}
}

// A nil validator is a programming error, not a way to skip the check.
func TestTakeCollectionRequiresValidator(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)

	taker := newTestProfile(t, db, "taker")

	if _, err := db.TakeCollection(ctx, taker, uuid.New(), nil); err == nil {
		t.Fatal("TakeCollection with a nil validator succeeded, want an error")
	}
}

// A folder ref whose catalog vanished between the two reads a copy makes is
// dropped, not carried through as a dangling foreign key that would fail the
// whole copy.
func TestCopyCollectionDropsDanglingFolderRefs(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)

	owner := newTestProfile(t, db, "owner")
	taker := newTestProfile(t, db, "taker")

	kept, err := db.CreateUserCatalog(ctx, owner, listedCatalogForm("Kept"))
	if err != nil {
		t.Fatalf("create kept catalog: %v", err)
	}
	doomed, err := db.CreateUserCatalog(ctx, owner, listedCatalogForm("Doomed"))
	if err != nil {
		t.Fatalf("create doomed catalog: %v", err)
	}

	source, err := db.CreateUserCollection(ctx, owner, CollectionForm{
		Title:    "Source",
		IsPublic: true,
		Folders: []FolderData{
			{Title: "Folder 1", Catalogs: CatalogRefs(kept.ID, doomed.ID)},
		},
	})
	if err != nil {
		t.Fatalf("create source collection: %v", err)
	}

	// Stand in for a concurrent delete: drop the catalog row but leave the
	// folder ref behind, which is the disagreement the two reads can see.
	if _, err := db.conn.ExecContext(ctx, `PRAGMA foreign_keys = off`); err != nil {
		t.Fatalf("disabling foreign keys: %v", err)
	}
	if _, err := db.conn.ExecContext(ctx, `DELETE FROM catalogs WHERE id = ?`, doomed.ID.String()); err != nil {
		t.Fatalf("deleting doomed catalog: %v", err)
	}
	if _, err := db.conn.ExecContext(ctx, `PRAGMA foreign_keys = on`); err != nil {
		t.Fatalf("re-enabling foreign keys: %v", err)
	}

	taken, err := db.TakeCollection(ctx, taker, source.ID, allowAnyCatalogParams)
	if err != nil {
		t.Fatalf("TakeCollection with a dangling ref: %v", err)
	}
	if len(taken.Folders) != 1 {
		t.Fatalf("taken collection has %d folders, want 1", len(taken.Folders))
	}
	if got := taken.Folders[0].CatalogIDs(); len(got) != 1 {
		t.Fatalf("taken folder has %d catalog refs, want 1 (the dangling one dropped)", len(got))
	}
	if len(taken.Catalogs) != 1 || taken.Catalogs[0].Name != "Kept" {
		t.Fatalf("taken collection catalogs = %+v, want just the surviving %q", taken.Catalogs, "Kept")
	}
}

// deleteCatalogLeavingRefs deletes catalogID with foreign keys off, so its
// folder refs survive it: the tree a concurrent delete can leave between the
// two reads that assemble it.
func deleteCatalogLeavingRefs(t *testing.T, db *DB, catalogID uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	if _, err := db.conn.ExecContext(ctx, `PRAGMA foreign_keys = off`); err != nil {
		t.Fatalf("disabling foreign keys: %v", err)
	}
	if _, err := db.conn.ExecContext(ctx, `DELETE FROM catalogs WHERE id = ?`, catalogID.String()); err != nil {
		t.Fatalf("deleting catalog: %v", err)
	}
	if _, err := db.conn.ExecContext(ctx, `PRAGMA foreign_keys = on`); err != nil {
		t.Fatalf("re-enabling foreign keys: %v", err)
	}
}

// newLinkSourceCollection creates a public collection owned by owner holding
// every shape the link hash must see through unchanged: a listed catalog and
// a scoped one, one catalog under two genres in a folder, one catalog in two
// folders, and a ref whose catalog is gone. Returns the collection's id.
func newLinkSourceCollection(t *testing.T, db *DB, owner uuid.UUID) uuid.UUID {
	t.Helper()
	ctx := context.Background()

	listedForm := listedCatalogForm("Listed")
	listed, err := db.CreateUserCatalog(ctx, owner, listedForm)
	if err != nil {
		t.Fatalf("create listed catalog: %v", err)
	}
	doomed, err := db.CreateUserCatalog(ctx, owner, listedCatalogForm("Doomed"))
	if err != nil {
		t.Fatalf("create doomed catalog: %v", err)
	}
	scoped := &NewScopedCatalog{
		Key: "scoped", Type: "movie", Name: "Scoped", Provider: "tmdb",
		Params: `{"sort_by":"vote_average.desc"}`,
	}

	source, err := db.CreateUserCollection(ctx, owner, CollectionForm{
		Title:    "Source",
		IsPublic: true,
		ViewMode: "TABBED_GRID",
		Folders: []FolderData{
			{Title: "Folder 1", Catalogs: []FolderCatalogRef{
				{CatalogID: &listed.ID},
				{CatalogID: &listed.ID, Genre: "Horror"},
				{New: scoped},
			}},
			{Title: "Folder 2", Catalogs: []FolderCatalogRef{
				{New: scoped},
				{CatalogID: &listed.ID},
				{CatalogID: &doomed.ID},
			}},
		},
	})
	if err != nil {
		t.Fatalf("create source collection: %v", err)
	}
	deleteCatalogLeavingRefs(t, db, doomed.ID)
	return source.ID
}

// communityCollectionRow returns the row for sourceID in profileID's
// community collection list.
func communityCollectionRow(t *testing.T, db *DB, profileID, sourceID uuid.UUID) CommunityCollection {
	t.Helper()
	rows, err := db.GetCommunityCollections(context.Background(), profileID)
	if err != nil {
		t.Fatalf("GetCommunityCollections: %v", err)
	}
	for _, row := range rows {
		if row.ID == sourceID {
			return row
		}
	}
	t.Fatalf("GetCommunityCollections = %+v, want a row for %s", rows, sourceID)
	return CommunityCollection{}
}

// communityCatalogRow returns the row for sourceID in profileID's community
// catalog list.
func communityCatalogRow(t *testing.T, db *DB, profileID, sourceID uuid.UUID) CommunityCatalog {
	t.Helper()
	rows, err := db.GetCommunityCatalogs(context.Background(), profileID)
	if err != nil {
		t.Fatalf("GetCommunityCatalogs: %v", err)
	}
	for _, row := range rows {
		if row.ID == sourceID {
			return row
		}
	}
	t.Fatalf("GetCommunityCatalogs = %+v, want a row for %s", rows, sourceID)
	return CommunityCatalog{}
}

// requireSameTree fails unless got is want as stored: the same collection
// id, version and updated_at, the same folder ids in order, and the same refs.
func requireSameTree(t *testing.T, got, want CollectionWithFolders) {
	t.Helper()
	if got.ID != want.ID || got.Version != want.Version || !got.UpdatedAt.Equal(want.UpdatedAt) {
		t.Fatalf("collection = %s v%d at %s, want %s v%d at %s",
			got.ID, got.Version, got.UpdatedAt, want.ID, want.Version, want.UpdatedAt)
	}
	if len(got.Folders) != len(want.Folders) {
		t.Fatalf("collection has %d folders, want %d", len(got.Folders), len(want.Folders))
	}
	for i := range want.Folders {
		if got.Folders[i].ID != want.Folders[i].ID {
			t.Fatalf("folder %d id = %s, want %s", i, got.Folders[i].ID, want.Folders[i].ID)
		}
		if fmt.Sprint(got.Folders[i].Refs) != fmt.Sprint(want.Folders[i].Refs) {
			t.Fatalf("folder %d refs = %v, want %v", i, got.Folders[i].Refs, want.Folders[i].Refs)
		}
	}
}

// Straight after a Take, the copy matches its original: Community shows it
// taken with no update, and Update changes nothing. The source holds every
// shape the hash has to see through, so a hash that isn't stable across the
// copy fails here first.
func TestTakenCollectionIsCurrent(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)

	owner := newTestProfile(t, db, "owner")
	taker := newTestProfile(t, db, "taker")
	sourceID := newLinkSourceCollection(t, db, owner)

	taken, err := db.TakeCollection(ctx, taker, sourceID, allowAnyCatalogParams)
	if err != nil {
		t.Fatalf("TakeCollection: %v", err)
	}

	row := communityCollectionRow(t, db, taker, sourceID)
	if !row.Taken || row.UpdateAvailable {
		t.Fatalf("community row taken = %v, update_available = %v, want true, false", row.Taken, row.UpdateAvailable)
	}

	updated, err := db.UpdateTakenCollection(ctx, taker, sourceID, allowAnyCatalogParams)
	if err != nil {
		t.Fatalf("UpdateTakenCollection: %v", err)
	}
	requireSameTree(t, updated, taken)
}

// Straight after a Take, a catalog copy matches its original: Community shows
// it taken with no update, and Update changes nothing.
func TestTakenCatalogIsCurrent(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)

	owner := newTestProfile(t, db, "owner")
	taker := newTestProfile(t, db, "taker")

	form := publicCatalogForm("Source")
	source, err := db.CreateUserCatalog(ctx, owner, form)
	if err != nil {
		t.Fatalf("create source catalog: %v", err)
	}
	if _, err := db.TakeCatalog(ctx, taker, source.ID, allowAnyCatalogParams); err != nil {
		t.Fatalf("TakeCatalog: %v", err)
	}
	stored, err := db.GetUserCatalogs(ctx, taker)
	if err != nil || len(stored) != 1 {
		t.Fatalf("GetUserCatalogs = %+v, %v, want the one taken copy", stored, err)
	}

	row := communityCatalogRow(t, db, taker, source.ID)
	if !row.Taken || row.UpdateAvailable {
		t.Fatalf("community row taken = %v, update_available = %v, want true, false", row.Taken, row.UpdateAvailable)
	}

	updated, err := db.UpdateTakenCatalog(ctx, taker, source.ID, allowAnyCatalogParams)
	if err != nil {
		t.Fatalf("UpdateTakenCatalog: %v", err)
	}
	if updated.ID != stored[0].ID || updated.Name != stored[0].Name || !updated.UpdatedAt.Equal(stored[0].UpdatedAt) {
		t.Fatalf("UpdateTakenCatalog = %+v, want the stored copy %+v unchanged", updated, stored[0])
	}
}
