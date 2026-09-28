package migrations

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strconv"
	"strings"
)

// recipes moves every catalog's recipe — its type, provider and params — into
// a recipes table with one row per distinct recipe, addressed by a hash of
// its content and shared by every catalog that asks for it:
//
//  1. Each catalog's params are made canonical (canonicalRecipeParams) and
//     hashed (recipeHash). Params that don't decode, or a provider other
//     than tmdb, fail the migration, naming the catalog.
//  2. Each linked copy's taken_hash, a link hash over stored fingerprints, is
//     rewritten as the same link hash over recipe hashes (remapLinks), so
//     every copy stays exactly as in or out of step as it was.
//  3. The recipes are inserted, catalogs gains recipe_hash naming its recipe,
//     and type, provider, params and fingerprint are dropped from catalogs.
//  4. Two triggers delete a recipe once no catalog references it, in the
//     transaction of the delete or repoint that left it unused.
func recipes(ctx context.Context, tx *sql.Tx) ([]string, error) {
	catalogs, err := loadRecipeCatalogs(ctx, tx)
	if err != nil {
		return nil, err
	}
	links, err := remapLinks(ctx, tx, catalogs)
	if err != nil {
		return nil, err
	}
	if err := writeRecipes(ctx, tx, catalogs, links); err != nil {
		return nil, err
	}
	return append(recipeNotes(catalogs), links.notes()...), nil
}

// recipeCatalog is one catalogs row as migration 2 reads it, with the
// canonical params and recipe hash it gives the row.
type recipeCatalog struct {
	id, name, catalogType, provider, params, fingerprint, createdAt string
	takenFrom, takenHash                                            sql.NullString
	canonical, hash                                                 string
}

// loadRecipeCatalogs reads every catalog, in id order, and gives each its
// canonical params and recipe hash.
func loadRecipeCatalogs(ctx context.Context, tx *sql.Tx) ([]recipeCatalog, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT id, name, type, provider, params, fingerprint, taken_from, taken_hash, created_at
		FROM catalogs ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("reading catalogs: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var catalogs []recipeCatalog
	for rows.Next() {
		var c recipeCatalog
		if err := rows.Scan(&c.id, &c.name, &c.catalogType, &c.provider, &c.params, &c.fingerprint, &c.takenFrom, &c.takenHash, &c.createdAt); err != nil {
			return nil, fmt.Errorf("reading catalogs: %w", err)
		}
		if c.canonical, err = canonicalRecipeParams(c.catalogType, c.provider, c.params); err != nil {
			return nil, fmt.Errorf("catalog %s (%s): params %s: %w", c.id, c.name, c.params, err)
		}
		c.hash = recipeHash(c.catalogType, c.provider, c.canonical)
		catalogs = append(catalogs, c)
	}
	return catalogs, rows.Err()
}

// recipeHashFormat opens every recipe hash's input. A different canonical
// form or hash input is a new format, and a new migration.
const recipeHashFormat = "uno-recipe/1"

// recipeHash is a recipe's content address: sha256 hex over the format, the
// catalog type, the provider and the canonical params.
func recipeHash(catalogType, provider, canonical string) string {
	sum := sha256.Sum256([]byte(recipeHashFormat + "\n" + catalogType + "\n" + provider + "\n" + canonical))
	return hex.EncodeToString(sum[:])
}

// recipeBaseParams, recipeCommonParams, recipeMovieParams and
// recipeSeriesParams are the TMDB recipe params as this migration knows
// them: every key, its JSON type, and omitempty on each, so a zero value is
// left out.
type recipeBaseParams struct {
	Randomized bool `json:"randomized,omitempty"`
}

type recipeCommonParams struct {
	recipeBaseParams
	SortBy               string  `json:"sort_by,omitempty"`
	WithGenres           string  `json:"with_genres,omitempty"`
	WithoutGenres        string  `json:"without_genres,omitempty"`
	WithOriginalLanguage string  `json:"with_original_language,omitempty"`
	VoteAverageGte       float64 `json:"vote_average_gte,omitempty"`
	VoteAverageLte       float64 `json:"vote_average_lte,omitempty"`
	VoteCountGte         int     `json:"vote_count_gte,omitempty"`
	VoteCountLte         int     `json:"vote_count_lte,omitempty"`
	WithRuntimeGte       int     `json:"with_runtime_gte,omitempty"`
	WithRuntimeLte       int     `json:"with_runtime_lte,omitempty"`
	WithWatchProviders   string  `json:"with_watch_providers,omitempty"`
	WatchRegion          string  `json:"watch_region,omitempty"`
	WithCompanies        string  `json:"with_companies,omitempty"`
	WithKeywords         string  `json:"with_keywords,omitempty"`
	WithoutCompanies     string  `json:"without_companies,omitempty"`
	WithoutKeywords      string  `json:"without_keywords,omitempty"`
	Certification        string  `json:"certification,omitempty"`
	CertificationGte     string  `json:"certification_gte,omitempty"`
	CertificationLte     string  `json:"certification_lte,omitempty"`
	CertificationCountry string  `json:"certification_country,omitempty"`
}

type recipeMovieParams struct {
	recipeCommonParams
	PrimaryReleaseDateGte string `json:"primary_release_date_gte,omitempty"`
	PrimaryReleaseDateLte string `json:"primary_release_date_lte,omitempty"`
	ReleasedWithinDays    int    `json:"released_within_days,omitempty"`
	WithCollection        string `json:"with_collection,omitempty"`
}

type recipeSeriesParams struct {
	recipeCommonParams
	FirstAirDateGte string `json:"first_air_date_gte,omitempty"`
	FirstAirDateLte string `json:"first_air_date_lte,omitempty"`
	AiredWithinDays int    `json:"aired_within_days,omitempty"`
	WithNetworks    string `json:"with_networks,omitempty"`
}

// recipeParamsFor returns an empty params struct for a recipe of catalogType
// and catalogProvider. TMDB is the one provider these structs describe, so a
// recipe of any other has none.
func recipeParamsFor(catalogType, catalogProvider string) (any, error) {
	if catalogProvider != "tmdb" {
		return nil, fmt.Errorf("unknown provider %q", catalogProvider)
	}
	switch catalogType {
	case "movie":
		return &recipeMovieParams{}, nil
	case "series":
		return &recipeSeriesParams{}, nil
	}
	return nil, fmt.Errorf("unknown catalog type %q", catalogType)
}

// canonicalRecipeParams decodes params as the params struct of a recipe of
// catalogType and catalogProvider, which keeps only its known keys, and
// encodes it again, which leaves out zero values, then sorts the keys.
func canonicalRecipeParams(catalogType, catalogProvider, params string) (string, error) {
	p, err := recipeParamsFor(catalogType, catalogProvider)
	if err != nil {
		return "", err
	}
	if err := json.Unmarshal([]byte(params), p); err != nil {
		return "", err
	}
	encoded, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		return "", err
	}
	sorted, err := json.Marshal(fields)
	return string(sorted), err
}

// unknownParamsKeys returns the keys of params, a JSON object, that match
// none of the keys of a recipe of catalogType and catalogProvider, ignoring
// case as decoding does: the keys canonicalRecipeParams drops for being
// unknown rather than zero. Params that aren't an object, or aren't a recipe
// these structs describe, have none.
func unknownParamsKeys(catalogType, catalogProvider, params string) []string {
	p, err := recipeParamsFor(catalogType, catalogProvider)
	var fields map[string]json.RawMessage
	if err != nil || json.Unmarshal([]byte(params), &fields) != nil {
		return nil
	}
	known := jsonKeys(reflect.TypeOf(p).Elem())
	var unknown []string
	for _, key := range slices.Sorted(maps.Keys(fields)) {
		if !slices.ContainsFunc(known, func(k string) bool { return strings.EqualFold(k, key) }) {
			unknown = append(unknown, key)
		}
	}
	return unknown
}

// jsonKeys lists the JSON keys of struct type t, embedded structs' included.
func jsonKeys(t reflect.Type) []string {
	var keys []string
	for i := range t.NumField() {
		field := t.Field(i)
		if field.Anonymous {
			keys = append(keys, jsonKeys(field.Type)...)
			continue
		}
		name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
		keys = append(keys, name)
	}
	return keys
}

// recipeNotes says how many recipes the catalogs share, how many catalogs'
// params were rewritten, and every unknown key dropped, with the catalogs it
// was dropped from.
func recipeNotes(catalogs []recipeCatalog) []string {
	hashes := map[string]bool{}
	rewritten := 0
	dropped := map[string][]string{}
	for _, c := range catalogs {
		hashes[c.hash] = true
		if c.canonical != c.params {
			rewritten++
		}
		for _, key := range unknownParamsKeys(c.catalogType, c.provider, c.params) {
			dropped[key] = append(dropped[key], c.id)
		}
	}
	notes := []string{
		fmt.Sprintf("stored %d recipes for %d catalogs", len(hashes), len(catalogs)),
		fmt.Sprintf("rewrote the params of %d catalogs in canonical form", rewritten),
	}
	for _, key := range slices.Sorted(maps.Keys(dropped)) {
		notes = append(notes, fmt.Sprintf("dropped the unknown params key %q from %d catalogs: %s", key, len(dropped[key]), strings.Join(dropped[key], ", ")))
	}
	return notes
}

// recipesDDL creates the recipes table. hash is recipeHash's.
const recipesDDL = `
CREATE TABLE recipes (
    hash       TEXT PRIMARY KEY, -- sha256 hex of uno-recipe/1, type, provider and params
    type       TEXT NOT NULL,    -- Stremio's word: movie | series
    provider   TEXT NOT NULL,    -- tmdb, for now
    params     TEXT NOT NULL,    -- canonical JSON: known keys, no zero values, keys sorted
    created_at TEXT NOT NULL     -- RFC3339 UTC
);
ALTER TABLE catalogs ADD COLUMN recipe_hash TEXT REFERENCES recipes(hash);
`

// catalogsWithoutRecipeDDL drops the columns recipes now holds, and adds the
// index and triggers that delete a recipe no catalog references any more.
const catalogsWithoutRecipeDDL = `
ALTER TABLE catalogs DROP COLUMN type;
ALTER TABLE catalogs DROP COLUMN provider;
ALTER TABLE catalogs DROP COLUMN params;
ALTER TABLE catalogs DROP COLUMN fingerprint;
CREATE INDEX catalogs_by_recipe ON catalogs (recipe_hash);
CREATE TRIGGER recipes_drop_unused_on_delete AFTER DELETE ON catalogs
WHEN NOT EXISTS (SELECT 1 FROM catalogs WHERE recipe_hash = OLD.recipe_hash)
BEGIN
    DELETE FROM recipes WHERE hash = OLD.recipe_hash;
END;
CREATE TRIGGER recipes_drop_unused_on_repoint AFTER UPDATE OF recipe_hash ON catalogs
WHEN OLD.recipe_hash IS NOT NEW.recipe_hash
 AND NOT EXISTS (SELECT 1 FROM catalogs WHERE recipe_hash = OLD.recipe_hash)
BEGIN
    DELETE FROM recipes WHERE hash = OLD.recipe_hash;
END;
`

// writeRecipes creates recipes, stores every catalog's recipe and remapped
// link, then drops the columns recipes replaced. It fails if a catalog is
// left without a recipe.
func writeRecipes(ctx context.Context, tx *sql.Tx, catalogs []recipeCatalog, links linkRemap) error {
	if _, err := tx.ExecContext(ctx, recipesDDL); err != nil {
		return fmt.Errorf("creating recipes: %w", err)
	}
	if err := insertRecipes(ctx, tx, catalogs); err != nil {
		return err
	}
	if err := pointCatalogs(ctx, tx, catalogs, links.catalogs); err != nil {
		return err
	}
	if err := writeCollectionLinks(ctx, tx, links.collections); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, catalogsWithoutRecipeDDL); err != nil {
		return fmt.Errorf("dropping the recipe columns from catalogs: %w", err)
	}
	return requireEveryRecipe(ctx, tx)
}

// insertRecipes inserts one recipes row per distinct hash, created when the
// oldest catalog asking for it was.
func insertRecipes(ctx context.Context, tx *sql.Tx, catalogs []recipeCatalog) error {
	oldest := map[string]recipeCatalog{}
	for _, c := range catalogs {
		if first, ok := oldest[c.hash]; !ok || c.createdAt < first.createdAt {
			oldest[c.hash] = c
		}
	}
	for _, hash := range slices.Sorted(maps.Keys(oldest)) {
		c := oldest[hash]
		if _, err := tx.ExecContext(ctx, `INSERT INTO recipes (hash, type, provider, params, created_at) VALUES (?, ?, ?, ?, ?)`,
			hash, c.catalogType, c.provider, c.canonical, c.createdAt); err != nil {
			return fmt.Errorf("inserting recipe %s: %w", hash, err)
		}
	}
	return nil
}

// pointCatalogs sets each catalog's recipe_hash, and the taken_hash takenHashes
// gives it, if any.
func pointCatalogs(ctx context.Context, tx *sql.Tx, catalogs []recipeCatalog, takenHashes map[string]string) error {
	for _, c := range catalogs {
		takenHash := c.takenHash
		if remapped, ok := takenHashes[c.id]; ok {
			takenHash = sql.NullString{String: remapped, Valid: true}
		}
		if _, err := tx.ExecContext(ctx, `UPDATE catalogs SET recipe_hash = ?, taken_hash = ? WHERE id = ?`, c.hash, takenHash, c.id); err != nil {
			return fmt.Errorf("pointing catalog %s at its recipe: %w", c.id, err)
		}
	}
	return nil
}

// writeCollectionLinks sets each collection's taken_hash to the one
// takenHashes gives it.
func writeCollectionLinks(ctx context.Context, tx *sql.Tx, takenHashes map[string]string) error {
	for _, id := range slices.Sorted(maps.Keys(takenHashes)) {
		if _, err := tx.ExecContext(ctx, `UPDATE collections SET taken_hash = ? WHERE id = ?`, takenHashes[id], id); err != nil {
			return fmt.Errorf("remapping collection %s's link: %w", id, err)
		}
	}
	return nil
}

// requireEveryRecipe fails if any catalog has no recipe_hash. The column
// can't be NOT NULL when added to a table that has rows, so this is its check.
func requireEveryRecipe(ctx context.Context, tx *sql.Tx) error {
	var missing int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM catalogs WHERE recipe_hash IS NULL`).Scan(&missing); err != nil {
		return fmt.Errorf("checking recipe_hash: %w", err)
	}
	if missing > 0 {
		return fmt.Errorf("%d catalogs have no recipe_hash", missing)
	}
	return nil
}

// linkRemap is every linked copy's new taken_hash, by row id, and how many
// fell in each of remapTakenHash's cases.
type linkRemap struct {
	catalogs, collections           map[string]string
	catalogCounts, collectionCounts [linkCases]int
}

// The cases remapTakenHash tells apart.
const (
	linkInStep = iota
	linkBehind
	linkEdited
	linkNeither
	linkCases
)

// notes says how many catalog and collection links fell in each case.
func (r linkRemap) notes() []string {
	return []string{linkNote("catalog", r.catalogCounts), linkNote("collection", r.collectionCounts)}
}

// linkNote is one kind of link's case counts.
func linkNote(kind string, counts [linkCases]int) string {
	return fmt.Sprintf("remapped %s links: %d in step, %d behind their source, %d edited since taken, %d matching neither hash (left as they were)",
		kind, counts[linkInStep], counts[linkBehind], counts[linkEdited], counts[linkNeither])
}

// linkHashes is one row's link hash over fingerprints (old) and over recipe
// hashes (new).
type linkHashes struct{ old, new string }

// remapTakenHash is the taken_hash a linked copy gets, given its current one
// and the old and new link hashes of the copy and of its source now, with
// the case it fell in. A taken_hash that matched the copy or the source
// becomes that row's new hash, so every comparison Update and Community make
// comes out as it did; one that matched neither is kept, and still matches
// neither.
func remapTakenHash(taken string, copyHashes, source linkHashes) (string, int) {
	switch {
	case taken == copyHashes.old && taken == source.old:
		return copyHashes.new, linkInStep
	case taken == copyHashes.old:
		return copyHashes.new, linkBehind
	case taken == source.old:
		return source.new, linkEdited
	}
	return taken, linkNeither
}

// remapLinks works out the new taken_hash of every linked listed catalog and
// linked collection.
func remapLinks(ctx context.Context, tx *sql.Tx, catalogs []recipeCatalog) (linkRemap, error) {
	byID := make(map[string]recipeCatalog, len(catalogs))
	for _, c := range catalogs {
		byID[c.id] = c
	}
	r := linkRemap{catalogs: map[string]string{}, collections: map[string]string{}}
	for _, c := range catalogs {
		source, linked := byID[c.takenFrom.String]
		if !linked || !c.takenHash.Valid {
			continue
		}
		var linkCase int
		r.catalogs[c.id], linkCase = remapTakenHash(c.takenHash.String, catalogLinkHashes(c), catalogLinkHashes(source))
		r.catalogCounts[linkCase]++
	}
	return r, remapCollectionLinks(ctx, tx, byID, &r)
}

// catalogLinkHashes is c's catalog link hash over its fingerprint and over its
// recipe hash.
func catalogLinkHashes(c recipeCatalog) linkHashes {
	return linkHashes{old: catalogLinkHash(c.name, c.fingerprint), new: catalogLinkHash(c.name, c.hash)}
}

// catalogLinkHash is a listed catalog's link hash: sha256 hex of its
// length-prefixed name and its recipe identity.
func catalogLinkHash(name, identity string) string {
	return sha256Hex([]byte(strconv.Itoa(len(name)) + ":" + name + identity))
}

// sha256Hex is the hex sha256 of b.
func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// remapCollectionLinks adds the new taken_hash of every linked collection
// to r. A source many copies share is hashed once.
func remapCollectionLinks(ctx context.Context, tx *sql.Tx, byID map[string]recipeCatalog, r *linkRemap) error {
	links, err := loadCollectionLinks(ctx, tx)
	if err != nil {
		return err
	}
	hasher := collectionHasher{byID: byID, hashes: map[string]linkHashes{}}
	for _, link := range links {
		copyHashes, err := hasher.linkHashes(ctx, tx, link.id)
		if err != nil {
			return err
		}
		source, err := hasher.linkHashes(ctx, tx, link.takenFrom)
		if err != nil {
			return err
		}
		var linkCase int
		r.collections[link.id], linkCase = remapTakenHash(link.takenHash, copyHashes, source)
		r.collectionCounts[linkCase]++
	}
	return nil
}

// collectionHasher hashes collections for remapCollectionLinks, each once.
type collectionHasher struct {
	byID   map[string]recipeCatalog
	hashes map[string]linkHashes
}

// linkHashes is collectionLinkHashes for collection id, from the ones worked
// out already when it has been hashed before.
func (h collectionHasher) linkHashes(ctx context.Context, tx *sql.Tx, id string) (linkHashes, error) {
	if hashes, ok := h.hashes[id]; ok {
		return hashes, nil
	}
	hashes, err := collectionLinkHashes(ctx, tx, id, h.byID)
	h.hashes[id] = hashes
	return hashes, err
}

// collectionLink is one linked collection: its id, its source's, and its
// taken_hash.
type collectionLink struct{ id, takenFrom, takenHash string }

// loadCollectionLinks reads every collection with a source and a
// taken_hash, in id order.
func loadCollectionLinks(ctx context.Context, tx *sql.Tx) ([]collectionLink, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT id, taken_from, taken_hash FROM collections
		WHERE taken_from IS NOT NULL AND taken_hash IS NOT NULL ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("reading linked collections: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var links []collectionLink
	for rows.Next() {
		var link collectionLink
		if err := rows.Scan(&link.id, &link.takenFrom, &link.takenHash); err != nil {
			return nil, fmt.Errorf("reading linked collections: %w", err)
		}
		links = append(links, link)
	}
	return links, rows.Err()
}

// linkBundleCollection, linkBundleCatalog, linkBundleFolder and
// linkBundleRef are the bundle form a collection link hash is taken over:
// the collection's content with every row id replaced by a key, keys c1,
// c2, … in the order its folders first reference each catalog, and each
// catalog's params replaced by its recipe identity as a JSON string.
type linkBundleCollection struct {
	Title            string              `json:"title"`
	ViewMode         string              `json:"view_mode"`
	ShowAllTab       bool                `json:"show_all_tab"`
	BackdropImageURL string              `json:"backdrop_image_url"`
	FocusGlowEnabled bool                `json:"focus_glow_enabled"`
	Catalogs         []linkBundleCatalog `json:"catalogs"`
	Folders          []linkBundleFolder  `json:"folders"`
}

type linkBundleCatalog struct {
	Key      string          `json:"key"`
	Name     string          `json:"name"`
	Type     string          `json:"type"`
	Provider string          `json:"provider"`
	Params   json.RawMessage `json:"params"`
}

type linkBundleFolder struct {
	Title           string          `json:"title"`
	TileShape       string          `json:"tile_shape"`
	HideTitle       bool            `json:"hide_title"`
	CoverEmoji      string          `json:"cover_emoji"`
	CoverImageURL   string          `json:"cover_image_url"`
	FocusGIFURL     string          `json:"focus_gif_url"`
	FocusGIFEnabled bool            `json:"focus_gif_enabled"`
	HeroBackdropURL string          `json:"hero_backdrop_url"`
	HeroVideoURL    string          `json:"hero_video_url"`
	TitleLogoURL    string          `json:"title_logo_url"`
	Refs            []linkBundleRef `json:"refs"`
}

type linkBundleRef struct {
	Catalog string `json:"catalog"`
	Genre   string `json:"genre"`
}

// collectionLinkHashes is collection id's link hash over fingerprints and
// over recipe hashes.
func collectionLinkHashes(ctx context.Context, tx *sql.Tx, id string, byID map[string]recipeCatalog) (linkHashes, error) {
	bundle, catalogIDs, err := loadLinkBundle(ctx, tx, id)
	if err != nil {
		return linkHashes{}, err
	}
	old, err := collectionLinkHash(bundle, catalogIDs, byID, func(c recipeCatalog) string { return c.fingerprint })
	if err != nil {
		return linkHashes{}, err
	}
	next, err := collectionLinkHash(bundle, catalogIDs, byID, func(c recipeCatalog) string { return c.hash })
	return linkHashes{old: old, new: next}, err
}

// collectionLinkHash is the sha256 hex of bundle's JSON with its catalogs,
// catalogIDs in key order, filled in from byID and identified by identity.
func collectionLinkHash(bundle linkBundleCollection, catalogIDs []string, byID map[string]recipeCatalog, identity func(recipeCatalog) string) (string, error) {
	bundle.Catalogs = make([]linkBundleCatalog, len(catalogIDs))
	for i, id := range catalogIDs {
		c := byID[id]
		params, err := json.Marshal(identity(c))
		if err != nil {
			return "", err
		}
		bundle.Catalogs[i] = linkBundleCatalog{Key: "c" + strconv.Itoa(i+1), Name: c.name, Type: c.catalogType, Provider: c.provider, Params: params}
	}
	b, err := json.Marshal(bundle)
	if err != nil {
		return "", fmt.Errorf("hashing collection: %w", err)
	}
	return sha256Hex(b), nil
}

// loadLinkBundle reads collection id's bundle form, its Catalogs left for
// collectionLinkHash, and the id of each catalog its refs name, in key order.
// A ref to a catalog that isn't there is dropped.
func loadLinkBundle(ctx context.Context, tx *sql.Tx, id string) (linkBundleCollection, []string, error) {
	var b linkBundleCollection
	err := tx.QueryRowContext(ctx, `
		SELECT title, view_mode, show_all_tab <> 0, backdrop_image_url, focus_glow_enabled <> 0 FROM collections WHERE id = ?
	`, id).Scan(&b.Title, &b.ViewMode, &b.ShowAllTab, &b.BackdropImageURL, &b.FocusGlowEnabled)
	if err != nil {
		return b, nil, fmt.Errorf("reading collection %s: %w", id, err)
	}
	folderIDs, folders, err := loadLinkFolders(ctx, tx, id)
	if err != nil {
		return b, nil, err
	}
	keys := linkKeys{byID: map[string]string{}}
	for i := range folders {
		if folders[i].Refs, err = keys.loadRefs(ctx, tx, folderIDs[i]); err != nil {
			return b, nil, err
		}
	}
	b.Folders = folders
	return b, keys.ids, nil
}

// loadLinkFolders reads collection id's folders in order, each with its id.
func loadLinkFolders(ctx context.Context, tx *sql.Tx, id string) ([]string, []linkBundleFolder, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT id, title, tile_shape, hide_title <> 0, cover_emoji, cover_image_url, focus_gif_url, focus_gif_enabled <> 0,
		       hero_backdrop_url, hero_video_url, title_logo_url
		FROM folders WHERE collection_id = ? ORDER BY sort_order`, id)
	if err != nil {
		return nil, nil, fmt.Errorf("reading collection %s's folders: %w", id, err)
	}
	defer func() { _ = rows.Close() }()
	ids, folders := []string{}, []linkBundleFolder{}
	for rows.Next() {
		var folderID string
		var f linkBundleFolder
		if err := rows.Scan(&folderID, &f.Title, &f.TileShape, &f.HideTitle, &f.CoverEmoji, &f.CoverImageURL, &f.FocusGIFURL, &f.FocusGIFEnabled,
			&f.HeroBackdropURL, &f.HeroVideoURL, &f.TitleLogoURL); err != nil {
			return nil, nil, fmt.Errorf("reading collection %s's folders: %w", id, err)
		}
		ids, folders = append(ids, folderID), append(folders, f)
	}
	return ids, folders, rows.Err()
}

// linkKeys hands out a collection's catalog keys, c1, c2, …, in the order
// its refs first name each catalog.
type linkKeys struct {
	byID map[string]string
	ids  []string
}

// key returns catalogID's key, handing out the next one the first time.
func (k *linkKeys) key(catalogID string) string {
	if key, ok := k.byID[catalogID]; ok {
		return key
	}
	k.ids = append(k.ids, catalogID)
	k.byID[catalogID] = "c" + strconv.Itoa(len(k.ids))
	return k.byID[catalogID]
}

// loadRefs reads folder id's refs in order, each naming its catalog's key.
// A ref whose catalog is gone is dropped.
func (k *linkKeys) loadRefs(ctx context.Context, tx *sql.Tx, folderID string) ([]linkBundleRef, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT fc.catalog_id, fc.genre FROM folder_catalogs fc JOIN catalogs c ON c.id = fc.catalog_id
		WHERE fc.folder_id = ? ORDER BY fc.sort_order`, folderID)
	if err != nil {
		return nil, fmt.Errorf("reading folder %s's refs: %w", folderID, err)
	}
	defer func() { _ = rows.Close() }()
	refs := []linkBundleRef{}
	for rows.Next() {
		var catalogID, genre string
		if err := rows.Scan(&catalogID, &genre); err != nil {
			return nil, fmt.Errorf("reading folder %s's refs: %w", folderID, err)
		}
		refs = append(refs, linkBundleRef{Catalog: k.key(catalogID), Genre: genre})
	}
	return refs, rows.Err()
}
