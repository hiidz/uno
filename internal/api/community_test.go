package api

import (
	"net/http"
	"strings"
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
		{name: "detail on a server sharing every site", method: http.MethodGet, path: theirCollection, wantStatus: http.StatusOK, wantBody: `"withheld":[]`},
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

// On a server whose Community is off, every publish, unpublish and Community
// route is a 404, the rest of the builder answers as ever, and what was
// shared before stays: once Community is on again the publication is listed
// and the caller's copy still follows it.
func TestSharingRoutesAreAbsentWhenCommunityIsOff(t *testing.T) {
	x := newSharingFixture(t)
	if _, err := x.f.db.Subscribe(t.Context(), x.f.caller.ID, x.theirCollection); err != nil {
		t.Fatal(err)
	}
	off := newCommunityServer(t, x.f.db, CommunityOff)
	ownCatalog := "/api/p/1/catalogs/" + x.ownCatalog.String()
	ownCollection := "/api/p/1/collections/" + x.ownCollection.String()
	theirs := "/api/p/1/community/" + x.theirCollection.String()
	for _, route := range []struct{ method, path string }{
		{http.MethodPost, ownCatalog + "/publish"},
		{http.MethodPost, ownCatalog + "/unpublish"},
		{http.MethodGet, ownCatalog + "/changes-since-publish"},
		{http.MethodPost, ownCollection + "/publish"},
		{http.MethodPost, ownCollection + "/unpublish"},
		{http.MethodGet, ownCollection + "/changes-since-publish"},
		{http.MethodGet, ownCollection + "/withheld-media"},
		{http.MethodGet, "/api/p/1/community?kind=collection&sort=name"},
		{http.MethodGet, theirs},
		{http.MethodPost, theirs + "/subscribe"},
		{http.MethodGet, theirs + "/changes"},
		{http.MethodPost, theirs + "/update"},
		{http.MethodPost, theirs + "/duplicate"},
	} {
		if w := serve(t, off, route.method, route.path, "", false); w.Code != http.StatusNotFound {
			t.Errorf("%s %s = %d %s, want 404", route.method, route.path, w.Code, w.Body.String())
		}
	}
	if w := serve(t, off, http.MethodGet, "/api/p/1/library", "", false); w.Code != http.StatusOK {
		t.Errorf("GET library with Community off = %d %s, want 200", w.Code, w.Body.String())
	}

	w := serve(t, x.f.s, http.MethodGet, "/api/p/1/community?kind=collection&sort=name", "", false)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), x.theirCollection.String()) {
		t.Errorf("Community on again = %d %s, want the publication listed", w.Code, w.Body.String())
	}
	subscribedCopy(t, x.f, x.theirCollection)
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

// A publish from an account the operator has blocked fails as any failed
// publish does, a 500 in the route's own words, for a catalog and a
// collection: nothing says the account is blocked.
func TestPublishFromABlockedAccount(t *testing.T) {
	f := newRouteFixture(t)
	published, err := f.db.PublishCatalog(t.Context(), f.caller.ID, f.mine.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.TakeDown(t.Context(), published.Publication.ID, true); err != nil {
		t.Fatal(err)
	}
	for path, words := range map[string]string{
		"/api/p/1/catalogs/" + f.mine.ID.String() + "/publish":        "failed to publish catalog",
		"/api/p/1/collections/" + f.mineColl.ID.String() + "/publish": "failed to publish collection",
	} {
		w := serve(t, f.s, http.MethodPost, path, "", false)
		if body := w.Body.String(); w.Code != http.StatusInternalServerError || !strings.Contains(body, words) || strings.Contains(body, "block") {
			t.Errorf("%s = %d %q, want 500 %q and no word of the block", path, w.Code, body, words)
		}
	}
}

// The withheld-media route answers, for the caller's collection as saved,
// the pictures Community would leave out and the sites it shares pictures
// from: none and none on a server that shares every site, the collection's
// backdrop then each folder's untrusted addresses on one that trusts some.
// Another profile's collection is a 404 and a subscribed copy a 400.
func TestCollectionWithheldMediaRoute(t *testing.T) {
	x := newSharingFixture(t)
	ctx := t.Context()
	catalog, err := x.f.db.CreateUserCatalog(ctx, x.f.caller.ID, vault.CatalogForm{Type: "movie", Name: "Art", Provider: "tmdb", Params: popular})
	if err != nil {
		t.Fatal(err)
	}
	art := vault.FolderArt{
		TileShape: "POSTER", CoverImageURL: "https://image.tmdb.org/t/p/w500/cover.jpg",
		TitleLogoURL: "https://Cdn.Example.com:8443/logo.png",
	}
	collection, err := x.f.db.CreateUserCollection(ctx, x.f.caller.ID, vault.CollectionForm{
		Title: "Art", ViewMode: "ROWS", BackdropImageURL: "https://i.postimg.cc/backdrop.jpg",
		Folders: []vault.FolderData{
			{FolderArt: art, Title: "One", Catalogs: vault.CatalogRefs(catalog.ID)},
			{FolderArt: art, Title: "Two", Catalogs: vault.CatalogRefs(catalog.ID)},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	path := "/api/p/1/collections/" + collection.ID.String() + "/withheld-media"

	if w := serve(t, x.f.s, http.MethodGet, path, "", false); w.Code != http.StatusOK || strings.TrimSpace(w.Body.String()) != `{"pictures":[],"sites":[]}` {
		t.Errorf("on a server sharing every site: %d %s, want nothing withheld", w.Code, w.Body.String())
	}

	x.f.db.ShareCommunityMediaFrom([]string{"image.tmdb.org", "*.example.org"})
	trusted := newCommunityServer(t, x.f.db, CommunityTrustedImages)
	logo := `"field":"title_logo_url","url":"https://Cdn.Example.com:8443/logo.png","site":"cdn.example.com"}`
	want := `{"pictures":[` +
		`{"title":"Art","field":"backdrop_image_url","url":"https://i.postimg.cc/backdrop.jpg","site":"i.postimg.cc"},` +
		`{"title":"One",` + logo + `,{"title":"Two",` + logo +
		`],"sites":["image.tmdb.org","*.example.org"]}`
	if w := serve(t, trusted, http.MethodGet, path, "", false); w.Code != http.StatusOK || strings.TrimSpace(w.Body.String()) != want {
		t.Errorf("on a server trusting some sites: %d %s\nwant %s", w.Code, w.Body.String(), want)
	}

	if w := serve(t, trusted, http.MethodGet, "/api/p/1/collections/"+x.theirCollectionSrc.String()+"/withheld-media", "", false); w.Code != http.StatusNotFound {
		t.Errorf("another profile's collection: %d %s, want 404", w.Code, w.Body.String())
	}
	if _, err := x.f.db.Subscribe(ctx, x.f.caller.ID, x.theirCollection); err != nil {
		t.Fatal(err)
	}
	copied := subscribedCopy(t, x.f, x.theirCollection)
	if w := serve(t, trusted, http.MethodGet, "/api/p/1/collections/"+copied.String()+"/withheld-media", "", false); w.Code != http.StatusBadRequest {
		t.Errorf("a subscribed copy: %d %s, want 400", w.Code, w.Body.String())
	}
}

// A publication's page carries what Community leaves out of it: on a server
// trusting some sites, the backdrop off them, under the empty key.
func TestPublicationDetailWithheld(t *testing.T) {
	x := newSharingFixture(t)
	ctx := t.Context()
	collection, err := x.f.db.CreateUserCollection(ctx, x.publisher, vault.CollectionForm{
		Title: "Art", ViewMode: "ROWS", BackdropImageURL: "https://i.postimg.cc/backdrop.jpg",
		Folders: []vault.FolderData{{
			FolderArt: vault.FolderArt{TileShape: "POSTER", CoverImageURL: "https://image.tmdb.org/t/p/w500/cover.jpg"},
			Title:     "One", Catalogs: vault.CatalogRefs(x.theirCatalogSource),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if collection, err = x.f.db.PublishCollection(ctx, x.publisher, collection.ID); err != nil {
		t.Fatal(err)
	}
	x.f.db.ShareCommunityMediaFrom([]string{"image.tmdb.org"})
	trusted := newCommunityServer(t, x.f.db, CommunityTrustedImages)

	want := `"withheld":[{"key":"","field":"backdrop_image_url","url":"https://i.postimg.cc/backdrop.jpg"}]`
	w := serve(t, trusted, http.MethodGet, "/api/p/1/community/"+collection.Publication.ID.String(), "", false)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), want) {
		t.Errorf("detail: %d %s\nwant it to hold %s", w.Code, w.Body.String(), want)
	}
}
