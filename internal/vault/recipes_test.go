package vault

import (
	"testing"
)

// RecipeHash is pinned to a literal: the import check matches a bundle's
// catalogs to stored ones by it, and import reuse holds a target to it.
func TestRecipeHashIsPinned(t *testing.T) {
	const want = "e445db512121e490bde38e1f096775df84e1493ded6ebf5e6e1c57a91ad31730"
	if got := RecipeHash("movie", "tmdb", `{"sort_by":"popularity.desc"}`); got != want {
		t.Errorf("RecipeHash = %s, pinned %s", got, want)
	}
}
