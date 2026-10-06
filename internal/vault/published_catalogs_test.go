package vault

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

// GetPublishedCatalogs is the union of listed catalogs on the home screen
// plus every catalog referenced by a folder of a collection on the home
// screen, deduped by id with the home row winning ShowInHome.
func TestGetPublishedCatalogs(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)

	owner := newTestProfile(t, db, "owner")

	onHome, err := db.CreateUserCatalog(ctx, owner, listedCatalogForm("On home and in a folder"))
	if err != nil {
		t.Fatalf("create on-home catalog: %v", err)
	}
	folderOnly, err := db.CreateUserCatalog(ctx, owner, listedCatalogForm("Folder only"))
	if err != nil {
		t.Fatalf("create folder-only catalog: %v", err)
	}
	offTV, err := db.CreateUserCatalog(ctx, owner, listedCatalogForm("In a folder of an off-TV collection"))
	if err != nil {
		t.Fatalf("create off-TV catalog: %v", err)
	}

	onTVCollection := newTestCollection(t, db, owner, "On TV")
	// folderOnly is referenced by two folders of the same on-TV collection —
	// branch 2 of the union emits two rows for it (different o2/o3), and
	// plain UNION's row-level DISTINCT does not collapse them since the
	// other scanned columns differ too; only the Go id-keyed dedupe does.
	if _, err := db.UpdateUserCollection(ctx, owner, onTVCollection, CollectionForm{
		Title: "On TV",
		Folders: []FolderData{
			{Title: "Folder 1", Catalogs: CatalogRefs(onHome.ID, folderOnly.ID)},
			{Title: "Folder 2", Catalogs: CatalogRefs(folderOnly.ID)},
		},
	}); err != nil {
		t.Fatalf("saving on-TV collection: %v", err)
	}

	offTVCollection := newTestCollection(t, db, owner, "Off TV")
	if _, err := db.UpdateUserCollection(ctx, owner, offTVCollection, CollectionForm{
		Title:   "Off TV",
		Folders: []FolderData{{Title: "Folder", Catalogs: CatalogRefs(offTV.ID)}},
	}); err != nil {
		t.Fatalf("saving off-TV collection: %v", err)
	}

	// Put onHome on the home screen and onTVCollection on the TV.
	// offTVCollection stays off; folderOnly and offTV are never selected
	// directly.
	savePush(t, db, owner,
		PushedHome{Catalogs: []SelectedCatalogInput{{CatalogID: onHome.ID, ShowInHome: true}}, Collections: []SelectedCollectionInput{{CollectionID: onTVCollection}}})

	published, err := db.GetPublishedCatalogs(ctx, owner)
	if err != nil {
		t.Fatalf("GetPublishedCatalogs: %v", err)
	}

	byID := make(map[uuid.UUID]Catalog, len(published))
	for _, sc := range published {
		byID[sc.ID] = sc
	}

	if sc, ok := byID[onHome.ID]; !ok || !sc.ShowInHome {
		t.Fatalf("onHome catalog present=%v, ShowInHome=%v, want present with ShowInHome=true", ok, sc.ShowInHome)
	}
	if sc, ok := byID[folderOnly.ID]; !ok || sc.ShowInHome {
		t.Fatalf("folderOnly catalog present=%v, ShowInHome=%v, want present with ShowInHome=false", ok, sc.ShowInHome)
	}
	if _, ok := byID[offTV.ID]; ok {
		t.Fatalf("offTV catalog appeared in the published set; its collection isn't on the TV")
	}
	if len(published) != 2 {
		t.Fatalf("GetPublishedCatalogs returned %d catalogs, want 2 (onHome, folderOnly), got %+v", len(published), published)
	}

	// The query's ORDER BY rank, o1, o2, o3: the home row comes before any
	// folder-derived row.
	if published[0].ID != onHome.ID || published[1].ID != folderOnly.ID {
		t.Fatalf("GetPublishedCatalogs order = [%s, %s], want [onHome, folderOnly]",
			published[0].ID, published[1].ID)
	}
}

// profileToken is profileID's addon token.
func profileToken(t *testing.T, db *DB, profileID uuid.UUID) string {
	t.Helper()
	var token string
	if err := db.conn.QueryRow(`SELECT token FROM profiles WHERE id = ?`, profileID.String()).Scan(&token); err != nil {
		t.Fatal(err)
	}
	return token
}

// ServedCatalog finds a profile's catalog only while it is on the TV, as
// GetPublishedCatalogs lists it: with its own home row, Discover-only
// included, or in a folder of an on-home collection, listed or scoped. A
// catalog off the TV, one only an off-home collection uses, a deleted one,
// another profile's, another type or provider, and an unknown token are all
// ErrCatalogNotFound.
func TestServedCatalog(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner, other := newTestProfile(t, db, "owner"), newTestProfile(t, db, "other")
	token := profileToken(t, db, owner)
	create := func(profileID uuid.UUID, name string) Catalog {
		t.Helper()
		c, err := db.CreateUserCatalog(ctx, profileID, listedCatalogForm(name))
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	homeRow, discover, inFolder, offTV, offHomeOnly, deleted := create(owner, "Home row"), create(owner, "Discover"),
		create(owner, "In a folder"), create(owner, "Off the TV"), create(owner, "Off-home folder only"), create(owner, "Deleted")
	theirs := create(other, "Theirs")
	if err := db.DeleteUserCatalog(ctx, owner, deleted.ID); err != nil {
		t.Fatal(err)
	}

	onHome, offHome := newTestCollection(t, db, owner, "On home"), newTestCollection(t, db, owner, "Off home")
	scopedForm := listedCatalogForm("Scoped")
	scopedForm.CollectionID = &onHome
	scoped, err := db.CreateUserCatalog(ctx, owner, scopedForm)
	if err != nil {
		t.Fatal(err)
	}
	for id, refs := range map[uuid.UUID][]uuid.UUID{onHome: {inFolder.ID, scoped.ID}, offHome: {offHomeOnly.ID}} {
		if _, err := db.UpdateUserCollection(ctx, owner, id, CollectionForm{
			Title: "C", Folders: []FolderData{{Title: "Folder", Catalogs: CatalogRefs(refs...)}},
		}); err != nil {
			t.Fatal(err)
		}
	}
	savePush(t, db, owner,
		PushedHome{Catalogs: []SelectedCatalogInput{
			{CatalogID: homeRow.ID, ShowInHome: true}, {CatalogID: discover.ID, Position: 1},
		}, Collections: []SelectedCollectionInput{{CollectionID: onHome, Position: 2}}})
	savePush(t, db, other,
		PushedHome{Catalogs: []SelectedCatalogInput{{CatalogID: theirs.ID, ShowInHome: true}}})

	for _, tc := range []struct {
		name                  string
		token                 string
		id                    uuid.UUID
		catalogType, provider string
		found                 bool
	}{
		{"its own home row", token, homeRow.ID, "movie", "tmdb", true},
		{"Discover-only", token, discover.ID, "movie", "tmdb", true},
		{"listed, in an on-home collection", token, inFolder.ID, "movie", "tmdb", true},
		{"scoped to an on-home collection", token, scoped.ID, "movie", "tmdb", true},
		{"off the TV", token, offTV.ID, "movie", "tmdb", false},
		{"only in an off-home collection", token, offHomeOnly.ID, "movie", "tmdb", false},
		{"deleted", token, deleted.ID, "movie", "tmdb", false},
		{"another type", token, homeRow.ID, "series", "tmdb", false},
		{"another provider", token, homeRow.ID, "movie", "other", false},
		{"another profile's", token, theirs.ID, "movie", "tmdb", false},
		{"unknown token", "no-such-token", homeRow.ID, "movie", "tmdb", false},
		{"unknown id", token, uuid.New(), "movie", "tmdb", false},
	} {
		served, err := db.ServedCatalog(ctx, tc.token, tc.id, tc.catalogType, tc.provider)
		if tc.found && (err != nil || served.Params != "{}" || served.Account != "owner") {
			t.Errorf("%s: %+v, %v; want the params and the owner's account", tc.name, served, err)
		}
		if !tc.found && !errors.Is(err, ErrCatalogNotFound) {
			t.Errorf("%s: %+v, %v; want ErrCatalogNotFound", tc.name, served, err)
		}
	}
}

// A push record that doesn't decode is an error, not ErrCatalogNotFound: the
// addon answers it as its own failure rather than as a catalog the profile
// doesn't have.
func TestServedCatalogUndecodableRecord(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner := newTestProfile(t, db, "owner")
	savePush(t, db, owner, PushedHome{})
	if _, err := db.conn.Exec(`UPDATE push_records SET record = 'not json' WHERE profile_id = ?`, owner.String()); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ServedCatalog(ctx, profileToken(t, db, owner), uuid.New(), "movie", "tmdb"); err == nil || errors.Is(err, ErrCatalogNotFound) {
		t.Errorf("ServedCatalog over an undecodable record = %v, want a decoding error", err)
	}
}
