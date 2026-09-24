// The bundle: the portable, ID-free form of catalogs and collections, its
// file-level rules, the two conversions that connect it to stored rows, and
// the content hashes a linked copy is compared by. extractBundle turns stored
// trees into a Bundle; collectionFormFromBundle turns one of its collections
// into the CollectionForm the create core writes.

package vault

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

// BundleFormat and BundleVersion identify a bundle file.
const (
	BundleFormat  = "uno"
	BundleVersion = 1
)

// Bundle is catalogs and collections with every row ID replaced by a key.
// Catalogs holds listed catalogs; a collection's own Catalogs are scoped to
// it. Refs name a catalog by its key, which is unique across the bundle.
type Bundle struct {
	Format      string             `json:"format"`
	Version     int                `json:"version"`
	Catalogs    []BundleCatalog    `json:"catalogs"`
	Collections []BundleCollection `json:"collections"`
}

// BundleCatalog is one catalog of a Bundle. Params is the stored recipe
// string, carried byte for byte. SourceID and Fingerprint remember the row it
// was extracted from and that row's stored fingerprint; neither is part of
// the format.
type BundleCatalog struct {
	Key         string          `json:"key"`
	Name        string          `json:"name"`
	Type        string          `json:"type"`
	Provider    string          `json:"provider"`
	Params      json.RawMessage `json:"params"`
	SourceID    *uuid.UUID      `json:"-"`
	Fingerprint string          `json:"-"`
}

// BundleCollection is one collection of a Bundle: its own fields, the
// catalogs scoped to it, and its folders in order. SourceID remembers the
// row it was extracted from and is not part of the format.
type BundleCollection struct {
	Title            string          `json:"title"`
	PinToTop         bool            `json:"pin_to_top"`
	ViewMode         string          `json:"view_mode"`
	ShowAllTab       bool            `json:"show_all_tab"`
	BackdropImageURL string          `json:"backdrop_image_url"`
	FocusGlowEnabled bool            `json:"focus_glow_enabled"`
	Catalogs         []BundleCatalog `json:"catalogs"`
	Folders          []BundleFolder  `json:"folders"`
	SourceID         *uuid.UUID      `json:"-"`
}

// BundleFolder is one folder of a BundleCollection, with its refs in order.
type BundleFolder struct {
	Title           string      `json:"title"`
	TileShape       string      `json:"tile_shape"`
	HideTitle       bool        `json:"hide_title"`
	CoverEmoji      string      `json:"cover_emoji"`
	CoverImageURL   string      `json:"cover_image_url"`
	FocusGIFURL     string      `json:"focus_gif_url"`
	FocusGIFEnabled bool        `json:"focus_gif_enabled"`
	HeroBackdropURL string      `json:"hero_backdrop_url"`
	HeroVideoURL    string      `json:"hero_video_url"`
	TitleLogoURL    string      `json:"title_logo_url"`
	Refs            []BundleRef `json:"refs"`
}

// UnmarshalJSON decodes c with its params compacted, so an imported recipe is
// stored in the same form a saved one is. Params that don't compact are kept
// as they came, for Bundle.Validate to report.
func (c *BundleCatalog) UnmarshalJSON(data []byte) error {
	type plain BundleCatalog
	var p plain
	if err := json.Unmarshal(data, &p); err != nil {
		return err
	}
	var compact bytes.Buffer
	if json.Compact(&compact, p.Params) == nil {
		p.Params = compact.Bytes()
	}
	*c = BundleCatalog(p)
	return nil
}

// BundleRef is one folder ref: a catalog key and the genre the ref is
// narrowed to ("" for unfiltered).
type BundleRef struct {
	Catalog string `json:"catalog"`
	Genre   string `json:"genre"`
}

// The whole-bundle limits. maxBundleKeyLen bounds a catalog key, which
// export writes as c1, c2, …; maxBundleCatalogs counts top-level and
// collection catalogs together. Each collection inside a bundle is further
// bounded by CollectionForm.Validate's own limits.
const (
	maxBundleKeyLen      = 64
	maxBundleCatalogs    = 200
	maxBundleCollections = 50
)

// Validate checks the rules that belong to the file rather than to any one
// row: the format and version, the whole-bundle limits, every catalog key
// non-empty, within maxBundleKeyLen and unique across the bundle, every
// catalog's params a JSON object, and every ref naming a top-level key or
// one of its own collection's keys. It returns an ErrInvalidInput-wrapped
// error listing every problem found. Everything else is bounded by the form
// validators the rows are written through.
func (b Bundle) Validate() error {
	k := bundleKeyChecker{seen: map[string]bool{}, problems: bundleHeaderProblems(b)}
	top := k.catalogs("", b.Catalogs)
	for i, bc := range b.Collections {
		prefix := fmt.Sprintf("collection %d: ", i)
		own := k.catalogs(prefix, bc.Catalogs)
		k.problems = append(k.problems, bundleRefProblems(prefix, bc.Folders, top, own)...)
	}
	if len(k.problems) == 0 {
		return nil
	}
	return fmt.Errorf("%w: bundle: %s", ErrInvalidInput, strings.Join(k.problems, "; "))
}

// bundleHeaderProblems collects the problems with b as a whole: its format,
// its version and its size. Part of Bundle.Validate.
func bundleHeaderProblems(b Bundle) []string {
	var problems []string
	if b.Format != BundleFormat {
		problems = append(problems, fmt.Sprintf("format must be %q", BundleFormat))
	}
	if b.Version != BundleVersion {
		problems = append(problems, fmt.Sprintf("version must be %d", BundleVersion))
	}
	if b.catalogCount() > maxBundleCatalogs {
		problems = append(problems, fmt.Sprintf("a bundle holds at most %d catalogs", maxBundleCatalogs))
	}
	if len(b.Collections) > maxBundleCollections {
		problems = append(problems, fmt.Sprintf("a bundle holds at most %d collections", maxBundleCollections))
	}
	return problems
}

// catalogCount is how many catalogs b holds, top-level and in collections.
func (b Bundle) catalogCount() int {
	n := len(b.Catalogs)
	for _, bc := range b.Collections {
		n += len(bc.Catalogs)
	}
	return n
}

// bundleKeyChecker is the state Bundle.Validate shares across the bundle's
// catalog lists: every key seen so far, and the problems found so far.
type bundleKeyChecker struct {
	seen     map[string]bool
	problems []string
}

// catalogs checks one catalog list, each problem prefixed with where the
// list sits, and returns the set of its keys.
func (k *bundleKeyChecker) catalogs(prefix string, catalogs []BundleCatalog) map[string]bool {
	keys := make(map[string]bool, len(catalogs))
	for i, c := range catalogs {
		at := fmt.Sprintf("%scatalog %d: ", prefix, i)
		k.problems = appendProblem(k.problems, bundleKeyProblem(at, c.Key, k.seen))
		k.problems = appendProblem(k.problems, paramsObjectProblem(at, c.Params))
		k.seen[c.Key] = true
		keys[c.Key] = true
	}
	return keys
}

// bundleKeyProblem reports what is wrong with a catalog key, given every key
// seen before it, or "" when nothing is.
func bundleKeyProblem(prefix, key string, seen map[string]bool) string {
	switch {
	case key == "":
		return prefix + "key is required"
	case len(key) > maxBundleKeyLen:
		return fmt.Sprintf("%skey is longer than %d characters", prefix, maxBundleKeyLen)
	case seen[key]:
		return fmt.Sprintf("%skey %q is used by another catalog", prefix, key)
	}
	return ""
}

// paramsObjectProblem reports that params is not a JSON object, or "" when
// it is. Missing or null params are not one.
func paramsObjectProblem(prefix string, params json.RawMessage) string {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(params, &object); err != nil || object == nil {
		return prefix + "params must be a JSON object"
	}
	return ""
}

// bundleRefProblems reports every ref in folders that names neither a
// top-level key nor one of own, the keys of the collection the folders
// belong to.
func bundleRefProblems(prefix string, folders []BundleFolder, top, own map[string]bool) []string {
	var problems []string
	for i, f := range folders {
		for j, ref := range f.Refs {
			if !top[ref.Catalog] && !own[ref.Catalog] {
				problems = append(problems, fmt.Sprintf("%sfolder %d: ref %d: catalog %q is neither a top-level catalog nor one of this collection's own", prefix, i, j, ref.Catalog))
			}
		}
	}
	return problems
}

// extractBundle builds the Bundle form of listed and trees. listed is
// emitted first, as top-level catalogs; then each tree's folders are walked
// in order, and each ref's catalog is emitted the first time it is seen, with
// keys c1, c2, … handed out in emission order.
//
// With scopeAll, every catalog a collection references goes into that
// collection's own list. Otherwise a listed catalog goes top-level, once
// across every collection, and only scoped catalogs stay inside theirs.
//
// A tree's Catalogs is used only to look a ref's catalog up; a ref whose
// catalog isn't there is dropped.
func extractBundle(listed []Catalog, trees []CollectionWithFolders, scopeAll bool) Bundle {
	x := &bundleExtractor{scopeAll: scopeAll, topKeys: map[uuid.UUID]string{}, top: []BundleCatalog{}}
	for _, c := range listed {
		x.topKey(c)
	}
	collections := make([]BundleCollection, len(trees))
	for i, tree := range trees {
		collections[i] = x.collection(tree)
	}
	return Bundle{Format: BundleFormat, Version: BundleVersion, Catalogs: x.top, Collections: collections}
}

// bundleExtractor is the state one extractBundle call shares across its
// collections: the next key, and the top-level catalogs emitted so far.
type bundleExtractor struct {
	scopeAll bool
	keys     int
	topKeys  map[uuid.UUID]string
	top      []BundleCatalog
}

// newKey hands out the next catalog key.
func (x *bundleExtractor) newKey() string {
	x.keys++
	return "c" + strconv.Itoa(x.keys)
}

// topKey returns c's top-level key, emitting c the first time it is seen.
func (x *bundleExtractor) topKey(c Catalog) string {
	if key, ok := x.topKeys[c.ID]; ok {
		return key
	}
	bc := bundleCatalogFrom(x.newKey(), c)
	x.topKeys[c.ID] = bc.Key
	x.top = append(x.top, bc)
	return bc.Key
}

// collection extracts one tree.
func (x *bundleExtractor) collection(tree CollectionWithFolders) BundleCollection {
	cx := collectionExtractor{x: x, byID: make(map[uuid.UUID]Catalog, len(tree.Catalogs)), ownKeys: map[uuid.UUID]string{}, own: []BundleCatalog{}}
	for _, c := range tree.Catalogs {
		cx.byID[c.ID] = c
	}
	folders := make([]BundleFolder, len(tree.Folders))
	for i, f := range tree.Folders {
		folders[i] = cx.folder(f)
	}
	sourceID := tree.ID
	return BundleCollection{
		Title:            tree.Title,
		PinToTop:         tree.PinToTop,
		ViewMode:         tree.ViewMode,
		ShowAllTab:       tree.ShowAllTab,
		BackdropImageURL: tree.BackdropImageURL,
		FocusGlowEnabled: tree.FocusGlowEnabled,
		Catalogs:         cx.own,
		Folders:          folders,
		SourceID:         &sourceID,
	}
}

// collectionExtractor is the state of one tree's extraction: its catalogs by
// id, and the catalogs emitted into its own list so far.
type collectionExtractor struct {
	x       *bundleExtractor
	byID    map[uuid.UUID]Catalog
	ownKeys map[uuid.UUID]string
	own     []BundleCatalog
}

// folder extracts one folder, dropping any ref whose catalog is missing.
func (cx *collectionExtractor) folder(f FolderWithCatalogs) BundleFolder {
	refs := make([]BundleRef, 0, len(f.Refs))
	for _, ref := range f.Refs {
		if key, ok := cx.refKey(ref.CatalogID); ok {
			refs = append(refs, BundleRef{Catalog: key, Genre: ref.Genre})
		}
	}
	return BundleFolder{
		Title:           f.Title,
		TileShape:       f.TileShape,
		HideTitle:       f.HideTitle,
		CoverEmoji:      f.CoverEmoji,
		CoverImageURL:   f.CoverImageURL,
		FocusGIFURL:     f.FocusGIFURL,
		FocusGIFEnabled: f.FocusGIFEnabled,
		HeroBackdropURL: f.HeroBackdropURL,
		HeroVideoURL:    f.HeroVideoURL,
		TitleLogoURL:    f.TitleLogoURL,
		Refs:            refs,
	}
}

// refKey returns the key of the catalog a ref names, emitting the catalog
// where it belongs the first time, or false when the tree has no such
// catalog.
func (cx *collectionExtractor) refKey(id uuid.UUID) (string, bool) {
	c, ok := cx.byID[id]
	if !ok {
		return "", false
	}
	if !cx.x.scopeAll && c.CollectionID == nil {
		return cx.x.topKey(c), true
	}
	return cx.ownKey(c), true
}

// ownKey returns c's key in this collection's own list, emitting c the first
// time it is seen.
func (cx *collectionExtractor) ownKey(c Catalog) string {
	if key, ok := cx.ownKeys[c.ID]; ok {
		return key
	}
	bc := bundleCatalogFrom(cx.x.newKey(), c)
	cx.ownKeys[c.ID] = bc.Key
	cx.own = append(cx.own, bc)
	return bc.Key
}

// bundleCatalogFrom is c under key.
func bundleCatalogFrom(key string, c Catalog) BundleCatalog {
	sourceID := c.ID
	return BundleCatalog{
		Key:         key,
		Name:        c.Name,
		Type:        c.Type,
		Provider:    c.Provider,
		Params:      json.RawMessage(c.Params),
		SourceID:    &sourceID,
		Fingerprint: c.Fingerprint,
	}
}

// collectionFormFromBundle builds the CollectionForm that writes bc as a new
// collection. A ref to one of bc's own catalogs becomes a New entry carrying
// that catalog's spec and stored fingerprint, plus its SourceID as TakenFrom
// when link is set; every ref to one key shares one spec. A ref to a
// top-level key becomes a CatalogID ref to topIDs[key]. Folder IDs are nil
// and the collection is private.
func collectionFormFromBundle(bc BundleCollection, topIDs map[string]uuid.UUID, link bool) CollectionForm {
	specs := newSpecsByKey(bc.Catalogs, link)
	folders := make([]FolderData, len(bc.Folders))
	for i, f := range bc.Folders {
		folders[i] = folderDataFromBundle(f, specs, topIDs)
	}
	return CollectionForm{
		Title:            bc.Title,
		PinToTop:         bc.PinToTop,
		ViewMode:         bc.ViewMode,
		ShowAllTab:       bc.ShowAllTab,
		BackdropImageURL: bc.BackdropImageURL,
		FocusGlowEnabled: bc.FocusGlowEnabled,
		Folders:          folders,
	}
}

// newSpecsByKey builds the New spec for each of a collection's own catalogs,
// keyed by catalog key.
func newSpecsByKey(catalogs []BundleCatalog, link bool) map[string]*NewScopedCatalog {
	specs := make(map[string]*NewScopedCatalog, len(catalogs))
	for _, c := range catalogs {
		spec := &NewScopedCatalog{
			Key:         c.Key,
			Type:        c.Type,
			Name:        c.Name,
			Provider:    c.Provider,
			Params:      string(c.Params),
			Fingerprint: c.Fingerprint,
		}
		if link {
			spec.TakenFrom = c.SourceID
		}
		specs[c.Key] = spec
	}
	return specs
}

// folderDataFromBundle is f as a new folder; see collectionFormFromBundle.
func folderDataFromBundle(f BundleFolder, specs map[string]*NewScopedCatalog, topIDs map[string]uuid.UUID) FolderData {
	refs := make([]FolderCatalogRef, len(f.Refs))
	for i, ref := range f.Refs {
		refs[i] = FolderCatalogRef{Genre: ref.Genre}
		if spec, ok := specs[ref.Catalog]; ok {
			refs[i].New = spec
			continue
		}
		id := topIDs[ref.Catalog]
		refs[i].CatalogID = &id
	}
	return FolderData{
		Title:           f.Title,
		TileShape:       f.TileShape,
		HideTitle:       f.HideTitle,
		CoverEmoji:      f.CoverEmoji,
		CoverImageURL:   f.CoverImageURL,
		FocusGIFURL:     f.FocusGIFURL,
		FocusGIFEnabled: f.FocusGIFEnabled,
		HeroBackdropURL: f.HeroBackdropURL,
		HeroVideoURL:    f.HeroVideoURL,
		TitleLogoURL:    f.TitleLogoURL,
		Catalogs:        refs,
	}
}

// catalogHash is the content hash of a listed catalog: its name and stored
// fingerprint, the two things a catalog save or Update can change. The name
// is length-prefixed, so no two (name, fingerprint) pairs share an input.
func catalogHash(name, fingerprint string) string {
	return sha256Hex([]byte(strconv.Itoa(len(name)) + ":" + name + fingerprint))
}

// collectionHash is bundleCollectionHash over tree's bundle form, with every
// catalog it references in its own list.
func collectionHash(tree CollectionWithFolders) (string, error) {
	return bundleCollectionHash(extractBundle(nil, []CollectionWithFolders{tree}, true).Collections[0])
}

// bundleCollectionHash is the content hash of bc: sha256 hex over its JSON
// form, with each catalog's params replaced by that catalog's stored
// fingerprint. A copy carries its original's fingerprints unchanged, so the
// two hash alike whatever their params bytes. Whatever the bundle form
// leaves out — ids, scope, is_public, the home fields, version and
// timestamps — the hash leaves out too, and a content field added to the
// form is hashed with no change here.
func bundleCollectionHash(bc BundleCollection) (string, error) {
	catalogs := make([]BundleCatalog, len(bc.Catalogs))
	for i, c := range bc.Catalogs {
		fingerprint, err := json.Marshal(c.Fingerprint)
		if err != nil {
			return "", fmt.Errorf("hashing collection: %w", err)
		}
		c.Params = fingerprint
		catalogs[i] = c
	}
	bc.Catalogs = catalogs
	b, err := json.Marshal(bc)
	if err != nil {
		return "", fmt.Errorf("hashing collection: %w", err)
	}
	return sha256Hex(b), nil
}

// storedCollectionHash loads collection id's tree through q and hashes it.
// Returns ErrCollectionNotFound if there is no such row.
func storedCollectionHash(ctx context.Context, q querier, id uuid.UUID) (string, error) {
	tree, err := selectTree(ctx, q, "id = ?", id.String())
	if err != nil {
		return "", err
	}
	return collectionHash(tree)
}

// sha256Hex is the hex sha256 of b.
func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
