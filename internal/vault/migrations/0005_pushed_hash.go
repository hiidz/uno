package migrations

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// pushedHashes replaces the version counters Home compared to tell that a
// collection needs a push with a hash of what push last sent:
// collections.pushed_hash, the sha256 hex of the exact JSON push sent to
// Nuvio for the collection.
//
//  1. A collection whose version equals its pushed_version is as push last
//     sent it, so it gets the hash of its push JSON as it stands, built by
//     this migration's own copy of the push payload. Every other collection
//     stays NULL, which reads as needing a push: it changed since its last
//     push, or was never pushed.
//  2. version and pushed_version are dropped.
//  3. folder_catalogs gains an index by catalog, which the addon's catalog
//     route and a catalog delete look a catalog's folder refs up by.
//
// The payload is frozen here: the same field order, keys, omitted empties
// and escaping as the push payload the vault builds today, so a collection
// backfilled here reads as pushed.
func pushedHashes(ctx context.Context, tx *sql.Tx) ([]string, error) {
	if _, err := tx.ExecContext(ctx, `ALTER TABLE collections ADD COLUMN pushed_hash TEXT`); err != nil {
		return nil, fmt.Errorf("adding collections.pushed_hash: %w", err)
	}
	backfilled, err := backfillPushedHashes(ctx, tx)
	if err != nil {
		return nil, err
	}
	var pending int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM collections WHERE pushed_hash IS NULL`).Scan(&pending); err != nil {
		return nil, fmt.Errorf("counting pending collections: %w", err)
	}
	if _, err := tx.ExecContext(ctx, pushedHashDDL); err != nil {
		return nil, fmt.Errorf("dropping collections' version columns: %w", err)
	}
	return []string{fmt.Sprintf("backfilled %d push hashes; left %d collections pending (changed since their last push, or never pushed)", backfilled, pending)}, nil
}

// pushedHashDDL drops the version counters, which no index, trigger, CHECK
// or foreign key names, and indexes folder refs by catalog.
const pushedHashDDL = `
ALTER TABLE collections DROP COLUMN version;
ALTER TABLE collections DROP COLUMN pushed_version;
CREATE INDEX folder_catalogs_by_catalog ON folder_catalogs (catalog_id);
`

// sentAddonID is Uno's addon id, which every pushed source carries.
const sentAddonID = "hiidz.uno.catalog"

// sentSource is one folder catalogSources entry as push sends it.
type sentSource struct {
	AddonID   string `json:"addonId"`
	Type      string `json:"type"`
	CatalogID string `json:"catalogId"`
	Genre     string `json:"genre,omitempty"`
}

// sentFolder is one folder as push sends it.
type sentFolder struct {
	ID              string       `json:"id"`
	Title           string       `json:"title"`
	CoverImageURL   string       `json:"coverImageUrl,omitempty"`
	CoverEmoji      string       `json:"coverEmoji,omitempty"`
	FocusGIFURL     string       `json:"focusGifUrl,omitempty"`
	FocusGIFEnabled bool         `json:"focusGifEnabled"`
	HeroBackdropURL string       `json:"heroBackdropUrl,omitempty"`
	HeroVideoURL    string       `json:"heroVideoUrl,omitempty"`
	TitleLogoURL    string       `json:"titleLogoUrl,omitempty"`
	TileShape       string       `json:"tileShape"`
	HideTitle       bool         `json:"hideTitle"`
	CatalogSources  []sentSource `json:"catalogSources"`
}

// sentCollection is one collection as push sends it.
type sentCollection struct {
	ID               string       `json:"id"`
	Title            string       `json:"title"`
	BackdropImageURL string       `json:"backdropImageUrl,omitempty"`
	PinToTop         bool         `json:"pinToTop"`
	FocusGlowEnabled bool         `json:"focusGlowEnabled"`
	ViewMode         string       `json:"viewMode"`
	ShowAllTab       bool         `json:"showAllTab"`
	Folders          []sentFolder `json:"folders"`
}

// backfillPushedHashes stores the push hash of every collection whose
// version equals its pushed_version, and returns how many it stored.
func backfillPushedHashes(ctx context.Context, tx *sql.Tx) (int, error) {
	ids, err := inStepCollectionIDs(ctx, tx)
	if err != nil {
		return 0, err
	}
	for _, id := range ids {
		hash, err := sentHash(ctx, tx, id)
		if err != nil {
			return 0, err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE collections SET pushed_hash = ? WHERE id = ?`, hash, id); err != nil {
			return 0, fmt.Errorf("setting collection %s's pushed_hash: %w", id, err)
		}
	}
	return len(ids), nil
}

// inStepCollectionIDs is the id of every collection push has sent and that
// hasn't changed since, in id order.
func inStepCollectionIDs(ctx context.Context, tx *sql.Tx) ([]string, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT id FROM collections WHERE pushed_version IS NOT NULL AND version = pushed_version ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("reading pushed collections: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("reading pushed collections: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// sentHash is the sha256 hex of collection id's push JSON.
func sentHash(ctx context.Context, tx *sql.Tx, id string) (string, error) {
	c, err := loadSentCollection(ctx, tx, id)
	if err != nil {
		return "", err
	}
	b, err := json.Marshal(c)
	if err != nil {
		return "", fmt.Errorf("writing collection %s's push JSON: %w", id, err)
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

// loadSentCollection reads collection id as push sends it: its own fields,
// then its folders in order, each with its sources.
func loadSentCollection(ctx context.Context, tx *sql.Tx, id string) (sentCollection, error) {
	var c sentCollection
	var pinToTop, focusGlowEnabled, showAllTab int
	if err := tx.QueryRowContext(ctx, `
		SELECT id, title, backdrop_image_url, pin_to_top, focus_glow_enabled, view_mode, show_all_tab
		FROM collections WHERE id = ?
	`, id).Scan(&c.ID, &c.Title, &c.BackdropImageURL, &pinToTop, &focusGlowEnabled, &c.ViewMode, &showAllTab); err != nil {
		return sentCollection{}, fmt.Errorf("reading collection %s: %w", id, err)
	}
	c.PinToTop, c.FocusGlowEnabled, c.ShowAllTab = pinToTop != 0, focusGlowEnabled != 0, showAllTab != 0
	folders, err := loadSentFolders(ctx, tx, id)
	if err != nil {
		return sentCollection{}, err
	}
	for i := range folders {
		if folders[i].CatalogSources, err = loadSentSources(ctx, tx, folders[i].ID); err != nil {
			return sentCollection{}, err
		}
	}
	c.Folders = folders
	return c, nil
}

// loadSentFolders reads collection id's folders by sort_order, each with no
// sources yet.
func loadSentFolders(ctx context.Context, tx *sql.Tx, id string) ([]sentFolder, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT id, title, cover_image_url, cover_emoji, focus_gif_url, focus_gif_enabled,
		       hero_backdrop_url, hero_video_url, title_logo_url, tile_shape, hide_title
		FROM folders WHERE collection_id = ? ORDER BY sort_order
	`, id)
	if err != nil {
		return nil, fmt.Errorf("reading collection %s's folders: %w", id, err)
	}
	defer func() { _ = rows.Close() }()
	folders := []sentFolder{}
	for rows.Next() {
		var f sentFolder
		var focusGIFEnabled, hideTitle int
		if err := rows.Scan(&f.ID, &f.Title, &f.CoverImageURL, &f.CoverEmoji, &f.FocusGIFURL, &focusGIFEnabled,
			&f.HeroBackdropURL, &f.HeroVideoURL, &f.TitleLogoURL, &f.TileShape, &hideTitle); err != nil {
			return nil, fmt.Errorf("reading collection %s's folders: %w", id, err)
		}
		f.FocusGIFEnabled, f.HideTitle = focusGIFEnabled != 0, hideTitle != 0
		folders = append(folders, f)
	}
	return folders, rows.Err()
}

// loadSentSources reads folder id's catalog refs by sort_order as push sends
// them: each catalog's type and provider from its recipe, and its manifest
// id, the provider, "-" and the catalog's id.
func loadSentSources(ctx context.Context, tx *sql.Tx, id string) ([]sentSource, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT r.type, r.provider, fc.catalog_id, fc.genre
		FROM folder_catalogs fc
		JOIN catalogs c ON c.id = fc.catalog_id
		JOIN recipes r  ON r.hash = c.recipe_hash
		WHERE fc.folder_id = ?
		ORDER BY fc.sort_order
	`, id)
	if err != nil {
		return nil, fmt.Errorf("reading folder %s's catalogs: %w", id, err)
	}
	defer func() { _ = rows.Close() }()
	sources := []sentSource{}
	for rows.Next() {
		var s sentSource
		var provider, catalogID string
		if err := rows.Scan(&s.Type, &provider, &catalogID, &s.Genre); err != nil {
			return nil, fmt.Errorf("reading folder %s's catalogs: %w", id, err)
		}
		s.AddonID, s.CatalogID = sentAddonID, provider+"-"+catalogID
		sources = append(sources, s)
	}
	return sources, rows.Err()
}
