package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hiidz/uno/internal/vault"
)

// TestCommunityRoutes drives the community POSTs through the real router,
// as the authenticated caller, against one public catalog and one public
// collection of another profile's. The steps run in order, since each one's
// answer depends on what the steps before it took: Update before any Take
// finds no linked copy, Duplicate never links, Take links, Update on a
// fresh Take changes nothing, and a second Take is a conflict.
func TestCommunityRoutes(t *testing.T) {
	db := newTestVaultDB(t)
	s := newProfileTestServer(t, db)
	ctx := t.Context()

	if _, err := db.ResolveOrCreateProfile(ctx, "test-sub", 1, "nuvio-profile-caller"); err != nil {
		t.Fatalf("ResolveOrCreateProfile (caller): %v", err)
	}
	owner, err := db.ResolveOrCreateProfile(ctx, "owner", 1, "nuvio-profile-owner")
	if err != nil {
		t.Fatalf("ResolveOrCreateProfile (owner): %v", err)
	}
	catalog, err := db.CreateUserCatalog(ctx, owner.ID, vault.CatalogForm{
		Type: "movie", Name: "Theirs", Provider: "tmdb", Params: `{"sort_by":"popularity.desc"}`, IsPublic: true, Fingerprint: "fp-theirs",
	})
	if err != nil {
		t.Fatalf("create public catalog: %v", err)
	}
	collection, err := db.CreateUserCollection(ctx, owner.ID, vault.CollectionForm{
		Title: "Theirs", IsPublic: true, ViewMode: "TABBED_GRID",
		Folders: []vault.FolderData{{Title: "Folder", Catalogs: vault.CatalogRefs(catalog.ID)}},
	})
	if err != nil {
		t.Fatalf("create public collection: %v", err)
	}
	catalogs := "/api/p/1/community/catalogs/" + catalog.ID.String()
	collections := "/api/p/1/community/collections/" + collection.ID.String()

	steps := []struct {
		name, path string
		wantStatus int
		wantBody   string // for an error, a fragment of the body
		wantLinked bool   // for a success, the result's linked
	}{
		{name: "update a catalog before taking it", path: catalogs + "/update", wantStatus: http.StatusNotFound, wantBody: "catalog not found"},
		{name: "update a collection before taking it", path: collections + "/update", wantStatus: http.StatusNotFound, wantBody: "collection not found"},
		{name: "duplicate the catalog", path: catalogs + "/duplicate", wantStatus: http.StatusCreated},
		{name: "duplicate the collection", path: collections + "/duplicate", wantStatus: http.StatusCreated},
		{name: "take the catalog", path: catalogs + "/take", wantStatus: http.StatusCreated, wantLinked: true},
		{name: "take the collection", path: collections + "/take", wantStatus: http.StatusCreated, wantLinked: true},
		{name: "update the taken catalog", path: catalogs + "/update", wantStatus: http.StatusOK, wantLinked: true},
		{name: "update the taken collection", path: collections + "/update", wantStatus: http.StatusOK, wantLinked: true},
		{name: "take the catalog again", path: catalogs + "/take", wantStatus: http.StatusConflict, wantBody: "already taken"},
		{name: "take the collection again", path: collections + "/take", wantStatus: http.StatusConflict, wantBody: "already taken"},
		{name: "a path id that isn't a uuid", path: "/api/p/1/community/catalogs/nope/update", wantStatus: http.StatusBadRequest, wantBody: "invalid catalog id"},
	}

	for _, step := range steps {
		t.Run(step.name, func(t *testing.T) {
			req := httptest.NewRequestWithContext(ctx, http.MethodPost, step.path, nil)
			req.Header.Set("Authorization", "Bearer token")
			w := httptest.NewRecorder()
			s.ServeHTTP(w, req)

			if w.Code != step.wantStatus {
				t.Fatalf("status = %d, want %d (body %q)", w.Code, step.wantStatus, w.Body.String())
			}
			if w.Code >= http.StatusBadRequest {
				if !strings.Contains(w.Body.String(), step.wantBody) {
					t.Fatalf("body = %q, want it to contain %q", strings.TrimSpace(w.Body.String()), step.wantBody)
				}
				return
			}
			var got struct {
				Linked bool `json:"linked"`
			}
			if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
				t.Fatalf("decoding body: %v", err)
			}
			if got.Linked != step.wantLinked {
				t.Fatalf("linked = %v, want %v", got.Linked, step.wantLinked)
			}
		})
	}
}
