// Recipes: what a catalog asks its provider for — its type, provider and
// params — stored once per distinct recipe in the recipes table, addressed
// by a hash of its content and shared by every catalog that asks for it.

package vault

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
)

// recipeHashFormat opens every recipe hash's input. Changing it or the
// canonical params form changes every stored hash, which makes it a schema
// change (schemaVersion).
const recipeHashFormat = "uno-recipe/1"

// RecipeHash is a recipe's content address: sha256 hex over the format, the
// catalog type, the provider and the params. The params must already be in
// canonical form (provider.CanonicalParams), which every write puts them in
// before they reach this package, so one recipe always has one hash. The
// vault computes it from the bytes it stores, so a hash never names content
// other than its own.
func RecipeHash(catalogType, catalogProvider, params string) string {
	sum := sha256.Sum256([]byte(recipeHashFormat + "\n" + catalogType + "\n" + catalogProvider + "\n" + params))
	return hex.EncodeToString(sum[:])
}

// ensureRecipe stores the recipe for catalogType, catalogProvider and params,
// created at createdAt (RFC3339), unless it is stored already, and returns
// its hash. A recipe no catalog references any more is deleted by the
// recipes_drop_unused triggers, in the transaction of the delete or repoint
// that left it unused, so a caller stores the recipe and points its catalog
// at it inside one transaction.
func ensureRecipe(ctx context.Context, tx *sql.Tx, catalogType, catalogProvider, params, createdAt string) (string, error) {
	hash := RecipeHash(catalogType, catalogProvider, params)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO recipes (hash, type, provider, params, created_at) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (hash) DO NOTHING
	`, hash, catalogType, catalogProvider, params, createdAt); err != nil {
		return "", fmt.Errorf("storing recipe: %w", err)
	}
	return hash, nil
}
