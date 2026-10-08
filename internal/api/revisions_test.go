package api

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/hiidz/uno/internal/nuvio"
	"github.com/hiidz/uno/internal/vault"
)

// staleSave is the whole answer to a save built from a row another write has
// changed since: the fixed words, and no code.
const staleSave = `{"error":"Error saving."}`

// A catalog or collection save carries the revision its editor was built
// from: the row's own is saved and answered raised, and a stale or absent one
// is a 409 in fixed words that writes nothing.
func TestStaleSavesAreRefused(t *testing.T) {
	f := newRouteFixture(t)
	catalog := "/api/p/1/catalogs/" + f.mine.ID.String()
	collection := "/api/p/1/collections/" + f.mineColl.ID.String()
	catalogAt := func(name, revision string) string {
		return `{"type":"movie","name":"` + name + `","provider":"tmdb","params":"{\"sort_by\":\"popularity.desc\"}"` + revision + `}`
	}
	collectionAt := func(title string, revision int64) string {
		form := saveFormOf(f.mineColl)
		form.Title = title
		return string(mustJSON(t, collectionSave{CollectionForm: form, Revision: revision}))
	}
	collectionWithout := func(title string) string {
		form := saveFormOf(f.mineColl)
		form.Title = title
		return string(mustJSON(t, form))
	}

	for _, tc := range []struct {
		name, method, path, body string
		code                     int
		want                     string
	}{
		{"a catalog save with no revision", http.MethodPut, catalog, catalogAt("Absent", ""), http.StatusConflict, staleSave},
		{"a catalog save at its revision", http.MethodPut, catalog, catalogAt("Saved", `,"revision":1`), http.StatusOK, `"revision":2`},
		{"a catalog save at the revision before", http.MethodPut, catalog, catalogAt("Stale", `,"revision":1`), http.StatusConflict, staleSave},
		{"a collection save with no revision", http.MethodPut, collection, collectionWithout("Absent"), http.StatusConflict, staleSave},
		{"a collection save at its revision", http.MethodPut, collection, collectionAt("Saved", 1), http.StatusOK, `"revision":2`},
		{"a collection save at the revision before", http.MethodPut, collection, collectionAt("Stale", 1), http.StatusConflict, staleSave},
	} {
		w := serve(t, f.s, tc.method, tc.path, tc.body, false)
		if body := strings.TrimSpace(w.Body.String()); w.Code != tc.code || !strings.Contains(body, tc.want) {
			t.Errorf("%s = %d %s, want %d with %s", tc.name, w.Code, body, tc.code, tc.want)
		}
	}

	lib, err := f.db.GetLibrary(t.Context(), f.caller.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range lib.Catalogs {
		if c.ID == f.mine.ID && (c.Name != "Saved" || c.Revision != 2) {
			t.Errorf("catalog after the refused saves = %q at %d, want Saved at 2", c.Name, c.Revision)
		}
	}
	for _, c := range lib.Collections {
		if c.ID == f.mineColl.ID && (c.Title != "Saved" || c.Revision != 2) {
			t.Errorf("collection after the refused saves = %q at %d, want Saved at 2", c.Title, c.Revision)
		}
	}
}

// saveFormOf is tree as a collection save would send it back unchanged: its
// settings and folders, each folder's refs by catalog id.
func saveFormOf(tree vault.CollectionWithFolders) vault.CollectionForm {
	form := vault.CollectionForm{
		Title: tree.Title, ViewMode: tree.ViewMode, ShowAllTab: tree.ShowAllTab,
		BackdropImageURL: tree.BackdropImageURL, FocusGlowEnabled: tree.FocusGlowEnabled,
	}
	for _, folder := range tree.Folders {
		id := folder.ID
		data := vault.FolderData{ID: &id, Title: folder.Title, FolderArt: folder.FolderArt}
		for _, ref := range folder.Refs {
			catalogID := ref.CatalogID
			data.Catalogs = append(data.Catalogs, vault.FolderCatalogRef{CatalogID: &catalogID, Genre: ref.Genre})
		}
		form.Folders = append(form.Folders, data)
	}
	return form
}

// A push carries the home_revision its Home was built from. The library
// reads it; a successful push answers the one it raised it to, which the
// tab's next push is built from; a push from a revision another push has
// raised since, or with none, is refused with push's plain failure body
// before anything reaches Nuvio.
func TestPushChecksHomeRevision(t *testing.T) {
	db := newTestVaultDB(t)
	profile, err := db.ResolveOrCreateProfile(t.Context(), "test-sub", 1, "nuvio-profile-caller")
	if err != nil {
		t.Fatal(err)
	}
	fake := &fakeNuvio{profiles: liveAs(profile)}
	s := newTestServer(t, db, fake)
	push := func(body string) (int, string) {
		t.Helper()
		w := serve(t, s, http.MethodPost, "/api/p/1/push", body, false)
		return w.Code, strings.TrimSpace(w.Body.String())
	}
	homeRevision := func() string {
		t.Helper()
		w := serve(t, s, http.MethodGet, "/api/p/1/library", "", false)
		return fmt.Sprint(decodeAs[vault.Library](t, w).HomeRevision)
	}

	if got := homeRevision(); got != "1" {
		t.Fatalf("library home_revision = %s, want 1", got)
	}
	for _, want := range []string{"2", "3"} {
		from := homeRevision()
		if code, body := push(`{"rows":[],"home_revision":` + from + `}`); code != http.StatusOK || body != `{"success":true,"home_revision":`+want+`}` {
			t.Fatalf("push from %s = %d %s, want 200 answering %s", from, code, body, want)
		}
	}

	writes := func() int {
		return len(fake.pushAddonsCalls) + len(fake.pushCollectionsCalls) + len(fake.pushHomeOrderCalls)
	}
	before := writes()
	fake.listProfilesErr = fmt.Errorf("%w: reached Nuvio", nuvio.ErrNuvioRequestFailed)
	fake.pullHomeOrderErr = errors.New("reached Nuvio")
	for _, body := range []string{`{"rows":[],"home_revision":2}`, `{"rows":[]}`} {
		if code, answer := push(body); code != http.StatusConflict || answer != `{"success":false}` {
			t.Errorf("push %s = %d %s, want 409 {\"success\":false}", body, code, answer)
		}
	}
	if writes() != before {
		t.Errorf("a refused push wrote to Nuvio")
	}
	if got := homeRevision(); got != "3" {
		t.Errorf("library home_revision after the refusals = %s, want 3", got)
	}
}
