package vault

import (
	"context"
	"errors"
	"testing"
)

func TestRecipeSource(t *testing.T) {
	for _, tc := range []struct{ params, want string }{
		{`{}`, sourceFilters},
		{``, sourceFilters},
		{`{"sort_by":"popularity.desc","with_genres":"27"}`, sourceFilters},
		{`{"with_collection":"10"}`, sourceCollection},
		{`{"with_collection":"10","randomized":true}`, sourceCollection},
		{`{"tmdb_list":"trending/week"}`, sourceList},
	} {
		if got := recipeSource(tc.params); got != tc.want {
			t.Errorf("recipeSource(%s) = %s, want %s", tc.params, got, tc.want)
		}
	}
}

// A listed catalog's save keeps its source: a Filters catalog can't become a
// TMDB collection or back, while a TMDB collection catalog can name another
// collection.
func TestUpdateUserCatalogLocksSource(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner := newTestProfile(t, db, "owner")

	create := func(params string) Catalog {
		t.Helper()
		form := listedCatalogForm("Row")
		form.Params = params
		c, err := db.CreateUserCatalog(ctx, owner, form)
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	save := func(c Catalog, params string) error {
		form := listedCatalogForm("Row")
		form.Params = params
		_, err := db.UpdateUserCatalog(ctx, owner, c.ID, catalogRevision(t, db, c.ID), form)
		return err
	}

	filters := create(`{"with_genres":"27"}`)
	if err := save(filters, `{"with_collection":"10"}`); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("Filters to TMDB collection = %v, want ErrInvalidInput", err)
	}
	if err := save(filters, `{}`); err != nil {
		t.Errorf("Filters to other filters = %v, want it saved", err)
	}

	collection := create(`{"with_collection":"10"}`)
	if err := save(collection, `{"with_collection":"263"}`); err != nil {
		t.Errorf("TMDB collection to another collection = %v, want it saved", err)
	}
	if err := save(collection, `{"sort_by":"popularity.desc"}`); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("TMDB collection to Filters = %v, want ErrInvalidInput", err)
	}
}

// An Update writes its publisher's catalog over a copy whatever source it now
// has: the copy follows its publisher, as it did before sources were locked.
func TestUpdateSubscriptionFollowsAChangedSource(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner, subscriber := newTestProfile(t, db, "owner"), newTestProfile(t, db, "subscriber")

	catalog := publishCatalog(t, db, owner, "Row", `{"with_genres":"27"}`)
	subscribe(t, db, subscriber, catalog.Publication.ID)

	collection := publishCollection(t, db, owner, CollectionForm{Title: "Set", ViewMode: "TABBED_GRID", Folders: []FolderData{
		{FolderArt: FolderArt{TileShape: "POSTER"}, Title: "A", Catalogs: []FolderCatalogRef{newScoped("s1", "S1", `{"with_genres":"27"}`)}},
	}})
	subscribe(t, db, subscriber, collection.Publication.ID)
	scoped := catalogNamed(t, collection, "S1")

	// A source change that predates the lock, written straight to the rows.
	for _, id := range []string{catalog.ID.String(), scoped.ID.String()} {
		if _, err := db.conn.ExecContext(ctx, `UPDATE catalogs SET params = ?, revision = revision + 1 WHERE id = ?`, `{"with_collection":"10"}`, id); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.PublishCatalog(ctx, owner, catalog.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.PublishCollection(ctx, owner, collection.ID); err != nil {
		t.Fatal(err)
	}

	updated, err := db.UpdateSubscription(ctx, subscriber, catalog.Publication.ID)
	if err != nil {
		t.Fatalf("Update of the catalog copy: %v", err)
	}
	if got := updated.Catalog.Params; got != `{"with_collection":"10"}` {
		t.Errorf("catalog copy params = %s, want the publisher's TMDB collection", got)
	}
	updatedTree, err := db.UpdateSubscription(ctx, subscriber, collection.Publication.ID)
	if err != nil {
		t.Fatalf("Update of the collection copy: %v", err)
	}
	if got := catalogNamed(t, *updatedTree.Collection, "S1").Params; got != `{"with_collection":"10"}` {
		t.Errorf("scoped catalog copy params = %s, want the publisher's TMDB collection", got)
	}
}
