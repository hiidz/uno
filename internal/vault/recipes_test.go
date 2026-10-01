package vault

import (
	"context"
	"testing"
)

// A recipe goes as soon as no catalog references it: when its catalog is
// deleted, directly or by its collection's cascade, and when its catalog is
// repointed at another recipe.
func TestUnusedRecipesAreDeleted(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	owner := newTestProfile(t, db, "owner")
	recipeCount := func() int {
		t.Helper()
		var n int
		if err := db.conn.QueryRowContext(ctx, `SELECT count(*) FROM recipes`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	form := func(name, sortBy string) CatalogForm {
		f := listedCatalogForm(name)
		f.Params = `{"sort_by":"` + sortBy + `"}`
		return f
	}

	a, err := db.CreateUserCatalog(ctx, owner, form("A", "popularity.desc"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateUserCatalog(ctx, owner, form("Same recipe as A", "popularity.desc")); err != nil {
		t.Fatal(err)
	}
	if got := recipeCount(); got != 1 {
		t.Fatalf("recipes after two catalogs with one recipe = %d, want 1", got)
	}

	if _, err := db.UpdateUserCatalog(ctx, owner, a.ID, form("A", "revenue.desc")); err != nil {
		t.Fatal(err)
	}
	if got := recipeCount(); got != 2 {
		t.Fatalf("recipes after A moved to a recipe of its own = %d, want 2: the shared one is still used", got)
	}
	if _, err := db.UpdateUserCatalog(ctx, owner, a.ID, form("A", "vote_count.desc")); err != nil {
		t.Fatal(err)
	}
	if got := recipeCount(); got != 2 {
		t.Fatalf("recipes after A moved again = %d, want 2: the one it left is gone", got)
	}
	if err := db.DeleteUserCatalog(ctx, owner, a.ID); err != nil {
		t.Fatal(err)
	}
	if got := recipeCount(); got != 1 {
		t.Fatalf("recipes after deleting A = %d, want 1", got)
	}

	collectionID := newTestCollection(t, db, owner, "C")
	scoped, saved := scopedCatalogInFolder(t, db, owner, collectionID, "Scoped")
	if got := recipeCount(); got != 2 {
		t.Fatalf("recipes with a scoped catalog = %d, want 2", got)
	}
	edit := editOf(scoped)
	edit.Params = `{"sort_by":"title.asc"}`
	if _, err := db.UpdateUserCollection(ctx, owner, collectionID, CollectionForm{
		Title: "C", Folders: sameFolders(saved), CatalogEdits: []ScopedCatalogEdit{edit},
	}); err != nil {
		t.Fatal(err)
	}
	if got := recipeCount(); got != 2 {
		t.Fatalf("recipes after the scoped catalog's edit = %d, want 2: the one it left is gone", got)
	}
	if err := db.DeleteUserCollection(ctx, owner, collectionID); err != nil {
		t.Fatal(err)
	}
	if got := recipeCount(); got != 1 {
		t.Fatalf("recipes after the collection's cascade = %d, want 1", got)
	}

	orphaned := newTestCollection(t, db, owner, "D")
	scopedCatalogInFolder(t, db, owner, orphaned, "Orphan")
	if _, err := db.UpdateUserCollection(ctx, owner, orphaned, CollectionForm{Title: "D"}); err != nil {
		t.Fatal(err)
	}
	if got := recipeCount(); got != 1 {
		t.Fatalf("recipes after the save that dropped the scoped catalog's last ref = %d, want 1", got)
	}
}

// RecipeHash is pinned to a literal: every stored recipe_hash, and every
// subscribed_hash built over them, was computed by it. A change here is a schema
// change (schemaVersion).
func TestRecipeHashIsPinned(t *testing.T) {
	const want = "e445db512121e490bde38e1f096775df84e1493ded6ebf5e6e1c57a91ad31730"
	if got := RecipeHash("movie", "tmdb", `{"sort_by":"popularity.desc"}`); got != want {
		t.Errorf("RecipeHash = %s, pinned %s", got, want)
	}
}
