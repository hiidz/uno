package api

// import (
// 	"bytes"
// 	"database/sql"
// 	"encoding/json"
// 	"net/http"
// 	"net/http/httptest"
// 	"path/filepath"
// 	"testing"

// 	"github.com/google/uuid"

// 	"github.com/hiidz/uno/internal/provider"
// 	"github.com/hiidz/uno/internal/vault"
// )

// // ---------------------------------------------------------------------------
// // Shared fixtures
// // ---------------------------------------------------------------------------

// // newTestServer spins up a Server backed by a fresh temp-file SQLite database
// // and returns it along with the raw DB path, so tests can seed rows (e.g.
// // profiles) directly via SQL — vault has no exported profile-creation method.
// func newTestServer(t *testing.T) (*Server, string) {
// 	t.Helper()

// 	dir := t.TempDir()
// 	dbPath := filepath.Join(dir, "test.db")

// 	db, err := vault.InitDB(dbPath)
// 	if err != nil {
// 		t.Fatalf("vault.InitDB() error = %v", err)
// 	}
// 	t.Cleanup(func() {
// 		if err := db.Close(); err != nil {
// 			t.Errorf("db.Close() error = %v", err)
// 		}
// 	})

// 	tmdb := provider.NewTMDBClient("test-key")

// 	return New(db, tmdb), dbPath
// }

// // insertProfile opens its own connection to the same on-disk database and
// // inserts a profile row. A second connection is used because *vault.DB does
// // not expose its underlying *sql.DB to other packages.
// func insertProfile(t *testing.T, dbPath, token, name string) uuid.UUID {
// 	t.Helper()

// 	conn, err := sql.Open("sqlite", "file:"+dbPath)
// 	if err != nil {
// 		t.Fatalf("opening test connection: %v", err)
// 	}
// 	defer conn.Close()

// 	id := uuid.New()
// 	_, err = conn.Exec(`INSERT INTO profiles (id, token, name) VALUES (?, ?, ?)`,
// 		id.String(), token, name)
// 	if err != nil {
// 		t.Fatalf("inserting test profile: %v", err)
// 	}
// 	return id
// }

// func doRequest(t *testing.T, s *Server, method, target string, body any) *httptest.ResponseRecorder {
// 	t.Helper()
// 	return doRawRequest(t, s, method, target, marshalBody(t, body), body != nil)
// }

// // doRawRequest sends a request with a pre-built body reader, letting callers
// // exercise malformed payloads that json.Marshal could never produce.
// func doRawRequest(t *testing.T, s *Server, method, target string, body *bytes.Reader, setContentType bool) *httptest.ResponseRecorder {
// 	t.Helper()

// 	req := httptest.NewRequest(method, target, body)
// 	if setContentType {
// 		req.Header.Set("Content-Type", "application/json")
// 	}
// 	rec := httptest.NewRecorder()
// 	s.ServeHTTP(rec, req)
// 	return rec
// }

// func marshalBody(t *testing.T, body any) *bytes.Reader {
// 	t.Helper()
// 	if body == nil {
// 		return bytes.NewReader(nil)
// 	}
// 	b, err := json.Marshal(body)
// 	if err != nil {
// 		t.Fatalf("marshaling request body: %v", err)
// 	}
// 	return bytes.NewReader(b)
// }

// // decode unmarshals a response body into a fresh value of type T, failing
// // the test on error.
// func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
// 	t.Helper()
// 	var v T
// 	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
// 		t.Fatalf("decoding response: %v, body = %s", err, rec.Body.String())
// 	}
// 	return v
// }

// // requireStatus fails the test with the response body attached if the
// // recorded status doesn't match want.
// func requireStatus(t *testing.T, rec *httptest.ResponseRecorder, want int) {
// 	t.Helper()
// 	if rec.Code != want {
// 		t.Fatalf("status = %d, want %d, body = %s", rec.Code, want, rec.Body.String())
// 	}
// }

// func testCatalogInput(name string) vault.CatalogForm {
// 	return vault.CatalogForm{
// 		Type:     "movie",
// 		Name:     name,
// 		Provider: "tmdb",
// 		Kind:     "discover",
// 		Endpoint: "/discover/movie",
// 		Params:   `{"sort_by":"popularity.desc"}`,
// 		IsPublic: false,
// 	}
// }

// func testCollectionInput(title string) vault.CollectionForm {
// 	return vault.CollectionForm{
// 		Title:      title,
// 		IsPublic:   false,
// 		PinToTop:   false,
// 		ViewMode:   "TABBED_GRID",
// 		ShowAllTab: false,
// 	}
// }

// // createTestCatalog creates a catalog owned by the given profile token via
// // the catalogs endpoint and returns its ID, for use as a folder catalog ref.
// func createTestCatalog(t *testing.T, s *Server, profileToken, name string) uuid.UUID {
// 	t.Helper()

// 	rec := doRequest(t, s, http.MethodPost, "/u/"+profileToken+"/catalogs", testCatalogInput(name))
// 	requireStatus(t, rec, http.StatusCreated)
// 	return decode[vault.Catalog](t, rec).ID
// }

// // ---------------------------------------------------------------------------
// // Catalogs
// // ---------------------------------------------------------------------------

// func TestListUserCatalogs(t *testing.T) {
// 	t.Run("empty for a profile with no catalogs", func(t *testing.T) {
// 		s, dbPath := newTestServer(t)
// 		insertProfile(t, dbPath, "alice-token", "Alice")

// 		rec := doRequest(t, s, http.MethodGet, "/u/alice-token/catalogs", nil)
// 		requireStatus(t, rec, http.StatusOK)

// 		if catalogs := decode[[]vault.Catalog](t, rec); len(catalogs) != 0 {
// 			t.Errorf("catalogs = %+v, want empty", catalogs)
// 		}
// 	})

// 	t.Run("unknown profile", func(t *testing.T) {
// 		s, _ := newTestServer(t)
// 		rec := doRequest(t, s, http.MethodGet, "/u/ghost-token/catalogs", nil)
// 		requireStatus(t, rec, http.StatusNotFound)
// 	})
// }

// func TestCreateUserCatalog(t *testing.T) {
// 	t.Run("creates and persists a catalog", func(t *testing.T) {
// 		s, dbPath := newTestServer(t)
// 		insertProfile(t, dbPath, "alice-token", "Alice")

// 		rec := doRequest(t, s, http.MethodPost, "/u/alice-token/catalogs", testCatalogInput("Popular Movies"))
// 		requireStatus(t, rec, http.StatusCreated)

// 		created := decode[vault.Catalog](t, rec)
// 		if created.Name != "Popular Movies" {
// 			t.Errorf("Name = %q, want %q", created.Name, "Popular Movies")
// 		}
// 		if created.ID == uuid.Nil {
// 			t.Error("created catalog has zero ID")
// 		}

// 		// Confirm it shows up on a subsequent list call.
// 		listRec := doRequest(t, s, http.MethodGet, "/u/alice-token/catalogs", nil)
// 		catalogs := decode[[]vault.Catalog](t, listRec)
// 		if len(catalogs) != 1 || catalogs[0].ID != created.ID {
// 			t.Errorf("catalogs after create = %+v, want single catalog with ID %v", catalogs, created.ID)
// 		}
// 	})

// 	t.Run("unknown profile", func(t *testing.T) {
// 		s, _ := newTestServer(t)
// 		rec := doRequest(t, s, http.MethodPost, "/u/ghost-token/catalogs", testCatalogInput("X"))
// 		requireStatus(t, rec, http.StatusNotFound)
// 	})

// 	t.Run("malformed JSON body", func(t *testing.T) {
// 		s, dbPath := newTestServer(t)
// 		insertProfile(t, dbPath, "alice-token", "Alice")

// 		rec := doRawRequest(t, s, http.MethodPost, "/u/alice-token/catalogs", bytes.NewReader([]byte("{not-json")), true)
// 		requireStatus(t, rec, http.StatusBadRequest)
// 	})

// 	validationCases := []struct {
// 		name  string
// 		input vault.CatalogForm
// 	}{
// 		{"empty name", testCatalogInput("")},
// 		{"invalid type", func() vault.CatalogForm { c := testCatalogInput("Documentaries"); c.Type = "documentary"; return c }()},
// 	}
// 	for _, tc := range validationCases {
// 		t.Run(tc.name, func(t *testing.T) {
// 			s, dbPath := newTestServer(t)
// 			insertProfile(t, dbPath, "alice-token", "Alice")

// 			rec := doRequest(t, s, http.MethodPost, "/u/alice-token/catalogs", tc.input)
// 			requireStatus(t, rec, http.StatusBadRequest)
// 		})
// 	}
// }

// func TestUpdateUserCatalog(t *testing.T) {
// 	t.Run("updates and persists fields", func(t *testing.T) {
// 		s, dbPath := newTestServer(t)
// 		insertProfile(t, dbPath, "alice-token", "Alice")

// 		createRec := doRequest(t, s, http.MethodPost, "/u/alice-token/catalogs", testCatalogInput("Original"))
// 		requireStatus(t, createRec, http.StatusCreated)
// 		created := decode[vault.Catalog](t, createRec)

// 		update := testCatalogInput("Updated")
// 		update.IsPublic = true

// 		rec := doRequest(t, s, http.MethodPut, "/u/alice-token/catalogs/"+created.ID.String(), update)
// 		requireStatus(t, rec, http.StatusOK)

// 		updated := decode[vault.Catalog](t, rec)
// 		if updated.Name != "Updated" || !updated.IsPublic {
// 			t.Errorf("updated = %+v, want Name=Updated, IsPublic=true", updated)
// 		}
// 	})

// 	t.Run("invalid id", func(t *testing.T) {
// 		s, dbPath := newTestServer(t)
// 		insertProfile(t, dbPath, "alice-token", "Alice")

// 		rec := doRequest(t, s, http.MethodPut, "/u/alice-token/catalogs/not-a-uuid", testCatalogInput("X"))
// 		requireStatus(t, rec, http.StatusBadRequest)
// 	})

// 	t.Run("unknown catalog", func(t *testing.T) {
// 		s, dbPath := newTestServer(t)
// 		insertProfile(t, dbPath, "alice-token", "Alice")

// 		rec := doRequest(t, s, http.MethodPut, "/u/alice-token/catalogs/"+uuid.New().String(), testCatalogInput("X"))
// 		requireStatus(t, rec, http.StatusNotFound)
// 	})
// }

// func TestDeleteUserCatalog(t *testing.T) {
// 	t.Run("deletes a catalog", func(t *testing.T) {
// 		s, dbPath := newTestServer(t)
// 		insertProfile(t, dbPath, "alice-token", "Alice")

// 		createRec := doRequest(t, s, http.MethodPost, "/u/alice-token/catalogs", testCatalogInput("To Delete"))
// 		requireStatus(t, createRec, http.StatusCreated)
// 		created := decode[vault.Catalog](t, createRec)

// 		rec := doRequest(t, s, http.MethodDelete, "/u/alice-token/catalogs/"+created.ID.String(), nil)
// 		requireStatus(t, rec, http.StatusNoContent)

// 		listRec := doRequest(t, s, http.MethodGet, "/u/alice-token/catalogs", nil)
// 		if catalogs := decode[[]vault.Catalog](t, listRec); len(catalogs) != 0 {
// 			t.Errorf("catalogs after delete = %+v, want empty", catalogs)
// 		}
// 	})

// 	t.Run("invalid id", func(t *testing.T) {
// 		s, dbPath := newTestServer(t)
// 		insertProfile(t, dbPath, "alice-token", "Alice")

// 		rec := doRequest(t, s, http.MethodDelete, "/u/alice-token/catalogs/not-a-uuid", nil)
// 		requireStatus(t, rec, http.StatusBadRequest)
// 	})

// 	t.Run("unknown catalog", func(t *testing.T) {
// 		s, dbPath := newTestServer(t)
// 		insertProfile(t, dbPath, "alice-token", "Alice")

// 		rec := doRequest(t, s, http.MethodDelete, "/u/alice-token/catalogs/"+uuid.New().String(), nil)
// 		requireStatus(t, rec, http.StatusNotFound)
// 	})
// }

// func TestListCommunityCatalogs(t *testing.T) {
// 	s, dbPath := newTestServer(t)
// 	insertProfile(t, dbPath, "alice-token", "Alice")
// 	insertProfile(t, dbPath, "bob-token", "Bob")

// 	publicInput := testCatalogInput("Public Catalog")
// 	publicInput.IsPublic = true
// 	privateInput := testCatalogInput("Private Catalog")

// 	requireStatus(t, doRequest(t, s, http.MethodPost, "/u/alice-token/catalogs", publicInput), http.StatusCreated)
// 	requireStatus(t, doRequest(t, s, http.MethodPost, "/u/bob-token/catalogs", privateInput), http.StatusCreated)

// 	rec := doRequest(t, s, http.MethodGet, "/catalogs", nil)
// 	requireStatus(t, rec, http.StatusOK)

// 	catalogs := decode[[]vault.Catalog](t, rec)
// 	if len(catalogs) != 1 || catalogs[0].Name != "Public Catalog" {
// 		t.Errorf("community catalogs = %+v, want only Public Catalog", catalogs)
// 	}
// }

// // ---------------------------------------------------------------------------
// // Collections
// // ---------------------------------------------------------------------------

// func TestListUserCollections(t *testing.T) {
// 	t.Run("empty for a profile with no collections", func(t *testing.T) {
// 		s, dbPath := newTestServer(t)
// 		insertProfile(t, dbPath, "alice-token", "Alice")

// 		rec := doRequest(t, s, http.MethodGet, "/u/alice-token/collections", nil)
// 		requireStatus(t, rec, http.StatusOK)

// 		if collections := decode[[]vault.CollectionWithFolders](t, rec); len(collections) != 0 {
// 			t.Errorf("collections = %+v, want empty", collections)
// 		}
// 	})

// 	t.Run("unknown profile", func(t *testing.T) {
// 		s, _ := newTestServer(t)
// 		rec := doRequest(t, s, http.MethodGet, "/u/ghost-token/collections", nil)
// 		requireStatus(t, rec, http.StatusNotFound)
// 	})
// }

// func TestCreateUserCollection(t *testing.T) {
// 	t.Run("no folders", func(t *testing.T) {
// 		s, dbPath := newTestServer(t)
// 		insertProfile(t, dbPath, "alice-token", "Alice")

// 		rec := doRequest(t, s, http.MethodPost, "/u/alice-token/collections", testCollectionInput("Watchlist"))
// 		requireStatus(t, rec, http.StatusCreated)

// 		created := decode[vault.CollectionWithFolders](t, rec)
// 		if created.Title != "Watchlist" {
// 			t.Errorf("Title = %q, want %q", created.Title, "Watchlist")
// 		}
// 		if created.ID == uuid.Nil {
// 			t.Error("created collection has zero ID")
// 		}
// 		if len(created.Folders) != 0 {
// 			t.Errorf("Folders = %+v, want empty", created.Folders)
// 		}
// 	})

// 	t.Run("with folders", func(t *testing.T) {
// 		s, dbPath := newTestServer(t)
// 		insertProfile(t, dbPath, "alice-token", "Alice")

// 		cat1 := createTestCatalog(t, s, "alice-token", "Catalog One")
// 		cat2 := createTestCatalog(t, s, "alice-token", "Catalog Two")

// 		input := testCollectionInput("Curated")
// 		input.Folders = []vault.FolderData{
// 			{
// 				Title:      "Folder A",
// 				TileShape:  "LANDSCAPE",
// 				CatalogIDs: []uuid.UUID{cat1, cat2},
// 			},
// 		}

// 		rec := doRequest(t, s, http.MethodPost, "/u/alice-token/collections", input)
// 		requireStatus(t, rec, http.StatusCreated)

// 		created := decode[vault.CollectionWithFolders](t, rec)
// 		if len(created.Folders) != 1 {
// 			t.Fatalf("Folders = %+v, want 1 folder", created.Folders)
// 		}
// 		f := created.Folders[0]
// 		if f.ID == uuid.Nil {
// 			t.Error("folder has zero ID")
// 		}
// 		if f.Title != "Folder A" {
// 			t.Errorf("folder Title = %q, want %q", f.Title, "Folder A")
// 		}
// 		if len(f.CatalogIDs) != 2 || f.CatalogIDs[0] != cat1 || f.CatalogIDs[1] != cat2 {
// 			t.Errorf("folder CatalogIDs = %v, want [%v %v] in order", f.CatalogIDs, cat1, cat2)
// 		}
// 	})

// 	t.Run("unknown profile", func(t *testing.T) {
// 		s, _ := newTestServer(t)
// 		rec := doRequest(t, s, http.MethodPost, "/u/ghost-token/collections", testCollectionInput("X"))
// 		requireStatus(t, rec, http.StatusNotFound)
// 	})

// 	t.Run("malformed JSON body", func(t *testing.T) {
// 		s, dbPath := newTestServer(t)
// 		insertProfile(t, dbPath, "alice-token", "Alice")

// 		rec := doRawRequest(t, s, http.MethodPost, "/u/alice-token/collections", bytes.NewReader([]byte("{not-json")), true)
// 		requireStatus(t, rec, http.StatusBadRequest)
// 	})

// 	validationCases := []struct {
// 		name  string
// 		input vault.CollectionForm
// 	}{
// 		{"empty title", testCollectionInput("")},
// 		{"invalid view mode", func() vault.CollectionForm {
// 			c := testCollectionInput("Bad View Mode")
// 			c.ViewMode = "CAROUSEL"
// 			return c
// 		}()},
// 		{"folder with empty title", func() vault.CollectionForm {
// 			c := testCollectionInput("Has Bad Folder")
// 			c.Folders = []vault.FolderData{{Title: ""}}
// 			return c
// 		}()},
// 	}
// 	for _, tc := range validationCases {
// 		t.Run(tc.name, func(t *testing.T) {
// 			s, dbPath := newTestServer(t)
// 			insertProfile(t, dbPath, "alice-token", "Alice")

// 			rec := doRequest(t, s, http.MethodPost, "/u/alice-token/collections", tc.input)
// 			requireStatus(t, rec, http.StatusBadRequest)
// 		})
// 	}

// 	t.Run("inaccessible catalog is rejected", func(t *testing.T) {
// 		s, dbPath := newTestServer(t)
// 		insertProfile(t, dbPath, "alice-token", "Alice")
// 		insertProfile(t, dbPath, "bob-token", "Bob")

// 		// Bob's catalog is private, so Alice has no access to it.
// 		bobsCatalog := createTestCatalog(t, s, "bob-token", "Bob's Private Catalog")

// 		input := testCollectionInput("Sneaky Collection")
// 		input.Folders = []vault.FolderData{
// 			{Title: "Folder A", CatalogIDs: []uuid.UUID{bobsCatalog}},
// 		}

// 		rec := doRequest(t, s, http.MethodPost, "/u/alice-token/collections", input)
// 		requireStatus(t, rec, http.StatusBadRequest)
// 	})

// 	t.Run("public catalog from another profile is allowed", func(t *testing.T) {
// 		s, dbPath := newTestServer(t)
// 		insertProfile(t, dbPath, "alice-token", "Alice")
// 		insertProfile(t, dbPath, "bob-token", "Bob")

// 		publicInput := testCatalogInput("Alice's Public Catalog")
// 		publicInput.IsPublic = true
// 		catRec := doRequest(t, s, http.MethodPost, "/u/alice-token/catalogs", publicInput)
// 		requireStatus(t, catRec, http.StatusCreated)
// 		alicesCatalog := decode[vault.Catalog](t, catRec)

// 		input := testCollectionInput("Bob's Collection")
// 		input.Folders = []vault.FolderData{
// 			{Title: "Folder A", CatalogIDs: []uuid.UUID{alicesCatalog.ID}},
// 		}

// 		bobRec := doRequest(t, s, http.MethodPost, "/u/bob-token/collections", input)
// 		requireStatus(t, bobRec, http.StatusCreated)

// 		created := decode[vault.CollectionWithFolders](t, bobRec)
// 		if len(created.Folders) != 1 || len(created.Folders[0].CatalogIDs) != 1 || created.Folders[0].CatalogIDs[0] != alicesCatalog.ID {
// 			t.Errorf("Folders = %+v, want one folder referencing catalog %v", created.Folders, alicesCatalog.ID)
// 		}
// 	})
// }

// func TestUpdateUserCollection(t *testing.T) {
// 	t.Run("updates top-level fields", func(t *testing.T) {
// 		s, dbPath := newTestServer(t)
// 		insertProfile(t, dbPath, "alice-token", "Alice")

// 		createRec := doRequest(t, s, http.MethodPost, "/u/alice-token/collections", testCollectionInput("Original"))
// 		requireStatus(t, createRec, http.StatusCreated)
// 		created := decode[vault.CollectionWithFolders](t, createRec)

// 		update := testCollectionInput("Renamed")
// 		update.IsPublic = true
// 		update.PinToTop = true

// 		rec := doRequest(t, s, http.MethodPut, "/u/alice-token/collections/"+created.ID.String(), update)
// 		requireStatus(t, rec, http.StatusOK)

// 		updated := decode[vault.CollectionWithFolders](t, rec)
// 		if updated.Title != "Renamed" || !updated.IsPublic || !updated.PinToTop {
// 			t.Errorf("updated = %+v, want Title=Renamed, IsPublic=true, PinToTop=true", updated)
// 		}
// 	})

// 	t.Run("diffs folders: keeps+renames, drops, adds", func(t *testing.T) {
// 		s, dbPath := newTestServer(t)
// 		insertProfile(t, dbPath, "alice-token", "Alice")

// 		cat1 := createTestCatalog(t, s, "alice-token", "Catalog One")
// 		cat2 := createTestCatalog(t, s, "alice-token", "Catalog Two")

// 		createInput := testCollectionInput("Curated")
// 		createInput.Folders = []vault.FolderData{
// 			{Title: "Folder A", CatalogIDs: []uuid.UUID{cat1}},
// 			{Title: "Folder B", CatalogIDs: []uuid.UUID{cat2}},
// 		}
// 		createRec := doRequest(t, s, http.MethodPost, "/u/alice-token/collections", createInput)
// 		requireStatus(t, createRec, http.StatusCreated)
// 		created := decode[vault.CollectionWithFolders](t, createRec)
// 		if len(created.Folders) != 2 {
// 			t.Fatalf("setup: Folders = %+v, want 2", created.Folders)
// 		}
// 		folderAID := created.Folders[0].ID
// 		folderBID := created.Folders[1].ID

// 		// Update: keep+rename Folder A with reordered/expanded catalog refs,
// 		// drop Folder B, add a brand-new Folder C.
// 		update := testCollectionInput("Curated")
// 		update.Folders = []vault.FolderData{
// 			{ID: &folderAID, Title: "Folder A Updated", CatalogIDs: []uuid.UUID{cat2, cat1}},
// 			{Title: "Folder C", CatalogIDs: []uuid.UUID{}},
// 		}

// 		rec := doRequest(t, s, http.MethodPut, "/u/alice-token/collections/"+created.ID.String(), update)
// 		requireStatus(t, rec, http.StatusOK)

// 		updated := decode[vault.CollectionWithFolders](t, rec)
// 		if len(updated.Folders) != 2 {
// 			t.Fatalf("Folders = %+v, want 2", updated.Folders)
// 		}

// 		var gotA, gotC *vault.FolderWithCatalogs
// 		for i := range updated.Folders {
// 			f := &updated.Folders[i]
// 			switch f.ID {
// 			case folderAID:
// 				gotA = f
// 			case folderBID:
// 				t.Errorf("Folder B (id=%v) still present, want deleted", folderBID)
// 			default:
// 				gotC = f
// 			}
// 		}

// 		if gotA == nil {
// 			t.Fatal("Folder A missing from update response")
// 		}
// 		if gotA.Title != "Folder A Updated" {
// 			t.Errorf("Folder A Title = %q, want %q", gotA.Title, "Folder A Updated")
// 		}
// 		if len(gotA.CatalogIDs) != 2 || gotA.CatalogIDs[0] != cat2 || gotA.CatalogIDs[1] != cat1 {
// 			t.Errorf("Folder A CatalogIDs = %v, want [%v %v] in order", gotA.CatalogIDs, cat2, cat1)
// 		}

// 		if gotC == nil {
// 			t.Fatal("Folder C missing from update response")
// 		}
// 		if gotC.ID == uuid.Nil || gotC.ID == folderAID || gotC.ID == folderBID {
// 			t.Errorf("Folder C ID = %v, want a freshly generated ID", gotC.ID)
// 		}
// 		if gotC.Title != "Folder C" {
// 			t.Errorf("Folder C Title = %q, want %q", gotC.Title, "Folder C")
// 		}
// 		if len(gotC.CatalogIDs) != 0 {
// 			t.Errorf("Folder C CatalogIDs = %v, want empty", gotC.CatalogIDs)
// 		}
// 	})

// 	t.Run("rejects a folder ID belonging to another collection", func(t *testing.T) {
// 		s, dbPath := newTestServer(t)
// 		insertProfile(t, dbPath, "alice-token", "Alice")

// 		rec1 := doRequest(t, s, http.MethodPost, "/u/alice-token/collections", testCollectionInput("Collection One"))
// 		requireStatus(t, rec1, http.StatusCreated)
// 		c1 := decode[vault.CollectionWithFolders](t, rec1)

// 		input2 := testCollectionInput("Collection Two")
// 		input2.Folders = []vault.FolderData{{Title: "Folder X"}}
// 		rec2 := doRequest(t, s, http.MethodPost, "/u/alice-token/collections", input2)
// 		requireStatus(t, rec2, http.StatusCreated)
// 		c2 := decode[vault.CollectionWithFolders](t, rec2)
// 		foreignFolderID := c2.Folders[0].ID

// 		// Attempt to update Collection One referencing a folder ID that belongs to Collection Two.
// 		update := testCollectionInput("Collection One")
// 		update.Folders = []vault.FolderData{{ID: &foreignFolderID, Title: "Hijacked"}}

// 		rec := doRequest(t, s, http.MethodPut, "/u/alice-token/collections/"+c1.ID.String(), update)
// 		requireStatus(t, rec, http.StatusBadRequest)
// 	})

// 	t.Run("invalid id", func(t *testing.T) {
// 		s, dbPath := newTestServer(t)
// 		insertProfile(t, dbPath, "alice-token", "Alice")

// 		rec := doRequest(t, s, http.MethodPut, "/u/alice-token/collections/not-a-uuid", testCollectionInput("X"))
// 		requireStatus(t, rec, http.StatusBadRequest)
// 	})

// 	t.Run("unknown collection", func(t *testing.T) {
// 		s, dbPath := newTestServer(t)
// 		insertProfile(t, dbPath, "alice-token", "Alice")

// 		rec := doRequest(t, s, http.MethodPut, "/u/alice-token/collections/"+uuid.New().String(), testCollectionInput("X"))
// 		requireStatus(t, rec, http.StatusNotFound)
// 	})
// }

// func TestDeleteUserCollection(t *testing.T) {
// 	t.Run("deletes a collection", func(t *testing.T) {
// 		s, dbPath := newTestServer(t)
// 		insertProfile(t, dbPath, "alice-token", "Alice")

// 		createRec := doRequest(t, s, http.MethodPost, "/u/alice-token/collections", testCollectionInput("To Delete"))
// 		requireStatus(t, createRec, http.StatusCreated)
// 		created := decode[vault.CollectionWithFolders](t, createRec)

// 		rec := doRequest(t, s, http.MethodDelete, "/u/alice-token/collections/"+created.ID.String(), nil)
// 		requireStatus(t, rec, http.StatusNoContent)

// 		listRec := doRequest(t, s, http.MethodGet, "/u/alice-token/collections", nil)
// 		if collections := decode[[]vault.CollectionWithFolders](t, listRec); len(collections) != 0 {
// 			t.Errorf("collections after delete = %+v, want empty", collections)
// 		}
// 	})

// 	t.Run("invalid id", func(t *testing.T) {
// 		s, dbPath := newTestServer(t)
// 		insertProfile(t, dbPath, "alice-token", "Alice")

// 		rec := doRequest(t, s, http.MethodDelete, "/u/alice-token/collections/not-a-uuid", nil)
// 		requireStatus(t, rec, http.StatusBadRequest)
// 	})

// 	t.Run("unknown collection", func(t *testing.T) {
// 		s, dbPath := newTestServer(t)
// 		insertProfile(t, dbPath, "alice-token", "Alice")

// 		rec := doRequest(t, s, http.MethodDelete, "/u/alice-token/collections/"+uuid.New().String(), nil)
// 		requireStatus(t, rec, http.StatusNotFound)
// 	})
// }

// func TestListCommunityCollections(t *testing.T) {
// 	s, dbPath := newTestServer(t)
// 	insertProfile(t, dbPath, "alice-token", "Alice")
// 	insertProfile(t, dbPath, "bob-token", "Bob")

// 	publicInput := testCollectionInput("Public Collection")
// 	publicInput.IsPublic = true
// 	privateInput := testCollectionInput("Private Collection")

// 	requireStatus(t, doRequest(t, s, http.MethodPost, "/u/alice-token/collections", publicInput), http.StatusCreated)
// 	requireStatus(t, doRequest(t, s, http.MethodPost, "/u/bob-token/collections", privateInput), http.StatusCreated)

// 	rec := doRequest(t, s, http.MethodGet, "/collections", nil)
// 	requireStatus(t, rec, http.StatusOK)

// 	collections := decode[[]vault.CollectionWithFolders](t, rec)
// 	if len(collections) != 1 || collections[0].Title != "Public Collection" {
// 		t.Errorf("community collections = %+v, want only Public Collection", collections)
// 	}
// }

// // ---------------------------------------------------------------------------
// // Catalog selection
// // ---------------------------------------------------------------------------

// func TestListCurrentCatalogSelection(t *testing.T) {
// 	t.Run("empty for a profile with no selection", func(t *testing.T) {
// 		s, dbPath := newTestServer(t)
// 		insertProfile(t, dbPath, "alice-token", "Alice")

// 		rec := doRequest(t, s, http.MethodGet, "/u/alice-token/catalogs/selection", nil)
// 		requireStatus(t, rec, http.StatusOK)

// 		if got := decode[[]vault.SelectedCatalog](t, rec); len(got) != 0 {
// 			t.Errorf("selection = %+v, want empty", got)
// 		}
// 	})

// 	t.Run("creating a catalog does not auto-select it", func(t *testing.T) {
// 		s, dbPath := newTestServer(t)
// 		insertProfile(t, dbPath, "alice-token", "Alice")
// 		createTestCatalog(t, s, "alice-token", "Unselected")

// 		rec := doRequest(t, s, http.MethodGet, "/u/alice-token/catalogs/selection", nil)
// 		requireStatus(t, rec, http.StatusOK)

// 		if got := decode[[]vault.SelectedCatalog](t, rec); len(got) != 0 {
// 			t.Errorf("selection = %+v, want empty (creation must not auto-select)", got)
// 		}
// 	})

// 	t.Run("unknown profile", func(t *testing.T) {
// 		s, _ := newTestServer(t)
// 		rec := doRequest(t, s, http.MethodGet, "/u/ghost-token/catalogs/selection", nil)
// 		requireStatus(t, rec, http.StatusNotFound)
// 	})
// }

// func TestSaveCatalogSelection(t *testing.T) {
// 	t.Run("selects and persists with show_in_home", func(t *testing.T) {
// 		s, dbPath := newTestServer(t)
// 		insertProfile(t, dbPath, "alice-token", "Alice")
// 		cat := createTestCatalog(t, s, "alice-token", "Catalog")

// 		body := vault.CatalogSelectionForm{Catalogs: []vault.SelectedCatalogInput{{CatalogID: cat, ShowInHome: true}}}
// 		rec := doRequest(t, s, http.MethodPut, "/u/alice-token/catalogs/selection", body)
// 		requireStatus(t, rec, http.StatusNoContent)

// 		listRec := doRequest(t, s, http.MethodGet, "/u/alice-token/catalogs/selection", nil)
// 		requireStatus(t, listRec, http.StatusOK)
// 		got := decode[[]vault.SelectedCatalog](t, listRec)
// 		if len(got) != 1 || got[0].ID != cat || !got[0].ShowInHome {
// 			t.Errorf("selection = %+v, want catalog %v with ShowInHome=true", got, cat)
// 		}
// 	})

// 	t.Run("second save drops omitted catalogs", func(t *testing.T) {
// 		s, dbPath := newTestServer(t)
// 		insertProfile(t, dbPath, "alice-token", "Alice")
// 		cat1 := createTestCatalog(t, s, "alice-token", "Catalog One")
// 		cat2 := createTestCatalog(t, s, "alice-token", "Catalog Two")

// 		first := vault.CatalogSelectionForm{Catalogs: []vault.SelectedCatalogInput{
// 			{CatalogID: cat1, ShowInHome: true},
// 			{CatalogID: cat2, ShowInHome: true},
// 		}}
// 		requireStatus(t, doRequest(t, s, http.MethodPut, "/u/alice-token/catalogs/selection", first), http.StatusNoContent)

// 		second := vault.CatalogSelectionForm{Catalogs: []vault.SelectedCatalogInput{{CatalogID: cat1, ShowInHome: true}}}
// 		requireStatus(t, doRequest(t, s, http.MethodPut, "/u/alice-token/catalogs/selection", second), http.StatusNoContent)

// 		listRec := doRequest(t, s, http.MethodGet, "/u/alice-token/catalogs/selection", nil)
// 		got := decode[[]vault.SelectedCatalog](t, listRec)
// 		if len(got) != 1 || got[0].ID != cat1 {
// 			t.Errorf("selection = %+v, want only catalog %v", got, cat1)
// 		}
// 	})

// 	t.Run("inaccessible catalog is rejected", func(t *testing.T) {
// 		s, dbPath := newTestServer(t)
// 		insertProfile(t, dbPath, "alice-token", "Alice")
// 		insertProfile(t, dbPath, "bob-token", "Bob")
// 		bobsCatalog := createTestCatalog(t, s, "bob-token", "Bob's Private Catalog")

// 		body := vault.CatalogSelectionForm{Catalogs: []vault.SelectedCatalogInput{{CatalogID: bobsCatalog, ShowInHome: true}}}
// 		rec := doRequest(t, s, http.MethodPut, "/u/alice-token/catalogs/selection", body)
// 		requireStatus(t, rec, http.StatusBadRequest)
// 	})

// 	t.Run("malformed JSON body", func(t *testing.T) {
// 		s, dbPath := newTestServer(t)
// 		insertProfile(t, dbPath, "alice-token", "Alice")

// 		rec := doRawRequest(t, s, http.MethodPut, "/u/alice-token/catalogs/selection", bytes.NewReader([]byte("{not-json")), true)
// 		requireStatus(t, rec, http.StatusBadRequest)
// 	})

// 	t.Run("unknown profile", func(t *testing.T) {
// 		s, _ := newTestServer(t)
// 		rec := doRequest(t, s, http.MethodPut, "/u/ghost-token/catalogs/selection", vault.CatalogSelectionForm{})
// 		requireStatus(t, rec, http.StatusNotFound)
// 	})
// }

// // ---------------------------------------------------------------------------
// // Collection selection
// // ---------------------------------------------------------------------------

// func TestListCurrentCollectionSelection(t *testing.T) {
// 	t.Run("empty for a profile with no selection", func(t *testing.T) {
// 		s, dbPath := newTestServer(t)
// 		insertProfile(t, dbPath, "alice-token", "Alice")

// 		rec := doRequest(t, s, http.MethodGet, "/u/alice-token/collections/selection", nil)
// 		requireStatus(t, rec, http.StatusOK)

// 		if got := decode[[]vault.CollectionWithFolders](t, rec); len(got) != 0 {
// 			t.Errorf("selection = %+v, want empty", got)
// 		}
// 	})

// 	t.Run("creating a collection does not auto-select it", func(t *testing.T) {
// 		s, dbPath := newTestServer(t)
// 		insertProfile(t, dbPath, "alice-token", "Alice")
// 		requireStatus(t, doRequest(t, s, http.MethodPost, "/u/alice-token/collections", testCollectionInput("Unselected")), http.StatusCreated)

// 		rec := doRequest(t, s, http.MethodGet, "/u/alice-token/collections/selection", nil)
// 		requireStatus(t, rec, http.StatusOK)

// 		if got := decode[[]vault.CollectionWithFolders](t, rec); len(got) != 0 {
// 			t.Errorf("selection = %+v, want empty (creation must not auto-select)", got)
// 		}
// 	})

// 	t.Run("unknown profile", func(t *testing.T) {
// 		s, _ := newTestServer(t)
// 		rec := doRequest(t, s, http.MethodGet, "/u/ghost-token/collections/selection", nil)
// 		requireStatus(t, rec, http.StatusNotFound)
// 	})
// }

// func TestSaveCollectionSelection(t *testing.T) {
// 	t.Run("selects and persists", func(t *testing.T) {
// 		s, dbPath := newTestServer(t)
// 		insertProfile(t, dbPath, "alice-token", "Alice")

// 		createRec := doRequest(t, s, http.MethodPost, "/u/alice-token/collections", testCollectionInput("Collection"))
// 		requireStatus(t, createRec, http.StatusCreated)
// 		c := decode[vault.CollectionWithFolders](t, createRec)

// 		body := vault.CollectionSelectionForm{CollectionIDs: []uuid.UUID{c.ID}}
// 		rec := doRequest(t, s, http.MethodPut, "/u/alice-token/collections/selection", body)
// 		requireStatus(t, rec, http.StatusNoContent)

// 		listRec := doRequest(t, s, http.MethodGet, "/u/alice-token/collections/selection", nil)
// 		got := decode[[]vault.CollectionWithFolders](t, listRec)
// 		if len(got) != 1 || got[0].ID != c.ID {
// 			t.Errorf("selection = %+v, want collection %v selected", got, c.ID)
// 		}
// 	})

// 	t.Run("second save drops omitted collections", func(t *testing.T) {
// 		s, dbPath := newTestServer(t)
// 		insertProfile(t, dbPath, "alice-token", "Alice")

// 		rec1 := doRequest(t, s, http.MethodPost, "/u/alice-token/collections", testCollectionInput("Collection One"))
// 		requireStatus(t, rec1, http.StatusCreated)
// 		c1 := decode[vault.CollectionWithFolders](t, rec1)

// 		rec2 := doRequest(t, s, http.MethodPost, "/u/alice-token/collections", testCollectionInput("Collection Two"))
// 		requireStatus(t, rec2, http.StatusCreated)
// 		c2 := decode[vault.CollectionWithFolders](t, rec2)

// 		first := vault.CollectionSelectionForm{CollectionIDs: []uuid.UUID{c1.ID, c2.ID}}
// 		requireStatus(t, doRequest(t, s, http.MethodPut, "/u/alice-token/collections/selection", first), http.StatusNoContent)

// 		second := vault.CollectionSelectionForm{CollectionIDs: []uuid.UUID{c1.ID}}
// 		requireStatus(t, doRequest(t, s, http.MethodPut, "/u/alice-token/collections/selection", second), http.StatusNoContent)

// 		listRec := doRequest(t, s, http.MethodGet, "/u/alice-token/collections/selection", nil)
// 		got := decode[[]vault.CollectionWithFolders](t, listRec)
// 		if len(got) != 1 || got[0].ID != c1.ID {
// 			t.Errorf("selection = %+v, want only collection %v", got, c1.ID)
// 		}
// 	})

// 	t.Run("inaccessible collection is rejected", func(t *testing.T) {
// 		s, dbPath := newTestServer(t)
// 		insertProfile(t, dbPath, "alice-token", "Alice")
// 		insertProfile(t, dbPath, "bob-token", "Bob")

// 		rec := doRequest(t, s, http.MethodPost, "/u/bob-token/collections", testCollectionInput("Bob's Private Collection"))
// 		requireStatus(t, rec, http.StatusCreated)
// 		bobsCollection := decode[vault.CollectionWithFolders](t, rec)

// 		body := vault.CollectionSelectionForm{CollectionIDs: []uuid.UUID{bobsCollection.ID}}
// 		saveRec := doRequest(t, s, http.MethodPut, "/u/alice-token/collections/selection", body)
// 		requireStatus(t, saveRec, http.StatusBadRequest)
// 	})

// 	t.Run("malformed JSON body", func(t *testing.T) {
// 		s, dbPath := newTestServer(t)
// 		insertProfile(t, dbPath, "alice-token", "Alice")

// 		rec := doRawRequest(t, s, http.MethodPut, "/u/alice-token/collections/selection", bytes.NewReader([]byte("{not-json")), true)
// 		requireStatus(t, rec, http.StatusBadRequest)
// 	})

// 	t.Run("unknown profile", func(t *testing.T) {
// 		s, _ := newTestServer(t)
// 		rec := doRequest(t, s, http.MethodPut, "/u/ghost-token/collections/selection", vault.CollectionSelectionForm{})
// 		requireStatus(t, rec, http.StatusNotFound)
// 	})
// }
