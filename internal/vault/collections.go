package vault

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/hiidz/uno/internal/jsonwire"
)

// queryCollections runs a SELECT over collections with the given WHERE
// clause and args, parsing the result rows. where is built from this
// package's own literals and buildInClause placeholders — never from client
// input, which reaches the query only as a bound arg.
//
//nolint:gosec // G202: see above — the concatenated where is an internal literal.
func (db *DB) queryCollections(ctx context.Context, where string, args ...any) ([]Collection, error) {
	rows, err := db.conn.QueryContext(ctx, `
		SELECT id, title, owner_id, is_public, pin_to_top, view_mode, show_all_tab, backdrop_image_url,
		       focus_glow_enabled, home_sort_order, version, pushed_version, taken_from, created_at, updated_at
		FROM collections
		WHERE `+where, args...)
	if err != nil {
		return nil, fmt.Errorf("querying collections: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return parseCollections(rows)
}

// GetUserCollections returns the collections owned by profileID, each with
// its folders assembled.
func (db *DB) GetUserCollections(ctx context.Context, profileID uuid.UUID) ([]CollectionWithFolders, error) {
	collections, err := db.queryCollections(ctx, "owner_id = ?", profileID.String())
	if err != nil {
		return nil, err
	}
	return db.assembleCollectionTree(ctx, collections)
}

// GetCommunityCollections returns the community collection list for
// profileID: every collection marked public and owned by someone else, each
// with its folders assembled, sorted by title, then created_at, then id so
// equal titles don't swap between requests (no fingerprint collapse —
// that's a catalog-only concept), flagged with whether profileID has
// already taken a copy.
func (db *DB) GetCommunityCollections(ctx context.Context, profileID uuid.UUID) ([]CommunityCollection, error) {
	collections, err := db.queryCollections(ctx, "is_public = TRUE AND owner_id != ?", profileID.String())
	if err != nil {
		return nil, err
	}
	slices.SortFunc(collections, func(a, b Collection) int {
		if c := cmp.Compare(a.Title, b.Title); c != 0 {
			return c
		}
		return compareCreatedThenID(a.CreatedAt, b.CreatedAt, a.ID, b.ID)
	})

	trees, err := db.assembleCollectionTree(ctx, collections)
	if err != nil {
		return nil, err
	}

	taken, err := db.takenSourceIDs(ctx, "collections", profileID)
	if err != nil {
		return nil, err
	}

	out := make([]CommunityCollection, len(trees))
	for i, c := range trees {
		out[i] = CommunityCollection{CollectionWithFolders: c, Taken: taken[c.ID]}
	}
	return out, nil
}

// GetCollectionsByIDs batch-loads collections (with folders) by id, no
// ownership check and no ordering guarantee — push uses this to resolve the
// pending collection selection straight from the request body rather than
// reading the persisted selection back, so it needs the same shape
// GetCurrentCollectionSelection returns, keyed by an explicit id list
// instead of a home_sort_order filter. Mirrors internal/vault/catalogs.go's
// GetCatalogsByIDs.
func (db *DB) GetCollectionsByIDs(ctx context.Context, ids []uuid.UUID) ([]CollectionWithFolders, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	placeholders, args := buildInClause(ids)
	collections, err := db.queryCollections(ctx, fmt.Sprintf("id IN (%s)", placeholders), args...)
	if err != nil {
		return nil, err
	}
	return db.assembleCollectionTree(ctx, collections)
}

// GetOwnedCollectionIDs lists just the IDs of collections this profile owns,
// skipping the folder-tree assembly GetUserCollections does — push's merge
// only needs identity, to know which entries in a pulled Nuvio blob are
// Uno's own and safe to replace or drop.
func (db *DB) GetOwnedCollectionIDs(ctx context.Context, profileID uuid.UUID) ([]uuid.UUID, error) {
	return queryUUIDs(ctx, db.conn, "owned collection id",
		`SELECT id FROM collections WHERE owner_id = ?`, profileID.String())
}

// CreateUserCollection validates input, checks profileID has access to
// every referenced catalog, and inserts the collection with its folders in
// one transaction.
func (db *DB) CreateUserCollection(ctx context.Context, profileID uuid.UUID, input CollectionForm) (CollectionWithFolders, error) {
	if err := input.validateCreate(); err != nil {
		return CollectionWithFolders{}, err
	}

	tx, err := db.conn.BeginTx(ctx, nil)
	if err != nil {
		return CollectionWithFolders{}, fmt.Errorf("starting transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // no-op once Commit succeeds

	var existingCatalogIDs []uuid.UUID
	for _, fd := range input.Folders {
		existingCatalogIDs = append(existingCatalogIDs, existingRefIDs(fd.Catalogs)...)
	}
	if err := validateFolderRefs(ctx, tx, profileID, nil, existingCatalogIDs); err != nil {
		return CollectionWithFolders{}, err
	}

	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339)
	c := Collection{
		ID:               uuid.New(),
		Title:            input.Title,
		OwnerID:          profileID,
		IsPublic:         input.IsPublic,
		PinToTop:         input.PinToTop,
		ViewMode:         input.ViewMode,
		ShowAllTab:       input.ShowAllTab,
		BackdropImageURL: input.BackdropImageURL,
		FocusGlowEnabled: input.FocusGlowEnabled,
		Version:          1,
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO collections (id, title, owner_id, is_public, pin_to_top, view_mode, show_all_tab, backdrop_image_url,
		                          focus_glow_enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, c.ID.String(), c.Title, c.OwnerID.String(), c.IsPublic, c.PinToTop, c.ViewMode, c.ShowAllTab, c.BackdropImageURL,
		c.FocusGlowEnabled, nowStr, nowStr)
	if err != nil {
		return CollectionWithFolders{}, fmt.Errorf("inserting collection: %w", err)
	}

	folders := make([]FolderWithCatalogs, len(input.Folders))
	var allCatalogIDs []uuid.UUID
	created := map[string]uuid.UUID{}
	for i, fd := range input.Folders {
		f, err := insertFolder(ctx, tx, c.ID, i, fd)
		if err != nil {
			return CollectionWithFolders{}, err
		}
		refs, err := writeFolderCatalogRefs(ctx, tx, profileID, c.ID, f.ID, fd.Catalogs, created)
		if err != nil {
			return CollectionWithFolders{}, err
		}

		folders[i] = FolderWithCatalogs{Folder: f, Refs: refs}
		allCatalogIDs = append(allCatalogIDs, folders[i].CatalogIDs()...)
	}

	if err := tx.Commit(); err != nil {
		return CollectionWithFolders{}, fmt.Errorf("committing transaction: %w", err)
	}

	catalogs, err := db.GetCatalogsByIDs(ctx, dedupeUUIDs(allCatalogIDs))
	if err != nil {
		return CollectionWithFolders{}, err
	}

	return CollectionWithFolders{Collection: c, Folders: folders, Catalogs: jsonwire.OrEmpty(catalogs)}, nil
}

// collectionUpdateState is the pre-update state UpdateUserCollection reads
// before writing: the fields its response carries that the UPDATE itself
// doesn't return. createdAtStr stays unparsed here — UpdateUserCollection
// parses it after the commit.
type collectionUpdateState struct {
	createdAtStr  string
	homeSortOrder sql.NullInt64
	version       int
	pushedVersion sql.NullInt64
}

// loadCollectionForUpdate reads collectionID's pre-update state, confirming
// the row exists and is owned by profileID. Returns ErrCollectionNotFound
// otherwise.
func loadCollectionForUpdate(ctx context.Context, tx *sql.Tx, profileID, collectionID uuid.UUID) (collectionUpdateState, error) {
	var s collectionUpdateState
	err := tx.QueryRowContext(ctx, `
		SELECT created_at, home_sort_order, version, pushed_version FROM collections WHERE id = ? AND owner_id = ?
	`, collectionID.String(), profileID.String()).Scan(&s.createdAtStr, &s.homeSortOrder, &s.version, &s.pushedVersion)
	if errors.Is(err, sql.ErrNoRows) {
		return collectionUpdateState{}, ErrCollectionNotFound
	}
	if err != nil {
		return collectionUpdateState{}, fmt.Errorf("loading collection: %w", err)
	}
	return s, nil
}

// updateCollectionRow writes the collection's own columns and bumps its
// version. Returns ErrCollectionNotFound if the row is no longer there.
func updateCollectionRow(ctx context.Context, tx *sql.Tx, profileID, collectionID uuid.UUID, input CollectionForm, nowStr string) error {
	result, err := tx.ExecContext(ctx, `
		UPDATE collections
		SET title = ?, is_public = ?, pin_to_top = ?, view_mode = ?, show_all_tab = ?, backdrop_image_url = ?, focus_glow_enabled = ?,
		    updated_at = ?, version = version + 1
		WHERE id = ? AND owner_id = ?
	`, input.Title, input.IsPublic, input.PinToTop, input.ViewMode, input.ShowAllTab, input.BackdropImageURL, input.FocusGlowEnabled, nowStr,
		collectionID.String(), profileID.String())
	if err != nil {
		return fmt.Errorf("updating collection: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return ErrCollectionNotFound
	}
	return nil
}

// updateCollectionRowAndEdits writes the collection's own columns (bumping
// its version) and then every catalog edit input carries: the rows a save
// rewrites ahead of its folder set. The edits land before that rewrite
// because its orphan cleanup deletes any scoped catalog the save no longer
// references, and an edit must still find its catalog inside this collection.
func updateCollectionRowAndEdits(ctx context.Context, tx *sql.Tx, profileID, collectionID uuid.UUID, input CollectionForm, nowStr string) error {
	if err := updateCollectionRow(ctx, tx, profileID, collectionID, input, nowStr); err != nil {
		return err
	}
	return applyCatalogEdits(ctx, tx, profileID, collectionID, input.CatalogEdits, nowStr)
}

// applyCatalogEdits writes each of edits in turn; see applyCatalogEdit.
func applyCatalogEdits(ctx context.Context, tx *sql.Tx, profileID, collectionID uuid.UUID, edits []ScopedCatalogEdit, nowStr string) error {
	for _, e := range edits {
		if err := applyCatalogEdit(ctx, tx, profileID, collectionID, e, nowStr); err != nil {
			return err
		}
	}
	return nil
}

// applyCatalogEdit writes one ScopedCatalogEdit: the new name, params and
// fingerprint and, for MoveToLibrary, a cleared collection_id, which makes the
// catalog listed. An edit that changes nothing is skipped, so the row's
// updated_at stays put.
func applyCatalogEdit(ctx context.Context, tx *sql.Tx, profileID, collectionID uuid.UUID, e ScopedCatalogEdit, nowStr string) error {
	stored, err := loadEditedCatalog(ctx, tx, profileID, collectionID, e)
	if err != nil {
		return err
	}
	if e.changesNothing(stored) {
		return nil
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE catalogs
		SET name = ?, params = ?, fingerprint = ?, updated_at = ?,
		    collection_id = CASE WHEN ? THEN NULL ELSE collection_id END
		WHERE id = ? AND owner_id = ?
	`, e.Name, e.Params, e.Fingerprint, nowStr, e.MoveToLibrary, e.ID.String(), profileID.String()); err != nil {
		return fmt.Errorf("updating edited catalog: %w", err)
	}
	return nil
}

// storedRecipe is the part of a catalog row a ScopedCatalogEdit rewrites,
// read back to tell a real edit from one that changes nothing.
type storedRecipe struct {
	name, params, fingerprint string
}

// loadEditedCatalog reads the row e rewrites, confirming it is owned by
// profileID, scoped to collectionID, and of the type and provider e names.
// Anything else is ErrInvalidInput: the id and both fields came from the
// client.
func loadEditedCatalog(ctx context.Context, tx *sql.Tx, profileID, collectionID uuid.UUID, e ScopedCatalogEdit) (storedRecipe, error) {
	var s storedRecipe
	var catalogType, catalogProvider string
	err := tx.QueryRowContext(ctx, `
		SELECT type, provider, name, params, fingerprint FROM catalogs
		WHERE id = ? AND owner_id = ? AND collection_id = ?
	`, e.ID.String(), profileID.String(), collectionID.String()).Scan(&catalogType, &catalogProvider, &s.name, &s.params, &s.fingerprint)
	if errors.Is(err, sql.ErrNoRows) {
		return storedRecipe{}, fmt.Errorf("%w: catalog %s is not inside this collection", ErrInvalidInput, e.ID)
	}
	if err != nil {
		return storedRecipe{}, fmt.Errorf("loading edited catalog: %w", err)
	}
	if catalogType != e.Type || catalogProvider != e.Provider {
		return storedRecipe{}, fmt.Errorf("%w: catalog %s: a catalog's type and provider can't be changed", ErrInvalidInput, e.ID)
	}
	return s, nil
}

// changesNothing reports whether e would leave stored exactly as it is. The
// fingerprint counts too: an edit that only brings a stale fingerprint up to
// date still writes.
func (e ScopedCatalogEdit) changesNothing(stored storedRecipe) bool {
	return !e.MoveToLibrary && e.Name == stored.name && e.Params == stored.params && e.Fingerprint == stored.fingerprint
}

// deleteOrphanedScopedCatalogs removes every catalog scoped to collectionID
// that no folder of it references any more. Runs in the same transaction as
// the folder rewrite — this is what catches a scoped catalog created via
// POST .../catalogs and never referenced (editor abandoned before Save).
func deleteOrphanedScopedCatalogs(ctx context.Context, tx *sql.Tx, collectionID uuid.UUID) error {
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM catalogs
		WHERE collection_id = ?
		  AND id NOT IN (SELECT fc.catalog_id FROM folder_catalogs fc
		                 JOIN folders f ON f.id = fc.folder_id
		                 WHERE f.collection_id = ?)
	`, collectionID.String(), collectionID.String()); err != nil {
		return fmt.Errorf("deleting orphaned scoped catalogs: %w", err)
	}
	return nil
}

// UpdateUserCollection validates input and replaces the collection
// identified by collectionID (title, settings, and its full folder set),
// provided it's owned by profileID.
func (db *DB) UpdateUserCollection(ctx context.Context, profileID uuid.UUID, collectionID uuid.UUID, input CollectionForm) (CollectionWithFolders, error) {
	if err := input.Validate(); err != nil {
		return CollectionWithFolders{}, err
	}

	tx, err := db.conn.BeginTx(ctx, nil)
	if err != nil {
		return CollectionWithFolders{}, fmt.Errorf("starting transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // no-op once Commit succeeds

	before, err := loadCollectionForUpdate(ctx, tx, profileID, collectionID)
	if err != nil {
		return CollectionWithFolders{}, err
	}

	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339)
	if err := updateCollectionRowAndEdits(ctx, tx, profileID, collectionID, input, nowStr); err != nil {
		return CollectionWithFolders{}, err
	}

	removed, err := removedFolderIDs(ctx, tx, collectionID, input.Folders)
	if err != nil {
		return CollectionWithFolders{}, err
	}
	if err := deleteFolders(ctx, tx, removed); err != nil {
		return CollectionWithFolders{}, err
	}

	// One validation query for every catalog ID across every folder, up
	// front — New entries are skipped, they aren't rows yet (existingRefIDs).
	var existingCatalogIDs []uuid.UUID
	for _, fd := range input.Folders {
		existingCatalogIDs = append(existingCatalogIDs, existingRefIDs(fd.Catalogs)...)
	}
	if err := validateFolderRefs(ctx, tx, profileID, &collectionID, existingCatalogIDs); err != nil {
		return CollectionWithFolders{}, err
	}

	folders, allCatalogIDs, err := upsertFolders(ctx, tx, profileID, collectionID, input.Folders)
	if err != nil {
		return CollectionWithFolders{}, err
	}

	if err := deleteOrphanedScopedCatalogs(ctx, tx, collectionID); err != nil {
		return CollectionWithFolders{}, err
	}

	if err := tx.Commit(); err != nil {
		return CollectionWithFolders{}, fmt.Errorf("committing transaction: %w", err)
	}

	createdAt, err := parseTimestamp(before.createdAtStr, "collection created_at")
	if err != nil {
		return CollectionWithFolders{}, err
	}

	c := Collection{
		ID: collectionID, Title: input.Title, OwnerID: profileID,
		IsPublic: input.IsPublic,
		PinToTop: input.PinToTop, ViewMode: input.ViewMode,
		ShowAllTab: input.ShowAllTab, BackdropImageURL: input.BackdropImageURL,
		FocusGlowEnabled: input.FocusGlowEnabled,
		// HomeSortOrder/PushedVersion are unchanged by this update, read back
		// before it for an accurate response. Version is the freshly bumped
		// value updateCollectionRow just wrote.
		HomeSortOrder: nullableInt(before.homeSortOrder), Version: before.version + 1, PushedVersion: nullableInt(before.pushedVersion),
		CreatedAt: createdAt, UpdatedAt: now,
	}

	catalogs, err := db.GetCatalogsByIDs(ctx, dedupeUUIDs(allCatalogIDs))
	if err != nil {
		return CollectionWithFolders{}, err
	}

	return CollectionWithFolders{Collection: c, Folders: folders, Catalogs: jsonwire.OrEmpty(catalogs)}, nil
}

// DeleteUserCollection deletes the collection identified by collectionID,
// provided it's owned by profileID. Returns ErrCollectionNotFound
// otherwise.
func (db *DB) DeleteUserCollection(ctx context.Context, profileID uuid.UUID, collectionID uuid.UUID) error {
	result, err := db.conn.ExecContext(ctx, `
		DELETE FROM collections WHERE id = ? AND owner_id = ?
	`, collectionID.String(), profileID.String())
	if err != nil {
		return fmt.Errorf("deleting collection: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if rows == 0 {
		return ErrCollectionNotFound
	}

	return nil
}

// compareCollectionsByHomeSortOrder orders collections by HomeSortOrder,
// nil-safe: a nil order (not currently selected) sorts after every non-nil
// one rather than panicking, so this doesn't depend on the caller's WHERE
// clause having filtered them out.
func compareCollectionsByHomeSortOrder(a, b Collection) int {
	switch {
	case a.HomeSortOrder == nil && b.HomeSortOrder == nil:
		return 0
	case a.HomeSortOrder == nil:
		return 1
	case b.HomeSortOrder == nil:
		return -1
	default:
		return cmp.Compare(*a.HomeSortOrder, *b.HomeSortOrder)
	}
}

// GetCurrentCollectionSelection returns profileID's active collection
// selection — every owned collection with a non-nil home_sort_order —
// ordered by it, each with its folders assembled.
func (db *DB) GetCurrentCollectionSelection(ctx context.Context, profileID uuid.UUID) ([]CollectionWithFolders, error) {
	collections, err := db.queryCollections(ctx, "owner_id = ? AND home_sort_order IS NOT NULL", profileID.String())
	if err != nil {
		return nil, err
	}

	slices.SortFunc(collections, compareCollectionsByHomeSortOrder)

	return db.assembleCollectionTree(ctx, collections)
}

// saveCollectionSelectionTx resets this profile's collection selection to
// exactly input, in order, and stamps pushed_version on every collection it
// includes with the version pushCollections read for it — this is push's
// only caller, so every collection reaching this point is, by definition,
// being pushed right now. Every owned collection's home_sort_order is
// cleared first, then each incoming id is set in turn; a 0-rows-affected
// update (an id that isn't owned) is ErrInvalidInput naming the id, the same
// pattern as saveCatalogSelectionTx.
//
// versions is keyed by collection id, built by pushCollections from the same
// read that fed Nuvio — never the row's current version and never a clock,
// which is what keeps a Save landing between that read and this write from
// being mistaken for pushed. A missing entry (the collection vanished
// between push's read and this write) leaves pushed_version untouched rather
// than guessing.
//
// Takes a caller-supplied transaction — see saveCatalogSelectionTx in
// catalogs.go for why, and for why there is no exported single-selection
// wrapper.
func saveCollectionSelectionTx(ctx context.Context, tx *sql.Tx, profileID uuid.UUID, input CollectionSelectionForm, versions map[uuid.UUID]int) error {
	if _, err := tx.ExecContext(ctx, `
		UPDATE collections SET home_sort_order = NULL WHERE owner_id = ?
	`, profileID.String()); err != nil {
		return fmt.Errorf("clearing collection home selection: %w", err)
	}

	for i, id := range input.CollectionIDs {
		var pushedVersion any
		if v, ok := versions[id]; ok {
			pushedVersion = v
		}
		result, err := tx.ExecContext(ctx, `
			UPDATE collections
			SET home_sort_order = ?, pushed_version = COALESCE(?, pushed_version)
			WHERE id = ? AND owner_id = ?
		`, i, pushedVersion, id.String(), profileID.String())
		if err != nil {
			return fmt.Errorf("saving collection selection: %w", err)
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("checking rows affected: %w", err)
		}
		if rows == 0 {
			return fmt.Errorf("%w: collection %s is not accessible to this profile", ErrInvalidInput, id)
		}
	}

	return nil
}
