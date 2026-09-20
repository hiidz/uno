package vault

import (
	"errors"
	"fmt"
	"net/url"
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

// maxMediaURLLen bounds every collection and folder media URL. These are
// image, GIF and video addresses; anything past this isn't one.
const maxMediaURLLen = 2048

// mediaURLProblem reports what is wrong with one of a collection's or
// folder's media URLs, or "" when nothing is. Empty is always allowed —
// these fields are optional and omitempty on the way out to Nuvio.
//
// The scheme allowlist is the point: every one of these strings is pushed
// into Nuvio's collections blob and rendered by its clients, and a
// collection can reach a profile that never authored it (community take),
// so a javascript: or data: URL must not survive the trip.
func mediaURLProblem(field, raw string) string {
	if raw == "" {
		return ""
	}
	if len(raw) > maxMediaURLLen {
		return fmt.Sprintf("%s is longer than %d characters", field, maxMediaURLLen)
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Sprintf("%s is not a valid URL", field)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Sprintf("%s must be an http or https URL", field)
	}
	if u.Host == "" {
		return fmt.Sprintf("%s must be an absolute URL", field)
	}
	return ""
}

// folderMediaURLProblems collects every media-URL problem on one folder,
// prefixed with its index. Shared by CollectionForm.Validate and the
// take/duplicate copy path, which checks rows it is about to copy out of
// another profile rather than a form.
func folderMediaURLProblems(index int, fd FolderData) []string {
	var problems []string
	for _, f := range []struct{ name, value string }{
		{"cover image url", fd.CoverImageURL},
		{"focus gif url", fd.FocusGIFURL},
		{"hero backdrop url", fd.HeroBackdropURL},
		{"hero video url", fd.HeroVideoURL},
		{"title logo url", fd.TitleLogoURL},
	} {
		if p := mediaURLProblem(fmt.Sprintf("folder %d: %s", index, f.name), f.value); p != "" {
			problems = append(problems, p)
		}
	}
	return problems
}

// maxNewKeyLen bounds a New entry's Key. It is a client-minted handle (a
// "draft:" prefix and a UUID); anything much longer isn't one.
const maxNewKeyLen = 128

// folderRefKey is what makes an existing-catalog ref unique within its folder.
type folderRefKey struct {
	catalogID uuid.UUID
	genre     string
}

// folderNewRefKey is folderRefKey for a New entry, which has no catalog id
// yet: its Key stands for the catalog the save creates.
type folderNewRefKey struct {
	key   string
	genre string
}

// Validate checks that in has a title and, for it and every folder, only
// recognized enum values, ref genres no longer than maxGenreLen, no catalog
// repeated under the same genre within a folder, and New entries that share a
// Key sharing one spec, returning an ErrInvalidInput-wrapped error listing
// every problem found.
func (in CollectionForm) Validate() error {
	var problems []string
	specByKey := map[string]NewScopedCatalog{}

	if strings.TrimSpace(in.Title) == "" {
		problems = append(problems, "title is required")
	}
	if in.ViewMode != "" && !validViewModes[in.ViewMode] {
		problems = append(problems, `view mode must be "TABBED_GRID", "ROWS", or "FOLLOW_LAYOUT"`)
	}
	if p := mediaURLProblem("backdrop image url", in.BackdropImageURL); p != "" {
		problems = append(problems, p)
	}

	for i, f := range in.Folders {
		if strings.TrimSpace(f.Title) == "" {
			problems = append(problems, fmt.Sprintf("folder %d: title is required", i))
		}
		if f.TileShape != "" && !validTileShapes[f.TileShape] {
			problems = append(problems, fmt.Sprintf(`folder %d: tile shape must be "POSTER", "LANDSCAPE", or "SQUARE"`, i))
		}
		problems = append(problems, folderMediaURLProblems(i, f)...)
		seen := map[folderRefKey]bool{}
		seenNew := map[folderNewRefKey]bool{}
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
			// the same one. New entries sharing a Key become one catalog, so
			// they collide the same way.
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
			switch key := ref.New.Key; {
			case key == "":
				problems = append(problems, fmt.Sprintf("folder %d: new catalog %d: key is required", i, j))
			case len(key) > maxNewKeyLen:
				problems = append(problems, fmt.Sprintf("folder %d: new catalog %d: key is longer than %d characters", i, j, maxNewKeyLen))
			default:
				newKey := folderNewRefKey{key, strings.TrimSpace(ref.Genre)}
				if seenNew[newKey] {
					problems = append(problems, fmt.Sprintf("folder %d: catalog ref %d repeats a catalog with the same genre", i, j))
				}
				seenNew[newKey] = true
				if first, ok := specByKey[key]; !ok {
					specByKey[key] = *ref.New
				} else if first != *ref.New {
					problems = append(problems, fmt.Sprintf("folder %d: new catalog %d: key is shared with a different catalog spec", i, j))
				}
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
