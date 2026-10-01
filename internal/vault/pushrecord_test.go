package vault

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/google/uuid"
)

// pushRecordFixture is one profile with a Home that reaches catalogs every
// way: its own home row, a Discover row, a listed catalog only a folder uses
// (in two folders), a scoped one, and a home row a folder also uses; plus a
// catalog only an off-Home collection uses.
type pushRecordFixture struct {
	db                 *DB
	owner              uuid.UUID
	catalogs           CatalogSelectionForm
	collections        CollectionSelectionForm
	homeRow, offHomeID uuid.UUID
}

func newPushRecordFixture(t *testing.T) pushRecordFixture {
	t.Helper()
	ctx := context.Background()
	db := newTestDB(t)
	owner := newTestProfile(t, db, "owner")
	create := func(name, params string) Catalog {
		t.Helper()
		form := listedCatalogForm(name)
		form.Params = params
		c, err := db.CreateUserCatalog(ctx, owner, form)
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	homeRow := create("Home row", `{"sort_by":"popularity.desc"}`)
	discover := create("Discover", `{"sort_by":"vote_average.desc"}`)
	folderOnly := create("Folder only", `{"with_genres":"27"}`)
	offHomeOnly := create("Off-home only", "{}")

	first, second := newTestCollection(t, db, owner, "First"), newTestCollection(t, db, owner, "Second")
	scopedForm := listedCatalogForm("Scoped")
	scopedForm.CollectionID = &second
	scoped, err := db.CreateUserCatalog(ctx, owner, scopedForm)
	if err != nil {
		t.Fatal(err)
	}
	offHome := newTestCollection(t, db, owner, "Off home")
	for id, folders := range map[uuid.UUID][]FolderData{
		first: {
			{Title: "A", Catalogs: CatalogRefs(folderOnly.ID, homeRow.ID)},
			{Title: "B", Catalogs: CatalogRefs(folderOnly.ID)},
		},
		second:  {{Title: "C", Catalogs: CatalogRefs(scoped.ID)}},
		offHome: {{Title: "D", Catalogs: CatalogRefs(offHomeOnly.ID)}},
	} {
		if _, err := db.UpdateUserCollection(ctx, owner, id, CollectionForm{Title: "Saved", Folders: folders}); err != nil {
			t.Fatal(err)
		}
	}
	return pushRecordFixture{
		db: db, owner: owner,
		catalogs: CatalogSelectionForm{Catalogs: []SelectedCatalogInput{
			{CatalogID: homeRow.ID, ShowInHome: true}, {CatalogID: discover.ID},
		}},
		collections: CollectionSelectionForm{Collections: []SelectedCollectionInput{
			{CollectionID: second, PinToTop: true}, {CollectionID: first},
		}},
		homeRow: homeRow.ID, offHomeID: offHomeOnly.ID,
	}
}

// storedRecord reads profileID's push record and its stamp, failing the
// test when there is none.
func storedRecord(t *testing.T, db *DB, profileID uuid.UUID) (PushRecord, string) {
	t.Helper()
	var raw, stamp string
	if err := db.conn.QueryRow(`SELECT record, nuvio_profile_uuid FROM push_records WHERE profile_id = ?`,
		profileID.String()).Scan(&raw, &stamp); err != nil {
		t.Fatalf("reading push record: %v", err)
	}
	var record PushRecord
	if err := json.Unmarshal([]byte(raw), &record); err != nil {
		t.Fatal(err)
	}
	return record, stamp
}

// The record holds every catalog the addon's manifest lists for the same
// Home, in its order, with name, type and params inline; the Home selection
// as pushed; and each collection as the bytes push sends, in Home order with
// the pending pin.
func TestPushRecordHoldsWhatNuvioReaches(t *testing.T) {
	ctx := context.Background()
	f := newPushRecordFixture(t)
	savePush(t, f.db, f.owner, f.catalogs, f.collections)

	record, stamp := storedRecord(t, f.db, f.owner)
	if stamp != "nuvio-profile-owner" {
		t.Errorf("stamp = %q, want the profile's Nuvio profile id", stamp)
	}

	published, err := f.db.GetPublishedCatalogs(ctx, f.owner)
	if err != nil {
		t.Fatal(err)
	}
	if len(record.Catalogs) != len(published) {
		t.Fatalf("record has %d catalogs, the manifest lists %d", len(record.Catalogs), len(published))
	}
	for i, p := range published {
		got := record.Catalogs[i]
		if got.ID != p.ID || got.Name != p.Name || got.Type != p.Type || got.Provider != p.Provider || string(got.Params) != p.Params {
			t.Errorf("catalog %d = %+v (params %s), want %s %q %s %s %s", i, got, got.Params, p.ID, p.Name, p.Type, p.Provider, p.Params)
		}
	}
	for _, c := range record.Catalogs {
		if c.ID == f.offHomeID {
			t.Error("the record holds a catalog only an off-Home collection uses")
		}
	}

	if !reflect.DeepEqual(record.Home, PushedHome{Catalogs: f.catalogs.Catalogs, Collections: f.collections.Collections}) {
		t.Errorf("Home = %+v, want the pushed selection", record.Home)
	}

	selection, err := f.db.GetCurrentCollectionSelection(ctx, f.owner)
	if err != nil || len(selection) != 2 || len(record.Collections) != 2 {
		t.Fatalf("selection %d, record %d collections (%v); want 2 each", len(selection), len(record.Collections), err)
	}
	for i, tree := range selection {
		want, err := tree.PushJSON()
		if err != nil {
			t.Fatal(err)
		}
		if string(record.Collections[i]) != string(want) {
			t.Errorf("collection %d = %s, want %s", i, record.Collections[i], want)
		}
	}
	if !selection[0].PinToTop {
		t.Error("the first collection on Home: want the pending pin stored")
	}
}

// A push replaces the record whole, stamped with the Nuvio profile id the
// profile has at that push.
func TestAPushReplacesTheRecord(t *testing.T) {
	ctx := context.Background()
	f := newPushRecordFixture(t)
	savePush(t, f.db, f.owner, f.catalogs, f.collections)
	if _, err := f.db.ResolveOrCreateProfile(ctx, "owner", 1, "reused-slot"); err != nil {
		t.Fatal(err)
	}
	savePush(t, f.db, f.owner, CatalogSelectionForm{}, CollectionSelectionForm{})

	record, stamp := storedRecord(t, f.db, f.owner)
	if stamp != "reused-slot" || len(record.Collections) != 0 || len(record.Catalogs) != 0 ||
		len(record.Home.Catalogs) != 0 || len(record.Home.Collections) != 0 {
		t.Errorf("after an empty push: stamp %q, record %+v; want the new stamp and nothing held", stamp, record)
	}
	var rows int
	if err := f.db.conn.QueryRow(`SELECT count(*) FROM push_records`).Scan(&rows); err != nil || rows != 1 {
		t.Errorf("push records = %d (%v), want 1", rows, err)
	}
}

// The backfill's builder reads the Home the last push stored, so right after
// a push it rebuilds the record that push stored.
func TestStoredPushRecordRebuildsTheLastPush(t *testing.T) {
	ctx := context.Background()
	f := newPushRecordFixture(t)
	savePush(t, f.db, f.owner, f.catalogs, f.collections)
	want, _ := storedRecord(t, f.db, f.owner)

	var got PushRecord
	err := f.db.inTx(ctx, func(tx *sql.Tx) error {
		var err error
		got, err = StoredPushRecord(ctx, tx, f.owner)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	gotJSON, _ := json.Marshal(got)
	wantJSON, _ := json.Marshal(want)
	if string(gotJSON) != string(wantJSON) {
		t.Errorf("StoredPushRecord = %s\nwant %s", gotJSON, wantJSON)
	}
}

// A record for a profile that doesn't exist is refused, and a build reads
// only the caller's own rows.
func TestPushRecordRefusals(t *testing.T) {
	ctx := context.Background()
	f := newPushRecordFixture(t)
	if err := f.db.SavePush(ctx, uuid.New(), PushRecord{}); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("SavePush for an unknown profile = %v, want ErrInvalidInput", err)
	}
	other := newTestProfile(t, f.db, "other")
	record, err := f.db.BuildPushRecord(ctx, other, f.catalogs, f.collections)
	if err != nil || len(record.Collections) != 0 || len(record.Catalogs) != 0 {
		t.Errorf("another profile's build = %+v, %v; want none of the owner's rows", record, err)
	}
}
