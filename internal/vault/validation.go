package vault

import (
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
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

// maxGenreLen bounds a folder ref's genre. It is a TMDB genre name, the
// longest of which is well under this; anything longer can't be one.
const maxGenreLen = 64

// folderRefKey is what makes a folder ref unique within its folder.
type folderRefKey struct {
	catalogID uuid.UUID
	genre     string
}

// Validate checks that in has a title and, for it and every folder, only
// recognized enum values, ref genres no longer than maxGenreLen, and no
// catalog repeated under the same genre within a folder, returning an
// ErrInvalidInput-wrapped error
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
		seen := map[folderRefKey]bool{}
		for j, ref := range f.Catalogs {
			if (ref.CatalogID == nil) == (ref.New == nil) {
				problems = append(problems, fmt.Sprintf("folder %d: catalog ref %d must set exactly one of catalog_id or new", i, j))
				continue
			}
			if len(ref.Genre) > maxGenreLen {
				problems = append(problems, fmt.Sprintf("folder %d: catalog ref %d: genre is longer than %d characters", i, j, maxGenreLen))
			}
			// folder_catalogs is PRIMARY KEY (folder_id, catalog_id, genre): a
			// catalog may repeat in a folder under different genres, never under
			// the same one. A New entry always becomes its own row, so only
			// existing-catalog refs can collide.
			if ref.CatalogID != nil {
				key := folderRefKey{*ref.CatalogID, strings.TrimSpace(ref.Genre)}
				if seen[key] {
					problems = append(problems, fmt.Sprintf("folder %d: catalog ref %d repeats a catalog with the same genre", i, j))
				}
				seen[key] = true
			}
			if ref.New == nil {
				continue
			}
			// Same rules as CatalogForm.Validate — a New entry becomes
			// exactly such a row, in the same transaction as this save.
			if strings.TrimSpace(ref.New.Name) == "" {
				problems = append(problems, fmt.Sprintf("folder %d: new catalog %d: name is required", i, j))
			}
			if !validProviders[ref.New.Provider] {
				problems = append(problems, fmt.Sprintf(`folder %d: new catalog %d: provider must be "tmdb"`, i, j))
			}
			if !validCatalogTypes[ref.New.Type] {
				problems = append(problems, fmt.Sprintf(`folder %d: new catalog %d: type must be "movie" or "series"`, i, j))
			}
		}
	}

	if len(problems) == 0 {
		return nil
	}
	return fmt.Errorf("%w: %s", ErrInvalidInput, strings.Join(problems, "; "))
}
