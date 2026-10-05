package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/hiidz/uno/internal/provider"
	"github.com/hiidz/uno/internal/vault"
)

// bundleRouteFixture is the caller's library for the bundle route tests:
// listed catalogs A, "A again" (A's recipe) and B, "S copy" (S's recipe), and
// collection X, whose one folder references its scoped catalog S and B.
type bundleRouteFixture struct {
	db                   *vault.DB
	s                    *Server
	a, aAgain, b, sCopy  vault.Catalog
	x                    vault.CollectionWithFolders
	popular, rated, rich string
}

func newBundleRouteFixture(t *testing.T) bundleRouteFixture {
	t.Helper()
	ctx := t.Context()
	db := newTestVaultDB(t)
	f := bundleRouteFixture{
		db: db, s: newProfileTestServer(t, db),
		popular: `{"sort_by":"popularity.desc"}`, rated: `{"sort_by":"vote_average.desc"}`, rich: `{"sort_by":"revenue.desc"}`,
	}
	caller, err := db.ResolveOrCreateProfile(ctx, "test-sub", 1, "nuvio-profile-caller")
	if err != nil {
		t.Fatalf("ResolveOrCreateProfile: %v", err)
	}
	listed := func(name, params string) vault.Catalog {
		c, err := db.CreateUserCatalog(ctx, caller.ID, vault.CatalogForm{
			Type: "movie", Name: name, Provider: "tmdb", Params: params,
		})
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		return c
	}
	f.a, f.aAgain, f.b, f.sCopy = listed("A", f.popular), listed("A again", f.popular), listed("B", f.rated), listed("S copy", f.rich)
	f.x, err = db.CreateUserCollection(ctx, caller.ID, vault.CollectionForm{
		Title: "X", ViewMode: "ROWS",
		Folders: []vault.FolderData{{Title: "F", Catalogs: []vault.FolderCatalogRef{
			{New: &vault.NewScopedCatalog{Key: "s", Type: "movie", Name: "S", Provider: "tmdb", Params: f.rich}},
			{CatalogID: &f.b.ID},
		}}},
	})
	if err != nil {
		t.Fatalf("create X: %v", err)
	}
	return f
}

// recipeHashOf is the hash of the recipe a TMDB movie catalog with params is
// stored under.
func recipeHashOf(t *testing.T, params string) string {
	t.Helper()
	canonical, err := provider.CanonicalParams("movie", "tmdb", params)
	if err != nil {
		t.Fatalf("CanonicalParams: %v", err)
	}
	return vault.RecipeHash("movie", "tmdb", canonical)
}

// post sends body to path as the authenticated caller.
func (f bundleRouteFixture) post(t *testing.T, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/p/1/"+path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer token")
	w := httptest.NewRecorder()
	f.s.ServeHTTP(w, req)
	return w
}

// postOK is post, failing unless the answer is want, decoding it into v.
func (f bundleRouteFixture) postOK(t *testing.T, path, body string, want int, v any) {
	t.Helper()
	w := f.post(t, path, body)
	if w.Code != want {
		t.Fatalf("POST %s: status = %d, want %d (body %q)", path, w.Code, want, w.Body.String())
	}
	if err := json.NewDecoder(w.Body).Decode(v); err != nil {
		t.Fatalf("POST %s: decoding body: %v", path, err)
	}
}

// requireAnswer fails unless w is status with a body containing fragment.
func requireAnswer(t *testing.T, w *httptest.ResponseRecorder, status int, fragment string) {
	t.Helper()
	if w.Code != status || !strings.Contains(w.Body.String(), fragment) {
		t.Fatalf("status = %d, body %q; want %d containing %q", w.Code, strings.TrimSpace(w.Body.String()), status, fragment)
	}
}

// storedRecipeHash is the recipe hash stored for catalog id.
func (f bundleRouteFixture) storedRecipeHash(t *testing.T, id uuid.UUID) string {
	t.Helper()
	rows, err := f.db.GetCatalogsByIDs(context.Background(), []uuid.UUID{id})
	if err != nil || len(rows) != 1 {
		t.Fatalf("GetCatalogsByIDs(%s) = %d rows (%v)", id, len(rows), err)
	}
	return rows[0].RecipeHash
}

// Export, check and import through the router: the export of A and X
// carries A and B top-level and S inside X; the check matches each against
// the caller's catalogs of the same recipe; a plain import writes new rows
// stored under their recipes, and one reusing B points X's ref at B.
func TestBundleRoutes(t *testing.T) {
	f := newBundleRouteFixture(t)

	var exported json.RawMessage
	f.postOK(t, "export", `{"catalog_ids":["`+f.a.ID.String()+`"],"collection_ids":["`+f.x.ID.String()+`"]}`, http.StatusOK, &exported)
	var b vault.Bundle
	if err := json.Unmarshal(exported, &b); err != nil {
		t.Fatalf("decoding the export: %v", err)
	}
	if got := []string{b.Catalogs[0].Name, b.Catalogs[1].Name, b.Collections[0].Catalogs[0].Name}; !slices.Equal(got, []string{"A", "B", "S"}) {
		t.Fatalf("exported catalogs = %v, want A and B top-level and S in X", got)
	}
	bundleBody := `{"bundle":` + string(exported)

	var check importCheck
	f.postOK(t, "import/check", bundleBody+`}`, http.StatusOK, &check)
	wantCheck := importCheck{Catalogs: 3, Collections: 1, Folders: 1, Matches: []importMatch{
		{Key: "c1", Name: "A", Type: "movie", Scope: "listed", Existing: []existingCatalog{{f.a.ID, "A"}, {f.aAgain.ID, "A again"}}},
		{Key: "c3", Name: "B", Type: "movie", Scope: "listed", Existing: []existingCatalog{{f.b.ID, "B"}}},
		{Key: "c2", Name: "S", Type: "movie", Scope: "scoped", Collection: "X", Existing: []existingCatalog{{f.sCopy.ID, "S copy"}}},
	}}
	if got, _ := json.Marshal(check); string(got) != string(mustJSON(t, wantCheck)) {
		t.Fatalf("check = %s\nwant    %s", got, mustJSON(t, wantCheck))
	}

	var imported importResult
	f.postOK(t, "import", bundleBody+`}`, http.StatusCreated, &imported)
	if len(imported.Catalogs) != 2 || len(imported.Collections) != 1 {
		t.Fatalf("imported %d catalogs and %d collections, want 2 and 1", len(imported.Catalogs), len(imported.Collections))
	}
	if got := f.storedRecipeHash(t, imported.Catalogs[0].ID); got != recipeHashOf(t, f.popular) {
		t.Errorf("imported A's recipe hash = %q", got)
	}
	scoped := imported.Collections[0].Folders[0].Refs[0].CatalogID
	if got := f.storedRecipeHash(t, scoped); got != recipeHashOf(t, f.rich) {
		t.Errorf("imported S's recipe hash = %q", got)
	}

	var reused importResult
	f.postOK(t, "import", bundleBody+`,"reuse":{"c3":"`+f.b.ID.String()+`"}}`, http.StatusCreated, &reused)
	if len(reused.Catalogs) != 1 || reused.Catalogs[0].Name != "A" {
		t.Fatalf("reusing B imported %v, want A alone", reused.Catalogs)
	}
	if got := reused.Collections[0].Folders[0].Refs[1].CatalogID; got != f.b.ID {
		t.Errorf("X's ref to B = %s, want the reused row %s", got, f.b.ID)
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

// A check that matches nothing answers matches as [].
func TestImportCheckWithoutMatches(t *testing.T) {
	f := newBundleRouteFixture(t)
	w := f.post(t, "import/check", `{"bundle":{"format":"uno","version":1,"catalogs":[
		{"key":"c1","name":"New","type":"series","provider":"tmdb","params":{"sort_by":"popularity.desc"}}],"collections":[]}}`)
	requireAnswer(t, w, http.StatusOK, `"matches":[]`)
}

// Every way a bundle route refuses a request, each before anything is
// written.
func TestBundleRoutesRefuse(t *testing.T) {
	f := newBundleRouteFixture(t)
	bundleOf := func(params string) string {
		return `{"bundle":{"format":"uno","version":1,"catalogs":[{"key":"c1","name":"N","type":"movie","provider":"tmdb","params":` + params + `}],"collections":[]}`
	}

	for _, tc := range []struct {
		name, path, body string
		status           int
		fragment         string
	}{
		{"an empty export", "export", `{}`, http.StatusBadRequest, "select at least one"},
		{"a malformed export body", "export", `{`, http.StatusBadRequest, "invalid request body"},
		{"a malformed import body", "import", `{`, http.StatusBadRequest, "invalid request body"},
		{"a bundle failing Validate", "import/check", `{"bundle":{"format":"zip","version":1}}`, http.StatusBadRequest, `bundle: format must be "uno"`},
		{"a bad recipe on check", "import/check", bundleOf(`{"sort_by":"bogus.desc"}`) + `}`, http.StatusBadRequest, "catalog c1: invalid input"},
		{"a bad recipe on import", "import", bundleOf(`{"sort_by":"bogus.desc"}`) + `}`, http.StatusBadRequest, "catalog c1: invalid input"},
		{"a mistyped bundle key on check", "import/check", `{"bundle":{"format":"uno","version":1,"catalogs":[],"collection":[]}}`, http.StatusBadRequest, `unknown field "collection"`},
		{"a mistyped catalog key on check", "import/check", `{"bundle":{"format":"uno","version":1,"catalogs":[{"key":"c1","nam":"N"}],"collections":[]}}`, http.StatusBadRequest, `unknown field "nam"`},
		{"a mistyped folder key on import", "import", `{"bundle":{"format":"uno","version":1,"catalogs":[],"collections":[{"title":"X","folders":[{"title":"F","tile_shap":"wide"}]}]}}`, http.StatusBadRequest, `unknown field "tile_shap"`},
		{"a mistyped request key on import", "import", bundleOf(`{}`) + `,"reus":{}}`, http.StatusBadRequest, `unknown field "reus"`},
		{"a reuse key the bundle lacks", "import", bundleOf(`{}`) + `,"reuse":{"c9":"` + f.a.ID.String() + `"}}`, http.StatusBadRequest, `reuse names catalog key "c9"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requireAnswer(t, f.post(t, tc.path, tc.body), tc.status, tc.fragment)
		})
	}

	listed, err := f.db.GetUserCatalogs(t.Context(), f.a.OwnerID)
	if err != nil || len(listed) != 4 {
		t.Fatalf("the caller owns %d listed catalogs (%v), want the fixture's 4", len(listed), err)
	}
}

// The import routes take a body up to 4 MiB and refuse a larger one with
// 413; every other route keeps the 1 MiB limit. Each padded body is valid
// JSON well past its limit, so the decoder reads up to the limit rather
// than stopping early on a syntax error.
func TestBundleRouteBodyLimits(t *testing.T) {
	f := newBundleRouteFixture(t)
	padded := func(mib int) string {
		return `{"bundle":{"format":"` + strings.Repeat("a", mib<<20) + `"}}`
	}
	for _, path := range []string{"import/check", "import"} {
		t.Run(path, func(t *testing.T) {
			requireAnswer(t, f.post(t, path, padded(2)), http.StatusBadRequest, "bundle: format must be")
			requireAnswer(t, f.post(t, path, padded(5)), http.StatusRequestEntityTooLarge, "request body too large")
		})
	}
	t.Run("export", func(t *testing.T) {
		body := `{"catalog_ids":[],"pad":"` + strings.Repeat("a", 2<<20) + `"}`
		requireAnswer(t, f.post(t, "export", body), http.StatusRequestEntityTooLarge, "request body too large")
	})
}
