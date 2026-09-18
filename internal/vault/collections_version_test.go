package vault

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

// A freshly created collection starts at version 1, never pushed.
func TestCreateUserCollectionStartsAtVersionOneUnpushed(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner := newTestProfile(t, db, "owner")

	c, err := db.CreateUserCollection(ctx, owner, CollectionForm{Title: "Fresh"})
	if err != nil {
		t.Fatalf("CreateUserCollection: %v", err)
	}
	if c.Version != 1 || c.PushedVersion != nil {
		t.Fatalf("new collection version=%d pushed_version=%v, want 1, nil", c.Version, c.PushedVersion)
	}
}

// Every content write through UpdateUserCollection bumps version by one.
func TestUpdateUserCollectionIncrementsVersion(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner := newTestProfile(t, db, "owner")

	c, err := db.CreateUserCollection(ctx, owner, CollectionForm{Title: "V1"})
	if err != nil {
		t.Fatalf("CreateUserCollection: %v", err)
	}
	updated, err := db.UpdateUserCollection(ctx, owner, c.ID, CollectionForm{Title: "V2"})
	if err != nil {
		t.Fatalf("UpdateUserCollection: %v", err)
	}
	if updated.Version != 2 {
		t.Fatalf("version after one update = %d, want 2", updated.Version)
	}
	again, err := db.UpdateUserCollection(ctx, owner, c.ID, CollectionForm{Title: "V3"})
	if err != nil {
		t.Fatalf("UpdateUserCollection: %v", err)
	}
	if again.Version != 3 {
		t.Fatalf("version after two updates = %d, want 3", again.Version)
	}
}

// SaveSelectionsForPush stamps pushed_version with the version the caller
// read at push time — never the row's current version. A Save landing
// between push's read and this write (even within the same second; there is
// no clock involved to make that a special case any more) must leave the
// collection looking pending rather than falsely cleared.
func TestSaveSelectionsForPushStampsReadVersionNotCurrent(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner := newTestProfile(t, db, "owner")

	c, err := db.CreateUserCollection(ctx, owner, CollectionForm{Title: "C"})
	if err != nil {
		t.Fatalf("CreateUserCollection: %v", err)
	}

	// Simulate push having read version 1, then a Save landing before the
	// local commit that follows — the row's current version is now 2.
	if _, err := db.UpdateUserCollection(ctx, owner, c.ID, CollectionForm{Title: "C2"}); err != nil {
		t.Fatalf("UpdateUserCollection: %v", err)
	}

	if err := db.SaveSelectionsForPush(ctx, owner,
		CatalogSelectionForm{},
		CollectionSelectionForm{CollectionIDs: []uuid.UUID{c.ID}},
		map[uuid.UUID]int{c.ID: 1},
	); err != nil {
		t.Fatalf("SaveSelectionsForPush: %v", err)
	}

	all, err := db.GetCollectionsByIDs(ctx, []uuid.UUID{c.ID})
	if err != nil {
		t.Fatalf("GetCollectionsByIDs: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("GetCollectionsByIDs returned %d rows, want 1", len(all))
	}
	if all[0].PushedVersion == nil || *all[0].PushedVersion != 1 {
		t.Fatalf("pushed_version = %v, want 1 (the version push read, not the current version 2)", all[0].PushedVersion)
	}
	if all[0].Version != 2 {
		t.Fatalf("version = %d, want 2 (untouched by push)", all[0].Version)
	}
}

// A push with no intervening Save stamps pushed_version equal to the
// current version, clearing the pending signal.
func TestSaveSelectionsForPushClearsPendingWhenNoInterveningSave(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner := newTestProfile(t, db, "owner")

	c, err := db.CreateUserCollection(ctx, owner, CollectionForm{Title: "C"})
	if err != nil {
		t.Fatalf("CreateUserCollection: %v", err)
	}

	if err := db.SaveSelectionsForPush(ctx, owner,
		CatalogSelectionForm{},
		CollectionSelectionForm{CollectionIDs: []uuid.UUID{c.ID}},
		map[uuid.UUID]int{c.ID: c.Version},
	); err != nil {
		t.Fatalf("SaveSelectionsForPush: %v", err)
	}

	all, err := db.GetCollectionsByIDs(ctx, []uuid.UUID{c.ID})
	if err != nil {
		t.Fatalf("GetCollectionsByIDs: %v", err)
	}
	if len(all) != 1 || all[0].PushedVersion == nil || *all[0].PushedVersion != all[0].Version {
		t.Fatalf("collection = %+v, want pushed_version == version", all)
	}
}

// TakeCollection and DuplicateCollection both start their new row at
// version 1 with pushed_version nil, same as CreateUserCollection.
func TestTakeAndDuplicateCollectionStartAtVersionOne(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner := newTestProfile(t, db, "owner")
	taker := newTestProfile(t, db, "taker")

	source, err := db.CreateUserCollection(ctx, owner, CollectionForm{Title: "Source", IsPublic: true})
	if err != nil {
		t.Fatalf("CreateUserCollection: %v", err)
	}

	taken, err := db.TakeCollection(ctx, taker, source.ID)
	if err != nil {
		t.Fatalf("TakeCollection: %v", err)
	}
	if taken.Version != 1 || taken.PushedVersion != nil {
		t.Fatalf("taken collection version=%d pushed_version=%v, want 1, nil", taken.Version, taken.PushedVersion)
	}

	dup, err := db.DuplicateCollection(ctx, owner, source.ID)
	if err != nil {
		t.Fatalf("DuplicateCollection: %v", err)
	}
	if dup.Version != 1 || dup.PushedVersion != nil {
		t.Fatalf("duplicated collection version=%d pushed_version=%v, want 1, nil", dup.Version, dup.PushedVersion)
	}
}
