package api

import (
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/hiidz/uno/internal/vault"
)

// sharingFixture is another profile's (publisher's) published catalog and
// collection, and the caller's own catalog and collection, all with recipes
// that TMDB never needs to check.
type sharingFixture struct {
	f                                      routeFixture
	publisher                              uuid.UUID
	theirCatalog, theirCollection          uuid.UUID
	ownCatalog, ownCollection              uuid.UUID
	theirCatalogSource, theirCollectionSrc uuid.UUID
}

func newSharingFixture(t *testing.T) sharingFixture {
	t.Helper()
	ctx := t.Context()
	f := newRouteFixture(t)
	owner, err := f.db.ResolveOrCreateProfile(ctx, "owner", 1, "nuvio-profile-owner")
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := f.db.CreateUserCatalog(ctx, owner.ID, vault.CatalogForm{Type: "movie", Name: "Theirs", Provider: "tmdb", Params: popular})
	if err != nil {
		t.Fatal(err)
	}
	catalog, err = f.db.PublishCatalog(ctx, owner.ID, catalog.ID)
	if err != nil {
		t.Fatal(err)
	}
	collection, err := f.db.CreateUserCollection(ctx, owner.ID, vault.CollectionForm{
		Title: "Their Weekend", ViewMode: "TABBED_GRID",
		Folders: []vault.FolderData{{FolderArt: vault.FolderArt{TileShape: "POSTER"}, Title: "Folder", Catalogs: vault.CatalogRefs(catalog.ID)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	collection, err = f.db.PublishCollection(ctx, owner.ID, collection.ID)
	if err != nil {
		t.Fatal(err)
	}
	return sharingFixture{
		f: f, publisher: owner.ID, theirCatalog: catalog.Publication.ID, theirCollection: collection.Publication.ID,
		ownCatalog: f.mine.ID, ownCollection: f.mineColl.ID,
		theirCatalogSource: catalog.ID, theirCollectionSrc: collection.ID,
	}
}

// TestSharingRoutes drives the Community, subscription and publication
// routes through the real router, as the caller, in order: each step's
// answer depends on what the steps before it did. None of them reaches TMDB:
// a publish, a subscribe, an Update and a duplicate read and write the vault
// only.
func TestSharingRoutes(t *testing.T) {
	x := newSharingFixture(t)
	noTMDB(t)
	community := "/api/p/1/community/"
	theirCatalog := community + x.theirCatalog.String()
	theirCollection := community + x.theirCollection.String()

	runSteps(t, x.f.s, []routeStep{
		{name: "list", method: http.MethodGet, path: "/api/p/1/community?kind=catalog&sort=newest", wantStatus: http.StatusOK, wantBody: x.theirCatalog.String()},
		{name: "list ends on one page", method: http.MethodGet, path: "/api/p/1/community?kind=catalog&sort=name", wantStatus: http.StatusOK, wantBody: `"next_cursor":null`},
		{name: "list with no kind", method: http.MethodGet, path: "/api/p/1/community?sort=name", wantStatus: http.StatusBadRequest, wantBody: "kind must be catalog or collection"},
		{name: "list in an unknown order", method: http.MethodGet, path: "/api/p/1/community?kind=catalog&sort=oldest", wantStatus: http.StatusBadRequest, wantBody: "sort must be name or newest"},
		{name: "list after a cursor that is none", method: http.MethodGet, path: "/api/p/1/community?kind=catalog&sort=name&cursor=nope", wantStatus: http.StatusBadRequest, wantBody: "cursor is not one of this list's"},
		{name: "list searched for too many words", method: http.MethodGet, path: "/api/p/1/community?kind=catalog&sort=name&q=a+b+c+d+e+f+g+h+i", wantStatus: http.StatusBadRequest, wantBody: "search has over 8 words"},
		{name: "list holds the collection", method: http.MethodGet, path: "/api/p/1/community?kind=collection&sort=name&q=weekend", wantStatus: http.StatusOK, wantBody: `"title":"Their Weekend"`},
		{name: "detail", method: http.MethodGet, path: theirCollection, wantStatus: http.StatusOK, wantBody: `"snapshot":{"format":"uno-publication"`},
		{name: "detail of an unknown publication", method: http.MethodGet, path: community + uuid.NewString(), wantStatus: http.StatusNotFound, wantBody: "not in Community any more"},
		{name: "detail with a path id that isn't a uuid", method: http.MethodGet, path: community + "nope", wantStatus: http.StatusBadRequest, wantBody: "invalid publication id"},
		{name: "update before subscribing", method: http.MethodPost, path: theirCatalog + "/update", wantStatus: http.StatusNotFound, wantBody: "not in Community any more"},
		{name: "duplicate", method: http.MethodPost, path: theirCatalog + "/duplicate", wantStatus: http.StatusCreated, wantBody: `"subscription":null`},
		{name: "subscribe to the catalog", method: http.MethodPost, path: theirCatalog + "/subscribe", wantStatus: http.StatusCreated, wantBody: `"subscription":{"publication_id":"` + x.theirCatalog.String()},
		{name: "subscribe again", method: http.MethodPost, path: theirCatalog + "/subscribe", wantStatus: http.StatusConflict, wantBody: "already added"},
		{name: "subscribe to the collection", method: http.MethodPost, path: theirCollection + "/subscribe", wantStatus: http.StatusCreated, wantBody: `"kind":"collection"`},
		{name: "update in step", method: http.MethodPost, path: theirCatalog + "/update", wantStatus: http.StatusOK, wantBody: `"update_available":false`},
		{name: "changes of a copy in step", method: http.MethodGet, path: theirCollection + "/changes", wantStatus: http.StatusOK, wantBody: `[]`},
		{name: "changes of a publication not added", method: http.MethodGet, path: community + uuid.NewString() + "/changes", wantStatus: http.StatusNotFound, wantBody: "not in Community any more"},
		{name: "changes with a path id that isn't a uuid", method: http.MethodGet, path: community + "nope/changes", wantStatus: http.StatusBadRequest, wantBody: "invalid publication id"},
		{name: "list after subscribing", method: http.MethodGet, path: "/api/p/1/community?kind=catalog&sort=newest", wantStatus: http.StatusOK, wantBody: `"subscribed":true`},
	})

	collectionCopy := subscribedCopy(t, x.f, x.theirCollection)
	catalogCopy := subscribedCatalog(t, x.f, x.theirCatalog)
	withSubscribed, err := x.f.db.CreateUserCollection(t.Context(), x.f.caller.ID, vault.CollectionForm{
		Title: "With a subscribed catalog", ViewMode: "ROWS",
		Folders: []vault.FolderData{{FolderArt: vault.FolderArt{TileShape: "POSTER"}, Title: "F", Catalogs: vault.CatalogRefs(catalogCopy)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	copyPath := "/api/p/1/collections/" + collectionCopy.String()
	catalogPath := "/api/p/1/catalogs/" + catalogCopy.String()
	ownCatalog := "/api/p/1/catalogs/" + x.ownCatalog.String()
	ownCollection := "/api/p/1/collections/" + x.ownCollection.String()
	runSteps(t, x.f.s, []routeStep{
		{name: "publish the subscribed copy", method: http.MethodPost, path: copyPath + "/publish", wantStatus: http.StatusBadRequest, wantBody: "only its publisher can change or publish it"},
		{name: "save the subscribed collection", method: http.MethodPut, path: copyPath, body: `{"title":"Mine now","view_mode":"TABBED_GRID"}`, wantStatus: http.StatusBadRequest, wantBody: "only its publisher can change or publish it"},
		{name: "save the subscribed catalog", method: http.MethodPut, path: catalogPath, body: `{"type":"movie","name":"Mine now","provider":"tmdb","params":"{}"}`, wantStatus: http.StatusBadRequest, wantBody: "only its publisher can change or publish it"},
		{name: "the subscribed collection is unchanged", method: http.MethodGet, path: "/api/p/1/library", wantStatus: http.StatusOK, wantBody: `"subscription":{"publication_id":"` + x.theirCollection.String()},
		{name: "publish a collection with a subscribed catalog", method: http.MethodPost, path: "/api/p/1/collections/" + withSubscribed.ID.String() + "/publish", wantStatus: http.StatusOK, wantBody: `"publication":{"id":"`},
		{name: "changes since publish of a catalog never published", method: http.MethodGet, path: ownCatalog + "/changes-since-publish", wantStatus: http.StatusOK, wantBody: `[]`},
		{name: "changes since publish of a subscribed catalog", method: http.MethodGet, path: catalogPath + "/changes-since-publish", wantStatus: http.StatusBadRequest, wantBody: "only its publisher can change or publish it"},
		{name: "changes since publish of a subscribed collection", method: http.MethodGet, path: copyPath + "/changes-since-publish", wantStatus: http.StatusBadRequest, wantBody: "only its publisher can change or publish it"},
		{name: "changes since publish of another profile's catalog", method: http.MethodGet, path: "/api/p/1/catalogs/" + x.theirCatalogSource.String() + "/changes-since-publish", wantStatus: http.StatusNotFound, wantBody: "catalog not found"},
		{name: "changes since publish of another profile's collection", method: http.MethodGet, path: "/api/p/1/collections/" + x.theirCollectionSrc.String() + "/changes-since-publish", wantStatus: http.StatusNotFound, wantBody: "collection not found"},
		{name: "publish my catalog", method: http.MethodPost, path: ownCatalog + "/publish", wantStatus: http.StatusOK, wantBody: `"changed_since_publish":false`},
		{name: "changes since publish of my catalog", method: http.MethodGet, path: ownCatalog + "/changes-since-publish", wantStatus: http.StatusOK, wantBody: `[]`},
		{name: "change my catalog", method: http.MethodPut, path: ownCatalog, body: `{"type":"movie","name":"Renamed","provider":"tmdb","params":"{}","revision":1}`, wantStatus: http.StatusOK},
		{name: "changes since publish after a rename", method: http.MethodGet, path: ownCatalog + "/changes-since-publish", wantStatus: http.StatusOK, wantBody: `"op":"changed","kind":"catalog","aspect":"recipe","name":"Renamed","was":"Mine"`},
		{name: "unpublish my catalog", method: http.MethodPost, path: ownCatalog + "/unpublish", wantStatus: http.StatusOK, wantBody: `"publication":null`},
		{name: "changes since publish of an unpublished catalog", method: http.MethodGet, path: ownCatalog + "/changes-since-publish", wantStatus: http.StatusOK, wantBody: `[]`},
		{name: "publish my collection", method: http.MethodPost, path: ownCollection + "/publish", wantStatus: http.StatusOK, wantBody: `"changed_since_publish":false`},
		{name: "changes since publish of my collection", method: http.MethodGet, path: ownCollection + "/changes-since-publish", wantStatus: http.StatusOK, wantBody: `[]`},
		{name: "unpublish my collection", method: http.MethodPost, path: ownCollection + "/unpublish", wantStatus: http.StatusOK, wantBody: `"publication":null`},
		{name: "publish another profile's catalog", method: http.MethodPost, path: "/api/p/1/catalogs/" + x.theirCatalogSource.String() + "/publish", wantStatus: http.StatusNotFound, wantBody: "catalog not found"},
		{name: "unpublish another profile's collection", method: http.MethodPost, path: "/api/p/1/collections/" + x.theirCollectionSrc.String() + "/unpublish", wantStatus: http.StatusNotFound, wantBody: "collection not found"},
		{name: "publish with a path id that isn't a uuid", method: http.MethodPost, path: "/api/p/1/catalogs/nope/publish", wantStatus: http.StatusBadRequest, wantBody: "invalid catalog id"},
	})

	if _, err := x.f.db.UnpublishCatalog(t.Context(), x.publisher, x.theirCatalogSource); err != nil {
		t.Fatal(err)
	}
	runSteps(t, x.f.s, []routeStep{
		{name: "update the released catalog", method: http.MethodPost, path: theirCatalog + "/update", wantStatus: http.StatusNotFound, wantBody: "not in Community any more"},
		{name: "save the released catalog", method: http.MethodPut, path: catalogPath, body: `{"type":"movie","name":"Mine now","provider":"tmdb","params":"{}","revision":1}`, wantStatus: http.StatusOK, wantBody: `"name":"Mine now"`},
	})
}

// subscribedCopy is the id of the caller's collection subscribed to
// publicationID.
func subscribedCopy(t *testing.T, f routeFixture, publicationID uuid.UUID) uuid.UUID {
	t.Helper()
	collections, err := f.db.GetUserCollections(t.Context(), f.caller.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range collections {
		if c.Subscription != nil && c.Subscription.PublicationID == publicationID {
			return c.ID
		}
	}
	t.Fatalf("the caller has no collection subscribed to %s", publicationID)
	return uuid.Nil
}

// subscribedCatalog is the id of the caller's catalog subscribed to
// publicationID.
func subscribedCatalog(t *testing.T, f routeFixture, publicationID uuid.UUID) uuid.UUID {
	t.Helper()
	catalogs, err := f.db.GetUserCatalogs(t.Context(), f.caller.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range catalogs {
		if c.Subscription != nil && c.Subscription.PublicationID == publicationID {
			return c.ID
		}
	}
	t.Fatalf("the caller has no catalog subscribed to %s", publicationID)
	return uuid.Nil
}

// A publish reads only the vault: every recipe it shares was checked against
// TMDB when its row was written, so TMDB being down doesn't stop it.
func TestPublishMakesNoTMDBCall(t *testing.T) {
	f := newRouteFixture(t)
	catalog, err := f.db.CreateUserCatalog(t.Context(), f.caller.ID, vault.CatalogForm{
		Type: "movie", Name: "Action", Provider: "tmdb", Params: `{"with_genres":"28"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	hits := fakeTMDB(t, tmdbDown)
	w := serve(t, f.s, http.MethodPost, "/api/p/1/catalogs/"+catalog.ID.String()+"/publish", "", false)
	if w.Code != http.StatusOK || hits.Load() != 0 {
		t.Fatalf("status = %d after %d TMDB calls, want 200 after none (body %q)", w.Code, hits.Load(), w.Body.String())
	}
}
