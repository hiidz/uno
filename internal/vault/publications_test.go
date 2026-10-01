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
	if p := c.Publication; p == nil || p.Status != statusLive || p.ChangedSincePublish {
		t.Fatalf("published catalog's publication = %+v, want a live one unchanged since", p)
	}
	first, err := db.GetPublication(ctx, other, c.Publication.ID)
	if err != nil {
		t.Fatalf("GetPublication: %v", err)
	}
	want := BundleCatalog{Key: stableKey(c.Publication.ID, c.ID), Name: "Popular", Type: "movie", Provider: "tmdb", Params: []byte(`{"sort_by":"popularity.desc"}`)}
	if got := first.Snapshot.Catalogs[0]; got.Key != want.Key || !sameCatalog(got, want) || first.Snapshot.Collection != nil {
		t.Errorf("snapshot = %+v, want just %+v", first.Snapshot, want)
	}
	if first.Kind != kindCatalog || first.Title != "Popular" || first.CatalogCount != 1 || first.FolderCount != 0 {
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

	again, err := db.PublishCatalog(ctx, owner, c.ID, allowAnyCatalogParams)
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
	c := publishCollection(t, db, owner, CollectionForm{Title: "Weekend", Folders: []FolderData{
		{Title: "A", Catalogs: []FolderCatalogRef{{CatalogID: &private.ID}, series}},
		{Title: "B", Catalogs: []FolderCatalogRef{{CatalogID: &private.ID, Genre: "Drama"}}},
	}})
	detail, err := db.GetPublication(context.Background(), other, c.Publication.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertStrings(t, "snapshot catalogs", bundleCatalogKeys(detail.Snapshot.Catalogs), []string{
		stableKey(c.Publication.ID, private.ID) + "=Private Picks",
		stableKey(c.Publication.ID, c.Catalogs[1].ID) + "=Ghosts",
	})
	if detail.CatalogCount != 2 || detail.FolderCount != 2 || detail.Snapshot.Collection.Folders[1].Key != stableKey(c.Publication.ID, c.Folders[1].ID) {
		t.Errorf("publication = %+v, folders %+v", detail.CommunityItem, detail.Snapshot.Collection.Folders)
	}
	if c.Publication.ChangedSincePublish {
		t.Error("a just-published collection reads as changed since publishing")
	}
}

// A publish refuses what can't be shared: a catalog inside a collection, a
// subscribed copy, someone else's row, a recipe the validator refuses, and a
// source edited while it was being checked. A refused publish writes
// nothing, and a nil validator is a programming error.
func TestPublishRefusals(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner, other := newTestProfile(t, db, "owner"), newTestProfile(t, db, "other")

	collectionID := newTestCollection(t, db, owner, "C")
	scopedForm := listedCatalogForm("Scoped")
	scopedForm.CollectionID = &collectionID
	scoped, err := db.CreateUserCatalog(ctx, owner, scopedForm)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.PublishCatalog(ctx, owner, scoped.ID, allowAnyCatalogParams); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("publish a scoped catalog = %v, want ErrInvalidInput", err)
	}

	listed, err := db.CreateUserCatalog(ctx, owner, listedCatalogForm("Listed"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.PublishCatalog(ctx, other, listed.ID, allowAnyCatalogParams); !errors.Is(err, ErrCatalogNotFound) {
		t.Errorf("publish someone else's catalog = %v, want ErrCatalogNotFound", err)
	}
	if _, err := db.PublishCollection(ctx, other, collectionID, allowAnyCatalogParams); !errors.Is(err, ErrCollectionNotFound) {
		t.Errorf("publish someone else's collection = %v, want ErrCollectionNotFound", err)
	}
	if _, err := db.PublishCatalog(ctx, owner, listed.ID, nil); err == nil || errors.Is(err, ErrInvalidInput) {
		t.Errorf("publish with no validator = %v, want a programming error", err)
	}
	refused := errors.New("recipe refused")
	if _, err := db.PublishCatalog(ctx, owner, listed.ID, func(_, _, _ string) error { return refused }); !errors.Is(err, refused) {
		t.Errorf("publish with a refusing validator = %v, want its error", err)
	}
	editMeanwhile := func(_, _, _ string) error {
		_, err := db.UpdateUserCatalog(ctx, owner, listed.ID, listedCatalogForm("Edited meanwhile"))
		return err
	}
	if _, err := db.PublishCatalog(ctx, owner, listed.ID, editMeanwhile); !errors.Is(err, ErrConflict) {
		t.Errorf("publish of a catalog edited while checked = %v, want ErrConflict", err)
	}
	if c := reloadCatalog(t, db, listed.ID); c.Publication != nil {
		t.Errorf("refused publishes wrote a publication: %+v", c.Publication)
	}

	source := publishCatalog(t, db, owner, "Source", "{}")
	copied := subscribe(t, db, other, source.Publication.ID)
	if _, err := db.PublishCatalog(ctx, other, copied.Catalog.ID, allowAnyCatalogParams); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("publish a subscribed catalog = %v, want ErrInvalidInput", err)
	}
	sourceCollection := publishCollection(t, db, owner, CollectionForm{Title: "Shared"})
	copiedCollection := subscribe(t, db, other, sourceCollection.Publication.ID)
	if _, err := db.PublishCollection(ctx, other, copiedCollection.Collection.ID, allowAnyCatalogParams); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("publish a subscribed collection = %v, want ErrInvalidInput", err)
	}
}

// Unpublishing takes a publication out of Community. Its subscribers keep
// their copies, marked unpublished with no update to take, and still see its
// last snapshot; nobody else can find it. Publishing again revives the same
// publication.
func TestUnpublishAndRevive(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner, subscriber, stranger := newTestProfile(t, db, "owner"), newTestProfile(t, db, "subscriber"), newTestProfile(t, db, "stranger")

	c := publishCatalog(t, db, owner, "Popular", "{}")
	copied := subscribe(t, db, subscriber, c.Publication.ID)

	unpublished, err := db.UnpublishCatalog(ctx, owner, c.ID)
	if err != nil {
		t.Fatalf("UnpublishCatalog: %v", err)
	}
	if unpublished.Publication.Status != "unpublished" {
		t.Errorf("unpublished catalog's publication = %+v", unpublished.Publication)
	}
	if items := listCommunity(t, db, stranger); len(items) != 0 {
		t.Errorf("Community after a unpublish = %+v, want nothing", items)
	}
	if got := reloadCatalog(t, db, copied.Catalog.ID).Subscription; got == nil || !got.Unpublished || got.UpdateAvailable {
		t.Errorf("subscriber's copy subscription = %+v, want unpublished with no update", got)
	}
	if detail, err := db.GetPublication(ctx, subscriber, c.Publication.ID); err != nil || !detail.Unpublished {
		t.Errorf("subscriber's GetPublication = %+v, %v; want the unpublished publication", detail.CommunityItem, err)
	}
	for name, err := range map[string]error{
		"a stranger's GetPublication": second(db.GetPublication(ctx, stranger, c.Publication.ID)),
		"a stranger's Subscribe":      second(db.Subscribe(ctx, stranger, c.Publication.ID)),
		"the subscriber's Update":     second(db.UpdateSubscription(ctx, subscriber, c.Publication.ID)),
	} {
		if !errors.Is(err, ErrPublicationNotFound) {
			t.Errorf("%s of a unpublished publication = %v, want ErrPublicationNotFound", name, err)
		}
	}
	if _, err := db.UnpublishCatalog(ctx, stranger, c.ID); !errors.Is(err, ErrCatalogNotFound) {
		t.Errorf("unpublish someone else's catalog = %v, want ErrCatalogNotFound", err)
	}

	revived, err := db.PublishCatalog(ctx, owner, c.ID, allowAnyCatalogParams)
	if err != nil {
		t.Fatal(err)
	}
	if revived.Publication.ID != c.Publication.ID || revived.Publication.Status != statusLive {
		t.Errorf("revived publication = %+v, want %s live again", revived.Publication, c.Publication.ID)
	}
	if got := reloadCatalog(t, db, copied.Catalog.ID).Subscription; got.Unpublished {
		t.Errorf("subscription after the revival = %+v, want it live", got)
	}
}

// Unpublishing a collection's publication marks its subscribers' copies
// unpublished, and another profile can't unpublish it.
func TestUnpublishCollection(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner, subscriber := newTestProfile(t, db, "owner"), newTestProfile(t, db, "subscriber")
	source := publishCollection(t, db, owner, CollectionForm{Title: "Shared"})
	copied := subscribe(t, db, subscriber, source.Publication.ID).Collection

	unpublished, err := db.UnpublishCollection(ctx, owner, source.ID)
	if err != nil || unpublished.Publication.Status != "unpublished" {
		t.Fatalf("UnpublishCollection = %+v, %v; want its publication unpublished", unpublished.Publication, err)
	}
	if s := mustOwnCollection(t, db, subscriber, copied.ID).Subscription; s == nil || !s.Unpublished {
		t.Errorf("subscriber's copy subscription = %+v, want unpublished", s)
	}
	if _, err := db.UnpublishCollection(ctx, subscriber, source.ID); !errors.Is(err, ErrCollectionNotFound) {
		t.Errorf("unpublish someone else's collection = %v, want ErrCollectionNotFound", err)
	}
}

// second is the error of a two-value call.
func second[T any](_ T, err error) error { return err }

// Deleting a published source unpublishes its publication. The subscriber's
// copy survives, marked unpublished.
func TestDeletingASourceUnpublishes(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner, subscriber := newTestProfile(t, db, "owner"), newTestProfile(t, db, "subscriber")

	deleted := publishCatalog(t, db, owner, "Deleted", `{"sort_by":"revenue.desc"}`)
	collection := publishCollection(t, db, owner, CollectionForm{Title: "Gone"})
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

	for i, sub := range []*SubscriptionState{
		reloadCatalog(t, db, copies[0].Catalog.ID).Subscription,
		mustOwnCollection(t, db, subscriber, copies[1].Collection.ID).Subscription,
	} {
		if sub == nil || !sub.Unpublished {
			t.Errorf("copy %d subscription = %+v, want unpublished", i, sub)
		}
	}
}

// Deleting a subscribed copy, directly or by a collection's cascade, removes
// its subscription from the publication's count.
func TestSubscriberCountFollowsDeletes(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner, a, b := newTestProfile(t, db, "owner"), newTestProfile(t, db, "a"), newTestProfile(t, db, "b")
	c := publishCollection(t, db, owner, CollectionForm{Title: "Shared", Folders: []FolderData{{Title: "F", Catalogs: []FolderCatalogRef{newScoped("k", "S", "{}")}}}})
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
	form := CollectionForm{Title: "Twins", Folders: []FolderData{{Title: "F", Catalogs: []FolderCatalogRef{newScoped("k", "S", `{"with_genres":"27"}`)}}}}
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
	collection, err := db.CreateUserCollection(ctx, owner, CollectionForm{Title: "Mine", Folders: []FolderData{
		{Title: "F", Catalogs: CatalogRefs(added.ID)},
	}})
	if err != nil {
		t.Fatal(err)
	}
	published, err := db.PublishCollection(ctx, owner, collection.ID, allowAnyCatalogParams)
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

// A publish checks each distinct recipe it shares once, however many of
// its catalogs ask for it.
func TestPublishChecksEachRecipeOnce(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner := newTestProfile(t, db, "owner")
	c, err := db.CreateUserCollection(ctx, owner, CollectionForm{Title: "C", Folders: []FolderData{{Title: "F", Catalogs: []FolderCatalogRef{
		newScoped("a", "A", `{"with_genres":"27"}`), newScoped("b", "B", `{"with_genres":"27"}`), newScoped("c", "C", `{"with_genres":"35"}`),
	}}}})
	if err != nil {
		t.Fatal(err)
	}
	checked := map[string]int{}
	count := func(_, _, params string) error {
		checked[params]++
		return nil
	}
	if _, err := db.PublishCollection(ctx, owner, c.ID, count); err != nil {
		t.Fatal(err)
	}
	if len(checked) != 2 || checked[`{"with_genres":"27"}`] != 1 {
		t.Errorf("recipes checked = %v, want each of the two once", checked)
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
	coll := publishCollection(t, db, owner, CollectionForm{Title: "C", Folders: []FolderData{{Title: "F", Catalogs: CatalogRefs(c.ID)}}})
	savePush(t, db, owner, CatalogSelectionForm{Catalogs: []SelectedCatalogInput{{CatalogID: c.ID, ShowInHome: true}}}, CollectionSelectionForm{})

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

// listCommunity is ListCommunity, failing the test on an error.
func listCommunity(t *testing.T, db *DB, profileID uuid.UUID) []CommunityItem {
	t.Helper()
	items, err := db.ListCommunity(context.Background(), profileID)
	if err != nil {
		t.Fatalf("ListCommunity: %v", err)
	}
	return items
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
