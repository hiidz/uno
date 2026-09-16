package provider

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// Fingerprint returns a stable hash of a catalog's recipe: its type,
// provider, and params re-marshaled into canonical form (Go struct field
// order), so that key order or whitespace differences in the stored params
// string don't change it while an actual filter change does. Used only to
// collapse duplicate community catalogs; never sent on the wire.
func Fingerprint(catalogType, catalogProvider, params string) (string, error) {
	canonical, err := canonicalParams(catalogType, params)
	if err != nil {
		return "", err
	}

	sum := sha256.Sum256([]byte(catalogType + "\n" + catalogProvider + "\n" + canonical))
	return hex.EncodeToString(sum[:]), nil
}

// canonicalParams unmarshals params into the typed struct for catalogType
// and re-marshals it, so two JSON encodings of the same recipe (different
// key order, whitespace, or omitted-but-zero fields) produce identical
// output.
func canonicalParams(catalogType, params string) (string, error) {
	var v any
	switch catalogType {
	case "movie":
		v = &TMDBMovieParams{}
	case "series":
		v = &TMDBTVParams{}
	default:
		return "", fmt.Errorf("%w: got %q", ErrInvalidCatalogType, catalogType)
	}

	if err := json.Unmarshal([]byte(params), v); err != nil {
		return "", fmt.Errorf("unmarshaling %s params: %w", catalogType, err)
	}
	canonical, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("marshaling %s params: %w", catalogType, err)
	}
	return string(canonical), nil
}
