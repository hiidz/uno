package vault

import (
	"errors"
	"fmt"
	"strings"
)

// Sentinel errors returned by the DB methods in this package. Handlers use
// errors.Is against these to pick an HTTP status (see api.writeVaultError).
var (
	ErrProfileNotFound    = errors.New("profile not found")
	ErrCatalogNotFound    = errors.New("catalog not found")
	ErrCollectionNotFound = errors.New("collection not found")
	ErrInvalidInput       = errors.New("invalid input")
)

var validCatalogTypes = map[string]bool{
	"movie":  true,
	"series": true,
}

// validProviders is the set of providers Uno can actually validate params
// for. Only "tmdb" exists — a deliberate "futureproofing, not
// implemented" state, see the "Recipe params (TMDB)" section of
// docs/data-model.md — but the check still
// matters now, not just once a second provider is real: an unrecognized
// provider stored here would bypass api.validateCatalogParams' params check
// entirely (that function only validates when provider == "tmdb"), and
// provider is not inert like endpoint — it round-trips into
// addon.ManifestID and Nuvio's catalogSources[].catalogId.
var validProviders = map[string]bool{
	"tmdb": true,
}

// Validate checks that in has a name, a supported provider, and a
// supported content type, returning an ErrInvalidInput-wrapped error
// listing every problem found.
func (in CatalogForm) Validate() error {
	var problems []string

	if strings.TrimSpace(in.Name) == "" {
		problems = append(problems, "name is required")
	}
	if !validProviders[in.Provider] {
		problems = append(problems, `provider must be "tmdb"`)
	}
	if !validCatalogTypes[in.Type] {
		problems = append(problems, `type must be "movie" or "series"`)
	}

	if len(problems) == 0 {
		return nil
	}
	return fmt.Errorf("%w: %s", ErrInvalidInput, strings.Join(problems, "; "))
}

var validViewModes = map[string]bool{
	"TABBED_GRID":   true,
	"ROWS":          true,
	"FOLLOW_LAYOUT": true,
}

var validTileShapes = map[string]bool{
	"POSTER":    true,
	"LANDSCAPE": true,
	"SQUARE":    true,
}

// Validate checks that in has a title and, for it and every folder, only
// recognized enum values, returning an ErrInvalidInput-wrapped error
// listing every problem found.
func (in CollectionForm) Validate() error {
	var problems []string

	if strings.TrimSpace(in.Title) == "" {
		problems = append(problems, "title is required")
	}
	if in.ViewMode != "" && !validViewModes[in.ViewMode] {
		problems = append(problems, `view mode must be "TABBED_GRID", "ROWS", or "FOLLOW_LAYOUT"`)
	}

	for i, f := range in.Folders {
		if strings.TrimSpace(f.Title) == "" {
			problems = append(problems, fmt.Sprintf("folder %d: title is required", i))
		}
		if f.TileShape != "" && !validTileShapes[f.TileShape] {
			problems = append(problems, fmt.Sprintf(`folder %d: tile shape must be "POSTER", "LANDSCAPE", or "SQUARE"`, i))
		}
	}

	if len(problems) == 0 {
		return nil
	}
	return fmt.Errorf("%w: %s", ErrInvalidInput, strings.Join(problems, "; "))
}
