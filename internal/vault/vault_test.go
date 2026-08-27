package vault

// import (
// 	"context"
// 	"errors"
// 	"path/filepath"
// 	"testing"

// 	"github.com/google/uuid"
// )

// // ---------------------------------------------------------------------------
// // Shared fixtures
// // ---------------------------------------------------------------------------

// // newTestDB creates a fresh, temp-file-backed database for a single test and
// // closes it automatically on cleanup. A temp file (rather than ":memory:") is
// // used so the WAL/foreign_keys pragmas in the real DSN behave exactly as they
// // do in production.
// func newTestDB(t *testing.T) *DB {
// 	t.Helper()

// 	dir := t.TempDir()
// 	path := filepath.Join(dir, "test.db")

// 	db, err := InitDB(path)
// 	if err != nil {
// 		t.Fatalf("InitDB() error = %v", err)
// 	}
// 	t.Cleanup(func() {
// 		if err := db.Close(); err != nil {
// 			t.Errorf("db.Close() error = %v", err)
// 		}
// 	})

// 	return db
// }

// // insertProfile inserts a profile row directly (there is currently no public
// // CreateProfile on *DB) and returns its generated ID.
// func insertProfile(t *testing.T, db *DB, token, name string) uuid.UUID {
// 	t.Helper()

// 	id := uuid.New()
// 	_, err := db.conn.Exec(
// 		`INSERT INTO profiles (id, token, name) VALUES (?, ?, ?)`,
// 		id.String(), token, name,
// 	)
// 	if err != nil {
// 		t.Fatalf("inserting test profile: %v", err)
// 	}
// 	return id
// }

// func testCatalogInput(name string) CatalogForm {
// 	return CatalogForm{
// 		Type:     "movie",
// 		Name:     name,
// 		Provider: "tmdb",
// 		Kind:     "discover",
// 		Endpoint: "/discover/movie",
// 		Params:   `{"sort_by":"popularity.desc"}`,
// 		IsPublic: false,
// 	}
// }

// func testCollectionInput(title string) CollectionForm {
// 	return CollectionForm{
// 		Title:      title,
// 		IsPublic:   false,
// 		PinToTop:   false,
// 		ViewMode:   "TABBED_GRID",
// 		ShowAllTab: false,
// 	}
// }

// // assertErr fails the test unless err matches want via errors.Is. Passing a
// // nil want asserts err is nil.
// func assertErr(t *testing.T, err, want error) {
// 	t.Helper()
// 	if !errors.Is(err, want) {
// 		t.Errorf("error = %v, want %v", err, want)
// 	}
// }

// // ---------------------------------------------------------------------------
// // Schema / bootstrap
// // ---------------------------------------------------------------------------

// func TestInitDB_CreatesSchemaAndIsIdempotent(t *testing.T) {
// 	dir := t.TempDir()
// 	path := filepath.Join(dir, "schema.db")

// 	db, err := InitDB(path)
// 	if err != nil {
// 		t.Fatalf("InitDB() error = %v", err)
// 	}
// 	defer db.Close()

// 	// Re-running InitDB against the same file should not fail, since the
// 	// schema uses CREATE TABLE/INDEX IF NOT EXISTS.
// 	db2, err := InitDB(path)
// 	if err != nil {
// 		t.Fatalf("second InitDB() on same path error = %v", err)
// 	}
// 	defer db2.Close()

// 	// Sanity check the pragmas took effect.
// 	var fk int
// 	if err := db.conn.QueryRow(`PRAGMA foreign_keys`).Scan(&fk); err != nil {
// 		t.Fatalf("querying foreign_keys pragma: %v", err)
// 	}
// 	if fk != 1 {
// 		t.Errorf("foreign_keys pragma = %d, want 1 (enabled)", fk)
// 	}
// }

// // ---------------------------------------------------------------------------
// // Profiles
// // ---------------------------------------------------------------------------

// // func TestResolveProfileID(t *testing.T) {
// // 	t.Run("resolves an existing token", func(t *testing.T) {
// // 		db := newTestDB(t)
// // 		ctx := context.Background()

// // 		want := insertProfile(t, db, "alice-token", "Alice")

// // 		got, err := db.resolveProfileID(ctx, "alice-token")
// // 		if err != nil {
// // 			t.Fatalf("resolveProfileID() error = %v", err)
// // 		}
// // 		if got != want {
// // 			t.Errorf("resolveProfileID() = %v, want %v", got, want)
// // 		}
// // 	})

// // 	t.Run("unknown token", func(t *testing.T) {
// // 		db := newTestDB(t)
// // 		_, err := db.resolveProfileID(context.Background(), "does-not-exist")
// // 		assertErr(t, err, ErrProfileNotFound)
// // 	})
// // }

// // ---------------------------------------------------------------------------
// // Catalogs
// // ---------------------------------------------------------------------------

// func TestCreateUserCatalog(t *testing.T) {
// 	t.Run("creates and persists a catalog", func(t *testing.T) {
// 		db := newTestDB(t)
// 		ctx := context.Background()

// 		ownerID := insertProfile(t, db, "alice-token", "Alice")
// 		input := testCatalogInput("Popular Movies")

// 		got, err := db.CreateUserCatalog(ctx, "alice-token", input)
// 		if err != nil {
// 			t.Fatalf("CreateUserCatalog() error = %v", err)
// 		}

// 		if got.ID == uuid.Nil {
// 			t.Error("CreateUserCatalog() returned zero ID")
// 		}
// 		if got.OwnerID != ownerID {
// 			t.Errorf("OwnerID = %v, want %v", got.OwnerID, ownerID)
// 		}
// 		if got.Name != input.Name || got.Type != input.Type || got.Provider != input.Provider ||
// 			got.Kind != input.Kind || got.Endpoint != input.Endpoint || got.Params != input.Params {
// 			t.Errorf("CreateUserCatalog() = %+v, want fields to match input %+v", got, input)
// 		}
// 		if got.IsDefault {
// 			t.Error("IsDefault = true, want false for a newly created catalog")
// 		}

// 		// Verify it was actually persisted, not just returned in memory.
// 		stored, err := db.GetUserCatalogs(ctx, "alice-token")
// 		if err != nil {
// 			t.Fatalf("GetUserCatalogs() error = %v", err)
// 		}
// 		if len(stored) != 1 || stored[0].ID != got.ID {
// 			t.Errorf("GetUserCatalogs() = %+v, want single catalog with ID %v", stored, got.ID)
// 		}
// 	})

// 	errCases := []struct {
// 		name  string
// 		token string
// 		input func() CatalogForm
// 		want  error
// 	}{
// 		{"unknown profile", "ghost-token", func() CatalogForm { return testCatalogInput("X") }, ErrProfileNotFound},
// 		{"invalid input", "alice-token", func() CatalogForm { return testCatalogInput("") }, ErrInvalidInput},
// 	}
// 	for _, tc := range errCases {
// 		t.Run(tc.name, func(t *testing.T) {
// 			db := newTestDB(t)
// 			insertProfile(t, db, "alice-token", "Alice")

// 			_, err := db.CreateUserCatalog(context.Background(), tc.token, tc.input())
// 			assertErr(t, err, tc.want)
// 		})
// 	}
// }

// func TestGetUserCatalogs(t *testing.T) {
// 	t.Run("empty for a profile with no catalogs", func(t *testing.T) {
// 		db := newTestDB(t)
// 		ctx := context.Background()
// 		insertProfile(t, db, "alice-token", "Alice")

// 		catalogs, err := db.GetUserCatalogs(ctx, "alice-token")
// 		if err != nil {
// 			t.Fatalf("GetUserCatalogs() error = %v", err)
// 		}
// 		if len(catalogs) != 0 {
// 			t.Errorf("GetUserCatalogs() = %+v, want empty slice", catalogs)
// 		}
// 	})

// 	t.Run("unknown profile", func(t *testing.T) {
// 		db := newTestDB(t)
// 		_, err := db.GetUserCatalogs(context.Background(), "ghost-token")
// 		assertErr(t, err, ErrProfileNotFound)
// 	})

// 	t.Run("scoped to owner", func(t *testing.T) {
// 		db := newTestDB(t)
// 		ctx := context.Background()

// 		insertProfile(t, db, "alice-token", "Alice")
// 		insertProfile(t, db, "bob-token", "Bob")

// 		if _, err := db.CreateUserCatalog(ctx, "alice-token", testCatalogInput("Alice's Catalog")); err != nil {
// 			t.Fatalf("creating alice's catalog: %v", err)
// 		}
// 		if _, err := db.CreateUserCatalog(ctx, "bob-token", testCatalogInput("Bob's Catalog")); err != nil {
// 			t.Fatalf("creating bob's catalog: %v", err)
// 		}

// 		aliceCatalogs, err := db.GetUserCatalogs(ctx, "alice-token")
// 		if err != nil {
// 			t.Fatalf("GetUserCatalogs(alice) error = %v", err)
// 		}
// 		if len(aliceCatalogs) != 1 || aliceCatalogs[0].Name != "Alice's Catalog" {
// 			t.Errorf("GetUserCatalogs(alice) = %+v, want only Alice's catalog", aliceCatalogs)
// 		}

// 		bobCatalogs, err := db.GetUserCatalogs(ctx, "bob-token")
// 		if err != nil {
// 			t.Fatalf("GetUserCatalogs(bob) error = %v", err)
// 		}
// 		if len(bobCatalogs) != 1 || bobCatalogs[0].Name != "Bob's Catalog" {
// 			t.Errorf("GetUserCatalogs(bob) = %+v, want only Bob's catalog", bobCatalogs)
// 		}
// 	})
// }

// func TestGetCommunityCatalogs(t *testing.T) {
// 	db := newTestDB(t)
// 	ctx := context.Background()

// 	insertProfile(t, db, "alice-token", "Alice")
// 	insertProfile(t, db, "bob-token", "Bob")

// 	publicInput := testCatalogInput("Public Catalog")
// 	publicInput.IsPublic = true
// 	privateInput := testCatalogInput("Private Catalog")
// 	privateInput.IsPublic = false

// 	if _, err := db.CreateUserCatalog(ctx, "alice-token", publicInput); err != nil {
// 		t.Fatalf("creating public catalog: %v", err)
// 	}
// 	if _, err := db.CreateUserCatalog(ctx, "bob-token", privateInput); err != nil {
// 		t.Fatalf("creating private catalog: %v", err)
// 	}

// 	community, err := db.GetCommunityCatalogs(ctx)
// 	if err != nil {
// 		t.Fatalf("GetCommunityCatalogs() error = %v", err)
// 	}
// 	if len(community) != 1 {
// 		t.Fatalf("GetCommunityCatalogs() returned %d catalogs, want 1", len(community))
// 	}
// 	if community[0].Name != "Public Catalog" || !community[0].IsPublic {
// 		t.Errorf("GetCommunityCatalogs() = %+v, want only the public catalog", community)
// 	}
// }

// func TestUpdateUserCatalog(t *testing.T) {
// 	t.Run("updates and persists fields", func(t *testing.T) {
// 		db := newTestDB(t)
// 		ctx := context.Background()

// 		insertProfile(t, db, "alice-token", "Alice")
// 		created, err := db.CreateUserCatalog(ctx, "alice-token", testCatalogInput("Original Name"))
// 		if err != nil {
// 			t.Fatalf("creating catalog: %v", err)
// 		}

// 		update := testCatalogInput("Updated Name")
// 		update.IsPublic = true

// 		updated, err := db.UpdateUserCatalog(ctx, "alice-token", created.ID, update)
// 		if err != nil {
// 			t.Fatalf("UpdateUserCatalog() error = %v", err)
// 		}
// 		if updated.Name != "Updated Name" || !updated.IsPublic {
// 			t.Errorf("UpdateUserCatalog() = %+v, want Name=Updated Name, IsPublic=true", updated)
// 		}

// 		// Confirm the change was persisted.
// 		stored, err := db.GetUserCatalogs(ctx, "alice-token")
// 		if err != nil {
// 			t.Fatalf("GetUserCatalogs() error = %v", err)
// 		}
// 		if len(stored) != 1 || stored[0].Name != "Updated Name" {
// 			t.Errorf("GetUserCatalogs() after update = %+v, want Name=Updated Name", stored)
// 		}
// 	})

// 	t.Run("invalid input is rejected", func(t *testing.T) {
// 		db := newTestDB(t)
// 		ctx := context.Background()

// 		insertProfile(t, db, "alice-token", "Alice")
// 		created, err := db.CreateUserCatalog(ctx, "alice-token", testCatalogInput("Original"))
// 		if err != nil {
// 			t.Fatalf("creating catalog: %v", err)
// 		}

// 		_, err = db.UpdateUserCatalog(ctx, "alice-token", created.ID, testCatalogInput(""))
// 		assertErr(t, err, ErrInvalidInput)
// 	})

// 	t.Run("unknown catalog", func(t *testing.T) {
// 		db := newTestDB(t)
// 		insertProfile(t, db, "alice-token", "Alice")

// 		_, err := db.UpdateUserCatalog(context.Background(), "alice-token", uuid.New(), testCatalogInput("X"))
// 		assertErr(t, err, ErrCatalogNotFound)
// 	})

// 	t.Run("unknown profile", func(t *testing.T) {
// 		db := newTestDB(t)
// 		_, err := db.UpdateUserCatalog(context.Background(), "ghost-token", uuid.New(), testCatalogInput("X"))
// 		assertErr(t, err, ErrProfileNotFound)
// 	})

// 	t.Run("wrong owner is rejected", func(t *testing.T) {
// 		db := newTestDB(t)
// 		ctx := context.Background()

// 		insertProfile(t, db, "alice-token", "Alice")
// 		insertProfile(t, db, "bob-token", "Bob")

// 		aliceCatalog, err := db.CreateUserCatalog(ctx, "alice-token", testCatalogInput("Alice's Catalog"))
// 		if err != nil {
// 			t.Fatalf("creating alice's catalog: %v", err)
// 		}

// 		// Bob tries to update Alice's catalog by ID.
// 		_, err = db.UpdateUserCatalog(ctx, "bob-token", aliceCatalog.ID, testCatalogInput("Hijacked"))
// 		assertErr(t, err, ErrCatalogNotFound)

// 		// Alice's catalog should be untouched.
// 		stored, err := db.GetUserCatalogs(ctx, "alice-token")
// 		if err != nil {
// 			t.Fatalf("GetUserCatalogs() error = %v", err)
// 		}
// 		if len(stored) != 1 || stored[0].Name != "Alice's Catalog" {
// 			t.Errorf("GetUserCatalogs() = %+v, want untouched Alice's Catalog", stored)
// 		}
// 	})
// }

// func TestDeleteUserCatalog(t *testing.T) {
// 	t.Run("deletes a catalog", func(t *testing.T) {
// 		db := newTestDB(t)
// 		ctx := context.Background()

// 		insertProfile(t, db, "alice-token", "Alice")
// 		created, err := db.CreateUserCatalog(ctx, "alice-token", testCatalogInput("To Delete"))
// 		if err != nil {
// 			t.Fatalf("creating catalog: %v", err)
// 		}

// 		if err := db.DeleteUserCatalog(ctx, "alice-token", created.ID); err != nil {
// 			t.Fatalf("DeleteUserCatalog() error = %v", err)
// 		}

// 		stored, err := db.GetUserCatalogs(ctx, "alice-token")
// 		if err != nil {
// 			t.Fatalf("GetUserCatalogs() error = %v", err)
// 		}
// 		if len(stored) != 0 {
// 			t.Errorf("GetUserCatalogs() after delete = %+v, want empty", stored)
// 		}
// 	})

// 	t.Run("unknown catalog", func(t *testing.T) {
// 		db := newTestDB(t)
// 		insertProfile(t, db, "alice-token", "Alice")

// 		err := db.DeleteUserCatalog(context.Background(), "alice-token", uuid.New())
// 		assertErr(t, err, ErrCatalogNotFound)
// 	})

// 	t.Run("wrong owner is rejected", func(t *testing.T) {
// 		db := newTestDB(t)
// 		ctx := context.Background()

// 		insertProfile(t, db, "alice-token", "Alice")
// 		insertProfile(t, db, "bob-token", "Bob")

// 		aliceCatalog, err := db.CreateUserCatalog(ctx, "alice-token", testCatalogInput("Alice's Catalog"))
// 		if err != nil {
// 			t.Fatalf("creating alice's catalog: %v", err)
// 		}

// 		err = db.DeleteUserCatalog(ctx, "bob-token", aliceCatalog.ID)
// 		assertErr(t, err, ErrCatalogNotFound)

// 		stored, err := db.GetUserCatalogs(ctx, "alice-token")
// 		if err != nil {
// 			t.Fatalf("GetUserCatalogs() error = %v", err)
// 		}
// 		if len(stored) != 1 {
// 			t.Errorf("GetUserCatalogs() = %+v, want Alice's catalog to still exist", stored)
// 		}
// 	})
// }

// // ---------------------------------------------------------------------------
// // Collections
// // ---------------------------------------------------------------------------

// func TestCreateUserCollection(t *testing.T) {
// 	t.Run("no folders", func(t *testing.T) {
// 		db := newTestDB(t)
// 		ctx := context.Background()
// 		ownerID := insertProfile(t, db, "alice-token", "Alice")

// 		got, err := db.CreateUserCollection(ctx, "alice-token", testCollectionInput("Watchlist"))
// 		if err != nil {
// 			t.Fatalf("CreateUserCollection() error = %v", err)
// 		}

// 		if got.ID == uuid.Nil {
// 			t.Error("CreateUserCollection() returned zero ID")
// 		}
// 		if got.OwnerID != ownerID {
// 			t.Errorf("OwnerID = %v, want %v", got.OwnerID, ownerID)
// 		}
// 		if got.Title != "Watchlist" {
// 			t.Errorf("Title = %q, want %q", got.Title, "Watchlist")
// 		}
// 		if len(got.Folders) != 0 {
// 			t.Errorf("Folders = %+v, want empty", got.Folders)
// 		}
// 	})

// 	t.Run("with folders", func(t *testing.T) {
// 		db := newTestDB(t)
// 		ctx := context.Background()

// 		insertProfile(t, db, "alice-token", "Alice")
// 		cat1, err := db.CreateUserCatalog(ctx, "alice-token", testCatalogInput("Catalog One"))
// 		if err != nil {
// 			t.Fatalf("creating catalog one: %v", err)
// 		}
// 		cat2, err := db.CreateUserCatalog(ctx, "alice-token", testCatalogInput("Catalog Two"))
// 		if err != nil {
// 			t.Fatalf("creating catalog two: %v", err)
// 		}

// 		input := testCollectionInput("Curated")
// 		input.Folders = []FolderData{
// 			{Title: "Folder A", CatalogIDs: []uuid.UUID{cat1.ID, cat2.ID}},
// 		}

// 		got, err := db.CreateUserCollection(ctx, "alice-token", input)
// 		if err != nil {
// 			t.Fatalf("CreateUserCollection() error = %v", err)
// 		}

// 		if len(got.Folders) != 1 {
// 			t.Fatalf("Folders = %+v, want 1 folder", got.Folders)
// 		}
// 		f := got.Folders[0]
// 		if f.ID == uuid.Nil {
// 			t.Error("folder has zero ID")
// 		}
// 		if f.CollectionID != got.ID {
// 			t.Errorf("folder CollectionID = %v, want %v", f.CollectionID, got.ID)
// 		}
// 		if f.SortOrder != 0 {
// 			t.Errorf("folder SortOrder = %d, want 0", f.SortOrder)
// 		}
// 		if len(f.CatalogIDs) != 2 || f.CatalogIDs[0] != cat1.ID || f.CatalogIDs[1] != cat2.ID {
// 			t.Errorf("folder CatalogIDs = %v, want [%v %v] in order", f.CatalogIDs, cat1.ID, cat2.ID)
// 		}
// 	})

// 	t.Run("unknown profile", func(t *testing.T) {
// 		db := newTestDB(t)
// 		_, err := db.CreateUserCollection(context.Background(), "ghost-token", testCollectionInput("X"))
// 		assertErr(t, err, ErrProfileNotFound)
// 	})

// 	t.Run("invalid input", func(t *testing.T) {
// 		db := newTestDB(t)
// 		insertProfile(t, db, "alice-token", "Alice")

// 		_, err := db.CreateUserCollection(context.Background(), "alice-token", testCollectionInput(""))
// 		assertErr(t, err, ErrInvalidInput)
// 	})

// 	t.Run("inaccessible catalog is rejected and nothing is left behind", func(t *testing.T) {
// 		db := newTestDB(t)
// 		ctx := context.Background()

// 		insertProfile(t, db, "alice-token", "Alice")
// 		insertProfile(t, db, "bob-token", "Bob")

// 		bobsCatalog, err := db.CreateUserCatalog(ctx, "bob-token", testCatalogInput("Bob's Private Catalog"))
// 		if err != nil {
// 			t.Fatalf("creating bob's catalog: %v", err)
// 		}

// 		input := testCollectionInput("Sneaky Collection")
// 		input.Folders = []FolderData{
// 			{Title: "Folder A", CatalogIDs: []uuid.UUID{bobsCatalog.ID}},
// 		}

// 		_, err = db.CreateUserCollection(ctx, "alice-token", input)
// 		assertErr(t, err, ErrInvalidInput)

// 		// Confirm nothing was left behind by the aborted transaction.
// 		stored, err := db.GetUserCollections(ctx, "alice-token")
// 		if err != nil {
// 			t.Fatalf("GetUserCollections() error = %v", err)
// 		}
// 		if len(stored) != 0 {
// 			t.Errorf("GetUserCollections() = %+v, want empty after rejected create", stored)
// 		}
// 	})

// 	t.Run("public catalog from another profile is allowed", func(t *testing.T) {
// 		db := newTestDB(t)
// 		ctx := context.Background()

// 		insertProfile(t, db, "alice-token", "Alice")
// 		insertProfile(t, db, "bob-token", "Bob")

// 		publicInput := testCatalogInput("Alice's Public Catalog")
// 		publicInput.IsPublic = true
// 		alicesCatalog, err := db.CreateUserCatalog(ctx, "alice-token", publicInput)
// 		if err != nil {
// 			t.Fatalf("creating alice's public catalog: %v", err)
// 		}

// 		input := testCollectionInput("Bob's Collection")
// 		input.Folders = []FolderData{
// 			{Title: "Folder A", CatalogIDs: []uuid.UUID{alicesCatalog.ID}},
// 		}

// 		got, err := db.CreateUserCollection(ctx, "bob-token", input)
// 		if err != nil {
// 			t.Fatalf("CreateUserCollection() error = %v", err)
// 		}
// 		if len(got.Folders) != 1 || len(got.Folders[0].CatalogIDs) != 1 || got.Folders[0].CatalogIDs[0] != alicesCatalog.ID {
// 			t.Errorf("Folders = %+v, want one folder referencing catalog %v", got.Folders, alicesCatalog.ID)
// 		}
// 	})
// }

// func TestGetUserCollections(t *testing.T) {
// 	t.Run("empty for a profile with no collections", func(t *testing.T) {
// 		db := newTestDB(t)
// 		insertProfile(t, db, "alice-token", "Alice")

// 		got, err := db.GetUserCollections(context.Background(), "alice-token")
// 		if err != nil {
// 			t.Fatalf("GetUserCollections() error = %v", err)
// 		}
// 		if len(got) != 0 {
// 			t.Errorf("GetUserCollections() = %+v, want empty slice", got)
// 		}
// 	})

// 	t.Run("unknown profile", func(t *testing.T) {
// 		db := newTestDB(t)
// 		_, err := db.GetUserCollections(context.Background(), "ghost-token")
// 		assertErr(t, err, ErrProfileNotFound)
// 	})

// 	t.Run("scoped to owner", func(t *testing.T) {
// 		db := newTestDB(t)
// 		ctx := context.Background()

// 		insertProfile(t, db, "alice-token", "Alice")
// 		insertProfile(t, db, "bob-token", "Bob")

// 		if _, err := db.CreateUserCollection(ctx, "alice-token", testCollectionInput("Alice's Collection")); err != nil {
// 			t.Fatalf("creating alice's collection: %v", err)
// 		}
// 		if _, err := db.CreateUserCollection(ctx, "bob-token", testCollectionInput("Bob's Collection")); err != nil {
// 			t.Fatalf("creating bob's collection: %v", err)
// 		}

// 		aliceCollections, err := db.GetUserCollections(ctx, "alice-token")
// 		if err != nil {
// 			t.Fatalf("GetUserCollections(alice) error = %v", err)
// 		}
// 		if len(aliceCollections) != 1 || aliceCollections[0].Title != "Alice's Collection" {
// 			t.Errorf("GetUserCollections(alice) = %+v, want only Alice's collection", aliceCollections)
// 		}
// 	})
// }

// func TestGetCommunityCollections(t *testing.T) {
// 	db := newTestDB(t)
// 	ctx := context.Background()

// 	insertProfile(t, db, "alice-token", "Alice")
// 	insertProfile(t, db, "bob-token", "Bob")

// 	publicInput := testCollectionInput("Public Collection")
// 	publicInput.IsPublic = true
// 	privateInput := testCollectionInput("Private Collection")

// 	if _, err := db.CreateUserCollection(ctx, "alice-token", publicInput); err != nil {
// 		t.Fatalf("creating public collection: %v", err)
// 	}
// 	if _, err := db.CreateUserCollection(ctx, "bob-token", privateInput); err != nil {
// 		t.Fatalf("creating private collection: %v", err)
// 	}

// 	community, err := db.GetCommunityCollections(ctx)
// 	if err != nil {
// 		t.Fatalf("GetCommunityCollections() error = %v", err)
// 	}
// 	if len(community) != 1 || community[0].Title != "Public Collection" || !community[0].IsPublic {
// 		t.Errorf("GetCommunityCollections() = %+v, want only the public collection", community)
// 	}
// }

// func TestUpdateUserCollection(t *testing.T) {
// 	t.Run("updates top-level fields", func(t *testing.T) {
// 		db := newTestDB(t)
// 		ctx := context.Background()

// 		insertProfile(t, db, "alice-token", "Alice")
// 		created, err := db.CreateUserCollection(ctx, "alice-token", testCollectionInput("Original"))
// 		if err != nil {
// 			t.Fatalf("creating collection: %v", err)
// 		}

// 		update := testCollectionInput("Renamed")
// 		update.IsPublic = true
// 		update.PinToTop = true

// 		updated, err := db.UpdateUserCollection(ctx, "alice-token", created.ID, update)
// 		if err != nil {
// 			t.Fatalf("UpdateUserCollection() error = %v", err)
// 		}
// 		if updated.Title != "Renamed" || !updated.IsPublic || !updated.PinToTop {
// 			t.Errorf("UpdateUserCollection() = %+v, want Title=Renamed, IsPublic=true, PinToTop=true", updated)
// 		}

// 		// Confirm persisted via a fresh read.
// 		stored, err := db.GetUserCollections(ctx, "alice-token")
// 		if err != nil {
// 			t.Fatalf("GetUserCollections() error = %v", err)
// 		}
// 		if len(stored) != 1 || stored[0].Title != "Renamed" {
// 			t.Errorf("GetUserCollections() after update = %+v, want Title=Renamed", stored)
// 		}
// 	})

// 	t.Run("diffs folders: keeps+renames, drops, adds", func(t *testing.T) {
// 		db := newTestDB(t)
// 		ctx := context.Background()

// 		insertProfile(t, db, "alice-token", "Alice")
// 		cat1, err := db.CreateUserCatalog(ctx, "alice-token", testCatalogInput("Catalog One"))
// 		if err != nil {
// 			t.Fatalf("creating catalog one: %v", err)
// 		}
// 		cat2, err := db.CreateUserCatalog(ctx, "alice-token", testCatalogInput("Catalog Two"))
// 		if err != nil {
// 			t.Fatalf("creating catalog two: %v", err)
// 		}

// 		createInput := testCollectionInput("Curated")
// 		createInput.Folders = []FolderData{
// 			{Title: "Folder A", CatalogIDs: []uuid.UUID{cat1.ID}},
// 			{Title: "Folder B", CatalogIDs: []uuid.UUID{cat2.ID}},
// 		}
// 		created, err := db.CreateUserCollection(ctx, "alice-token", createInput)
// 		if err != nil {
// 			t.Fatalf("creating collection: %v", err)
// 		}
// 		if len(created.Folders) != 2 {
// 			t.Fatalf("setup: Folders = %+v, want 2", created.Folders)
// 		}
// 		folderAID := created.Folders[0].ID
// 		folderBID := created.Folders[1].ID

// 		update := testCollectionInput("Curated")
// 		update.Folders = []FolderData{
// 			{ID: &folderAID, Title: "Folder A Updated", CatalogIDs: []uuid.UUID{cat2.ID, cat1.ID}},
// 			{Title: "Folder C", CatalogIDs: []uuid.UUID{}},
// 		}

// 		updated, err := db.UpdateUserCollection(ctx, "alice-token", created.ID, update)
// 		if err != nil {
// 			t.Fatalf("UpdateUserCollection() error = %v", err)
// 		}
// 		if len(updated.Folders) != 2 {
// 			t.Fatalf("Folders = %+v, want 2", updated.Folders)
// 		}

// 		var gotA, gotC *FolderWithCatalogs
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
// 			t.Fatal("Folder A missing from update result")
// 		}
// 		if gotA.Title != "Folder A Updated" || gotA.SortOrder != 0 {
// 			t.Errorf("Folder A = %+v, want Title=Folder A Updated, SortOrder=0", gotA)
// 		}
// 		if len(gotA.CatalogIDs) != 2 || gotA.CatalogIDs[0] != cat2.ID || gotA.CatalogIDs[1] != cat1.ID {
// 			t.Errorf("Folder A CatalogIDs = %v, want [%v %v] in order", gotA.CatalogIDs, cat2.ID, cat1.ID)
// 		}

// 		if gotC == nil {
// 			t.Fatal("Folder C missing from update result")
// 		}
// 		if gotC.SortOrder != 1 {
// 			t.Errorf("Folder C SortOrder = %d, want 1", gotC.SortOrder)
// 		}

// 		// Confirm the diff was actually persisted, not just returned in memory.
// 		stored, err := db.GetUserCollections(ctx, "alice-token")
// 		if err != nil {
// 			t.Fatalf("GetUserCollections() error = %v", err)
// 		}
// 		if len(stored) != 1 || len(stored[0].Folders) != 2 {
// 			t.Fatalf("GetUserCollections() = %+v, want 1 collection with 2 folders", stored)
// 		}
// 	})

// 	t.Run("rejects a folder ID belonging to another collection", func(t *testing.T) {
// 		db := newTestDB(t)
// 		ctx := context.Background()

// 		insertProfile(t, db, "alice-token", "Alice")

// 		c1, err := db.CreateUserCollection(ctx, "alice-token", testCollectionInput("Collection One"))
// 		if err != nil {
// 			t.Fatalf("creating collection one: %v", err)
// 		}

// 		input2 := testCollectionInput("Collection Two")
// 		input2.Folders = []FolderData{{Title: "Folder X"}}
// 		c2, err := db.CreateUserCollection(ctx, "alice-token", input2)
// 		if err != nil {
// 			t.Fatalf("creating collection two: %v", err)
// 		}
// 		foreignFolderID := c2.Folders[0].ID

// 		update := testCollectionInput("Collection One")
// 		update.Folders = []FolderData{{ID: &foreignFolderID, Title: "Hijacked"}}

// 		_, err = db.UpdateUserCollection(ctx, "alice-token", c1.ID, update)
// 		assertErr(t, err, ErrInvalidInput)
// 	})

// 	t.Run("unknown collection", func(t *testing.T) {
// 		db := newTestDB(t)
// 		insertProfile(t, db, "alice-token", "Alice")

// 		_, err := db.UpdateUserCollection(context.Background(), "alice-token", uuid.New(), testCollectionInput("X"))
// 		assertErr(t, err, ErrCollectionNotFound)
// 	})

// 	t.Run("unknown profile", func(t *testing.T) {
// 		db := newTestDB(t)
// 		_, err := db.UpdateUserCollection(context.Background(), "ghost-token", uuid.New(), testCollectionInput("X"))
// 		assertErr(t, err, ErrProfileNotFound)
// 	})

// 	t.Run("wrong owner is rejected", func(t *testing.T) {
// 		db := newTestDB(t)
// 		ctx := context.Background()

// 		insertProfile(t, db, "alice-token", "Alice")
// 		insertProfile(t, db, "bob-token", "Bob")

// 		aliceCollection, err := db.CreateUserCollection(ctx, "alice-token", testCollectionInput("Alice's Collection"))
// 		if err != nil {
// 			t.Fatalf("creating alice's collection: %v", err)
// 		}

// 		_, err = db.UpdateUserCollection(ctx, "bob-token", aliceCollection.ID, testCollectionInput("Hijacked"))
// 		assertErr(t, err, ErrCollectionNotFound)

// 		stored, err := db.GetUserCollections(ctx, "alice-token")
// 		if err != nil {
// 			t.Fatalf("GetUserCollections() error = %v", err)
// 		}
// 		if len(stored) != 1 || stored[0].Title != "Alice's Collection" {
// 			t.Errorf("GetUserCollections() = %+v, want untouched Alice's Collection", stored)
// 		}
// 	})
// }

// func TestDeleteUserCollection(t *testing.T) {
// 	t.Run("deletes a collection", func(t *testing.T) {
// 		db := newTestDB(t)
// 		ctx := context.Background()

// 		insertProfile(t, db, "alice-token", "Alice")
// 		created, err := db.CreateUserCollection(ctx, "alice-token", testCollectionInput("To Delete"))
// 		if err != nil {
// 			t.Fatalf("creating collection: %v", err)
// 		}

// 		if err := db.DeleteUserCollection(ctx, "alice-token", created.ID); err != nil {
// 			t.Fatalf("DeleteUserCollection() error = %v", err)
// 		}

// 		stored, err := db.GetUserCollections(ctx, "alice-token")
// 		if err != nil {
// 			t.Fatalf("GetUserCollections() error = %v", err)
// 		}
// 		if len(stored) != 0 {
// 			t.Errorf("GetUserCollections() after delete = %+v, want empty", stored)
// 		}
// 	})

// 	t.Run("unknown collection", func(t *testing.T) {
// 		db := newTestDB(t)
// 		insertProfile(t, db, "alice-token", "Alice")

// 		err := db.DeleteUserCollection(context.Background(), "alice-token", uuid.New())
// 		assertErr(t, err, ErrCollectionNotFound)
// 	})

// 	t.Run("wrong owner is rejected", func(t *testing.T) {
// 		db := newTestDB(t)
// 		ctx := context.Background()

// 		insertProfile(t, db, "alice-token", "Alice")
// 		insertProfile(t, db, "bob-token", "Bob")

// 		aliceCollection, err := db.CreateUserCollection(ctx, "alice-token", testCollectionInput("Alice's Collection"))
// 		if err != nil {
// 			t.Fatalf("creating alice's collection: %v", err)
// 		}

// 		err = db.DeleteUserCollection(ctx, "bob-token", aliceCollection.ID)
// 		assertErr(t, err, ErrCollectionNotFound)

// 		stored, err := db.GetUserCollections(ctx, "alice-token")
// 		if err != nil {
// 			t.Fatalf("GetUserCollections() error = %v", err)
// 		}
// 		if len(stored) != 1 {
// 			t.Errorf("GetUserCollections() = %+v, want Alice's collection to still exist", stored)
// 		}
// 	})

// 	// TestDeleteUserCollection_CascadesFolders confirms the ON DELETE CASCADE
// 	// chain (collections -> folders -> folder_catalogs) actually fires, by
// 	// querying those tables directly rather than trusting the vault API alone.
// 	t.Run("cascades to folders and folder_catalogs", func(t *testing.T) {
// 		db := newTestDB(t)
// 		ctx := context.Background()

// 		insertProfile(t, db, "alice-token", "Alice")
// 		cat, err := db.CreateUserCatalog(ctx, "alice-token", testCatalogInput("Catalog"))
// 		if err != nil {
// 			t.Fatalf("creating catalog: %v", err)
// 		}

// 		input := testCollectionInput("Has Folders")
// 		input.Folders = []FolderData{
// 			{Title: "Folder A", CatalogIDs: []uuid.UUID{cat.ID}},
// 		}
// 		created, err := db.CreateUserCollection(ctx, "alice-token", input)
// 		if err != nil {
// 			t.Fatalf("creating collection: %v", err)
// 		}
// 		folderID := created.Folders[0].ID

// 		if err := db.DeleteUserCollection(ctx, "alice-token", created.ID); err != nil {
// 			t.Fatalf("DeleteUserCollection() error = %v", err)
// 		}

// 		var folderCount int
// 		if err := db.conn.QueryRow(`SELECT COUNT(*) FROM folders WHERE id = ?`, folderID.String()).Scan(&folderCount); err != nil {
// 			t.Fatalf("querying folders: %v", err)
// 		}
// 		if folderCount != 0 {
// 			t.Errorf("folders row count = %d, want 0 (cascade should have deleted it)", folderCount)
// 		}

// 		var refCount int
// 		if err := db.conn.QueryRow(`SELECT COUNT(*) FROM folder_catalogs WHERE folder_id = ?`, folderID.String()).Scan(&refCount); err != nil {
// 			t.Fatalf("querying folder_catalogs: %v", err)
// 		}
// 		if refCount != 0 {
// 			t.Errorf("folder_catalogs row count = %d, want 0 (cascade should have deleted it)", refCount)
// 		}

// 		// The referenced catalog itself must survive — cascade should not reach past folder_catalogs.
// 		remainingCatalogs, err := db.GetUserCatalogs(ctx, "alice-token")
// 		if err != nil {
// 			t.Fatalf("GetUserCatalogs() error = %v", err)
// 		}
// 		if len(remainingCatalogs) != 1 {
// 			t.Errorf("GetUserCatalogs() = %+v, want the catalog to still exist", remainingCatalogs)
// 		}
// 	})
// }

// // ---------------------------------------------------------------------------
// // Catalog selection
// // ---------------------------------------------------------------------------

// func TestGetCurrentCatalogSelection(t *testing.T) {
// 	t.Run("empty for a profile with no selection", func(t *testing.T) {
// 		db := newTestDB(t)
// 		insertProfile(t, db, "alice-token", "Alice")

// 		got, err := db.GetCurrentCatalogSelection(context.Background(), "alice-token")
// 		if err != nil {
// 			t.Fatalf("GetCurrentCatalogSelection() error = %v", err)
// 		}
// 		if len(got) != 0 {
// 			t.Errorf("GetCurrentCatalogSelection() = %+v, want empty slice", got)
// 		}
// 	})

// 	t.Run("unknown profile", func(t *testing.T) {
// 		db := newTestDB(t)
// 		_, err := db.GetCurrentCatalogSelection(context.Background(), "ghost-token")
// 		assertErr(t, err, ErrProfileNotFound)
// 	})

// 	t.Run("creating a catalog does not auto-select it", func(t *testing.T) {
// 		db := newTestDB(t)
// 		ctx := context.Background()
// 		insertProfile(t, db, "alice-token", "Alice")

// 		if _, err := db.CreateUserCatalog(ctx, "alice-token", testCatalogInput("Unselected")); err != nil {
// 			t.Fatalf("creating catalog: %v", err)
// 		}

// 		got, err := db.GetCurrentCatalogSelection(ctx, "alice-token")
// 		if err != nil {
// 			t.Fatalf("GetCurrentCatalogSelection() error = %v", err)
// 		}
// 		if len(got) != 0 {
// 			t.Errorf("GetCurrentCatalogSelection() = %+v, want empty (creation must not auto-select)", got)
// 		}
// 	})

// 	t.Run("returns selected catalogs in sort order with show_in_home", func(t *testing.T) {
// 		db := newTestDB(t)
// 		ctx := context.Background()
// 		insertProfile(t, db, "alice-token", "Alice")

// 		cat1, err := db.CreateUserCatalog(ctx, "alice-token", testCatalogInput("Catalog One"))
// 		if err != nil {
// 			t.Fatalf("creating catalog one: %v", err)
// 		}
// 		cat2, err := db.CreateUserCatalog(ctx, "alice-token", testCatalogInput("Catalog Two"))
// 		if err != nil {
// 			t.Fatalf("creating catalog two: %v", err)
// 		}

// 		input := CatalogSelectionForm{Catalogs: []SelectedCatalogInput{
// 			{CatalogID: cat2.ID, ShowInHome: false},
// 			{CatalogID: cat1.ID, ShowInHome: true},
// 		}}
// 		if err := db.SaveCatalogSelection(ctx, "alice-token", input); err != nil {
// 			t.Fatalf("SaveCatalogSelection() error = %v", err)
// 		}

// 		got, err := db.GetCurrentCatalogSelection(ctx, "alice-token")
// 		if err != nil {
// 			t.Fatalf("GetCurrentCatalogSelection() error = %v", err)
// 		}
// 		if len(got) != 2 {
// 			t.Fatalf("GetCurrentCatalogSelection() = %+v, want 2", got)
// 		}
// 		if got[0].ID != cat2.ID || got[0].ShowInHome {
// 			t.Errorf("got[0] = %+v, want cat2 with ShowInHome=false", got[0])
// 		}
// 		if got[1].ID != cat1.ID || !got[1].ShowInHome {
// 			t.Errorf("got[1] = %+v, want cat1 with ShowInHome=true", got[1])
// 		}
// 	})

// 	t.Run("includes a selected public catalog owned by another profile", func(t *testing.T) {
// 		db := newTestDB(t)
// 		ctx := context.Background()
// 		insertProfile(t, db, "alice-token", "Alice")
// 		insertProfile(t, db, "bob-token", "Bob")

// 		publicInput := testCatalogInput("Alice's Public Catalog")
// 		publicInput.IsPublic = true
// 		alicesCatalog, err := db.CreateUserCatalog(ctx, "alice-token", publicInput)
// 		if err != nil {
// 			t.Fatalf("creating alice's public catalog: %v", err)
// 		}

// 		input := CatalogSelectionForm{Catalogs: []SelectedCatalogInput{{CatalogID: alicesCatalog.ID, ShowInHome: true}}}
// 		if err := db.SaveCatalogSelection(ctx, "bob-token", input); err != nil {
// 			t.Fatalf("SaveCatalogSelection(bob) error = %v", err)
// 		}

// 		got, err := db.GetCurrentCatalogSelection(ctx, "bob-token")
// 		if err != nil {
// 			t.Fatalf("GetCurrentCatalogSelection(bob) error = %v", err)
// 		}
// 		if len(got) != 1 || got[0].ID != alicesCatalog.ID {
// 			t.Errorf("GetCurrentCatalogSelection(bob) = %+v, want alice's public catalog", got)
// 		}
// 	})
// }

// func TestSaveCatalogSelection(t *testing.T) {
// 	t.Run("selects and persists", func(t *testing.T) {
// 		db := newTestDB(t)
// 		ctx := context.Background()
// 		insertProfile(t, db, "alice-token", "Alice")

// 		cat, err := db.CreateUserCatalog(ctx, "alice-token", testCatalogInput("Catalog"))
// 		if err != nil {
// 			t.Fatalf("creating catalog: %v", err)
// 		}

// 		input := CatalogSelectionForm{Catalogs: []SelectedCatalogInput{{CatalogID: cat.ID, ShowInHome: true}}}
// 		if err := db.SaveCatalogSelection(ctx, "alice-token", input); err != nil {
// 			t.Fatalf("SaveCatalogSelection() error = %v", err)
// 		}

// 		got, err := db.GetCurrentCatalogSelection(ctx, "alice-token")
// 		if err != nil {
// 			t.Fatalf("GetCurrentCatalogSelection() error = %v", err)
// 		}
// 		if len(got) != 1 || got[0].ID != cat.ID {
// 			t.Errorf("GetCurrentCatalogSelection() = %+v, want catalog %v selected", got, cat.ID)
// 		}
// 	})

// 	t.Run("second save drops omitted catalogs and updates survivors", func(t *testing.T) {
// 		db := newTestDB(t)
// 		ctx := context.Background()
// 		insertProfile(t, db, "alice-token", "Alice")

// 		cat1, err := db.CreateUserCatalog(ctx, "alice-token", testCatalogInput("Catalog One"))
// 		if err != nil {
// 			t.Fatalf("creating catalog one: %v", err)
// 		}
// 		cat2, err := db.CreateUserCatalog(ctx, "alice-token", testCatalogInput("Catalog Two"))
// 		if err != nil {
// 			t.Fatalf("creating catalog two: %v", err)
// 		}

// 		first := CatalogSelectionForm{Catalogs: []SelectedCatalogInput{
// 			{CatalogID: cat1.ID, ShowInHome: true},
// 			{CatalogID: cat2.ID, ShowInHome: true},
// 		}}
// 		if err := db.SaveCatalogSelection(ctx, "alice-token", first); err != nil {
// 			t.Fatalf("first SaveCatalogSelection() error = %v", err)
// 		}

// 		// Drop cat2, flip cat1's show_in_home.
// 		second := CatalogSelectionForm{Catalogs: []SelectedCatalogInput{
// 			{CatalogID: cat1.ID, ShowInHome: false},
// 		}}
// 		if err := db.SaveCatalogSelection(ctx, "alice-token", second); err != nil {
// 			t.Fatalf("second SaveCatalogSelection() error = %v", err)
// 		}

// 		got, err := db.GetCurrentCatalogSelection(ctx, "alice-token")
// 		if err != nil {
// 			t.Fatalf("GetCurrentCatalogSelection() error = %v", err)
// 		}
// 		if len(got) != 1 || got[0].ID != cat1.ID || got[0].ShowInHome {
// 			t.Errorf("GetCurrentCatalogSelection() = %+v, want only cat1 with ShowInHome=false", got)
// 		}
// 	})

// 	t.Run("empty payload clears the selection", func(t *testing.T) {
// 		db := newTestDB(t)
// 		ctx := context.Background()
// 		insertProfile(t, db, "alice-token", "Alice")

// 		cat, err := db.CreateUserCatalog(ctx, "alice-token", testCatalogInput("Catalog"))
// 		if err != nil {
// 			t.Fatalf("creating catalog: %v", err)
// 		}
// 		if err := db.SaveCatalogSelection(ctx, "alice-token", CatalogSelectionForm{Catalogs: []SelectedCatalogInput{{CatalogID: cat.ID}}}); err != nil {
// 			t.Fatalf("initial SaveCatalogSelection() error = %v", err)
// 		}

// 		if err := db.SaveCatalogSelection(ctx, "alice-token", CatalogSelectionForm{}); err != nil {
// 			t.Fatalf("clearing SaveCatalogSelection() error = %v", err)
// 		}

// 		got, err := db.GetCurrentCatalogSelection(ctx, "alice-token")
// 		if err != nil {
// 			t.Fatalf("GetCurrentCatalogSelection() error = %v", err)
// 		}
// 		if len(got) != 0 {
// 			t.Errorf("GetCurrentCatalogSelection() = %+v, want empty after clearing", got)
// 		}
// 	})

// 	t.Run("inaccessible catalog is rejected and nothing is left behind", func(t *testing.T) {
// 		db := newTestDB(t)
// 		ctx := context.Background()
// 		insertProfile(t, db, "alice-token", "Alice")
// 		insertProfile(t, db, "bob-token", "Bob")

// 		bobsCatalog, err := db.CreateUserCatalog(ctx, "bob-token", testCatalogInput("Bob's Private Catalog"))
// 		if err != nil {
// 			t.Fatalf("creating bob's catalog: %v", err)
// 		}

// 		err = db.SaveCatalogSelection(ctx, "alice-token", CatalogSelectionForm{
// 			Catalogs: []SelectedCatalogInput{{CatalogID: bobsCatalog.ID, ShowInHome: true}},
// 		})
// 		assertErr(t, err, ErrInvalidInput)

// 		got, err := db.GetCurrentCatalogSelection(ctx, "alice-token")
// 		if err != nil {
// 			t.Fatalf("GetCurrentCatalogSelection() error = %v", err)
// 		}
// 		if len(got) != 0 {
// 			t.Errorf("GetCurrentCatalogSelection() = %+v, want empty after rejected save", got)
// 		}
// 	})

// 	t.Run("unknown profile", func(t *testing.T) {
// 		db := newTestDB(t)
// 		err := db.SaveCatalogSelection(context.Background(), "ghost-token", CatalogSelectionForm{})
// 		assertErr(t, err, ErrProfileNotFound)
// 	})
// }

// // ---------------------------------------------------------------------------
// // Collection selection
// // ---------------------------------------------------------------------------

// func TestGetCurrentCollectionSelection(t *testing.T) {
// 	t.Run("empty for a profile with no selection", func(t *testing.T) {
// 		db := newTestDB(t)
// 		insertProfile(t, db, "alice-token", "Alice")

// 		got, err := db.GetCurrentCollectionSelection(context.Background(), "alice-token")
// 		if err != nil {
// 			t.Fatalf("GetCurrentCollectionSelection() error = %v", err)
// 		}
// 		if len(got) != 0 {
// 			t.Errorf("GetCurrentCollectionSelection() = %+v, want empty slice", got)
// 		}
// 	})

// 	t.Run("unknown profile", func(t *testing.T) {
// 		db := newTestDB(t)
// 		_, err := db.GetCurrentCollectionSelection(context.Background(), "ghost-token")
// 		assertErr(t, err, ErrProfileNotFound)
// 	})

// 	t.Run("creating a collection does not auto-select it", func(t *testing.T) {
// 		db := newTestDB(t)
// 		ctx := context.Background()
// 		insertProfile(t, db, "alice-token", "Alice")

// 		if _, err := db.CreateUserCollection(ctx, "alice-token", testCollectionInput("Unselected")); err != nil {
// 			t.Fatalf("creating collection: %v", err)
// 		}

// 		got, err := db.GetCurrentCollectionSelection(ctx, "alice-token")
// 		if err != nil {
// 			t.Fatalf("GetCurrentCollectionSelection() error = %v", err)
// 		}
// 		if len(got) != 0 {
// 			t.Errorf("GetCurrentCollectionSelection() = %+v, want empty (creation must not auto-select)", got)
// 		}
// 	})

// 	t.Run("returns selected collections in sort order, including folders", func(t *testing.T) {
// 		db := newTestDB(t)
// 		ctx := context.Background()
// 		insertProfile(t, db, "alice-token", "Alice")

// 		cat, err := db.CreateUserCatalog(ctx, "alice-token", testCatalogInput("Catalog"))
// 		if err != nil {
// 			t.Fatalf("creating catalog: %v", err)
// 		}

// 		input := testCollectionInput("Curated")
// 		input.Folders = []FolderData{{Title: "Folder A", CatalogIDs: []uuid.UUID{cat.ID}}}
// 		c1, err := db.CreateUserCollection(ctx, "alice-token", input)
// 		if err != nil {
// 			t.Fatalf("creating collection one: %v", err)
// 		}
// 		c2, err := db.CreateUserCollection(ctx, "alice-token", testCollectionInput("Plain"))
// 		if err != nil {
// 			t.Fatalf("creating collection two: %v", err)
// 		}

// 		selInput := CollectionSelectionForm{CollectionIDs: []uuid.UUID{c2.ID, c1.ID}}
// 		if err := db.SaveCollectionSelection(ctx, "alice-token", selInput); err != nil {
// 			t.Fatalf("SaveCollectionSelection() error = %v", err)
// 		}

// 		got, err := db.GetCurrentCollectionSelection(ctx, "alice-token")
// 		if err != nil {
// 			t.Fatalf("GetCurrentCollectionSelection() error = %v", err)
// 		}
// 		if len(got) != 2 {
// 			t.Fatalf("GetCurrentCollectionSelection() = %+v, want 2", got)
// 		}
// 		if got[0].ID != c2.ID {
// 			t.Errorf("got[0].ID = %v, want %v (c2 first)", got[0].ID, c2.ID)
// 		}
// 		if got[1].ID != c1.ID {
// 			t.Errorf("got[1].ID = %v, want %v (c1 second)", got[1].ID, c1.ID)
// 		}
// 		if len(got[1].Folders) != 1 || len(got[1].Folders[0].CatalogIDs) != 1 || got[1].Folders[0].CatalogIDs[0] != cat.ID {
// 			t.Errorf("got[1].Folders = %+v, want one folder referencing catalog %v", got[1].Folders, cat.ID)
// 		}
// 	})

// 	t.Run("includes a selected public collection owned by another profile", func(t *testing.T) {
// 		db := newTestDB(t)
// 		ctx := context.Background()
// 		insertProfile(t, db, "alice-token", "Alice")
// 		insertProfile(t, db, "bob-token", "Bob")

// 		publicInput := testCollectionInput("Alice's Public Collection")
// 		publicInput.IsPublic = true
// 		alicesCollection, err := db.CreateUserCollection(ctx, "alice-token", publicInput)
// 		if err != nil {
// 			t.Fatalf("creating alice's public collection: %v", err)
// 		}

// 		if err := db.SaveCollectionSelection(ctx, "bob-token", CollectionSelectionForm{CollectionIDs: []uuid.UUID{alicesCollection.ID}}); err != nil {
// 			t.Fatalf("SaveCollectionSelection(bob) error = %v", err)
// 		}

// 		got, err := db.GetCurrentCollectionSelection(ctx, "bob-token")
// 		if err != nil {
// 			t.Fatalf("GetCurrentCollectionSelection(bob) error = %v", err)
// 		}
// 		if len(got) != 1 || got[0].ID != alicesCollection.ID {
// 			t.Errorf("GetCurrentCollectionSelection(bob) = %+v, want alice's public collection", got)
// 		}
// 	})
// }

// func TestSaveCollectionSelection(t *testing.T) {
// 	t.Run("selects and persists", func(t *testing.T) {
// 		db := newTestDB(t)
// 		ctx := context.Background()
// 		insertProfile(t, db, "alice-token", "Alice")

// 		c, err := db.CreateUserCollection(ctx, "alice-token", testCollectionInput("Collection"))
// 		if err != nil {
// 			t.Fatalf("creating collection: %v", err)
// 		}

// 		if err := db.SaveCollectionSelection(ctx, "alice-token", CollectionSelectionForm{CollectionIDs: []uuid.UUID{c.ID}}); err != nil {
// 			t.Fatalf("SaveCollectionSelection() error = %v", err)
// 		}

// 		got, err := db.GetCurrentCollectionSelection(ctx, "alice-token")
// 		if err != nil {
// 			t.Fatalf("GetCurrentCollectionSelection() error = %v", err)
// 		}
// 		if len(got) != 1 || got[0].ID != c.ID {
// 			t.Errorf("GetCurrentCollectionSelection() = %+v, want collection %v selected", got, c.ID)
// 		}
// 	})

// 	t.Run("second save drops omitted collections", func(t *testing.T) {
// 		db := newTestDB(t)
// 		ctx := context.Background()
// 		insertProfile(t, db, "alice-token", "Alice")

// 		c1, err := db.CreateUserCollection(ctx, "alice-token", testCollectionInput("Collection One"))
// 		if err != nil {
// 			t.Fatalf("creating collection one: %v", err)
// 		}
// 		c2, err := db.CreateUserCollection(ctx, "alice-token", testCollectionInput("Collection Two"))
// 		if err != nil {
// 			t.Fatalf("creating collection two: %v", err)
// 		}

// 		if err := db.SaveCollectionSelection(ctx, "alice-token", CollectionSelectionForm{CollectionIDs: []uuid.UUID{c1.ID, c2.ID}}); err != nil {
// 			t.Fatalf("first SaveCollectionSelection() error = %v", err)
// 		}
// 		if err := db.SaveCollectionSelection(ctx, "alice-token", CollectionSelectionForm{CollectionIDs: []uuid.UUID{c1.ID}}); err != nil {
// 			t.Fatalf("second SaveCollectionSelection() error = %v", err)
// 		}

// 		got, err := db.GetCurrentCollectionSelection(ctx, "alice-token")
// 		if err != nil {
// 			t.Fatalf("GetCurrentCollectionSelection() error = %v", err)
// 		}
// 		if len(got) != 1 || got[0].ID != c1.ID {
// 			t.Errorf("GetCurrentCollectionSelection() = %+v, want only c1", got)
// 		}
// 	})

// 	t.Run("empty payload clears the selection", func(t *testing.T) {
// 		db := newTestDB(t)
// 		ctx := context.Background()
// 		insertProfile(t, db, "alice-token", "Alice")

// 		c, err := db.CreateUserCollection(ctx, "alice-token", testCollectionInput("Collection"))
// 		if err != nil {
// 			t.Fatalf("creating collection: %v", err)
// 		}
// 		if err := db.SaveCollectionSelection(ctx, "alice-token", CollectionSelectionForm{CollectionIDs: []uuid.UUID{c.ID}}); err != nil {
// 			t.Fatalf("initial SaveCollectionSelection() error = %v", err)
// 		}

// 		if err := db.SaveCollectionSelection(ctx, "alice-token", CollectionSelectionForm{}); err != nil {
// 			t.Fatalf("clearing SaveCollectionSelection() error = %v", err)
// 		}

// 		got, err := db.GetCurrentCollectionSelection(ctx, "alice-token")
// 		if err != nil {
// 			t.Fatalf("GetCurrentCollectionSelection() error = %v", err)
// 		}
// 		if len(got) != 0 {
// 			t.Errorf("GetCurrentCollectionSelection() = %+v, want empty after clearing", got)
// 		}
// 	})

// 	t.Run("inaccessible collection is rejected and nothing is left behind", func(t *testing.T) {
// 		db := newTestDB(t)
// 		ctx := context.Background()
// 		insertProfile(t, db, "alice-token", "Alice")
// 		insertProfile(t, db, "bob-token", "Bob")

// 		bobsCollection, err := db.CreateUserCollection(ctx, "bob-token", testCollectionInput("Bob's Private Collection"))
// 		if err != nil {
// 			t.Fatalf("creating bob's collection: %v", err)
// 		}

// 		err = db.SaveCollectionSelection(ctx, "alice-token", CollectionSelectionForm{CollectionIDs: []uuid.UUID{bobsCollection.ID}})
// 		assertErr(t, err, ErrInvalidInput)

// 		got, err := db.GetCurrentCollectionSelection(ctx, "alice-token")
// 		if err != nil {
// 			t.Fatalf("GetCurrentCollectionSelection() error = %v", err)
// 		}
// 		if len(got) != 0 {
// 			t.Errorf("GetCurrentCollectionSelection() = %+v, want empty after rejected save", got)
// 		}
// 	})

// 	t.Run("unknown profile", func(t *testing.T) {
// 		db := newTestDB(t)
// 		err := db.SaveCollectionSelection(context.Background(), "ghost-token", CollectionSelectionForm{})
// 		assertErr(t, err, ErrProfileNotFound)
// 	})
// }
