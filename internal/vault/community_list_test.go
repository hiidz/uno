package vault

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// Community lists every live publication of someone else's, newest first:
// the caller's own and unpublished ones are never listed. A catalog row
// carries its recipe and a collection row none, every row names the catalogs
// it holds, and a collection row lists its folders in order with their tile
// shapes and covers.
func TestListCommunity(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner, viewer := newTestProfile(t, db, "owner"), newTestProfile(t, db, "viewer")
	movie := publishCatalog(t, db, owner, "Movie", `{"sort_by":"popularity.desc"}`)
	publishCollection(t, db, owner, CollectionForm{Title: "Movie Night", ViewMode: "TABBED_GRID", Folders: []FolderData{
		{Title: "Ghosts", FolderArt: FolderArt{TileShape: "LANDSCAPE", CoverEmoji: "👻"},
			Catalogs: []FolderCatalogRef{newScoped("k1", "Ghost Stories", `{"with_genres":"27"}`)}},
		{Title: "Slashers", FolderArt: FolderArt{TileShape: "POSTER", CoverImageURL: "https://image.tmdb.org/t/p/w500/a.jpg"},
			Catalogs: []FolderCatalogRef{newScoped("k2", "Slashers", `{"with_genres":"53"}`)}},
	}})
	unpublished := publishCatalog(t, db, owner, "Unpublished", `{"sort_by":"revenue.desc"}`)
	if _, err := db.UnpublishCatalog(ctx, owner, unpublished.ID); err != nil {
		t.Fatal(err)
	}
	publishCatalog(t, db, viewer, "Viewer's own", `{"sort_by":"vote_average.desc"}`)
	if _, err := db.conn.ExecContext(ctx, `UPDATE publications SET published_at = '2026-01-01T00:00:00Z' WHERE id = ?`, movie.Publication.ID.String()); err != nil {
		t.Fatal(err)
	}

	items := listCommunity(t, db, viewer)
	assertStrings(t, "the catalogs, then the collections", itemTitles(items), []string{"Movie", "Movie Night"})
	for _, item := range items {
		if (item.Kind == kindCatalog) != (item.Catalog != nil) {
			t.Errorf("%s: catalog %+v, want a recipe on catalog rows only", item.Title, item.Catalog)
		}
	}
	assertStrings(t, "the collection's catalog names", items[1].CatalogNames, []string{"Ghost Stories", "Slashers"})
	wantFolders := []CommunityFolder{
		{Title: "Ghosts", TileShape: "LANDSCAPE", CoverEmoji: "👻"},
		{Title: "Slashers", TileShape: "POSTER", CoverImageURL: "https://image.tmdb.org/t/p/w500/a.jpg"},
	}
	if !slices.Equal(items[1].Folders, wantFolders) {
		t.Errorf("the collection's folders = %+v, want %+v", items[1].Folders, wantFolders)
	}
	if len(items[1].CatalogNames) != 2 {
		t.Errorf("collection row names %q, want 2 catalogs", items[1].CatalogNames)
	}
	if c := items[0]; string(c.Catalog.Params) != `{"sort_by":"popularity.desc"}` || !slices.Equal(c.CatalogNames, []string{"Movie"}) {
		t.Errorf("catalog row = recipe %s, names %q", c.Catalog.Params, c.CatalogNames)
	}
	if c := items[0]; c.Folders == nil || len(c.Folders) != 0 {
		t.Errorf("catalog row folders = %#v, want an empty, non-nil list", c.Folders)
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
	if _, err := db.PublishCatalog(ctx, owner, source.ID); err != nil {
		t.Fatal(err)
	}
	if item := row(); !item.Subscribed || !item.UpdateAvailable {
		t.Errorf("after a republish: subscribed %v, update %v; want both", item.Subscribed, item.UpdateAvailable)
	}
}

// A detail reads a publication; one by an id nobody published is not found.
func TestGetPublication(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner, viewer := newTestProfile(t, db, "owner"), newTestProfile(t, db, "viewer")
	source := publishCollection(t, db, owner, CollectionForm{Title: "Halloween", ViewMode: "TABBED_GRID", Folders: []FolderData{{FolderArt: FolderArt{TileShape: "POSTER"}, Title: "F", Catalogs: []FolderCatalogRef{newScoped("k", "Ghosts", "{}")}}}})
	detail, err := db.GetPublication(ctx, viewer, source.Publication.ID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Title != "Halloween" || detail.Catalog != nil || detail.Snapshot.Collection.Folders[0].Title != "F" || detail.Snapshot.Catalogs[0].Name != "Ghosts" {
		t.Errorf("detail = %+v", detail)
	}
	if _, err := db.GetPublication(ctx, viewer, uuid.New()); !errors.Is(err, ErrPublicationNotFound) {
		t.Errorf("GetPublication of an unknown id = %v, want ErrPublicationNotFound", err)
	}
}

// A publisher can read their own publication's page, as everyone else sees it,
// though the list leaves it out and they can neither subscribe to it nor
// duplicate it.
func TestGetPublicationAnswersForItsPublisher(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner := newTestProfile(t, db, "owner")
	source := publishCatalog(t, db, owner, "Popular", `{"sort_by":"popularity.desc"}`)

	detail, err := db.GetPublication(ctx, owner, source.Publication.ID)
	if err != nil || detail.Title != "Popular" {
		t.Fatalf("GetPublication by its publisher = %+v, %v, want the publication", detail, err)
	}
	if items := listCommunity(t, db, owner); len(items) != 0 {
		t.Errorf("the publisher's Community list = %v, want it to leave out their own", itemTitles(items))
	}
	if _, err := db.Subscribe(ctx, owner, source.Publication.ID); !errors.Is(err, ErrPublicationNotFound) {
		t.Errorf("Subscribe to one's own publication = %v, want ErrPublicationNotFound", err)
	}
	if _, err := db.DuplicatePublication(ctx, owner, source.Publication.ID); !errors.Is(err, ErrPublicationNotFound) {
		t.Errorf("Duplicate of one's own publication = %v, want ErrPublicationNotFound", err)
	}
}

// A page holds at most communityPageSize rows and a cursor to the next; read
// cursor by cursor, both orders list every row once, in order, even where
// every row was published in the same second and titles repeat, so only the
// id tells two rows apart.
func TestListCommunityPages(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner, viewer := newTestProfile(t, db, "owner"), newTestProfile(t, db, "viewer")
	total := communityPageSize + 11
	for i := range total {
		publishCatalog(t, db, owner, fmt.Sprintf("C%02d", i), fmt.Sprintf(`{"with_genres":"%d"}`, i))
	}
	if _, err := db.conn.ExecContext(ctx, `UPDATE publications SET published_at = '2026-01-01T00:00:00Z',
		title = CASE WHEN rowid % 2 = 0 THEN 'same' ELSE 'Same' END`); err != nil {
		t.Fatal(err)
	}

	first, err := db.ListCommunity(ctx, viewer, CommunityQuery{Kind: kindCatalog, Sort: "newest"})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Items) != communityPageSize || first.NextCursor == nil {
		t.Fatalf("first page = %d rows, cursor %v; want %d and a cursor", len(first.Items), first.NextCursor, communityPageSize)
	}
	for _, sort := range []string{"newest", "name"} {
		ids := itemIDs(communityPages(t, db, viewer, CommunityQuery{Kind: kindCatalog, Sort: sort}))
		want := slices.Clone(ids)
		slices.Sort(want)
		if sort == "newest" {
			slices.Reverse(want)
		}
		if !slices.Equal(ids, want) {
			t.Errorf("%s: ids = %v, want every row once by id %v", sort, ids, want)
		}
		if len(slices.Compact(slices.Clone(want))) != total {
			t.Errorf("%s: %d distinct rows, want %d", sort, len(slices.Compact(want)), total)
		}
	}
}

// The name order folds ASCII case and breaks ties by id; newest is by
// published_at.
func TestListCommunityOrders(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner, viewer := newTestProfile(t, db, "owner"), newTestProfile(t, db, "viewer")
	for i, name := range []string{"beta", "Alpha", "gamma"} {
		c := publishCatalog(t, db, owner, name, fmt.Sprintf(`{"with_genres":"%d"}`, i))
		if _, err := db.conn.ExecContext(ctx, `UPDATE publications SET published_at = ? WHERE id = ?`,
			fmt.Sprintf("2026-01-0%dT00:00:00Z", 3-i), c.Publication.ID.String()); err != nil {
			t.Fatal(err)
		}
	}
	byName := communityPages(t, db, viewer, CommunityQuery{Kind: kindCatalog, Sort: "name"})
	assertStrings(t, "by name", itemTitles(byName), []string{"Alpha", "beta", "gamma"})
	newest := communityPages(t, db, viewer, CommunityQuery{Kind: kindCatalog, Sort: "newest"})
	assertStrings(t, "newest", itemTitles(newest), []string{"beta", "Alpha", "gamma"})
}

// Every word of a search is in the title or a catalog's name, not
// necessarily the same one, whatever its ASCII case; % and _ match
// themselves.
func TestListCommunitySearch(t *testing.T) {
	db := newTestDB(t)
	owner, viewer := newTestProfile(t, db, "owner"), newTestProfile(t, db, "viewer")
	publishCollection(t, db, owner, CollectionForm{Title: "Horror Night", ViewMode: "TABBED_GRID", Folders: []FolderData{
		{Title: "F", FolderArt: FolderArt{TileShape: "POSTER"}, Catalogs: []FolderCatalogRef{newScoped("k", "Slashers", `{"with_genres":"27"}`)}},
	}})
	publishCollection(t, db, owner, CollectionForm{Title: "100% Comedy", ViewMode: "TABBED_GRID", Folders: []FolderData{
		{Title: "F", FolderArt: FolderArt{TileShape: "POSTER"}, Catalogs: []FolderCatalogRef{newScoped("k", "Stand_up", `{"with_genres":"35"}`)}},
	}})
	search := func(q string) []string {
		return itemTitles(communityPages(t, db, viewer, CommunityQuery{Kind: kindCollection, Sort: "name", Search: q}))
	}
	assertStrings(t, "no search", search("  "), []string{"100% Comedy", "Horror Night"})
	assertStrings(t, "title and catalog name", search("HORROR slashers"), []string{"Horror Night"})
	assertStrings(t, "catalog name alone", search("slash"), []string{"Horror Night"})
	assertStrings(t, "a word in neither", search("horror comedy"), []string{})
	assertStrings(t, "a literal %", search("%"), []string{"100% Comedy"})
	assertStrings(t, "a literal _", search("d_u"), []string{"100% Comedy"})
	assertStrings(t, "_ is no wildcard", search("r_n"), []string{})
}

// A query with an unknown kind or sort, a search over its bounds, or a cursor
// that is malformed or another order's is refused as invalid input.
func TestListCommunityRefusals(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	viewer := newTestProfile(t, db, "viewer")
	nameCursor := communityCursor{Sort: "name", Key: "a", ID: uuid.NewString()}.encode()
	for name, q := range map[string]CommunityQuery{
		"no kind":             {Sort: "name"},
		"unknown kind":        {Kind: "recipe", Sort: "name"},
		"no sort":             {Kind: kindCatalog},
		"unknown sort":        {Kind: kindCatalog, Sort: "oldest"},
		"long search":         {Kind: kindCatalog, Sort: "name", Search: strings.Repeat("a", maxSearchLen+1)},
		"too many words":      {Kind: kindCatalog, Sort: "name", Search: "a b c d e f g h i"},
		"malformed cursor":    {Kind: kindCatalog, Sort: "name", Cursor: "not a cursor"},
		"another's cursor":    {Kind: kindCatalog, Sort: "newest", Cursor: nameCursor},
		"cursor not base64'd": {Kind: kindCatalog, Sort: "name", Cursor: `{"s":"name"}`},
	} {
		if _, err := db.ListCommunity(ctx, viewer, q); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("%s: err = %v, want ErrInvalidInput", name, err)
		}
	}
	repeated := CommunityQuery{Kind: kindCatalog, Sort: "name", Search: "a A b B c C d D e E f F g G h H"}
	if _, err := db.ListCommunity(ctx, viewer, repeated); err != nil {
		t.Errorf("eight words, each twice: %v, want them counted once", err)
	}
}

// Each order reads a page as a range of its own index, with no sort of its
// own, from the first page and after a cursor.
func TestListCommunityUsesItsIndexes(t *testing.T) {
	db := newTestDB(t)
	viewer := newTestProfile(t, db, "viewer")
	for sort, index := range map[string]string{"newest": "publications_by_kind_newest", "name": "publications_by_kind_title"} {
		cursor := communityCursor{Sort: sort, Key: "k", ID: uuid.NewString()}.encode()
		for _, q := range []CommunityQuery{{Kind: kindCatalog, Sort: sort}, {Kind: kindCatalog, Sort: sort, Cursor: cursor}} {
			plan, err := q.plan()
			if err != nil {
				t.Fatal(err)
			}
			detail := queryPlan(t, db, plan.query(), plan.args(viewer))
			if !strings.Contains(detail, index) || strings.Contains(detail, "TEMP B-TREE") {
				t.Errorf("%s (cursor %t): plan %q, want %s and no temporary sort", sort, q.Cursor != "", detail, index)
			}
		}
	}
}

// queryPlan is EXPLAIN QUERY PLAN's detail lines for query, joined.
func queryPlan(t *testing.T, db *DB, query string, args []any) string {
	t.Helper()
	rows, err := db.conn.QueryContext(context.Background(), "EXPLAIN QUERY PLAN "+query, args...)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var lines []string
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		lines = append(lines, detail)
	}
	return strings.Join(lines, "; ")
}

// itemIDs lists each item's id.
func itemIDs(items []CommunityItem) []string {
	out := []string{}
	for _, item := range items {
		out = append(out, item.ID.String())
	}
	return out
}
