package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hiidz/uno/internal/nuvio"
	"github.com/hiidz/uno/internal/provider"
	"github.com/hiidz/uno/internal/vault"
)

// These tests drive whole user journeys through the real router over a real
// temp SQLite vault, with only the two outside services faked: Nuvio (a
// fakeNuvio per account) and TMDB (fakeTMDB). They assert what a client sees
// on the wire and in what Nuvio is sent, never a package's internals, so a
// refactor that keeps behaviour keeps them green.

// bearerIsSub authenticates a bearer token as the account named by it.
type bearerIsSub struct{}

func (bearerIsSub) Verify(_ context.Context, token string) (nuvio.Claims, error) {
	return nuvio.Claims{Sub: token}, nil
}

// flowTMDB serves discover (one film on page 1, none after), its IMDb id and a
// genre list.
func flowTMDB(w http.ResponseWriter, r *http.Request) {
	switch {
	case strings.HasPrefix(r.URL.Path, "/3/discover/") && r.URL.Query().Get("page") != "1":
		fmt.Fprint(w, `{"results":[],"total_results":1,"total_pages":1}`)
	case strings.HasPrefix(r.URL.Path, "/3/discover/"):
		fmt.Fprint(w, `{"results":[{"id":155,"title":"The Dark Knight","genre_ids":[80],"release_date":"2008-07-16"}],"total_results":1,"total_pages":1}`)
	case strings.HasSuffix(r.URL.Path, "/external_ids"):
		fmt.Fprint(w, `{"imdb_id":"tt0468569"}`)
	case strings.HasPrefix(r.URL.Path, "/3/genre/"):
		fmt.Fprint(w, `{"genres":[{"id":28,"name":"Action"},{"id":80,"name":"Crime"}]}`)
	default:
		http.NotFound(w, r)
	}
}

// account is one signed-in Nuvio account using its own router over the shared
// vault: its requests carry its sub as the bearer token, and its Nuvio holds
// one profile in slot 1.
type account struct {
	t     *testing.T
	s     *Server
	sub   string
	nuvio *fakeNuvio
}

func newAccount(t *testing.T, db *vault.DB, sub string) *account {
	t.Helper()
	n := &fakeNuvio{profiles: []nuvio.NuvioProfile{{ID: "nuvio-" + sub, UserID: sub, ProfileIndex: 1}}}
	s, err := New(Deps{
		Vault:        db,
		Provider:     provider.NewTMDBClient("key"),
		Verifier:     bearerIsSub{},
		Nuvio:        n,
		SiteBaseURL:  "http://uno.example",
		NuvioBaseURL: "https://nuvio.example.com",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return &account{t: t, s: s, sub: sub, nuvio: n}
}

func (a *account) do(method, path, body string, want int) *httptest.ResponseRecorder {
	a.t.Helper()
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req := httptest.NewRequestWithContext(a.t.Context(), method, path, rd)
	if !strings.HasPrefix(path, "/u/") {
		req.Header.Set("Authorization", "Bearer "+a.sub)
	}
	w := httptest.NewRecorder()
	a.s.ServeHTTP(w, req)
	if w.Code != want {
		a.t.Fatalf("%s %s as %s = %d, want %d (body %q)", method, path, a.sub, w.Code, want, w.Body.String())
	}
	return w
}

func decodeAs[T any](t *testing.T, w *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
		t.Fatalf("decoding %q: %v", w.Body.String(), err)
	}
	return v
}

// selectProfile picks the account's Nuvio profile in slot 1 and returns the
// path of its addon manifest.
func (a *account) selectProfile() string {
	a.t.Helper()
	w := a.do(http.MethodPost, "/api/profiles/select", `{"profile_index":1}`, http.StatusOK)
	url := decodeAs[struct {
		ManifestURL string `json:"manifest_url"`
	}](a.t, w).ManifestURL
	path, ok := strings.CutPrefix(url, "http://uno.example")
	if !ok || !strings.HasPrefix(path, "/u/") {
		a.t.Fatalf("manifest_url = %q, want one on the site under /u/", url)
	}
	return path
}

func catalogBody(name string) string {
	return fmt.Sprintf(`{"type":"movie","name":%q,"provider":"tmdb","params":"{\"sort_by\":\"popularity.desc\"}"}`, name)
}

// createCatalog makes a catalog in the account's slot 1 and returns its id.
func (a *account) createCatalog(name string) string {
	a.t.Helper()
	w := a.do(http.MethodPost, "/api/p/1/catalogs", catalogBody(name), http.StatusCreated)
	return decodeAs[struct{ ID string }](a.t, w).ID
}

// libraryCatalog is the account's catalog id from its library, as the builder
// reads it.
type libraryCatalog struct {
	ID           string
	Name         string
	Publication  *struct{ ID string }
	Subscription *struct {
		PublicationID   string `json:"publication_id"`
		UpdateAvailable bool   `json:"update_available"`
	}
}

type libraryView struct {
	Catalogs []libraryCatalog
	Pending  []struct{ Kind, ID, Name, Change string }
}

func (a *account) library() libraryView {
	a.t.Helper()
	return decodeAs[libraryView](a.t, a.do(http.MethodGet, "/api/p/1/library", "", http.StatusOK))
}

// push sends the whole Home as the given catalog ids, each shown on Home, and
// expects it to succeed.
func (a *account) push(catalogIDs ...string) {
	a.t.Helper()
	rows := make([]string, len(catalogIDs))
	for i, id := range catalogIDs {
		rows[i] = fmt.Sprintf(`{"catalog_id":%q,"show_in_home":true}`, id)
	}
	w := a.do(http.MethodPost, "/api/p/1/push", `{"rows":[`+strings.Join(rows, ",")+`]}`, http.StatusOK)
	if r := decodeAs[pushResult](a.t, w); !r.Success {
		a.t.Fatalf("push = %+v, want success", r)
	}
}

type manifestView struct {
	Catalogs []struct{ Type, ID, Name string }
}

// manifest is what Nuvio would read at the account's manifest path.
func (a *account) manifest(path string) manifestView {
	a.t.Helper()
	return decodeAs[manifestView](a.t, a.do(http.MethodGet, path, "", http.StatusOK))
}

// titles are the titles Nuvio would show for the manifest's catalog entry.
func (a *account) titles(manifestPath, manifestID string, want int) []string {
	a.t.Helper()
	path := strings.TrimSuffix(manifestPath, "manifest.json") + "catalog/movie/" + manifestID + ".json"
	w := a.do(http.MethodGet, path, "", want)
	if want != http.StatusOK {
		return nil
	}
	var page struct{ Metas []struct{ Name string } }
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
		a.t.Fatalf("decoding %q: %v", w.Body.String(), err)
	}
	names := make([]string, len(page.Metas))
	for i, m := range page.Metas {
		names[i] = m.Name
	}
	return names
}

// A catalog built in the builder reaches Nuvio only at a push: the addon
// serves what the last push stored, a Save or a delete waits, and the library
// says what is waiting.
func TestFlow_BuildPushServe(t *testing.T) {
	fakeTMDB(t, flowTMDB)
	db := newTestVaultDB(t)
	alice := newAccount(t, db, "alice")
	manifestPath := alice.selectProfile()

	id := alice.createCatalog("Crime Classics")

	if got := alice.manifest(manifestPath).Catalogs; len(got) != 0 {
		t.Fatalf("manifest before any push lists %+v, want nothing", got)
	}

	alice.push(id)

	if len(alice.nuvio.pushAddonsCalls) != 1 || len(alice.nuvio.pushAddonsCalls[0]) != 1 ||
		alice.nuvio.pushAddonsCalls[0][0].URL != "http://uno.example"+manifestPath {
		t.Fatalf("addons sent to Nuvio = %+v, want Uno's manifest URL once", alice.nuvio.pushAddonsCalls)
	}
	published := alice.manifest(manifestPath).Catalogs
	if len(published) != 1 || published[0].Name != "Crime Classics" || published[0].Type != "movie" {
		t.Fatalf("manifest after push lists %+v, want Crime Classics", published)
	}
	if got := alice.titles(manifestPath, published[0].ID, http.StatusOK); len(got) != 1 || got[0] != "The Dark Knight" {
		t.Fatalf("catalog page = %v, want The Dark Knight", got)
	}
	if lib := alice.library(); len(lib.Pending) != 0 {
		t.Fatalf("pending after a push = %+v, want none", lib.Pending)
	}

	alice.do(http.MethodPut, "/api/p/1/catalogs/"+id, catalogBody("Crime Renamed"), http.StatusOK)
	if got := alice.manifest(manifestPath).Catalogs; len(got) != 1 || got[0].Name != "Crime Classics" {
		t.Fatalf("manifest after a Save before a push lists %+v, want the pushed name", got)
	}
	if lib := alice.library(); len(lib.Pending) != 1 || lib.Pending[0].Change != vault.PendingChanged {
		t.Fatalf("pending after a Save = %+v, want one changed row", lib.Pending)
	}
	alice.push(id)
	if got := alice.manifest(manifestPath).Catalogs; len(got) != 1 || got[0].Name != "Crime Renamed" {
		t.Fatalf("manifest after the second push lists %+v, want the new name", got)
	}

	alice.do(http.MethodDelete, "/api/p/1/catalogs/"+id, "", http.StatusNoContent)
	alice.titles(manifestPath, published[0].ID, http.StatusOK)
	if lib := alice.library(); len(lib.Pending) != 1 || lib.Pending[0].Change != vault.PendingRemoved {
		t.Fatalf("pending after a delete = %+v, want one removed row", lib.Pending)
	}
	alice.push()
	alice.titles(manifestPath, published[0].ID, http.StatusNotFound)
	if got := alice.manifest(manifestPath).Catalogs; len(got) != 0 {
		t.Fatalf("manifest after pushing an empty Home lists %+v, want nothing", got)
	}
}

// Two accounts share a catalog end to end: the publisher publishes, the
// subscriber adds it and sees it in Community, the publisher changes it and
// publishes again, and the subscriber is told, reads what changed and updates.
// Unpublishing leaves the subscriber a copy that is theirs.
func TestFlow_PublishSubscribeUpdate(t *testing.T) {
	fakeTMDB(t, flowTMDB)
	db := newTestVaultDB(t)
	alice := newAccount(t, db, "alice")
	bob := newAccount(t, db, "bob")
	alice.selectProfile()
	bob.selectProfile()

	id := alice.createCatalog("Heist Films")
	published := decodeAs[libraryCatalog](t, alice.do(http.MethodPost, "/api/p/1/catalogs/"+id+"/publish", "", http.StatusOK))
	if published.Publication == nil {
		t.Fatal("publish answered a catalog with no publication")
	}
	pub := "/api/p/1/community/" + published.Publication.ID

	type page struct{ Items []struct{ ID string } }
	const list = "/api/p/1/community?kind=catalog&sort=newest"
	if got := decodeAs[page](t, alice.do(http.MethodGet, list, "", http.StatusOK)).Items; len(got) != 0 {
		t.Fatalf("the publisher's own Community = %+v, want others' work only", got)
	}
	if got := decodeAs[page](t, bob.do(http.MethodGet, list, "", http.StatusOK)).Items; len(got) != 1 || got[0].ID != published.Publication.ID {
		t.Fatalf("the subscriber's Community = %+v, want the publication", got)
	}

	copyID := decodeAs[struct{ Catalog libraryCatalog }](t, bob.do(http.MethodPost, pub+"/subscribe", "", http.StatusCreated)).Catalog.ID
	bob.do(http.MethodPost, pub+"/subscribe", "", http.StatusConflict)
	bob.do(http.MethodPut, "/api/p/1/catalogs/"+copyID, catalogBody("Mine now"), http.StatusBadRequest)
	bob.do(http.MethodPut, "/api/p/1/catalogs/"+id, catalogBody("Not mine"), http.StatusNotFound)
	if lib := bob.library(); len(lib.Catalogs) != 1 || lib.Catalogs[0].Name != "Heist Films" ||
		lib.Catalogs[0].Subscription == nil || lib.Catalogs[0].Subscription.UpdateAvailable {
		t.Fatalf("subscriber's library = %+v, want the copy, up to date", lib.Catalogs)
	}

	alice.do(http.MethodPut, "/api/p/1/catalogs/"+id, catalogBody("Heist Films 2"), http.StatusOK)
	bob.do(http.MethodGet, pub+"/changes", "", http.StatusOK)
	if lib := bob.library(); lib.Catalogs[0].Subscription.UpdateAvailable {
		t.Fatal("the subscriber was told of an update before the publisher published it")
	}
	alice.do(http.MethodPost, "/api/p/1/catalogs/"+id+"/publish", "", http.StatusOK)

	if lib := bob.library(); lib.Catalogs[0].Name != "Heist Films" || !lib.Catalogs[0].Subscription.UpdateAvailable {
		t.Fatalf("subscriber's library after a republish = %+v, want the old copy and an update available", lib.Catalogs)
	}
	if changes := bob.do(http.MethodGet, pub+"/changes", "", http.StatusOK).Body.String(); !strings.Contains(changes, "Heist Films 2") {
		t.Fatalf("changes = %s, want the rename", changes)
	}
	bob.do(http.MethodPost, pub+"/update", "", http.StatusOK)
	if lib := bob.library(); lib.Catalogs[0].Name != "Heist Films 2" || lib.Catalogs[0].Subscription.UpdateAvailable {
		t.Fatalf("subscriber's library after Update = %+v, want the new name, up to date", lib.Catalogs)
	}

	alice.do(http.MethodPost, "/api/p/1/catalogs/"+id+"/unpublish", "", http.StatusOK)
	bob.do(http.MethodPost, pub+"/update", "", http.StatusNotFound)
	bob.do(http.MethodPut, "/api/p/1/catalogs/"+copyID, catalogBody("Mine now"), http.StatusOK)
	if lib := bob.library(); len(lib.Catalogs) != 1 || lib.Catalogs[0].Name != "Mine now" || lib.Catalogs[0].Subscription != nil {
		t.Fatalf("subscriber's library after unpublish = %+v, want their own renamed copy", lib.Catalogs)
	}
	if got := decodeAs[page](t, bob.do(http.MethodGet, list, "", http.StatusOK)).Items; len(got) != 0 {
		t.Fatalf("Community after unpublish = %+v, want it gone", got)
	}
}

// A Home names each catalog or collection once, and a refused push reaches
// neither Nuvio nor the vault's record of what Nuvio holds.
func TestFlow_PushRefusesARepeatedRow(t *testing.T) {
	fakeTMDB(t, flowTMDB)
	alice := newAccount(t, newTestVaultDB(t), "alice")
	manifestPath := alice.selectProfile()
	id := alice.createCatalog("Crime Classics")

	row := fmt.Sprintf(`{"catalog_id":%q,"show_in_home":true}`, id)
	alice.do(http.MethodPost, "/api/p/1/push", `{"rows":[`+row+`,`+row+`]}`, http.StatusBadRequest)

	if n := len(alice.nuvio.pushAddonsCalls) + len(alice.nuvio.pushCollectionsCalls) + len(alice.nuvio.pushHomeOrderCalls); n != 0 {
		t.Fatalf("Nuvio was written to %d times by a refused push", n)
	}
	if got := alice.manifest(manifestPath).Catalogs; len(got) != 0 {
		t.Fatalf("manifest after a refused push lists %+v, want nothing", got)
	}
}
