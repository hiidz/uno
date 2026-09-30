package vault

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/google/uuid"
)

// Community lists every live publication of someone else's, newest first:
// the caller's own and withdrawn ones are never listed. A catalog row
// carries its recipe and a collection row none, and every row names the
// catalogs it holds.
func TestListCommunity(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner, viewer := newTestProfile(t, db, "owner"), newTestProfile(t, db, "viewer")
	movie := publishCatalog(t, db, owner, "Movie", `{"sort_by":"popularity.desc"}`)
	publishCollection(t, db, owner, CollectionForm{Title: "Movie Night", Folders: []FolderData{{Title: "F", Catalogs: []FolderCatalogRef{
		newScoped("k1", "Ghost Stories", `{"with_genres":"27"}`), newScoped("k2", "Slashers", `{"with_genres":"53"}`),
	}}}})
	withdrawn := publishCatalog(t, db, owner, "Withdrawn", `{"sort_by":"revenue.desc"}`)
	if _, err := db.WithdrawCatalog(ctx, owner, withdrawn.ID); err != nil {
		t.Fatal(err)
	}
	publishCatalog(t, db, viewer, "Viewer's own", `{"sort_by":"vote_average.desc"}`)
	if _, err := db.conn.ExecContext(ctx, `UPDATE publications SET published_at = '2026-01-01T00:00:00Z' WHERE id = ?`, movie.Publication.ID.String()); err != nil {
		t.Fatal(err)
	}

	items := listCommunity(t, db, viewer)
	assertStrings(t, "newest first", itemTitles(items), []string{"Movie Night", "Movie"})
	for _, item := range items {
		if (item.Kind == kindCatalog) != (item.Catalog != nil) {
			t.Errorf("%s: catalog %+v, want a recipe on catalog rows only", item.Title, item.Catalog)
		}
	}
	assertStrings(t, "the collection's catalog names", items[0].CatalogNames, []string{"Ghost Stories", "Slashers"})
	if items[0].FolderCount != 1 || items[0].CatalogCount != 2 {
		t.Errorf("collection row = %d folders, %d catalogs; want 1 and 2", items[0].FolderCount, items[0].CatalogCount)
	}
	if c := items[1]; string(c.Catalog.Params) != `{"sort_by":"popularity.desc"}` || !slices.Equal(c.CatalogNames, []string{"Movie"}) {
		t.Errorf("catalog row = recipe %s, names %q", c.Catalog.Params, c.CatalogNames)
	}
}

// itemTitles lists each item's title.
func itemTitles(items []CommunityItem) []string {
	out := []string{}
	for _, item := range items {
		out = append(out, item.Title)
	}
	return out
}

// A row counts its subscribers, reads subscribed when the caller subscribes,
// and has an update available once its publication is republished with new
// content.
func TestListCommunitySubscriptionFlags(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner, subscriber, other := newTestProfile(t, db, "owner"), newTestProfile(t, db, "subscriber"), newTestProfile(t, db, "other")
	source := publishCatalog(t, db, owner, "Popular", `{"sort_by":"popularity.desc"}`)
	row := func() CommunityItem { return listCommunity(t, db, subscriber)[0] }
	if item := row(); item.Subscribed || item.UpdateAvailable || item.SubscriberCount != 0 {
		t.Errorf("before subscribing: %+v", item)
	}
	subscribe(t, db, subscriber, source.Publication.ID)
	subscribe(t, db, other, source.Publication.ID)
	if item := row(); !item.Subscribed || item.UpdateAvailable || item.SubscriberCount != 2 {
		t.Errorf("after subscribing: subscribed %v, update %v, count %d; want subscribed, no update, 2", item.Subscribed, item.UpdateAvailable, item.SubscriberCount)
	}
	if _, err := db.UpdateUserCatalog(ctx, owner, source.ID, listedCatalogForm("Renamed")); err != nil {
		t.Fatal(err)
	}
	if _, err := db.PublishCatalog(ctx, owner, source.ID, allowAnyCatalogParams); err != nil {
		t.Fatal(err)
	}
	if item := row(); !item.Subscribed || !item.UpdateAvailable {
		t.Errorf("after a republish: subscribed %v, update %v; want both", item.Subscribed, item.UpdateAvailable)
	}
}

// A detail reads a live publication, or a withdrawn one the caller
// subscribes to; a publication by an id nobody published is not found.
func TestGetPublication(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner, viewer := newTestProfile(t, db, "owner"), newTestProfile(t, db, "viewer")
	source := publishCollection(t, db, owner, CollectionForm{Title: "Halloween", Folders: []FolderData{{Title: "F", Catalogs: []FolderCatalogRef{newScoped("k", "Ghosts", "{}")}}}})
	detail, err := db.GetPublication(ctx, viewer, source.Publication.ID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Title != "Halloween" || detail.Withdrawn || detail.Catalog != nil || detail.Snapshot.Collection.Folders[0].Title != "F" || detail.Snapshot.Catalogs[0].Name != "Ghosts" {
		t.Errorf("detail = %+v", detail)
	}
	if _, err := db.GetPublication(ctx, viewer, uuid.New()); !errors.Is(err, ErrPublicationNotFound) {
		t.Errorf("GetPublication of an unknown id = %v, want ErrPublicationNotFound", err)
	}
}
