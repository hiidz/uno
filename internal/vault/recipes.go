// Recipes: what a catalog asks its provider for — its type, provider and
// params — stored on the catalog's own row and addressed by a hash of that
// content, which two catalogs asking for the same thing share.

package vault

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// recipeHashFormat opens every recipe hash's input. Changing it or the
// canonical params form changes every recipe's hash, which the import check
// matches catalogs by.
const recipeHashFormat = "uno-recipe/1"

// RecipeHash is a recipe's content address: sha256 hex over the format, the
// catalog type, the provider and the params. The params must already be in
// canonical form (provider.CanonicalParams), which every write puts them in
// before they reach this package, so one recipe always has one hash. It is
// never stored: a catalog's is computed from its row when the row is read.
func RecipeHash(catalogType, catalogProvider, params string) string {
	sum := sha256.Sum256([]byte(recipeHashFormat + "\n" + catalogType + "\n" + catalogProvider + "\n" + params))
	return hex.EncodeToString(sum[:])
}

// The sources a catalog's titles come from, as recipeSource names them.
const (
	sourceFilters    = "filters"
	sourceCollection = "collection"
	sourceList       = "list"
)

// recipeSource is where a recipe's titles come from: one TMDB collection
// (with_collection), one of TMDB's own lists (tmdb_list), or discover
// filters, which empty params are. A catalog's source is chosen when it is
// made and never changes after, like its type; it is read from the params
// rather than stored.
func recipeSource(params string) string {
	var p struct {
		WithCollection string `json:"with_collection"`
		TMDBList       string `json:"tmdb_list"`
	}
	_ = json.Unmarshal([]byte(params), &p) // stored params are valid; anything else reads as filters
	switch {
	case p.TMDBList != "":
		return sourceList
	case p.WithCollection != "":
		return sourceCollection
	}
	return sourceFilters
}
