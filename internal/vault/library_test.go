package vault

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

// Every query of a read transaction sees the rows the first one saw, whatever
// commits meanwhile; a read after it sees them.
func TestReadTxSeesOneSnapshot(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner := newTestProfile(t, db, "owner")
	count := func(q dbtx) int {
		t.Helper()
		var n int
		if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM catalogs`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	var first, second int
	err := db.inReadTx(ctx, func(q dbtx) error {
		first = count(q)
		if _, err := db.CreateUserCatalog(ctx, owner, listedCatalogForm("Added meanwhile")); err != nil {
			t.Fatal(err)
		}
		second = count(q)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if first != 0 || second != 0 {
		t.Errorf("inside the read transaction counts = %d then %d, want 0 and 0", first, second)
	}
	if got := count(db.conn); got != 1 {
		t.Errorf("after it the count = %d, want 1", got)
	}
}

// The library is the profile's listed catalogs, its collections and what waits
// for a push, as the three reads give them one by one.
func TestGetLibrary(t *testing.T) {
	ctx := context.Background()
	f := newPendingFixture(t)
	f.rename(t, f.home, "Renamed", f.changed)

	lib, err := f.db.GetLibrary(ctx, f.owner)
	if err != nil {
		t.Fatalf("GetLibrary: %v", err)
	}
	catalogs, err := f.db.GetUserCatalogs(ctx, f.owner)
	if err != nil {
		t.Fatal(err)
	}
	collections, err := f.db.GetUserCollections(ctx, f.owner)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(lib.Catalogs, catalogs) || !reflect.DeepEqual(lib.Collections, collections) {
		t.Errorf("library rows differ from the separate reads:\n%+v\n%+v", lib, catalogs)
	}
	wantPending(t, lib.Pending, PendingChange{pendingCatalog, f.home.ID, "Renamed", PendingChanged})
}

// A publication's subscribers are found by index: ending a publication, by
// unpublishing it or deleting its source, cascades through them and its release
// trigger reads them, and neither may walk every subscription.
func TestSubscribersAreIndexedByPublication(t *testing.T) {
	db := newTestDB(t)
	detail := queryPlan(t, db, `SELECT collection_id FROM subscriptions WHERE publication_id = ?`, []any{"x"})
	if !strings.Contains(detail, "subscriptions_by_publication") {
		t.Errorf("plan %q, want subscriptions_by_publication", detail)
	}
}
