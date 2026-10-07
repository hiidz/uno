package vault

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

// A new collection, and every copy of one — a subscribe, a duplicate of a
// publication and a Duplicate — is off Home, so nothing waits for a push.
func TestNewCollectionsAreUnpushed(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner := newTestProfile(t, db, "owner")
	subscriber := newTestProfile(t, db, "subscriber")

	source := publishCollection(t, db, owner, CollectionForm{Title: "Source"})
	subscribed := subscribe(t, db, subscriber, source.Publication.ID).Collection
	duplicated, err := db.DuplicatePublication(ctx, subscriber, source.Publication.ID)
	if err != nil {
		t.Fatalf("DuplicatePublication: %v", err)
	}
	dup, err := db.DuplicateCollection(ctx, owner, source.ID)
	if err != nil {
		t.Fatalf("DuplicateCollection: %v", err)
	}
	for _, c := range []CollectionWithFolders{source, *subscribed, *duplicated.Collection, dup} {
		if c.HomeSortOrder != nil {
			t.Errorf("%q: on Home %v; want off Home", c.Title, *c.HomeSortOrder)
		}
	}
	for _, profileID := range []uuid.UUID{owner, subscriber} {
		if changes, err := db.PendingPush(ctx, profileID); err != nil || len(changes) != 0 {
			t.Errorf("pending = %+v, %v; want nothing waiting", changes, err)
		}
	}
}

// waitsForPush reports whether profileID's collection id is on the list of
// what waits for a push.
func waitsForPush(t *testing.T, db *DB, profileID, id uuid.UUID) bool {
	t.Helper()
	changes, err := db.PendingPush(context.Background(), profileID)
	if err != nil {
		t.Fatalf("PendingPush: %v", err)
	}
	for _, c := range changes {
		if c.Kind == pendingCollection && c.ID == id {
			return true
		}
	}
	return false
}

// saveCollection writes form over profileID's collection id, failing the
// test on an error.
func saveCollection(t *testing.T, db *DB, profileID, id uuid.UUID, form CollectionForm) CollectionWithFolders {
	t.Helper()
	saved, err := db.UpdateUserCollection(context.Background(), profileID, id, form)
	if err != nil {
		t.Fatalf("UpdateUserCollection: %v", err)
	}
	return saved
}

// A collection on Home waits for a push exactly when what push would send for
// it differs from what it last sent: a rename and back changes nothing Nuvio
// holds; a title, an image or a folder's genre does, until the next push.
func TestACollectionWaitsWhenWhatPushSendsDiffers(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner := newTestProfile(t, db, "owner")
	catalog, err := db.CreateUserCatalog(ctx, owner, listedCatalogForm("Popular"))
	if err != nil {
		t.Fatal(err)
	}
	c, err := db.CreateUserCollection(ctx, owner, CollectionForm{Title: "Night", Folders: []FolderData{{Title: "F", Catalogs: CatalogRefs(catalog.ID)}}})
	if err != nil {
		t.Fatal(err)
	}
	folderID := c.Folders[0].ID
	form := func(title, cover, genre string) CollectionForm {
		return CollectionForm{Title: title, Folders: []FolderData{{
			ID: &folderID, Title: "F", FolderArt: FolderArt{CoverImageURL: cover},
			Catalogs: []FolderCatalogRef{{CatalogID: &catalog.ID, Genre: genre}},
		}}}
	}

	pushSelection(t, db, owner, SelectedCollectionInput{CollectionID: c.ID})
	if waitsForPush(t, db, owner, c.ID) {
		t.Fatal("right after a push: want nothing waiting")
	}

	saveCollection(t, db, owner, c.ID, form("Renamed", "", ""))
	saveCollection(t, db, owner, c.ID, form("Night", "", ""))
	if waitsForPush(t, db, owner, c.ID) {
		t.Error("after a rename and back: want nothing waiting")
	}

	for _, change := range []struct {
		name string
		form CollectionForm
	}{
		{"title", form("Late Night", "", "")},
		{"image", form("Night", "https://example.com/cover.jpg", "")},
		{"genre", form("Night", "", "Horror")},
	} {
		saveCollection(t, db, owner, c.ID, change.form)
		if !waitsForPush(t, db, owner, c.ID) {
			t.Errorf("after a %s change: want it waiting", change.name)
		}
		pushSelection(t, db, owner, SelectedCollectionInput{CollectionID: c.ID})
		if waitsForPush(t, db, owner, c.ID) {
			t.Errorf("after pushing the %s change: want nothing waiting", change.name)
		}
	}

	pushSelection(t, db, owner)
	saveCollection(t, db, owner, c.ID, form("Off Home", "", ""))
	if waitsForPush(t, db, owner, c.ID) {
		t.Error("off Home: want nothing waiting, whatever changed")
	}
}

// Push stores the record it built and sent, never the row as it stands when
// it writes: a save landing between the two leaves the collection waiting.
func TestASaveDuringAPushLeavesItPending(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner := newTestProfile(t, db, "owner")
	c, err := db.CreateUserCollection(ctx, owner, CollectionForm{Title: "Read"})
	if err != nil {
		t.Fatal(err)
	}
	record, err := db.BuildPushRecord(ctx, owner, PushedHome{Collections: []SelectedCollectionInput{{CollectionID: c.ID}}})
	if err != nil {
		t.Fatal(err)
	}

	saveCollection(t, db, owner, c.ID, CollectionForm{Title: "Saved meanwhile"})
	if err := db.SavePush(ctx, owner, record); err != nil {
		t.Fatalf("SavePush: %v", err)
	}
	if !waitsForPush(t, db, owner, c.ID) {
		t.Error("after a save during the push: want it still waiting")
	}
}

// What push sends round-trips through the stored record byte for byte, so
// text JSON escapes — an & in a weserv URL, a < in a title — never read as a
// change.
func TestEscapedTextDoesNotWaitForAPush(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner := newTestProfile(t, db, "owner")
	c, err := db.CreateUserCollection(ctx, owner, CollectionForm{
		Title:            "Fish & <Chips>",
		BackdropImageURL: "https://images.weserv.nl/?url=image.tmdb.org/t/p/original/a.jpg&w=1280&output=webp",
		Folders:          []FolderData{{Title: "A < B > C & D", FolderArt: FolderArt{CoverEmoji: "🎃"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	pushSelection(t, db, owner, SelectedCollectionInput{CollectionID: c.ID})
	if waitsForPush(t, db, owner, c.ID) {
		t.Error("right after a push of escaped text: want nothing waiting")
	}
}

// A collection on Home that the last push sent nothing for — Home set by an
// older push whose record no longer holds it — waits for a push.
func TestOnHomeButNotInTheRecordWaitsForAPush(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner := newTestProfile(t, db, "owner")
	c, err := db.CreateUserCollection(ctx, owner, CollectionForm{Title: "Missing"})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SavePush(ctx, owner, PushRecord{Home: PushedHome{
		Collections: []SelectedCollectionInput{{CollectionID: c.ID}},
	}}); err != nil {
		t.Fatalf("SavePush: %v", err)
	}
	if !waitsForPush(t, db, owner, c.ID) {
		t.Error("on Home with no entry in the record: want it waiting")
	}
}

// Only push writes pin_to_top, from its selection: a save leaves it alone, a
// collection push leaves off Home keeps its last value, and a Duplicate, which
// isn't on Home, starts unpinned.
func TestPinToTopIsWrittenOnlyByPush(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner := newTestProfile(t, db, "owner")
	a, err := db.CreateUserCollection(ctx, owner, CollectionForm{Title: "A"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := db.CreateUserCollection(ctx, owner, CollectionForm{Title: "B"})
	if err != nil {
		t.Fatal(err)
	}

	pushSelection(t, db, owner, SelectedCollectionInput{CollectionID: a.ID, PinToTop: true}, SelectedCollectionInput{CollectionID: b.ID, Position: 1})
	if !mustOwnCollection(t, db, owner, a.ID).PinToTop || mustOwnCollection(t, db, owner, b.ID).PinToTop {
		t.Fatal("after the first push, want A pinned and B not")
	}
	if waitsForPush(t, db, owner, a.ID) {
		t.Error("A right after the push: want nothing waiting, its stored pin being the one sent")
	}

	if saved, err := db.UpdateUserCollection(ctx, owner, a.ID, CollectionForm{Title: "A2"}); err != nil || !saved.PinToTop {
		t.Fatalf("save = %v pinned %v; want the pin kept", err, saved.PinToTop)
	}

	pushSelection(t, db, owner, SelectedCollectionInput{CollectionID: b.ID, PinToTop: true})
	if offHome := mustOwnCollection(t, db, owner, a.ID); offHome.HomeSortOrder != nil || !offHome.PinToTop {
		t.Errorf("A off Home = order %v pinned %v; want no place and its last pin", offHome.HomeSortOrder, offHome.PinToTop)
	}
	if !mustOwnCollection(t, db, owner, b.ID).PinToTop {
		t.Error("B after the second push: want pinned")
	}

	duplicate, err := db.DuplicateCollection(ctx, owner, a.ID)
	if err != nil || duplicate.PinToTop {
		t.Errorf("duplicate = %v pinned %v; want unpinned", err, duplicate.PinToTop)
	}
}

// A Home selection's ids are its catalogs' and its collections', each in the
// order push places them.
func TestPushedHomeIDs(t *testing.T) {
	a, b, c := uuid.New(), uuid.New(), uuid.New()
	home := PushedHome{
		Catalogs:    []SelectedCatalogInput{{CatalogID: c, ShowInHome: true}},
		Collections: []SelectedCollectionInput{{CollectionID: a, PinToTop: true}, {CollectionID: b}},
	}
	if got := home.collectionIDs(); len(got) != 2 || got[0] != a || got[1] != b {
		t.Errorf("collectionIDs = %v, want [%s %s]", got, a, b)
	}
	if got := home.catalogIDs(); len(got) != 1 || got[0] != c {
		t.Errorf("catalogIDs = %v, want [%s]", got, c)
	}
	if got := (PushedHome{}).collectionIDs(); len(got) != 0 {
		t.Errorf("an empty selection's ids = %v, want none", got)
	}
}
