// Recipes: what a catalog asks its provider for — its type, provider and
// params — stored on the catalog's own row and addressed by a hash of that
// content, which two catalogs asking for the same thing share.

package vault

import (
	"crypto/sha256"
	"encoding/hex"
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
