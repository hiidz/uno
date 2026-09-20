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

// maxNameLen bounds every name a person types for a row: a catalog's name, a
// collection's title, a folder's title. All three are stored, served from the
// public addon route and pushed into Nuvio's collections blob, where they are
// read off a TV screen; a few hundred characters is far past anything that
// renders.
const maxNameLen = 200

// maxParamsLen bounds a catalog's params JSON. The widest recipe the builder
// can produce names every watch provider of a region alongside every genre,
// which runs to a couple of kilobytes; anything past this is not a recipe.
const maxParamsLen = 8192

// lengthProblem reports that field is longer than limit, or "" when it isn't.
// The message counts bytes and says "characters", the same way the other
// length messages in this file do.
func lengthProblem(field, value string, limit int) string {
	if len(value) > limit {
		return fmt.Sprintf("%s is longer than %d characters", field, limit)
	}
	return ""
}

// appendProblem adds problem to problems unless it is "", the empty value
// every *Problem function in this file returns for "nothing wrong".
func appendProblem(problems []string, problem string) []string {
	if problem == "" {
		return problems
	}
	return append(problems, problem)
}

// Validate checks that in has a name no longer than maxNameLen, a supported
// provider, a supported content type, and params no longer than
// maxParamsLen, returning an ErrInvalidInput-wrapped error listing every
// problem found.
func (in CatalogForm) Validate() error {
	var problems []string

	if strings.TrimSpace(in.Name) == "" {
		problems = append(problems, "name is required")
	}
	problems = appendProblem(problems, lengthProblem("name", in.Name, maxNameLen))
	if !validProviders[in.Provider] {
		problems = append(problems, `provider must be "tmdb"`)
	}
	if !validCatalogTypes[in.Type] {
		problems = append(problems, `type must be "movie" or "series"`)
	}
	problems = appendProblem(problems, lengthProblem("params", in.Params, maxParamsLen))

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

// maxCoverEmojiLen bounds a folder's cover emoji. The editor's input for it
// caps at eight UTF-16 code units, whose UTF-8 encoding never reaches this.
const maxCoverEmojiLen = 32

// maxFoldersPerCollection bounds how many folders one collection holds and
// maxRefsPerFolder how many catalog refs one folder holds. A collection is
// browsed with a remote, a folder tab at a time; both bounds sit well past a
// home screen anyone can navigate.
const (
	maxFoldersPerCollection = 100
	maxRefsPerFolder        = 100
)

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

// collectionProblems collects every problem on a collection's own fields —
// its title, its view mode, its backdrop, and how many folders it holds, but
// not the folders themselves. Shared by CollectionForm.Validate and the
// take/duplicate copy path, which checks rows it is about to copy out of
// another profile rather than a form.
func collectionProblems(title, viewMode, backdropImageURL string, folders int) []string {
	var problems []string
	if strings.TrimSpace(title) == "" {
		problems = append(problems, "title is required")
	}
	problems = appendProblem(problems, lengthProblem("title", title, maxNameLen))
	if viewMode != "" && !validViewModes[viewMode] {
		problems = append(problems, `view mode must be "TABBED_GRID", "ROWS", or "FOLLOW_LAYOUT"`)
	}
	problems = appendProblem(problems, mediaURLProblem("backdrop image url", backdropImageURL))
	if folders > maxFoldersPerCollection {
		problems = append(problems, fmt.Sprintf("a collection holds at most %d folders", maxFoldersPerCollection))
	}
	return problems
}

// folderProblems collects every problem on one folder's own fields — its
// title, its tile shape, its cover emoji and its media URLs, but not its
// catalog refs — prefixed with the folder's index. Shared by
// CollectionForm.Validate and the take/duplicate copy path.
func folderProblems(index int, fd FolderData) []string {
	var problems []string
	if strings.TrimSpace(fd.Title) == "" {
		problems = append(problems, fmt.Sprintf("folder %d: title is required", index))
	}
	for _, f := range []struct {
		name  string
		value string
		limit int
	}{
		{"title", fd.Title, maxNameLen},
		{"cover emoji", fd.CoverEmoji, maxCoverEmojiLen},
	} {
		problems = appendProblem(problems, lengthProblem(fmt.Sprintf("folder %d: %s", index, f.name), f.value, f.limit))
	}
	if fd.TileShape != "" && !validTileShapes[fd.TileShape] {
		problems = append(problems, fmt.Sprintf(`folder %d: tile shape must be "POSTER", "LANDSCAPE", or "SQUARE"`, index))
	}
	for _, f := range []struct{ name, value string }{
		{"cover image url", fd.CoverImageURL},
		{"focus gif url", fd.FocusGIFURL},
		{"hero backdrop url", fd.HeroBackdropURL},
		{"hero video url", fd.HeroVideoURL},
		{"title logo url", fd.TitleLogoURL},
	} {
		problems = appendProblem(problems, mediaURLProblem(fmt.Sprintf("folder %d: %s", index, f.name), f.value))
	}
	return problems
}

// refCountProblem reports that folder index holds more catalog refs than
// maxRefsPerFolder allows, or "" when it doesn't. Shared by
// CollectionForm.Validate, which counts a form's entries, and the
// take/duplicate copy path, which counts stored refs.
func refCountProblem(index, count int) string {
	if count > maxRefsPerFolder {
		return fmt.Sprintf("folder %d: holds more than %d catalogs", index, maxRefsPerFolder)
	}
	return ""
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
// recognized enum values, names and ref genres within their length bounds,
// folder and ref counts within theirs, no catalog repeated under the same
// genre within a folder, and New entries that share a Key sharing one spec,
// returning an ErrInvalidInput-wrapped error listing every problem found.
func (in CollectionForm) Validate() error {
	specByKey := map[string]NewScopedCatalog{}

	problems := collectionProblems(in.Title, in.ViewMode, in.BackdropImageURL, len(in.Folders))

	for i, f := range in.Folders {
		problems = append(problems, folderProblems(i, f)...)
		problems = appendProblem(problems, refCountProblem(i, len(f.Catalogs)))
		seen := map[folderRefKey]bool{}
		seenNew := map[folderNewRefKey]bool{}
		for j, ref := range f.Catalogs {
			if (ref.CatalogID == nil) == (ref.New == nil) {
				problems = append(problems, fmt.Sprintf("folder %d: catalog ref %d must set exactly one of catalog_id or new", i, j))
				continue
			}
			problems = appendProblem(problems, lengthProblem(fmt.Sprintf("folder %d: catalog ref %d: genre", i, j), ref.Genre, maxGenreLen))
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
			for _, f := range []struct {
				name  string
				value string
				limit int
			}{
				{"name", ref.New.Name, maxNameLen},
				{"params", ref.New.Params, maxParamsLen},
			} {
				problems = appendProblem(problems, lengthProblem(fmt.Sprintf("folder %d: new catalog %d: %s", i, j, f.name), f.value, f.limit))
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
