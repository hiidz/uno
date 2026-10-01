package vault

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"testing"

	"github.com/google/uuid"
)

// saveFormOf is the form that saves tree as it stands: its fields, its
// folders by id, and every ref as a CatalogID ref.
func saveFormOf(tree CollectionWithFolders) CollectionForm {
	form := CollectionForm{
		Title: tree.Title, ViewMode: tree.ViewMode, ShowAllTab: tree.ShowAllTab,
		BackdropImageURL: tree.BackdropImageURL, FocusGlowEnabled: tree.FocusGlowEnabled,
	}
	for _, f := range tree.Folders {
		id := f.ID
		fd := FolderData{ID: &id, Title: f.Title, TileShape: f.TileShape, FocusGIFEnabled: f.FocusGIFEnabled}
		for _, ref := range f.Refs {
			catalogID := ref.CatalogID
			fd.Catalogs = append(fd.Catalogs, FolderCatalogRef{CatalogID: &catalogID, Genre: ref.Genre})
		}
		form.Folders = append(form.Folders, fd)
	}
	return form
}

// catalogNamed is the catalog of tree named name.
func catalogNamed(t *testing.T, tree CollectionWithFolders, name string) Catalog {
	t.Helper()
	i := slices.IndexFunc(tree.Catalogs, func(c Catalog) bool { return c.Name == name })
	if i < 0 {
		t.Fatalf("no catalog named %q in %+v", name, tree.Catalogs)
	}
	return tree.Catalogs[i]
}

// Subscribing to a catalog writes it as the subscriber's own listed
// catalog: unpublished, off Home, on the publication's recipe, subscribed
// and in step. It makes no TMDB call: the vault is given no validator.
func TestSubscribeCatalog(t *testing.T) {
	db := newTestDB(t)
	owner, subscriber := newTestProfile(t, db, "owner"), newTestProfile(t, db, "subscriber")
	source := publishCatalog(t, db, owner, "Popular", `{"sort_by":"popularity.desc"}`)

	copied := subscribe(t, db, subscriber, source.Publication.ID)
	c := copied.Catalog
	if copied.Kind != kindCatalog || c == nil || copied.Collection != nil {
		t.Fatalf("copy = %+v, want a catalog", copied)
	}
	if c.ID == source.ID || c.OwnerID != subscriber || c.CollectionID != nil || c.HomeSortOrder != nil || c.Publication != nil {
		t.Errorf("copy = %+v, want the subscriber's own listed, unpublished row off Home", c)
	}
	if c.Name != "Popular" || c.RecipeHash != source.RecipeHash || c.SubKey != "" {
		t.Errorf("copy name %q, recipe %s, sub_key %q; want the source's name and recipe, no sub_key", c.Name, c.RecipeHash, c.SubKey)
	}
	if s := c.Subscription; s == nil || s.PublicationID != source.Publication.ID || s.UpdateAvailable || s.Withdrawn {
		t.Errorf("subscription = %+v, want one to %s, in step", s, source.Publication.ID)
	}
}

// Subscribing to a collection writes every catalog as scoped to the copy,
// one per snapshot catalog however many refs name it, and every catalog and
// folder carries its snapshot key as sub_key. The copy starts at version 1,
// never pushed, with the publication's title.
func TestSubscribeCollection(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner, subscriber := newTestProfile(t, db, "owner"), newTestProfile(t, db, "subscriber")
	listed, err := db.CreateUserCatalog(ctx, owner, listedCatalogForm("Listed"))
	if err != nil {
		t.Fatal(err)
	}
	source := publishCollection(t, db, owner, CollectionForm{Title: "Weekend", Folders: []FolderData{
		{Title: "A", Catalogs: []FolderCatalogRef{{CatalogID: &listed.ID}, newScoped("k", "Scoped", `{"with_genres":"27"}`)}},
		{Title: "B", Catalogs: []FolderCatalogRef{{CatalogID: &listed.ID, Genre: "War"}}},
	}})
	pubID := source.Publication.ID

	c := *subscribe(t, db, subscriber, pubID).Collection
	if c.Title != "Weekend" || c.pushedHash != "" || c.Publication != nil || c.Subscription == nil || c.Subscription.PublicationID != pubID {
		t.Errorf("copy = %+v, want Weekend unpushed, subscribed to %s", c.Collection, pubID)
	}
	if len(c.Catalogs) != 2 {
		t.Fatalf("copy catalogs = %+v, want two", c.Catalogs)
	}
	for _, catalog := range c.Catalogs {
		if catalog.CollectionID == nil || *catalog.CollectionID != c.ID || catalog.OwnerID != subscriber {
			t.Errorf("copy catalog %+v, want it scoped to the copy", catalog)
		}
	}
	if got := catalogNamed(t, c, "Listed").SubKey; got != stableKey(pubID, listed.ID) {
		t.Errorf("Listed's copy sub_key = %s, want its snapshot key", got)
	}
	for i, f := range c.Folders {
		if f.SubKey != stableKey(pubID, source.Folders[i].ID) {
			t.Errorf("folder %d sub_key = %s, want its source's key", i, f.SubKey)
		}
	}
	if c.Folders[0].Refs[0].CatalogID != c.Folders[1].Refs[0].CatalogID || c.Folders[1].Refs[0].Genre != "War" {
		t.Errorf("refs = %+v, %+v; want one copy of Listed referenced twice", c.Folders[0].Refs, c.Folders[1].Refs)
	}
}

// A subscribe or a fork needs a live publication of someone else's; a second
// subscription to one publication is ErrConflict.
func TestSubscribeRefusals(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner, subscriber := newTestProfile(t, db, "owner"), newTestProfile(t, db, "subscriber")
	source := publishCatalog(t, db, owner, "Popular", "{}")

	for name, err := range map[string]error{
		"own subscribe": second(db.Subscribe(ctx, owner, source.Publication.ID)),
		"own fork":      second(db.ForkPublication(ctx, owner, source.Publication.ID)),
		"unknown":       second(db.Subscribe(ctx, subscriber, uuid.New())),
	} {
		if !errors.Is(err, ErrPublicationNotFound) {
			t.Errorf("%s = %v, want ErrPublicationNotFound", name, err)
		}
	}
	subscribe(t, db, subscriber, source.Publication.ID)
	if _, err := db.Subscribe(ctx, subscriber, source.Publication.ID); !errors.Is(err, ErrConflict) {
		t.Errorf("second subscribe = %v, want ErrConflict", err)
	}
	if catalogs, _ := db.GetUserCatalogs(ctx, subscriber); len(catalogs) != 1 {
		t.Errorf("subscriber holds %d catalogs after the refused second subscribe, want 1", len(catalogs))
	}
	if _, err := db.ForkPublication(ctx, subscriber, source.Publication.ID); err != nil {
		t.Errorf("fork beside a subscription = %v, want nil", err)
	}
}

// A publication withdrawn after a subscribe read it, and before the
// subscribe wrote, is not subscribed to: the subscription insert checks the
// publication is live, and the copy rolls back with it.
func TestSubscribeRaceWithWithdraw(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner, subscriber := newTestProfile(t, db, "owner"), newTestProfile(t, db, "subscriber")
	source := publishCatalog(t, db, owner, "Popular", "{}")

	pub, err := db.takeablePublication(ctx, subscriber, source.Publication.ID)
	if err != nil {
		t.Fatalf("takeablePublication: %v", err)
	}
	if _, err := db.WithdrawCatalog(ctx, owner, source.ID); err != nil {
		t.Fatalf("WithdrawCatalog: %v", err)
	}
	err = db.inTx(ctx, func(tx *sql.Tx) error {
		copyID, err := writeCopy(ctx, tx, subscriber, pub, true)
		if err != nil {
			return err
		}
		return insertSubscription(ctx, tx, subscriber, pub, copyID)
	})
	if !errors.Is(err, ErrPublicationNotFound) {
		t.Fatalf("subscribe after a withdraw = %v, want ErrPublicationNotFound", err)
	}
	if catalogs, _ := db.GetUserCatalogs(ctx, subscriber); len(catalogs) != 0 {
		t.Errorf("subscriber holds %d catalogs after the refused subscribe, want 0", len(catalogs))
	}
	var count int
	if err := db.conn.QueryRowContext(ctx, "SELECT subscriber_count FROM publications WHERE id = ?", pub.id.String()).Scan(&count); err != nil || count != 0 {
		t.Errorf("subscriber_count = %d (%v), want 0", count, err)
	}
}

// A snapshot today's form rules refuse is not copied: a subscribe and a
// fork run the form validators, and nothing else, over it.
func TestSubscribeChecksTheSnapshot(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner, subscriber := newTestProfile(t, db, "owner"), newTestProfile(t, db, "subscriber")
	source := publishCollection(t, db, owner, CollectionForm{Title: "Shared"})
	if _, err := db.conn.ExecContext(ctx, `UPDATE publications SET snapshot = json_set(snapshot, '$.collection.view_mode', 'CAROUSEL') WHERE id = ?`,
		source.Publication.ID.String()); err != nil {
		t.Fatal(err)
	}
	for name, err := range map[string]error{
		"subscribe": second(db.Subscribe(ctx, subscriber, source.Publication.ID)),
		"fork":      second(db.ForkPublication(ctx, subscriber, source.Publication.ID)),
	} {
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("%s of a snapshot with an unknown view mode = %v, want ErrInvalidInput", name, err)
		}
	}
}

// Saving a subscribed catalog is refused with ErrInvalidInput and changes
// nothing: the name, the subscription and the subscriber count stay. Someone
// else's catalog is still not found.
func TestSavingASubscribedCatalogIsRefused(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner, subscriber := newTestProfile(t, db, "owner"), newTestProfile(t, db, "subscriber")
	pubID := publishCatalog(t, db, owner, "Popular", "{}").Publication.ID
	copied := subscribe(t, db, subscriber, pubID).Catalog

	if _, err := db.UpdateUserCatalog(ctx, subscriber, copied.ID, listedCatalogForm("Renamed")); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("save a subscribed catalog = %v, want ErrInvalidInput", err)
	}
	if after := reloadCatalog(t, db, copied.ID); after.Name != copied.Name || after.Subscription == nil {
		t.Errorf("after the refused save: %q %+v; want %q and its subscription", after.Name, after.Subscription, copied.Name)
	}
	if got := subscriberCount(t, db, owner, pubID); got != 1 {
		t.Errorf("subscriber count after the refused save = %d, want 1", got)
	}
	if _, err := db.UpdateUserCatalog(ctx, owner, copied.ID, listedCatalogForm("Renamed")); !errors.Is(err, ErrCatalogNotFound) {
		t.Errorf("save someone else's subscribed catalog = %v, want ErrCatalogNotFound", err)
	}
}

// Every content write to a subscribed collection is refused with
// ErrInvalidInput and changes nothing: a save, a save moving its catalog to
// the library, and a catalog created in it. The copy keeps its subscription,
// its folder and catalog ids and their keys. A delete of a copy is allowed.
func TestWritingASubscribedCollectionIsRefused(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner, subscriber := newTestProfile(t, db, "owner"), newTestProfile(t, db, "subscriber")
	writes := map[string]func(copied CollectionWithFolders) error{
		"a save": func(copied CollectionWithFolders) error {
			form := saveFormOf(copied)
			form.Title = "Renamed"
			return second(db.UpdateUserCollection(ctx, subscriber, copied.ID, form))
		},
		"a save moving its catalog to the library": func(copied CollectionWithFolders) error {
			form := saveFormOf(copied)
			s := catalogNamed(t, copied, "S")
			form.CatalogEdits = []ScopedCatalogEdit{{ID: s.ID, Type: s.Type, Provider: s.Provider, Name: s.Name, Params: s.Params, MoveToLibrary: true}}
			return second(db.UpdateUserCollection(ctx, subscriber, copied.ID, form))
		},
		"a catalog created in it": func(copied CollectionWithFolders) error {
			into := listedCatalogForm("Into the copy")
			into.CollectionID = &copied.ID
			return second(db.CreateUserCatalog(ctx, subscriber, into))
		},
	}
	for name, write := range writes {
		source := publishCollection(t, db, owner, CollectionForm{Title: name, Folders: []FolderData{{Title: "F", Catalogs: []FolderCatalogRef{newScoped("k", "S", "{}")}}}})
		copied := *subscribe(t, db, subscriber, source.Publication.ID).Collection
		if err := write(copied); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("%s = %v, want ErrInvalidInput", name, err)
		}
		after := mustOwnCollection(t, db, subscriber, copied.ID)
		if after.Subscription == nil || after.Title != copied.Title || len(after.Catalogs) != 1 ||
			after.Folders[0].ID != copied.Folders[0].ID || after.Folders[0].SubKey != copied.Folders[0].SubKey {
			t.Errorf("%s: left %+v with %d catalogs, folder %s keyed %q; want the copy as it was", name, after.Subscription, len(after.Catalogs), after.Folders[0].ID, after.Folders[0].SubKey)
		}
		if scoped := catalogNamed(t, after, "S"); scoped.ID != catalogNamed(t, copied, "S").ID || scoped.SubKey == "" || scoped.CollectionID == nil {
			t.Errorf("%s: scoped catalog %s keyed %q; want its id, key and scope kept", name, scoped.ID, scoped.SubKey)
		}
		if got := subscriberCount(t, db, owner, source.Publication.ID); got != 1 {
			t.Errorf("%s: subscriber count = %d, want 1", name, got)
		}
	}

	theirs := *subscribe(t, db, subscriber, publishCollection(t, db, owner, CollectionForm{Title: "Theirs"}).Publication.ID).Collection
	if err := second(db.UpdateUserCollection(ctx, owner, theirs.ID, saveFormOf(theirs))); !errors.Is(err, ErrCollectionNotFound) {
		t.Errorf("save of someone else's subscribed collection = %v, want ErrCollectionNotFound", err)
	}

	doomed := *subscribe(t, db, subscriber, publishCollection(t, db, owner, CollectionForm{Title: "Doomed"}).Publication.ID).Collection
	if err := db.DeleteUserCollection(ctx, subscriber, doomed.ID); err != nil {
		t.Errorf("delete a subscribed collection = %v, want nil", err)
	}
}

// A refused save of one subscriber's copy leaves every copy of the
// publication subscribed: after the publisher reorders its folders and
// republishes, each copy's Update pairs them by key and keeps their ids.
func TestRefusedSaveLeavesCopiesUpdatingByKey(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner, first, other := newTestProfile(t, db, "owner"), newTestProfile(t, db, "first"), newTestProfile(t, db, "other")
	source := publishCollection(t, db, owner, CollectionForm{Title: "Shared", Folders: []FolderData{
		{Title: "A", Catalogs: []FolderCatalogRef{newScoped("a", "SA", `{"with_genres":"27"}`)}},
		{Title: "B", Catalogs: []FolderCatalogRef{newScoped("b", "SB", `{"with_genres":"35"}`)}},
	}})
	pubID := source.Publication.ID
	mine := *subscribe(t, db, first, pubID).Collection
	theirs := *subscribe(t, db, other, pubID).Collection

	edit := saveFormOf(mine)
	edit.Title = "Mine now"
	if _, err := db.UpdateUserCollection(ctx, first, mine.ID, edit); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("save the first copy = %v, want ErrInvalidInput", err)
	}
	reordered := saveFormOf(source)
	reordered.Folders = []FolderData{reordered.Folders[1], reordered.Folders[0]}
	if _, err := db.UpdateUserCollection(ctx, owner, source.ID, reordered); err != nil {
		t.Fatal(err)
	}
	if _, err := db.PublishCollection(ctx, owner, source.ID, allowAnyCatalogParams); err != nil {
		t.Fatal(err)
	}

	updated, err := db.UpdateSubscription(ctx, other, pubID)
	if err != nil {
		t.Fatalf("Update of the other copy: %v", err)
	}
	after := *updated.Collection
	if after.Folders[0].ID != theirs.Folders[1].ID || after.Folders[1].ID != theirs.Folders[0].ID || after.Subscription.UpdateAvailable {
		t.Errorf("other copy's folders %s, %s, %+v; want %s, %s paired by key and in step",
			after.Folders[0].ID, after.Folders[1].ID, after.Subscription, theirs.Folders[1].ID, theirs.Folders[0].ID)
	}
	if _, err := db.UpdateSubscription(ctx, first, pubID); err != nil {
		t.Errorf("Update of the copy whose save was refused: %v", err)
	}
	if got := subscriberCount(t, db, owner, pubID); got != 2 {
		t.Errorf("subscriber count = %d, want 2", got)
	}
}

// subscriberCount is how many profiles subscribe to publicationID, as
// profileID reads it.
func subscriberCount(t *testing.T, db *DB, profileID, publicationID uuid.UUID) int {
	t.Helper()
	detail, err := db.GetPublication(context.Background(), profileID, publicationID)
	if err != nil {
		t.Fatal(err)
	}
	return detail.SubscriberCount
}

// Update overwrites a subscribed collection by key: the publisher renamed,
// added, removed and reordered folders, renamed a catalog, changed a
// recipe and a genre, added a catalog and dropped one. The diff lists
// exactly that; Update keeps the id of every folder and catalog whose key
// the snapshot still has, removes the others, adds the new ones under their
// keys, keeps the copy's pin, bumps its version once, and leaves it in step.
func TestUpdateSubscriptionByKey(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner, subscriber := newTestProfile(t, db, "owner"), newTestProfile(t, db, "subscriber")
	listed, err := db.CreateUserCatalog(ctx, owner, listedCatalogForm("Listed"))
	if err != nil {
		t.Fatal(err)
	}
	source := publishCollection(t, db, owner, CollectionForm{Title: "Weekend", Folders: []FolderData{
		{Title: "A", Catalogs: []FolderCatalogRef{{CatalogID: &listed.ID}, newScoped("s1", "S1", `{"with_genres":"27"}`)}},
		{Title: "B", Catalogs: []FolderCatalogRef{newScoped("s2", "S2", `{"with_genres":"35"}`)}},
		{Title: "C", Catalogs: []FolderCatalogRef{{CatalogID: &listed.ID, Genre: "War"}}},
	}})
	pubID := source.Publication.ID
	before := *subscribe(t, db, subscriber, pubID).Collection
	pushSelection(t, db, subscriber, SelectedCollectionInput{CollectionID: before.ID, PinToTop: true})
	if pinned := mustOwnCollection(t, db, subscriber, before.ID); pinned.NeedsPush {
		t.Fatal("the copy right after its push: want no push needed")
	}

	listedForm := listedCatalogForm("Listed")
	listedForm.Params = `{"sort_by":"revenue.desc"}`
	if _, err := db.UpdateUserCatalog(ctx, owner, listed.ID, listedForm); err != nil {
		t.Fatal(err)
	}
	s1 := catalogNamed(t, source, "S1")
	edit := saveFormOf(source)
	edit.Folders = []FolderData{edit.Folders[2], edit.Folders[0], {Title: "D", Catalogs: []FolderCatalogRef{newScoped("s3", "S3", `{"with_genres":"18"}`)}}}
	edit.Folders[0].Catalogs[0].Genre = "Western"
	edit.Folders[1].Title = "A2"
	edit.CatalogEdits = []ScopedCatalogEdit{{ID: s1.ID, Type: "movie", Provider: "tmdb", Name: "S1b", Params: s1.Params}}
	if _, err := db.UpdateUserCollection(ctx, owner, source.ID, edit); err != nil {
		t.Fatalf("publisher's edit: %v", err)
	}
	republished, err := db.PublishCollection(ctx, owner, source.ID, allowAnyCatalogParams)
	if err != nil {
		t.Fatal(err)
	}

	if s := mustOwnCollection(t, db, subscriber, before.ID).Subscription; !s.UpdateAvailable {
		t.Fatalf("subscription after the republish = %+v, want an update available", s)
	}
	key := func(id uuid.UUID) string { return stableKey(pubID, id) }
	updated, err := db.UpdateSubscription(ctx, subscriber, pubID)
	if err != nil {
		t.Fatalf("UpdateSubscription: %v", err)
	}
	after := *updated.Collection
	titles := []string{}
	for _, f := range after.Folders {
		titles = append(titles, f.Title)
	}
	assertStrings(t, "folders after", titles, []string{"C", "A2", "D"})
	if after.Folders[0].ID != before.Folders[2].ID || after.Folders[1].ID != before.Folders[0].ID {
		t.Errorf("kept folders' ids changed: %s, %s; want %s, %s", after.Folders[0].ID, after.Folders[1].ID, before.Folders[2].ID, before.Folders[0].ID)
	}
	if after.Folders[2].SubKey != key(republished.Folders[2].ID) {
		t.Errorf("new folder sub_key = %s, want its snapshot key", after.Folders[2].SubKey)
	}
	if got := catalogNamed(t, after, "S1b").ID; got != catalogNamed(t, before, "S1").ID {
		t.Errorf("renamed catalog's id = %s, want it kept", got)
	}
	listedCopy := catalogNamed(t, after, "Listed")
	if listedCopy.ID != catalogNamed(t, before, "Listed").ID || listedCopy.Params != `{"sort_by":"revenue.desc"}` {
		t.Errorf("Listed's copy = %+v, want its id kept and the new recipe", listedCopy)
	}
	if catalogNamed(t, after, "S3").SubKey != key(catalogNamed(t, republished, "S3").ID) || len(after.Catalogs) != 3 {
		t.Errorf("catalogs after = %+v, want S2 gone and S3 under its key", after.Catalogs)
	}
	if !after.NeedsPush || !after.PinToTop || after.Subscription.UpdateAvailable {
		t.Errorf("after: needs push %v, pinned %v, subscription %+v; want a push needed, the pin kept, in step",
			after.NeedsPush, after.PinToTop, after.Subscription)
	}
}

// A copy whose content already equals its snapshot, but is marked out of step,
// is only marked in step by Update: no row is written.
func TestUpdateSubscriptionRestampsAnIdenticalCopy(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner, subscriber := newTestProfile(t, db, "owner"), newTestProfile(t, db, "subscriber")
	source := publishCollection(t, db, owner, CollectionForm{Title: "Shared", Folders: []FolderData{{Title: "F", Catalogs: []FolderCatalogRef{newScoped("k", "S", "{}")}}}})
	copied := *subscribe(t, db, subscriber, source.Publication.ID).Collection
	if _, err := db.conn.ExecContext(ctx, `UPDATE subscriptions SET taken_hash = 'stale' WHERE collection_id = ?`, copied.ID.String()); err != nil {
		t.Fatal(err)
	}
	if s := mustOwnCollection(t, db, subscriber, copied.ID).Subscription; !s.UpdateAvailable {
		t.Fatalf("subscription = %+v, want an update available", s)
	}
	updated, err := db.UpdateSubscription(ctx, subscriber, source.Publication.ID)
	if err != nil {
		t.Fatal(err)
	}
	if c := updated.Collection; !c.UpdatedAt.Equal(copied.UpdatedAt) || c.Subscription.UpdateAvailable {
		t.Errorf("after = updated %s, %+v; want %s unwritten and in step", c.UpdatedAt, c.Subscription, copied.UpdatedAt)
	}
}

// Update of a catalog copy takes the snapshot's name and recipe, keeping the
// copy's id; a copy already in step is left as it is.
func TestUpdateSubscribedCatalog(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner, subscriber := newTestProfile(t, db, "owner"), newTestProfile(t, db, "subscriber")
	source := publishCatalog(t, db, owner, "Popular", `{"sort_by":"popularity.desc"}`)
	copied := subscribe(t, db, subscriber, source.Publication.ID).Catalog

	unchanged, err := db.UpdateSubscription(ctx, subscriber, source.Publication.ID)
	if err != nil || !unchanged.Catalog.UpdatedAt.Equal(copied.UpdatedAt) {
		t.Fatalf("Update in step = %+v, %v; want the copy unwritten", unchanged.Catalog, err)
	}

	form := listedCatalogForm("Top Grossing")
	form.Params = `{"sort_by":"revenue.desc"}`
	if _, err := db.UpdateUserCatalog(ctx, owner, source.ID, form); err != nil {
		t.Fatal(err)
	}
	if _, err := db.PublishCatalog(ctx, owner, source.ID, allowAnyCatalogParams); err != nil {
		t.Fatal(err)
	}
	updated, err := db.UpdateSubscription(ctx, subscriber, source.Publication.ID)
	if err != nil {
		t.Fatal(err)
	}
	if c := updated.Catalog; c.ID != copied.ID || c.Name != "Top Grossing" || c.Params != form.Params || c.Subscription.UpdateAvailable {
		t.Errorf("updated copy = %+v, want %s renamed onto the new recipe, in step", c, copied.ID)
	}
}

// Update runs the form validators over the snapshot it writes, for a
// catalog as for a collection: a snapshot today's rules refuse is not
// written, and the copy stays as it was.
func TestUpdateChecksTheSnapshot(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner, subscriber := newTestProfile(t, db, "owner"), newTestProfile(t, db, "subscriber")
	source := publishCatalog(t, db, owner, "Popular", "{}")
	copied := subscribe(t, db, subscriber, source.Publication.ID).Catalog
	if _, err := db.conn.ExecContext(ctx, `
		UPDATE publications SET snapshot = json_set(snapshot, '$.catalogs[0].name', ' '), content_hash = 'republished' WHERE id = ?
	`, source.Publication.ID.String()); err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpdateSubscription(ctx, subscriber, source.Publication.ID); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Update to a snapshot with a blank name = %v, want ErrInvalidInput", err)
	}
	if got := reloadCatalog(t, db, copied.ID); got.Name != "Popular" {
		t.Errorf("copy after the refused Update = %q, want it unchanged", got.Name)
	}

	if _, err := db.conn.ExecContext(ctx, `
		UPDATE publications SET snapshot = json_set(snapshot, '$.catalogs[0].name', 'Popular', '$.catalogs[0].type', 'series') WHERE id = ?
	`, source.Publication.ID.String()); err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpdateSubscription(ctx, subscriber, source.Publication.ID); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("Update to a snapshot of another type = %v, want ErrInvalidInput", err)
	}
}

// A copy that already equals a snapshot today's rules refuse is still
// marked in step: nothing is written, so there is nothing to check.
func TestUpdateRestampsAnIdenticalCopyTodaysRulesRefuse(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner, subscriber := newTestProfile(t, db, "owner"), newTestProfile(t, db, "subscriber")
	source := publishCatalog(t, db, owner, "Popular", "{}")
	copied := subscribe(t, db, subscriber, source.Publication.ID).Catalog
	if _, err := db.conn.ExecContext(ctx, `UPDATE catalogs SET name = ' ' WHERE id = ?`, copied.ID.String()); err != nil {
		t.Fatal(err)
	}
	detail, err := db.GetPublication(ctx, subscriber, source.Publication.ID)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, hash, err := copySnapshot(reloadCatalog(t, db, copied.ID), detail.Snapshot).encode()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.conn.ExecContext(ctx, `UPDATE publications SET snapshot = ?, content_hash = ? WHERE id = ?`,
		snapshot, hash, source.Publication.ID.String()); err != nil {
		t.Fatal(err)
	}
	updated, err := db.UpdateSubscription(ctx, subscriber, source.Publication.ID)
	if err != nil {
		t.Fatalf("Update of a copy equal to its blank-named snapshot = %v, want it marked in step", err)
	}
	if c := updated.Catalog; c.Name != " " || c.Subscription.UpdateAvailable {
		t.Errorf("after = %q, %+v; want the copy unwritten and in step", c.Name, c.Subscription)
	}
}

// Update needs a subscription of the caller's.
func TestUpdateWithoutSubscriptionIsNotFound(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner, stranger := newTestProfile(t, db, "owner"), newTestProfile(t, db, "stranger")
	source := publishCatalog(t, db, owner, "Popular", "{}")
	if _, err := db.UpdateSubscription(ctx, stranger, source.Publication.ID); !errors.Is(err, ErrPublicationNotFound) {
		t.Errorf("Update without a subscription = %v, want ErrPublicationNotFound", err)
	}
}

// A fork is the caller's own, fully editable copy: no subscription, no
// sub_keys, and a save of it goes through.
func TestForkPublication(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner, forker := newTestProfile(t, db, "owner"), newTestProfile(t, db, "forker")
	source := publishCollection(t, db, owner, CollectionForm{Title: "Shared", Folders: []FolderData{{Title: "F", Catalogs: []FolderCatalogRef{newScoped("k", "S", "{}")}}}})
	forked, err := db.ForkPublication(ctx, forker, source.Publication.ID)
	if err != nil {
		t.Fatal(err)
	}
	c := *forked.Collection
	if c.Subscription != nil || c.Title != "Shared" || c.Folders[0].SubKey != "" || c.Catalogs[0].SubKey != "" {
		t.Errorf("fork = %+v, want Shared with no subscription and no sub_keys", c)
	}
	if _, err := db.UpdateUserCollection(ctx, forker, c.ID, saveFormOf(c)); err != nil {
		t.Errorf("save a fork = %v, want nil", err)
	}
	if detail, _ := db.GetPublication(ctx, owner, source.Publication.ID); detail.SubscriberCount != 0 {
		t.Errorf("subscriber count after a fork = %d, want 0", detail.SubscriberCount)
	}
}
