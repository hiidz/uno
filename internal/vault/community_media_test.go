package vault

import (
	"context"
	"slices"
	"testing"

	"github.com/google/uuid"
)

// mediaCollection is a collection form whose backdrop and one folder carry
// every media address there is.
func mediaCollection(title string) CollectionForm {
	return CollectionForm{
		Title: title, ViewMode: "TABBED_GRID", BackdropImageURL: "https://img.example/backdrop.jpg",
		Folders: []FolderData{{
			FolderArt: FolderArt{
				TileShape: "POSTER", CoverEmoji: "🎬", CoverImageURL: "https://img.example/cover.jpg",
				FocusGIFURL: "https://img.example/focus.gif", FocusGIFEnabled: true,
				HeroBackdropURL: "https://img.example/hero.jpg", HeroVideoURL: "https://img.example/hero.mp4",
				TitleLogoURL: "https://img.example/logo.png",
			},
			Title: "F", Catalogs: []FolderCatalogRef{newScoped("k", "S", "{}")},
		}},
	}
}

// artMedia is a's media addresses, empty ones included.
func artMedia(a FolderArt) []string {
	return []string{a.CoverImageURL, a.FocusGIFURL, a.HeroBackdropURL, a.HeroVideoURL, a.TitleLogoURL}
}

// treeMedia is every media address tree holds.
func treeMedia(tree CollectionWithFolders) []string {
	got := []string{tree.BackdropImageURL}
	for _, f := range tree.Folders {
		got = append(got, artMedia(f.FolderArt)...)
	}
	return slices.DeleteFunc(got, func(s string) bool { return s == "" })
}

// snapshotMedia is every media address s's collection holds.
func snapshotMedia(s Snapshot) []string {
	got := []string{s.Collection.BackdropImageURL}
	for _, f := range s.Collection.Folders {
		got = append(got, artMedia(f.FolderArt)...)
	}
	return slices.DeleteFunc(got, func(s string) bool { return s == "" })
}

// sharingAll is db as a server that shares media sees it: the same
// database, without WithholdCommunityMedia.
func sharingAll(db *DB) *DB {
	return &DB{conn: db.conn}
}

// On a server that withholds media, Community shows none and every copy is
// written without any: the list, a publication's page, Add and Duplicate.
// The folder's emoji and its focus GIF switch stay. The publisher's own row
// and the stored snapshot keep every address.
func TestWithheldMediaLeavesCommunityAndCopies(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner, subscriber := newTestProfile(t, db, "owner"), newTestProfile(t, db, "subscriber")
	source := publishCollection(t, db, owner, mediaCollection("Art"))
	db.WithholdCommunityMedia()

	page, err := db.ListCommunity(ctx, subscriber, CommunityQuery{Kind: kindCollection, Sort: "name"})
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("ListCommunity = %+v, %v; want the one publication", page, err)
	}
	if f := page.Items[0].Folders[0]; f.CoverImageURL != "" || f.CoverEmoji != "🎬" {
		t.Errorf("listed folder = %+v, want the emoji and no cover", f)
	}

	detail, err := db.GetPublication(ctx, subscriber, source.Publication.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := snapshotMedia(detail.Snapshot); len(got) != 0 || detail.Folders[0].CoverImageURL != "" {
		t.Errorf("publication page holds %v and cover %q, want no media", got, detail.Folders[0].CoverImageURL)
	}

	added := *subscribe(t, db, subscriber, source.Publication.ID).Collection
	duplicated, err := db.DuplicatePublication(ctx, subscriber, source.Publication.ID)
	if err != nil {
		t.Fatal(err)
	}
	for name, tree := range map[string]CollectionWithFolders{"added": added, "duplicated": *duplicated.Collection} {
		if got := treeMedia(tree); len(got) != 0 {
			t.Errorf("%s copy holds %v, want no media", name, got)
		}
		if f := tree.Folders[0]; f.CoverEmoji != "🎬" || !f.FocusGIFEnabled {
			t.Errorf("%s copy's folder = %+v, want its emoji and focus GIF switch kept", name, f.FolderArt)
		}
	}

	if got := treeMedia(mustOwnCollection(t, db, owner, source.ID)); len(got) != 6 {
		t.Errorf("publisher's own row holds %v, want all 6 addresses", got)
	}
	stored, err := sharingAll(db).GetPublication(ctx, owner, source.Publication.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := snapshotMedia(stored.Snapshot); len(got) != 6 {
		t.Errorf("stored snapshot holds %v, want all 6 addresses", got)
	}
}

// A copy added while media was shared loses it at its next Update on a
// server that withholds it, and the Update's changes say so. Afterwards an
// update that only changes media offers an Update with nothing to list, which
// leaves the copy as it is and in step.
func TestWithheldMediaUpdate(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner, subscriber := newTestProfile(t, db, "owner"), newTestProfile(t, db, "subscriber")
	source := publishCollection(t, db, owner, mediaCollection("Art"))
	added := *subscribe(t, db, subscriber, source.Publication.ID).Collection
	if got := treeMedia(added); len(got) != 6 {
		t.Fatalf("copy added while media is shared holds %v, want all 6 addresses", got)
	}
	db.WithholdCommunityMedia()
	pubID := source.Publication.ID

	republish(t, db, owner, source.ID, func(form *CollectionForm) { form.Title = "Art 2" })
	changes, err := db.UpdateChanges(ctx, subscriber, pubID)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := changeAspects(changes), []string{"collection name", "collection settings", "folder art"}; !slices.Equal(got, want) {
		t.Errorf("changes = %v, want %v", got, want)
	}
	updated := *mustUpdate(t, db, subscriber, pubID).Collection
	if got := treeMedia(updated); len(got) != 0 || updated.Title != "Art 2" {
		t.Errorf("updated copy %q holds %v, want the new title and no media", updated.Title, got)
	}

	republish(t, db, owner, source.ID, func(form *CollectionForm) {
		form.Folders[0].CoverImageURL = "https://img.example/other.jpg"
	})
	if !updateAvailable(t, db, subscriber, pubID) {
		t.Fatal("no update available after a media-only republish")
	}
	if changes, err := db.UpdateChanges(ctx, subscriber, pubID); err != nil || len(changes) != 0 {
		t.Errorf("changes of a media-only update = %v, %v; want none", changes, err)
	}
	if got := treeMedia(*mustUpdate(t, db, subscriber, pubID).Collection); len(got) != 0 {
		t.Errorf("copy after a media-only update holds %v, want no media", got)
	}
	if updateAvailable(t, db, subscriber, pubID) {
		t.Error("an update is still available after Update")
	}
}

// republish edits owner's collection id, its folders' art kept, through edit
// and publishes it again.
func republish(t *testing.T, db *DB, owner, id uuid.UUID, edit func(*CollectionForm)) {
	t.Helper()
	tree := mustOwnCollection(t, db, owner, id)
	form := saveFormOf(tree)
	for i, f := range tree.Folders {
		form.Folders[i].FolderArt = f.FolderArt
	}
	edit(&form)
	if _, err := db.UpdateUserCollection(context.Background(), owner, id, collectionRevision(t, db, id), form); err != nil {
		t.Fatalf("UpdateUserCollection: %v", err)
	}
	if _, err := db.PublishCollection(context.Background(), owner, id); err != nil {
		t.Fatalf("PublishCollection: %v", err)
	}
}

// mustUpdate runs profileID's Update of publicationID, failing the test on
// an error.
func mustUpdate(t *testing.T, db *DB, profileID, publicationID uuid.UUID) CommunityCopy {
	t.Helper()
	copied, err := db.UpdateSubscription(context.Background(), profileID, publicationID)
	if err != nil {
		t.Fatalf("UpdateSubscription: %v", err)
	}
	return copied
}

// updateAvailable is whether Community offers profileID an Update of
// publicationID.
func updateAvailable(t *testing.T, db *DB, profileID, publicationID uuid.UUID) bool {
	t.Helper()
	detail, err := db.GetPublication(context.Background(), profileID, publicationID)
	if err != nil {
		t.Fatal(err)
	}
	return detail.UpdateAvailable
}

// changeAspects is each change's kind and aspect, sorted.
func changeAspects(changes []SnapshotChange) []string {
	got := make([]string, len(changes))
	for i, c := range changes {
		got[i] = c.Kind + " " + c.Aspect
	}
	slices.Sort(got)
	return got
}
