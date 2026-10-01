package provider

import (
	"encoding/json"
	"fmt"
)

// CanonicalParams returns params in the canonical form a stored recipe of
// catalogProvider and catalogType takes: decoded as the recipe's params
// struct, so only its known keys survive and zero values, which every field
// reads as unset, are dropped, then encoded compact with its keys sorted. Two
// encodings of one recipe give the same bytes, which is what lets catalogs
// with the same recipe share one recipes row. Changing the form changes every
// stored recipe, which makes it a schema change.
func CanonicalParams(catalogType, catalogProvider, params string) (string, error) {
	p, err := decodeRecipe(catalogType, catalogProvider, params)
	if err != nil {
		return "", err
	}
	return sortedJSON(p)
}

// decodeRecipe decodes params as the recipe of catalogProvider and
// catalogType. TMDB is the one provider this package has recipes for, so any
// other is an error.
func decodeRecipe(catalogType, catalogProvider, params string) (CatalogParams, error) {
	if catalogProvider != "tmdb" {
		return nil, fmt.Errorf("provider: no recipes for provider %q", catalogProvider)
	}
	return DecodeParams(catalogType, params)
}

// sortedJSON encodes v, then re-encodes the result with its keys sorted.
func sortedJSON(v any) (string, error) {
	encoded, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("provider: encode params: %w", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		return "", fmt.Errorf("provider: encode params: %w", err)
	}
	sorted, err := json.Marshal(fields)
	if err != nil {
		return "", fmt.Errorf("provider: encode params: %w", err)
	}
	return string(sorted), nil
}
