package vault

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

// A new collection, and every copy of one — a subscribe, a duplicate of a
// publication and a Duplicate — is off Home, so it needs no push.
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
		if c.HomeSortOrder != nil || c.NeedsPush {
			t.Errorf("%q: on Home %v, needs push %t; want off Home and no push needed", c.Title, c.HomeSortOrder, c.NeedsPush)
		}
	}
}

// needsPush reads profileID's collection id back and reports its NeedsPush.
func needsPush(t *testing.T, db *DB, profileID, id uuid.UUID) bool {
	t.Helper()
	return mustOwnCollection(t, db, profileID, id).NeedsPush
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

// A collection on Home needs a push exactly when what push would send for it
// differs from what it last sent: a rename and back, or a recipe-only edit to
// a catalog it uses, changes nothing Nuvio holds; a title, an image or a
// folder's genre does, until the next push.
func TestNeedsPushFollowsWhatPushWouldSend(t *testing.T) {
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
			ID: &folderID, Title: "F", CoverImageURL: cover,
			Catalogs: []FolderCatalogRef{{CatalogID: &catalog.ID, Genre: genre}},
		}}}
	}

	pushSelection(t, db, owner, SelectedCollectionInput{CollectionID: c.ID})
	if needsPush(t, db, owner, c.ID) {
		t.Fatal("right after a push: want no push needed")
	}

	saveCollection(t, db, owner, c.ID, form("Renamed", "", ""))
	saveCollection(t, db, owner, c.ID, form("Night", "", ""))
	edit := listedCatalogForm("Popular")
	edit.Params = `{"sort_by":"vote_average.desc"}`
	if _, err := db.UpdateUserCatalog(ctx, owner, catalog.ID, edit); err != nil {
		t.Fatal(err)
	}
	if needsPush(t, db, owner, c.ID) {
		t.Error("after a rename and back and a recipe-only edit: want no push needed")
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
		if !needsPush(t, db, owner, c.ID) {
			t.Errorf("after a %s change: want a push needed", change.name)
		}
		pushSelection(t, db, owner, SelectedCollectionInput{CollectionID: c.ID})
		if needsPush(t, db, owner, c.ID) {
			t.Errorf("after pushing the %s change: want no push needed", change.name)
		}
	}

	pushSelection(t, db, owner)
	saveCollection(t, db, owner, c.ID, form("Off Home", "", ""))
	if needsPush(t, db, owner, c.ID) {
		t.Error("off Home: want no push needed, whatever changed")
	}
}

// Push stores the record it built and sent, never the row as it stands when
// it writes: a save landing between the two leaves the collection needing a
// push.
func TestASaveDuringAPushLeavesItPending(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner := newTestProfile(t, db, "owner")
	c, err := db.CreateUserCollection(ctx, owner, CollectionForm{Title: "Read"})
	if err != nil {
		t.Fatal(err)
	}
	record, err := db.BuildPushRecord(ctx, owner, CatalogSelectionForm{},
		CollectionSelectionForm{Collections: []SelectedCollectionInput{{CollectionID: c.ID}}})
	if err != nil {
		t.Fatal(err)
	}

	saveCollection(t, db, owner, c.ID, CollectionForm{Title: "Saved meanwhile"})
	if err := db.SavePush(ctx, owner, record); err != nil {
		t.Fatalf("SavePush: %v", err)
	}
	if !needsPush(t, db, owner, c.ID) {
		t.Error("after a save during the push: want a push still needed")
	}
}

// What push sends round-trips through the stored record byte for byte, so
// text JSON escapes — an & in a weserv URL, a < in a title — never reads as a
// change.
func TestEscapedTextDoesNotNeedAPush(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner := newTestProfile(t, db, "owner")
	c, err := db.CreateUserCollection(ctx, owner, CollectionForm{
		Title:            "Fish & <Chips>",
		BackdropImageURL: "https://images.weserv.nl/?url=image.tmdb.org/t/p/original/a.jpg&w=1280&output=webp",
		Folders:          []FolderData{{Title: "A < B > C & D", CoverEmoji: "🎃"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	pushSelection(t, db, owner, SelectedCollectionInput{CollectionID: c.ID})
	if needsPush(t, db, owner, c.ID) {
		t.Error("right after a push of escaped text: want no push needed")
	}
}

// A collection on Home that the last push sent nothing for — Home set by an
// older push whose record no longer holds it — needs a push.
func TestOnHomeButNotInTheRecordNeedsAPush(t *testing.T) {
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
	if !needsPush(t, db, owner, c.ID) {
		t.Error("on Home with no entry in the record: want a push needed")
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

	pushSelection(t, db, owner, SelectedCollectionInput{CollectionID: a.ID, PinToTop: true}, SelectedCollectionInput{CollectionID: b.ID})
	if !mustOwnCollection(t, db, owner, a.ID).PinToTop || mustOwnCollection(t, db, owner, b.ID).PinToTop {
		t.Fatal("after the first push, want A pinned and B not")
	}
	if needsPush(t, db, owner, a.ID) {
		t.Error("A right after the push: want no push needed, its stored pin being the one sent")
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

// A selection's ids are its collections', in the order push places them.
func TestCollectionSelectionFormCollectionIDs(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	form := CollectionSelectionForm{Collections: []SelectedCollectionInput{{CollectionID: a, PinToTop: true}, {CollectionID: b}}}
	if got := form.CollectionIDs(); len(got) != 2 || got[0] != a || got[1] != b {
		t.Errorf("CollectionIDs = %v, want [%s %s]", got, a, b)
	}
	if got := (CollectionSelectionForm{}).CollectionIDs(); len(got) != 0 {
		t.Errorf("an empty selection's ids = %v, want none", got)
	}
}
