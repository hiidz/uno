package vault

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

// profileToken is profileID's addon token.
func profileToken(t *testing.T, db *DB, profileID uuid.UUID) string {
	t.Helper()
	var token string
	if err := db.conn.QueryRow(`SELECT token FROM profiles WHERE id = ?`, profileID.String()).Scan(&token); err != nil {
		t.Fatal(err)
	}
	return token
}

// ServedCatalog finds a profile's catalog only while its last push put it in
// Nuvio, as GetPublishedCatalogs lists it: with its own home row,
// Discover-only included, or in a folder of an on-home collection, listed or
// scoped. A catalog off Home, one only an off-home collection uses, a deleted
// one, another profile's, another type or provider, and an unknown token are
// all ErrCatalogNotFound.
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
	homeRow, discover, inFolder, offHomeCatalog, offHomeOnly, deleted := create(owner, "Home row"), create(owner, "Discover"),
		create(owner, "In a folder"), create(owner, "Off Home"), create(owner, "Off-home folder only"), create(owner, "Deleted")
	theirs := create(other, "Theirs")
	if err := db.DeleteUserCatalog(ctx, owner, deleted.ID); err != nil {
		t.Fatal(err)
	}

	onHome, offHome := newTestCollection(t, db, owner, "On home"), newTestCollection(t, db, owner, "Off home")
	scoped := createScopedCatalog(t, db, owner, onHome, listedCatalogForm("Scoped"))
	for id, refs := range map[uuid.UUID][]uuid.UUID{onHome: {inFolder.ID, scoped.ID}, offHome: {offHomeOnly.ID}} {
		if _, err := db.UpdateUserCollection(ctx, owner, id, collectionRevision(t, db, id), CollectionForm{
			Title: "C", ViewMode: "TABBED_GRID", Folders: []FolderData{{FolderArt: FolderArt{TileShape: "POSTER"}, Title: "Folder", Catalogs: CatalogRefs(refs...)}},
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
		{"off Home", token, offHomeCatalog.ID, "movie", "tmdb", false},
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
