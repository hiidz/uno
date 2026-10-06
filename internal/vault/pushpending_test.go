package vault

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

// pendingFixture is a profile whose last push put in Nuvio: a catalog on Home,
// a collection using a listed catalog and one scoped to it.
type pendingFixture struct {
	db                  *DB
	owner               uuid.UUID
	home, listed        Catalog
	scoped              Catalog
	collection          uuid.UUID
	selection           PushedHome
	homeParams, changed string
}

func newPendingFixture(t *testing.T) pendingFixture {
	t.Helper()
	ctx := context.Background()
	f := pendingFixture{db: newTestDB(t), homeParams: `{"sort_by":"popularity.desc"}`, changed: `{"sort_by":"vote_average.desc"}`}
	f.owner = newTestProfile(t, f.db, "owner")
	create := func(name, params string) Catalog {
		t.Helper()
		form := listedCatalogForm(name)
		form.Params = params
		c, err := f.db.CreateUserCatalog(ctx, f.owner, form)
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	f.home = create("On Home", f.homeParams)
	f.listed = create("In a folder", "{}")
	f.collection = newTestCollection(t, f.db, f.owner, "Pushed")
	f.scoped = createScopedCatalog(t, f.db, f.owner, f.collection, listedCatalogForm("Scoped"))
	if _, err := f.db.UpdateUserCollection(ctx, f.owner, f.collection, CollectionForm{
		Title:   "Pushed",
		Folders: []FolderData{{Title: "F", Catalogs: CatalogRefs(f.listed.ID, f.scoped.ID)}},
	}); err != nil {
		t.Fatal(err)
	}
	f.selection = PushedHome{
		Catalogs:    []SelectedCatalogInput{{CatalogID: f.home.ID, ShowInHome: true}},
		Collections: []SelectedCollectionInput{{CollectionID: f.collection}},
	}
	savePush(t, f.db, f.owner, f.selection)
	return f
}

func (f pendingFixture) push(t *testing.T) {
	t.Helper()
	savePush(t, f.db, f.owner, f.selection)
}

// rename saves catalog c as name with params, listed.
func (f pendingFixture) rename(t *testing.T, c Catalog, name, params string) {
	t.Helper()
	form := listedCatalogForm(name)
	form.Params = params
	if _, err := f.db.UpdateUserCatalog(context.Background(), f.owner, c.ID, form); err != nil {
		t.Fatal(err)
	}
}

func (f pendingFixture) pending(t *testing.T) []PendingChange {
	t.Helper()
	changes, err := f.db.PendingPush(context.Background(), f.owner)
	if err != nil {
		t.Fatalf("PendingPush: %v", err)
	}
	return changes
}

func wantPending(t *testing.T, got []PendingChange, want ...PendingChange) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("pending = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("pending[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// Right after a push nothing waits, and an edit and its undo is no change.
func TestNothingWaitsAfterAPush(t *testing.T) {
	f := newPendingFixture(t)
	if got := f.pending(t); got == nil || len(got) != 0 {
		t.Fatalf("pending after a push = %#v, want an empty list", got)
	}
	f.rename(t, f.home, "Renamed", f.homeParams)
	f.rename(t, f.home, "On Home", f.homeParams)
	if got := f.pending(t); len(got) != 0 {
		t.Errorf("pending after a rename and back = %+v, want none", got)
	}
}

// A catalog on Home edited since the push waits as changed, by its new name.
func TestACatalogEditWaitsForAPush(t *testing.T) {
	f := newPendingFixture(t)
	f.rename(t, f.home, "On Home", f.changed)
	wantPending(t, f.pending(t), PendingChange{pendingCatalog, f.home.ID, "On Home", PendingChanged})

	f.push(t)
	if got := f.pending(t); len(got) != 0 {
		t.Errorf("pending after the next push = %+v, want none", got)
	}
}

// A catalog a folder uses, listed or scoped, edited since the push shows
// against its collection: that is what Nuvio shows it through.
func TestAFolderCatalogEditWaitsAsItsCollection(t *testing.T) {
	ctx := context.Background()
	f := newPendingFixture(t)
	f.rename(t, f.listed, "In a folder", f.changed)
	want := PendingChange{pendingCollection, f.collection, "Pushed", PendingChanged}
	wantPending(t, f.pending(t), want)

	f.rename(t, f.listed, "In a folder", "{}")
	if _, err := f.db.UpdateUserCollection(ctx, f.owner, f.collection, CollectionForm{
		Title:        "Pushed",
		Folders:      []FolderData{{Title: "F", Catalogs: CatalogRefs(f.listed.ID, f.scoped.ID)}},
		CatalogEdits: []ScopedCatalogEdit{{ID: f.scoped.ID, Type: "movie", Provider: "tmdb", Name: "Scoped", Params: f.changed}},
	}); err != nil {
		t.Fatal(err)
	}
	wantPending(t, f.pending(t), want)
}

// A row deleted since the push waits as removed, by the name the push left,
// and a collection that lost a deleted catalog from its folders as changed.
func TestADeleteWaitsForAPush(t *testing.T) {
	ctx := context.Background()
	f := newPendingFixture(t)
	if err := f.db.DeleteUserCatalog(ctx, f.owner, f.home.ID); err != nil {
		t.Fatalf("deleting a catalog on Home: %v", err)
	}
	if err := f.db.DeleteUserCatalog(ctx, f.owner, f.listed.ID); err != nil {
		t.Fatalf("deleting a catalog a collection on Home uses: %v", err)
	}
	wantPending(t, f.pending(t),
		PendingChange{pendingCatalog, f.home.ID, "On Home", PendingRemoved},
		PendingChange{pendingCollection, f.collection, "Pushed", PendingChanged})

	if err := f.db.DeleteUserCollection(ctx, f.owner, f.collection); err != nil {
		t.Fatalf("deleting a collection on Home: %v", err)
	}
	wantPending(t, f.pending(t),
		PendingChange{pendingCatalog, f.home.ID, "On Home", PendingRemoved},
		PendingChange{pendingCollection, f.collection, "Pushed", PendingRemoved})

	if err := f.db.DeleteUserCollection(ctx, f.owner, f.collection); !errors.Is(err, ErrCollectionNotFound) {
		t.Errorf("deleting it again = %v, want ErrCollectionNotFound", err)
	}
}

// The addon serves what the last push left: after an edit, a delete and an
// Update of a catalog added from Community, until the next push.
func TestAddonReadsHoldUntilThePush(t *testing.T) {
	ctx := context.Background()
	f := newPendingFixture(t)
	token := profileToken(t, f.db, f.owner)

	publisher := newTestProfile(t, f.db, "publisher")
	source := publishCatalog(t, f.db, publisher, "From Community", `{"sort_by":"popularity.desc"}`)
	added := subscribe(t, f.db, f.owner, source.Publication.ID).Catalog
	f.selection.Catalogs = append(f.selection.Catalogs, SelectedCatalogInput{CatalogID: added.ID, ShowInHome: true})
	f.push(t)

	f.rename(t, f.home, "Edited", f.changed)
	form := listedCatalogForm("From Community, updated")
	form.Params = f.changed
	if _, err := f.db.UpdateUserCatalog(ctx, publisher, source.ID, form); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.PublishCatalog(ctx, publisher, source.ID, allowAnyCatalogParams); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.UpdateSubscription(ctx, f.owner, source.Publication.ID); err != nil {
		t.Fatalf("UpdateSubscription: %v", err)
	}
	if err := f.db.DeleteUserCatalog(ctx, f.owner, f.listed.ID); err != nil {
		t.Fatal(err)
	}

	published, err := f.db.GetPublishedCatalogs(ctx, f.owner)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, len(published))
	for i, p := range published {
		names[i] = p.Name
	}
	if want := []string{"On Home", "From Community", "In a folder", "Scoped"}; !equalStrings(names, want) {
		t.Errorf("manifest lists %v, want what the last push left, %v", names, want)
	}
	for _, c := range []struct {
		id     uuid.UUID
		params string
	}{{f.home.ID, f.homeParams}, {added.ID, `{"sort_by":"popularity.desc"}`}, {f.listed.ID, "{}"}} {
		served, err := f.db.ServedCatalog(ctx, token, c.id, "movie", "tmdb")
		if err != nil || served.Params != c.params {
			t.Errorf("ServedCatalog %s = %+v, %v; want params %s as pushed", c.id, served, err, c.params)
		}
	}

	f.selection.Catalogs = f.selection.Catalogs[:2]
	f.push(t)
	served, err := f.db.ServedCatalog(ctx, token, f.home.ID, "movie", "tmdb")
	if err != nil || served.Params != f.changed {
		t.Errorf("after the push ServedCatalog = %+v, %v; want the edited params", served, err)
	}
	if _, err := f.db.ServedCatalog(ctx, token, f.listed.ID, "movie", "tmdb"); !errors.Is(err, ErrCatalogNotFound) {
		t.Errorf("after the push the deleted catalog = %v, want ErrCatalogNotFound", err)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// A slot reused by a new Nuvio profile leaves the record describing a profile
// that is gone: Nuvio holds nothing, so the addon serves nothing, every row on
// Home waits for a push, and a push drops no collection by the old record. The
// library and Home layout are kept; the next push stores a fresh record.
func TestAReusedSlotHoldsNothing(t *testing.T) {
	ctx := context.Background()
	f := newPendingFixture(t)
	token := profileToken(t, f.db, f.owner)
	if _, err := f.db.ResolveOrCreateProfile(ctx, "owner", 1, "a-new-nuvio-profile"); err != nil {
		t.Fatal(err)
	}

	published, err := f.db.GetPublishedCatalogs(ctx, f.owner)
	if err != nil || published == nil || len(published) != 0 {
		t.Errorf("manifest = %#v, %v; want an empty list", published, err)
	}
	if _, err := f.db.ServedCatalog(ctx, token, f.home.ID, "movie", "tmdb"); !errors.Is(err, ErrCatalogNotFound) {
		t.Errorf("ServedCatalog = %v, want ErrCatalogNotFound", err)
	}
	ids, err := f.db.PushedCollectionIDs(ctx, f.owner)
	if err != nil || len(ids) != 0 {
		t.Errorf("PushedCollectionIDs = %v, %v; want none", ids, err)
	}
	wantPending(t, f.pending(t),
		PendingChange{pendingCatalog, f.home.ID, "On Home", PendingAdded},
		PendingChange{pendingCollection, f.collection, "Pushed", PendingAdded})
	if cats, err := f.db.GetUserCatalogs(ctx, f.owner); err != nil || len(cats) != 2 {
		t.Errorf("library = %d catalogs (%v), want it kept", len(cats), err)
	}

	f.push(t)
	if got := f.pending(t); len(got) != 0 {
		t.Errorf("pending after the first push = %+v, want none", got)
	}
	if _, err := f.db.ServedCatalog(ctx, token, f.home.ID, "movie", "tmdb"); err != nil {
		t.Errorf("ServedCatalog after the first push = %v", err)
	}
}

// A profile that never pushed holds nothing: an empty manifest, no catalog
// served, nothing waiting, and no pushed collections to drop.
func TestAProfileThatNeverPushedHoldsNothing(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner := newTestProfile(t, db, "owner")
	c, err := db.CreateUserCatalog(ctx, owner, listedCatalogForm("Library only"))
	if err != nil {
		t.Fatal(err)
	}
	if published, err := db.GetPublishedCatalogs(ctx, owner); err != nil || published == nil || len(published) != 0 {
		t.Errorf("manifest = %#v, %v; want an empty list", published, err)
	}
	if _, err := db.ServedCatalog(ctx, profileToken(t, db, owner), c.ID, "movie", "tmdb"); !errors.Is(err, ErrCatalogNotFound) {
		t.Errorf("ServedCatalog = %v, want ErrCatalogNotFound", err)
	}
	if changes, err := db.PendingPush(ctx, owner); err != nil || changes == nil || len(changes) != 0 {
		t.Errorf("pending = %#v, %v; want an empty list", changes, err)
	}
	if ids, err := db.PushedCollectionIDs(ctx, owner); err != nil || len(ids) != 0 {
		t.Errorf("PushedCollectionIDs = %v, %v; want none", ids, err)
	}
}

// PushedCollectionIDs names every collection the last push sent, one with no
// folders included, which push's merge can't tell from a Nuvio-native one.
func TestPushedCollectionIDsIncludeAnEmptyCollection(t *testing.T) {
	ctx := context.Background()
	f := newPendingFixture(t)
	empty := newTestCollection(t, f.db, f.owner, "Empty")
	f.selection.Collections = append(f.selection.Collections, SelectedCollectionInput{CollectionID: empty})
	f.push(t)
	if err := f.db.DeleteUserCollection(ctx, f.owner, empty); err != nil {
		t.Fatal(err)
	}
	ids, err := f.db.PushedCollectionIDs(ctx, f.owner)
	if err != nil || len(ids) != 2 || ids[1] != empty {
		t.Errorf("PushedCollectionIDs = %v, %v; want the collection and the deleted empty one", ids, err)
	}
}
