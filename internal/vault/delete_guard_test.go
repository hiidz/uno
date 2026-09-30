package vault

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

// pushCatalogs stands in for a push of catalogs alone: it puts entries on
// Home, in order, and takes every other catalog and every collection off.
func pushCatalogs(t *testing.T, db *DB, profileID uuid.UUID, entries ...SelectedCatalogInput) {
	t.Helper()
	if err := db.SaveSelectionsForPush(context.Background(), profileID,
		CatalogSelectionForm{Catalogs: entries}, CollectionSelectionForm{}, nil); err != nil {
		t.Fatalf("SaveSelectionsForPush: %v", err)
	}
}

// usingCollection creates profileID's collection title, with one folder
// using catalogID.
func usingCollection(t *testing.T, db *DB, profileID uuid.UUID, title string, catalogID uuid.UUID) uuid.UUID {
	t.Helper()
	c, err := db.CreateUserCollection(context.Background(), profileID, CollectionForm{
		Title: title, Folders: []FolderData{{Title: "F", Catalogs: CatalogRefs(catalogID)}},
	})
	if err != nil {
		t.Fatalf("CreateUserCollection: %v", err)
	}
	return c.ID
}

// A refusal is an ErrConflict, and its message is its reason alone, the
// sentence the SPA shows.
func TestConflictReasonIsAnErrConflict(t *testing.T) {
	for _, err := range []error{errTakeOffHome, errPushFirst, conflictReason("Remove it from “X” and push first.")} {
		if !errors.Is(err, ErrConflict) {
			t.Errorf("%q is not an ErrConflict", err)
		}
	}
	if got := errTakeOffHome.Error(); got != "Take it off Home and push first." {
		t.Errorf("message = %q, want the sentence alone", got)
	}
}

// A listed catalog Nuvio may still hold can't be deleted: one with its own
// Home row, Discover-only included; one a collection on Home uses, naming
// the first in Home order; and any while a collection on Home needs a push.
// Once nothing on Home holds it, it deletes.
func TestDeleteUserCatalogRefusesWhatNuvioMayHold(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T, db *DB, owner, doomed uuid.UUID)
		want  string // the refusal; "" when the delete goes through
	}{
		{"its own Home row", func(t *testing.T, db *DB, owner, doomed uuid.UUID) {
			pushCatalogs(t, db, owner, SelectedCatalogInput{CatalogID: doomed, ShowInHome: true})
		}, "Take it off Home and push first."},
		{"a Discover-only row", func(t *testing.T, db *DB, owner, doomed uuid.UUID) {
			pushCatalogs(t, db, owner, SelectedCatalogInput{CatalogID: doomed})
		}, "Take it off Home and push first."},
		{"collections on Home using it", func(t *testing.T, db *DB, owner, doomed uuid.UUID) {
			second := usingCollection(t, db, owner, "Second", doomed)
			first := usingCollection(t, db, owner, "First", doomed)
			pushSelection(t, db, owner, SelectedCollectionInput{CollectionID: first}, SelectedCollectionInput{CollectionID: second})
		}, "Remove it from “First” and push first."},
		{"a collection on Home needing a push", func(t *testing.T, db *DB, owner, _ uuid.UUID) {
			pending := newTestCollection(t, db, owner, "Pending")
			pushSelection(t, db, owner, SelectedCollectionInput{CollectionID: pending})
			saveCollection(t, db, owner, pending, CollectionForm{Title: "Renamed"})
		}, "Push first: Nuvio may still show it in a collection."},
		{"a collection off Home using it", func(t *testing.T, db *DB, owner, doomed uuid.UUID) {
			usingCollection(t, db, owner, "Off Home", doomed)
		}, ""},
		{"a collection on Home, pushed as it is, not using it", func(t *testing.T, db *DB, owner, _ uuid.UUID) {
			pushSelection(t, db, owner, SelectedCollectionInput{CollectionID: newTestCollection(t, db, owner, "Other")})
		}, ""},
		{"taken off Home by a push", func(t *testing.T, db *DB, owner, doomed uuid.UUID) {
			pushCatalogs(t, db, owner, SelectedCatalogInput{CatalogID: doomed, ShowInHome: true})
			pushCatalogs(t, db, owner)
		}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			db := newTestDB(t)
			owner := newTestProfile(t, db, "owner")
			doomed, err := db.CreateUserCatalog(ctx, owner, listedCatalogForm("Doomed"))
			if err != nil {
				t.Fatal(err)
			}
			tc.setup(t, db, owner, doomed.ID)

			err = db.DeleteUserCatalog(ctx, owner, doomed.ID)
			_, lookup := ownCatalog(ctx, db.conn, owner, doomed.ID)
			if tc.want == "" {
				if err != nil || !errors.Is(lookup, ErrCatalogNotFound) {
					t.Errorf("DeleteUserCatalog = %v, then the lookup = %v; want it deleted", err, lookup)
				}
				return
			}
			if !errors.Is(err, ErrConflict) || err.Error() != tc.want || lookup != nil {
				t.Errorf("DeleteUserCatalog = %v, then the lookup = %v; want the ErrConflict %q and the catalog kept", err, lookup, tc.want)
			}
		})
	}
}

// Another profile's catalog is not found before anything else is checked.
func TestDeleteUserCatalogOfAnotherProfileIsNotFound(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner, other := newTestProfile(t, db, "owner"), newTestProfile(t, db, "other")
	theirs, err := db.CreateUserCatalog(ctx, other, listedCatalogForm("Theirs"))
	if err != nil {
		t.Fatal(err)
	}
	pushCatalogs(t, db, other, SelectedCatalogInput{CatalogID: theirs.ID, ShowInHome: true})
	if err := db.DeleteUserCatalog(ctx, owner, theirs.ID); !errors.Is(err, ErrCatalogNotFound) {
		t.Errorf("DeleteUserCatalog = %v, want ErrCatalogNotFound", err)
	}
}

// A catalog inside a collection isn't held here: its delete is refused on
// its own terms, whatever is on Home.
func TestCatalogHeldByLeavesAScopedCatalogAlone(t *testing.T) {
	collectionID := uuid.New()
	scoped := Catalog{ID: uuid.New(), CollectionID: &collectionID}
	pending := []CollectionWithFolders{{Collection: Collection{Title: "Pending"}, NeedsPush: true, Catalogs: []Catalog{scoped}}}
	if err := catalogHeldBy(scoped, pending); err != nil {
		t.Errorf("catalogHeldBy = %v, want nil for a scoped catalog", err)
	}
}

// A collection on Home can't be deleted until a push has taken it off.
// Another profile's is not found.
func TestDeleteUserCollectionRefusesOneOnHome(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner, other := newTestProfile(t, db, "owner"), newTestProfile(t, db, "other")
	c := newTestCollection(t, db, owner, "On Home")
	pushSelection(t, db, owner, SelectedCollectionInput{CollectionID: c})

	err := db.DeleteUserCollection(ctx, owner, c)
	if !errors.Is(err, ErrConflict) || err.Error() != "Take it off Home and push first." {
		t.Errorf("DeleteUserCollection on Home = %v, want the ErrConflict to take it off Home first", err)
	}
	mustOwnCollection(t, db, owner, c)

	if err := db.DeleteUserCollection(ctx, other, c); !errors.Is(err, ErrCollectionNotFound) {
		t.Errorf("another profile's DeleteUserCollection = %v, want ErrCollectionNotFound", err)
	}

	pushSelection(t, db, owner)
	if err := db.DeleteUserCollection(ctx, owner, c); err != nil {
		t.Fatalf("DeleteUserCollection off Home: %v", err)
	}
	if _, err := ownCollection(ctx, db.conn, owner, c); !errors.Is(err, ErrCollectionNotFound) {
		t.Errorf("after the delete, the lookup = %v, want ErrCollectionNotFound", err)
	}
}
