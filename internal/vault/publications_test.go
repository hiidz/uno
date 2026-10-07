package vault

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

// Publishing a catalog freezes it as a snapshot under a stable key. The
// owner's later edits stay private, marking the row changed since
// publishing, until a republish, which keeps the publication's id and first
// publish time.
func TestPublishAndRepublishCatalog(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner, other := newTestProfile(t, db, "owner"), newTestProfile(t, db, "other")

	c := publishCatalog(t, db, owner, "Popular", `{"sort_by":"popularity.desc"}`)
	if p := c.Publication; p == nil || p.ChangedSincePublish {
		t.Fatalf("published catalog's publication = %+v, want one unchanged since", p)
	}
	first, err := db.GetPublication(ctx, other, c.Publication.ID)
	if err != nil {
		t.Fatalf("GetPublication: %v", err)
	}
	want := BundleCatalog{Key: stableKey(c.Publication.ID, c.ID), Name: "Popular", Type: "movie", Provider: "tmdb", Params: []byte(`{"sort_by":"popularity.desc"}`)}
	if got := first.Snapshot.Catalogs[0]; got.Key != want.Key || !sameCatalog(got, want) || first.Snapshot.Collection != nil {
		t.Errorf("snapshot = %+v, want just %+v", first.Snapshot, want)
	}
	if first.Kind != kindCatalog || first.Title != "Popular" || len(first.CatalogNames) != 1 || len(first.Folders) != 0 {
		t.Errorf("publication = %+v", first.CommunityItem)
	}

	form := listedCatalogForm("Renamed")
	form.Params = `{"sort_by":"popularity.desc"}`
	renamed, err := db.UpdateUserCatalog(ctx, owner, c.ID, form)
	if err != nil {
		t.Fatalf("UpdateUserCatalog: %v", err)
	}
	if !renamed.Publication.ChangedSincePublish {
		t.Error("an edited catalog reads as unchanged since publishing")
	}
	if still, _ := db.GetPublication(ctx, other, c.Publication.ID); still.Title != "Popular" {
		t.Errorf("publication title after a private edit = %q, want Popular", still.Title)
	}

	again, err := db.PublishCatalog(ctx, owner, c.ID)
	if err != nil {
		t.Fatalf("republish: %v", err)
	}
	if again.Publication.ID != c.Publication.ID || again.Publication.ChangedSincePublish {
		t.Errorf("republished publication = %+v, want id %s, unchanged", again.Publication, c.Publication.ID)
	}
	second, err := db.GetPublication(ctx, other, c.Publication.ID)
	if err != nil {
		t.Fatal(err)
	}
	if second.Title != "Renamed" || !second.PublishedAt.Equal(first.PublishedAt) || second.Snapshot.Catalogs[0].Key != want.Key {
		t.Errorf("republished = %+v, want Renamed under the same key, first published %s", second.CommunityItem, first.PublishedAt)
	}
}

// A collection's snapshot shares every catalog its folders reference, a
// private library catalog included: publishing is the consent to share it.
func TestPublishCollectionSharesEveryReferencedCatalog(t *testing.T) {
	db := newTestDB(t)
	owner, other := newTestProfile(t, db, "owner"), newTestProfile(t, db, "other")
	private, err := db.CreateUserCatalog(context.Background(), owner, listedCatalogForm("Private Picks"))
	if err != nil {
		t.Fatal(err)
	}
	series := newScoped("draft:s", "Ghosts", "{}")
	series.New.Type = "series"
	c := publishCollection(t, db, owner, CollectionForm{Title: "Weekend", ViewMode: "TABBED_GRID", Folders: []FolderData{
		{FolderArt: FolderArt{TileShape: "POSTER"}, Title: "A", Catalogs: []FolderCatalogRef{{CatalogID: &private.ID}, series}},
		{FolderArt: FolderArt{TileShape: "POSTER"}, Title: "B", Catalogs: []FolderCatalogRef{{CatalogID: &private.ID, Genre: "Drama"}}},
	}})
	detail, err := db.GetPublication(context.Background(), other, c.Publication.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertStrings(t, "snapshot catalogs", bundleCatalogKeys(detail.Snapshot.Catalogs), []string{
		stableKey(c.Publication.ID, private.ID) + "=Private Picks",
		stableKey(c.Publication.ID, c.Catalogs[1].ID) + "=Ghosts",
	})
	if len(detail.CatalogNames) != 2 || len(detail.Folders) != 2 || detail.Snapshot.Collection.Folders[1].Key != stableKey(c.Publication.ID, c.Folders[1].ID) {
		t.Errorf("publication = %+v, folders %+v", detail.CommunityItem, detail.Snapshot.Collection.Folders)
	}
	if c.Publication.ChangedSincePublish {
		t.Error("a just-published collection reads as changed since publishing")
	}
}

// A publish refuses what can't be shared: a catalog inside a collection, a
// subscribed copy and someone else's row. A refused publish writes nothing.
func TestPublishRefusals(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner, other := newTestProfile(t, db, "owner"), newTestProfile(t, db, "other")

	collectionID := newTestCollection(t, db, owner, "C")
	scoped := createScopedCatalog(t, db, owner, collectionID, listedCatalogForm("Scoped"))
	if _, err := db.PublishCatalog(ctx, owner, scoped.ID); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("publish a scoped catalog = %v, want ErrInvalidInput", err)
	}

	listed, err := db.CreateUserCatalog(ctx, owner, listedCatalogForm("Listed"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.PublishCatalog(ctx, other, listed.ID); !errors.Is(err, ErrCatalogNotFound) {
		t.Errorf("publish someone else's catalog = %v, want ErrCatalogNotFound", err)
	}
	if _, err := db.PublishCollection(ctx, other, collectionID); !errors.Is(err, ErrCollectionNotFound) {
		t.Errorf("publish someone else's collection = %v, want ErrCollectionNotFound", err)
	}
	if c := reloadCatalog(t, db, listed.ID); c.Publication != nil {
		t.Errorf("refused publishes wrote a publication: %+v", c.Publication)
	}

	source := publishCatalog(t, db, owner, "Source", "{}")
	copied := subscribe(t, db, other, source.Publication.ID)
	if _, err := db.PublishCatalog(ctx, other, copied.Catalog.ID); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("publish a subscribed catalog = %v, want ErrInvalidInput", err)
	}
	sourceCollection := publishCollection(t, db, owner, CollectionForm{Title: "Shared", ViewMode: "TABBED_GRID"})
	copiedCollection := subscribe(t, db, other, sourceCollection.Publication.ID)
	if _, err := db.PublishCollection(ctx, other, copiedCollection.Collection.ID); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("publish a subscribed collection = %v, want ErrInvalidInput", err)
	}
}

// Unpublishing deletes a publication. Its subscribers keep their copies as
// their own: no subscription, marked unpublished until their next save, and
// editable. Nobody can find the publication any more, and publishing the
// row again is a new publication nobody subscribes to.
func TestUnpublishReleasesSubscribers(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner, subscriber, stranger := newTestProfile(t, db, "owner"), newTestProfile(t, db, "subscriber"), newTestProfile(t, db, "stranger")

	c := publishCatalog(t, db, owner, "Popular", "{}")
	copied := subscribe(t, db, subscriber, c.Publication.ID)

	unpublished, err := db.UnpublishCatalog(ctx, owner, c.ID)
	if err != nil {
		t.Fatalf("UnpublishCatalog: %v", err)
	}
	if unpublished.Publication != nil {
		t.Errorf("unpublished catalog's publication = %+v, want none", unpublished.Publication)
	}
	if items := listCommunity(t, db, stranger); len(items) != 0 {
		t.Errorf("Community after an unpublish = %+v, want nothing", items)
	}
	released := reloadCatalog(t, db, copied.Catalog.ID)
	if released.Subscription != nil {
		t.Errorf("subscriber's copy's subscription = %+v, want none", released.Subscription)
	}
	for name, err := range map[string]error{
		"the subscriber's GetPublication": second(db.GetPublication(ctx, subscriber, c.Publication.ID)),
		"a stranger's Subscribe":          second(db.Subscribe(ctx, stranger, c.Publication.ID)),
		"the subscriber's Update":         second(db.UpdateSubscription(ctx, subscriber, c.Publication.ID)),
	} {
		if !errors.Is(err, ErrPublicationNotFound) {
			t.Errorf("%s of an unpublished publication = %v, want ErrPublicationNotFound", name, err)
		}
	}
	if _, err := db.UnpublishCatalog(ctx, stranger, c.ID); !errors.Is(err, ErrCatalogNotFound) {
		t.Errorf("unpublish someone else's catalog = %v, want ErrCatalogNotFound", err)
	}

	form := listedCatalogForm("Mine now")
	form.Params = released.Params
	saved, err := db.UpdateUserCatalog(ctx, subscriber, released.ID, form)
	if err != nil {
		t.Fatalf("save of the released copy: %v", err)
	}
	if saved.Name != "Mine now" {
		t.Errorf("released copy after a save = name %q, want renamed", saved.Name)
	}

	again, err := db.PublishCatalog(ctx, owner, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if again.Publication.ID == c.Publication.ID {
		t.Errorf("publishing again kept publication %s, want a new one", c.Publication.ID)
	}
	if got := subscriberCount(t, db, stranger, again.Publication.ID); got != 0 {
		t.Errorf("the new publication's subscribers = %d, want 0", got)
	}
	if got := reloadCatalog(t, db, copied.Catalog.ID); got.Subscription != nil {
		t.Errorf("released copy after publishing again = %+v, want it still its subscriber's own", got.Subscription)
	}
}

// Unpublishing a collection releases its subscribers' copies the same way,
// with no snapshot keys left on their folders and catalogs, and another
// profile can't unpublish it.
func TestUnpublishCollection(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner, subscriber := newTestProfile(t, db, "owner"), newTestProfile(t, db, "subscriber")
	source := publishCollection(t, db, owner, CollectionForm{Title: "Shared", ViewMode: "TABBED_GRID", Folders: []FolderData{{FolderArt: FolderArt{TileShape: "POSTER"}, Title: "F", Catalogs: []FolderCatalogRef{newScoped("k", "S", "{}")}}}})
	copied := subscribe(t, db, subscriber, source.Publication.ID).Collection

	if _, err := db.UnpublishCollection(ctx, subscriber, source.ID); !errors.Is(err, ErrCollectionNotFound) {
		t.Errorf("unpublish someone else's collection = %v, want ErrCollectionNotFound", err)
	}
	unpublished, err := db.UnpublishCollection(ctx, owner, source.ID)
	if err != nil || unpublished.Publication != nil {
		t.Fatalf("UnpublishCollection = %+v, %v; want no publication", unpublished.Publication, err)
	}
	released := mustOwnCollection(t, db, subscriber, copied.ID)
	if released.Subscription != nil {
		t.Errorf("subscriber's copy's subscription = %+v, want none", released.Subscription)
	}
	if f, c := released.Folders[0], released.Catalogs[0]; f.SubKey != "" || c.SubKey != "" {
		t.Errorf("released copy's keys = folder %q, catalog %q; want none", f.SubKey, c.SubKey)
	}

	if _, err := db.UpdateUserCollection(ctx, subscriber, copied.ID, saveFormOf(released)); err != nil {
		t.Fatalf("save of the released copy: %v", err)
	}
}

// second is the error of a two-value call.
func second[T any](_ T, err error) error { return err }

// Deleting a published source unpublishes it. The subscriber's copies
// survive as their own.
func TestDeletingASourceUnpublishes(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner, subscriber := newTestProfile(t, db, "owner"), newTestProfile(t, db, "subscriber")

	deleted := publishCatalog(t, db, owner, "Deleted", `{"sort_by":"revenue.desc"}`)
	collection := publishCollection(t, db, owner, CollectionForm{Title: "Gone", ViewMode: "TABBED_GRID"})
	copies := []CommunityCopy{
		subscribe(t, db, subscriber, deleted.Publication.ID),
		subscribe(t, db, subscriber, collection.Publication.ID),
	}

	if err := db.DeleteUserCatalog(ctx, owner, deleted.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteUserCollection(ctx, owner, collection.ID); err != nil {
		t.Fatal(err)
	}

	catalogCopy := reloadCatalog(t, db, copies[0].Catalog.ID)
	collectionCopy := mustOwnCollection(t, db, subscriber, copies[1].Collection.ID)
	if catalogCopy.Subscription != nil || collectionCopy.Subscription != nil {
		t.Errorf("copies' subscriptions = catalog %+v, collection %+v; want none", catalogCopy.Subscription, collectionCopy.Subscription)
	}
	for _, id := range []uuid.UUID{deleted.Publication.ID, collection.Publication.ID} {
		if _, err := db.GetPublication(ctx, subscriber, id); !errors.Is(err, ErrPublicationNotFound) {
			t.Errorf("GetPublication of a deleted source's publication = %v, want ErrPublicationNotFound", err)
		}
	}
}

// Deleting a subscribed copy, directly or by a collection's cascade, removes
// its subscription from the publication's count.
func TestSubscriberCountFollowsDeletes(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner, a, b := newTestProfile(t, db, "owner"), newTestProfile(t, db, "a"), newTestProfile(t, db, "b")
	c := publishCollection(t, db, owner, CollectionForm{Title: "Shared", ViewMode: "TABBED_GRID", Folders: []FolderData{{FolderArt: FolderArt{TileShape: "POSTER"}, Title: "F", Catalogs: []FolderCatalogRef{newScoped("k", "S", "{}")}}}})
	subscribe(t, db, a, c.Publication.ID)
	copyB := subscribe(t, db, b, c.Publication.ID)
	count := func() int {
		detail, err := db.GetPublication(ctx, owner, c.Publication.ID)
		if err != nil {
			t.Fatal(err)
		}
		return detail.SubscriberCount
	}
	if got := count(); got != 2 {
		t.Fatalf("subscriber count = %d, want 2", got)
	}
	if err := db.DeleteUserCollection(ctx, b, copyB.Collection.ID); err != nil {
		t.Fatal(err)
	}
	if got := count(); got != 1 {
		t.Errorf("subscriber count after a copy's delete = %d, want 1", got)
	}
}

// Community lists every live publication, even two of the same content:
// two profiles publishing one recipe, or one collection, are both listed.
func TestCommunityListsEveryLivePublication(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	a, b, viewer := newTestProfile(t, db, "a"), newTestProfile(t, db, "b"), newTestProfile(t, db, "viewer")
	first := publishCatalog(t, db, a, "Same Recipe", `{"sort_by":"revenue.desc"}`)
	publishCatalog(t, db, b, "Same Recipe Too", `{"sort_by":"revenue.desc"}`)
	form := CollectionForm{Title: "Twins", ViewMode: "TABBED_GRID", Folders: []FolderData{{FolderArt: FolderArt{TileShape: "POSTER"}, Title: "F", Catalogs: []FolderCatalogRef{newScoped("k", "S", `{"with_genres":"27"}`)}}}}
	publishCollection(t, db, a, form)
	publishCollection(t, db, b, form)

	for kind, want := range map[string]int{kindCatalog: 2, kindCollection: 2} {
		if got := countKind(listCommunity(t, db, viewer), kind); got != want {
			t.Errorf("%s publications listed = %d, want %d", kind, got, want)
		}
	}
	if _, err := db.UnpublishCatalog(ctx, a, first.ID); err != nil {
		t.Fatal(err)
	}
	if got := countKind(listCommunity(t, db, viewer), kindCatalog); got != 1 {
		t.Errorf("catalog publications listed after a unpublish = %d, want 1", got)
	}
}

// A collection that references a catalog its owner subscribes to publishes,
// with that catalog frozen as it stands. A profile that subscribes to the
// collection gets a scoped copy of it, which has no subscription of its own
// and is not counted among the original publication's subscribers.
func TestPublishAcceptsACollectionWithASubscribedCatalog(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	publisher, owner, subscriber := newTestProfile(t, db, "publisher"), newTestProfile(t, db, "owner"), newTestProfile(t, db, "subscriber")
	theirs := publishCatalog(t, db, publisher, "Theirs", "{}")
	added := subscribe(t, db, owner, theirs.Publication.ID).Catalog
	collection, err := db.CreateUserCollection(ctx, owner, CollectionForm{Title: "Mine", ViewMode: "TABBED_GRID", Folders: []FolderData{
		{FolderArt: FolderArt{TileShape: "POSTER"}, Title: "F", Catalogs: CatalogRefs(added.ID)},
	}})
	if err != nil {
		t.Fatal(err)
	}
	published, err := db.PublishCollection(ctx, owner, collection.ID)
	if err != nil {
		t.Fatalf("publish a collection with a subscribed catalog = %v, want nil", err)
	}
	if got := reloadCatalog(t, db, added.ID).Subscription; got == nil {
		t.Error("publishing the collection dropped the subscription of the catalog it uses")
	}

	copied := subscribe(t, db, subscriber, published.Publication.ID).Collection
	if len(copied.Catalogs) != 1 || copied.Catalogs[0].Name != "Theirs" {
		t.Fatalf("the subscriber's copy holds %+v, want one catalog named Theirs", copied.Catalogs)
	}
	scoped := copied.Catalogs[0]
	if scoped.CollectionID == nil || *scoped.CollectionID != copied.ID || scoped.Subscription != nil {
		t.Errorf("the subscriber's catalog = collection %v, subscription %+v, want scoped to its collection and no subscription", scoped.CollectionID, scoped.Subscription)
	}
	detail, err := db.GetPublication(ctx, publisher, theirs.Publication.ID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.SubscriberCount != 1 {
		t.Errorf("the original publication's subscribers = %d, want 1 (the owner's)", detail.SubscriberCount)
	}
}

// The sharing state is read for the owner's own lists and rows, the
// catalogs of their collection trees included, and not for the addon's
// published set or push's reads by id, which never show it.
func TestOnlyOwnReadsCarrySharingState(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner := newTestProfile(t, db, "owner")
	c := publishCatalog(t, db, owner, "Popular", "{}")
	coll := publishCollection(t, db, owner, CollectionForm{Title: "C", ViewMode: "TABBED_GRID", Folders: []FolderData{{FolderArt: FolderArt{TileShape: "POSTER"}, Title: "F", Catalogs: CatalogRefs(c.ID)}}})
	savePush(t, db, owner, PushedHome{Catalogs: []SelectedCatalogInput{{CatalogID: c.ID, ShowInHome: true}}})

	own, err := db.GetUserCatalogs(ctx, owner)
	if err != nil || own[0].Publication == nil {
		t.Errorf("GetUserCatalogs = %+v, %v; want the publication", own, err)
	}
	ownColls, err := db.GetUserCollections(ctx, owner)
	if err != nil || ownColls[0].Publication == nil || ownColls[0].Catalogs[0].Publication == nil {
		t.Errorf("GetUserCollections = %+v, %v; want the collection's and its catalog's publications", ownColls, err)
	}
	tree := mustOwnCollection(t, db, owner, coll.ID)
	if tree.Catalogs[0].Publication == nil {
		t.Errorf("own tree's catalogs = %+v, want the catalog's publication", tree.Catalogs)
	}
	byID, err := db.GetCatalogsByIDs(ctx, []uuid.UUID{c.ID})
	if err != nil || byID[0].Publication != nil {
		t.Errorf("GetCatalogsByIDs = %+v, %v; want no sharing state", byID, err)
	}
	collsByID, err := db.GetCollectionsByIDs(ctx, []uuid.UUID{coll.ID})
	if err != nil || collsByID[0].Publication != nil || collsByID[0].Catalogs[0].Publication != nil {
		t.Errorf("GetCollectionsByIDs = %+v, %v; want no sharing state", collsByID, err)
	}
	published, err := db.GetPublishedCatalogs(ctx, owner)
	if err != nil || len(published) != 1 || published[0].Publication != nil {
		t.Errorf("GetPublishedCatalogs = %+v, %v; want the catalog with no sharing state", published, err)
	}
}

// listCommunity is every row Community lists for profileID, newest first, the
// catalogs then the collections, failing the test on an error.
func listCommunity(t *testing.T, db *DB, profileID uuid.UUID) []CommunityItem {
	t.Helper()
	catalogs := communityPages(t, db, profileID, CommunityQuery{Kind: kindCatalog, Sort: "newest"})
	return append(catalogs, communityPages(t, db, profileID, CommunityQuery{Kind: kindCollection, Sort: "newest"})...)
}

// communityPages is every page of q, read cursor by cursor, failing the test
// on an error.
func communityPages(t *testing.T, db *DB, profileID uuid.UUID, q CommunityQuery) []CommunityItem {
	t.Helper()
	items := []CommunityItem{}
	for {
		page, err := db.ListCommunity(context.Background(), profileID, q)
		if err != nil {
			t.Fatalf("ListCommunity(%+v): %v", q, err)
		}
		items = append(items, page.Items...)
		if page.NextCursor == nil {
			return items
		}
		q.Cursor = *page.NextCursor
	}
}

// countKind is how many of items are of kind.
func countKind(items []CommunityItem, kind string) int {
	n := 0
	for _, item := range items {
		if item.Kind == kind {
			n++
		}
	}
	return n
}

// sameCatalog reports whether a and b have the same name and recipe.
func sameCatalog(a, b BundleCatalog) bool {
	return a.Name == b.Name && a.Type == b.Type && a.Provider == b.Provider && string(a.Params) == string(b.Params)
}
