package vault

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

// What an Update would change in a row added from Community: nothing while
// the copy is in step, then what the publisher's republish removed, added and
// changed, read from the copy under its own keys, and nothing again once the
// Update is applied.
func TestUpdateChangesForACollectionCopy(t *testing.T) {
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
	subscribe(t, db, subscriber, pubID)

	if changes, err := db.UpdateChanges(ctx, subscriber, pubID); err != nil || changes == nil || len(changes) != 0 {
		t.Fatalf("a copy in step: changes = %v, %v; want an empty list", changes, err)
	}

	form := saveFormOf(source)
	form.Folders[0].Title = "Family"
	form.Folders[0].Catalogs = form.Folders[0].Catalogs[:1]
	form.Folders = append(form.Folders, FolderData{Title: "C", Catalogs: []FolderCatalogRef{newScoped("n", "Fresh", `{"with_genres":"35"}`)}})
	if _, err := db.UpdateUserCollection(ctx, owner, source.ID, form); err != nil {
		t.Fatal(err)
	}
	renamed := listedCatalogForm("Listed too")
	if _, err := db.UpdateUserCatalog(ctx, owner, listed.ID, renamed); err != nil {
		t.Fatal(err)
	}
	if _, err := db.PublishCollection(ctx, owner, source.ID); err != nil {
		t.Fatal(err)
	}

	changes, err := db.UpdateChanges(ctx, subscriber, pubID)
	if err != nil {
		t.Fatal(err)
	}
	assertStrings(t, "changes", labels(changes), []string{
		"removed catalog Scoped @A",
		"added folder C",
		"added catalog Fresh @C",
		"changed folder/name Family (was A)",
		"changed catalog/name Listed too (was Listed)",
	})

	if _, err := db.UpdateSubscription(ctx, subscriber, pubID); err != nil {
		t.Fatal(err)
	}
	if changes, err := db.UpdateChanges(ctx, subscriber, pubID); err != nil || len(changes) != 0 {
		t.Errorf("after the Update: changes = %q, %v; want none", labels(changes), err)
	}
}

// A catalog copy's list is its catalog changing.
func TestUpdateChangesForACatalogCopy(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner, subscriber := newTestProfile(t, db, "owner"), newTestProfile(t, db, "subscriber")
	source := publishCatalog(t, db, owner, "Popular", `{"sort_by":"popularity.desc"}`)
	subscribe(t, db, subscriber, source.Publication.ID)

	form := listedCatalogForm("Popular")
	form.Params = `{"sort_by":"vote_average.desc"}`
	if _, err := db.UpdateUserCatalog(ctx, owner, source.ID, form); err != nil {
		t.Fatal(err)
	}
	if _, err := db.PublishCatalog(ctx, owner, source.ID); err != nil {
		t.Fatal(err)
	}
	changes, err := db.UpdateChanges(ctx, subscriber, source.Publication.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertStrings(t, "changes", labels(changes), []string{"changed catalog/recipe Popular +recipe"})
}

// Only a profile that subscribes to a live publication has an update to
// list.
func TestUpdateChangesRefusals(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner, subscriber, other := newTestProfile(t, db, "owner"), newTestProfile(t, db, "subscriber"), newTestProfile(t, db, "other")
	source := publishCatalog(t, db, owner, "Popular", "{}")
	subscribe(t, db, subscriber, source.Publication.ID)

	if _, err := db.UpdateChanges(ctx, other, source.Publication.ID); !errors.Is(err, ErrPublicationNotFound) {
		t.Errorf("not subscribed = %v, want ErrPublicationNotFound", err)
	}
	if _, err := db.UnpublishCatalog(ctx, owner, source.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpdateChanges(ctx, subscriber, source.Publication.ID); !errors.Is(err, ErrPublicationNotFound) {
		t.Errorf("unpublished = %v, want ErrPublicationNotFound", err)
	}
}

// What publishing a collection again would change is its saved tree against
// what it last published: nothing for a collection never published or still as
// published, the catalog a folder uses when only that catalog was edited, and
// nothing again once the edit is undone or published.
func TestCollectionChangesSincePublish(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner := newTestProfile(t, db, "owner")
	listed, err := db.CreateUserCatalog(ctx, owner, listedCatalogForm("Listed"))
	if err != nil {
		t.Fatal(err)
	}
	form := CollectionForm{Title: "Weekend", Folders: []FolderData{{Title: "A", Catalogs: []FolderCatalogRef{{CatalogID: &listed.ID}}}}}
	created, err := db.CreateUserCollection(ctx, owner, form)
	if err != nil {
		t.Fatal(err)
	}
	empty := func(label string) {
		t.Helper()
		if changes, err := db.CollectionChangesSincePublish(ctx, owner, created.ID); err != nil || changes == nil || len(changes) != 0 {
			t.Errorf("%s: changes = %q, %v; want an empty list", label, labels(changes), err)
		}
	}
	empty("never published")

	if _, err := db.PublishCollection(ctx, owner, created.ID); err != nil {
		t.Fatal(err)
	}
	empty("as published")

	edited := listedCatalogForm("Listed")
	edited.Params = `{"sort_by":"vote_average.desc"}`
	if _, err := db.UpdateUserCatalog(ctx, owner, listed.ID, edited); err != nil {
		t.Fatal(err)
	}
	changes, err := db.CollectionChangesSincePublish(ctx, owner, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertStrings(t, "an edited library catalog", labels(changes), []string{"changed catalog/recipe Listed +recipe"})

	if _, err := db.UpdateUserCatalog(ctx, owner, listed.ID, listedCatalogForm("Listed")); err != nil {
		t.Fatal(err)
	}
	empty("the edit undone")

	if _, err := db.UpdateUserCatalog(ctx, owner, listed.ID, edited); err != nil {
		t.Fatal(err)
	}
	if _, err := db.PublishCollection(ctx, owner, created.ID); err != nil {
		t.Fatal(err)
	}
	empty("republished")
}

// A catalog's list is the catalog changing; a row that can't be published, a
// catalog inside a collection or a copy added from Community, has none.
func TestCatalogChangesSincePublish(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner, subscriber := newTestProfile(t, db, "owner"), newTestProfile(t, db, "subscriber")
	source := publishCatalog(t, db, owner, "Popular", "{}")

	if changes, err := db.CatalogChangesSincePublish(ctx, owner, source.ID); err != nil || changes == nil || len(changes) != 0 {
		t.Errorf("as published: changes = %q, %v; want an empty list", labels(changes), err)
	}
	if _, err := db.UpdateUserCatalog(ctx, owner, source.ID, listedCatalogForm("Popular now")); err != nil {
		t.Fatal(err)
	}
	changes, err := db.CatalogChangesSincePublish(ctx, owner, source.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertStrings(t, "a rename", labels(changes), []string{"changed catalog/name Popular now (was Popular)"})

	never, err := db.CreateUserCatalog(ctx, owner, listedCatalogForm("Never"))
	if err != nil {
		t.Fatal(err)
	}
	if changes, err := db.CatalogChangesSincePublish(ctx, owner, never.ID); err != nil || len(changes) != 0 {
		t.Errorf("never published: changes = %q, %v; want none", labels(changes), err)
	}

	copied := subscribe(t, db, subscriber, source.Publication.ID).Catalog
	if _, err := db.CatalogChangesSincePublish(ctx, subscriber, copied.ID); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("an added row = %v, want ErrInvalidInput", err)
	}
	if _, err := db.CatalogChangesSincePublish(ctx, subscriber, uuid.New()); !errors.Is(err, ErrCatalogNotFound) {
		t.Errorf("an unknown catalog = %v, want ErrCatalogNotFound", err)
	}
	if _, err := db.CollectionChangesSincePublish(ctx, subscriber, uuid.New()); !errors.Is(err, ErrCollectionNotFound) {
		t.Errorf("an unknown collection = %v, want ErrCollectionNotFound", err)
	}
}
