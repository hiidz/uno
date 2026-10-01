package vault

import (
	"cmp"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"

	"github.com/google/uuid"
)

// Sentinel errors returned by the DB methods in this package. Handlers use
// errors.Is against these to pick an HTTP status (see api.writeVaultError).
var (
	ErrProfileNotFound    = errors.New("profile not found")
	ErrCatalogNotFound    = errors.New("catalog not found")
	ErrCollectionNotFound = errors.New("collection not found")
	// ErrPublicationNotFound is a publication the caller can't see: none by
	// that id, one unpublished, their own for a subscribe or duplicate, or, for an
	// Update, one they don't subscribe to.
	ErrPublicationNotFound = errors.New("publication not found")
	ErrInvalidInput        = errors.New("invalid input")
	// ErrConflict is a request the caller's own current state rules out: a
	// second subscription to one publication, or a publish racing an edit of
	// its source. Its message is safe to show the client.
	ErrConflict = errors.New("conflict")
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
	problems := catalogSpecProblems("", in.Type, in.Name, in.Provider, in.Params)
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

// The values stored for an empty view mode or tile shape: what every Nuvio
// client shows for one.
const (
	defaultViewMode  = "TABBED_GRID"
	defaultTileShape = "POSTER"
)

// normalized is in as a builder write or an import stores it: its name
// trimmed. Normalizing before validating means an untouched save, which the
// editor sends trimmed, writes back exactly what is stored.
func (in CatalogForm) normalized() CatalogForm {
	in.Name = strings.TrimSpace(in.Name)
	return in
}

// normalized is in as a builder write or an import stores it: every text
// field trimmed, including the names of its new catalogs and catalog edits,
// and an empty view mode or tile shape replaced by its default. A copy or an
// Update writes the stored values it read instead, so it hashes like its
// original.
func (in CollectionForm) normalized() CollectionForm {
	in.Title = strings.TrimSpace(in.Title)
	in.ViewMode = cmp.Or(in.ViewMode, defaultViewMode)
	in.BackdropImageURL = strings.TrimSpace(in.BackdropImageURL)
	in.Folders = slices.Clone(in.Folders)
	for i := range in.Folders {
		in.Folders[i] = in.Folders[i].normalized()
	}
	in.CatalogEdits = slices.Clone(in.CatalogEdits)
	for i := range in.CatalogEdits {
		in.CatalogEdits[i].Name = strings.TrimSpace(in.CatalogEdits[i].Name)
	}
	return in
}

// normalized is fd as CollectionForm.normalized stores it.
func (fd FolderData) normalized() FolderData {
	for _, s := range []*string{
		&fd.Title, &fd.CoverEmoji, &fd.CoverImageURL, &fd.FocusGIFURL,
		&fd.HeroBackdropURL, &fd.HeroVideoURL, &fd.TitleLogoURL,
	} {
		*s = strings.TrimSpace(*s)
	}
	fd.TileShape = cmp.Or(fd.TileShape, defaultTileShape)
	fd.Catalogs = slices.Clone(fd.Catalogs)
	for i, ref := range fd.Catalogs {
		if ref.New != nil {
			spec := *ref.New
			spec.Name = strings.TrimSpace(spec.Name)
			fd.Catalogs[i].New = &spec
		}
	}
	return fd
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
// collection can reach a profile that never authored it (a subscribe),
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
// not the folders themselves. Part of CollectionForm.Validate.
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
// catalog refs — prefixed with the folder's index. Part of
// CollectionForm.Validate.
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
// maxRefsPerFolder allows, or "" when it doesn't. Part of
// CollectionForm.Validate.
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
// genre within a folder, New entries that share a Key sharing one spec, and
// catalog edits that pass a catalog save's own checks with no catalog edited
// twice, returning an ErrInvalidInput-wrapped error listing every problem
// found.
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
			// A New entry becomes exactly such a row, in the same
			// transaction as this save.
			problems = append(problems, catalogSpecProblems(fmt.Sprintf("folder %d: new catalog %d: ", i, j),
				ref.New.Type, ref.New.Name, ref.New.Provider, ref.New.Params)...)
		}
	}

	problems = append(problems, catalogEditProblems(in.CatalogEdits)...)

	if len(problems) == 0 {
		return nil
	}
	return fmt.Errorf("%w: %s", ErrInvalidInput, strings.Join(problems, "; "))
}

// validateCreate is Validate plus the one rule that only applies to a
// collection being created: it has no scoped catalogs yet, so there is
// nothing for CatalogEdits to rewrite.
func (in CollectionForm) validateCreate() error {
	if len(in.CatalogEdits) > 0 {
		return fmt.Errorf("%w: a new collection has no catalogs to edit", ErrInvalidInput)
	}
	return in.Validate()
}

// catalogSpecProblems collects every problem on a catalog spec, each message
// prefixed with where the spec sits: the one set of catalog rules, behind
// CatalogForm.Validate (with no prefix), a collection save's New entries and
// CatalogEdits, and a bundle's catalogs.
func catalogSpecProblems(prefix, catalogType, name, catalogProvider, params string) []string {
	var problems []string
	if strings.TrimSpace(name) == "" {
		problems = append(problems, prefix+"name is required")
	}
	problems = appendProblem(problems, lengthProblem(prefix+"name", name, maxNameLen))
	if !validProviders[catalogProvider] {
		problems = append(problems, prefix+`provider must be "tmdb"`)
	}
	if !validCatalogTypes[catalogType] {
		problems = append(problems, prefix+`type must be "movie" or "series"`)
	}
	problems = appendProblem(problems, lengthProblem(prefix+"params", params, maxParamsLen))
	return problems
}

// catalogEditProblems collects every problem on a CollectionForm's
// CatalogEdits: each entry's spec, and a catalog edited twice in one save.
func catalogEditProblems(edits []ScopedCatalogEdit) []string {
	var problems []string
	seen := make(map[uuid.UUID]bool, len(edits))
	for i, e := range edits {
		prefix := fmt.Sprintf("catalog edit %d: ", i)
		if seen[e.ID] {
			problems = append(problems, prefix+"repeats a catalog another edit already changes")
		}
		seen[e.ID] = true
		problems = append(problems, catalogSpecProblems(prefix, e.Type, e.Name, e.Provider, e.Params)...)
	}
	return problems
}
