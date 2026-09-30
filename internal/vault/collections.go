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

// queryCollections is selectCollections against the pool.
func (db *DB) queryCollections(ctx context.Context, where string, args ...any) ([]Collection, error) {
	return selectCollections(ctx, db.conn, where, args...)
}

// collectionRows is every collection, as col, with its publication and
// subscription joined in, which sharingColumns reads.
const collectionRows = `collections col
	LEFT JOIN publications p ON p.collection_id = col.id
	LEFT JOIN subscriptions s ON s.collection_id = col.id
	LEFT JOIN publications sp ON sp.id = s.publication_id`

// collectionColumns are a collection's own columns, the ones scanCollection
// reads before the sharing state.
const collectionColumns = `col.id, col.title, col.owner_id, col.pin_to_top, col.view_mode, col.show_all_tab, col.backdrop_image_url,
	col.focus_glow_enabled, col.home_sort_order, col.pushed_hash, col.created_at, col.updated_at`

// selectCollections runs a SELECT over collectionRows through q with the
// given WHERE clause and args, parsing the result rows, each with its
// sharing state. where is built from this package's own literals — never
// from client input, which reaches the query only as a bound arg — and names
// every column through col, since the joined tables share column names.
func selectCollections(ctx context.Context, q querier, where string, args ...any) ([]Collection, error) {
	return queryCollectionRows(ctx, q, `SELECT `+collectionColumns+`, `+sharingColumns+` FROM `+collectionRows+` WHERE `+where, args...)
}

// selectLeanCollections is selectCollections without the sharing state,
// which saves the joins and the changed-since-publish hash: push's read.
func selectLeanCollections(ctx context.Context, q querier, where string, args ...any) ([]Collection, error) {
	return queryCollectionRows(ctx, q, `SELECT `+collectionColumns+`, `+noSharingColumns+` FROM collections col WHERE `+where, args...)
}

// queryCollectionRows runs query, built by selectCollections or
// selectLeanCollections from internal literals, through q and parses its
// rows.
func queryCollectionRows(ctx context.Context, q querier, query string, args ...any) ([]Collection, error) {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying collections: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return parseCollections(rows)
}

// GetUserCollections returns the collections owned by profileID, each with
// its folders assembled.
func (db *DB) GetUserCollections(ctx context.Context, profileID uuid.UUID) ([]CollectionWithFolders, error) {
	collections, err := db.queryCollections(ctx, "col.owner_id = ?", profileID.String())
	if err != nil {
		return nil, err
	}
	return assembleCollectionTree(ctx, db.conn, collections, catalogsByIDs)
}

// GetCollectionsByIDs batch-loads collections (with folders) by id, no
// ownership check and no ordering guarantee — push uses this to resolve the
// pending collection selection straight from the request body rather than
// reading the persisted selection back, so it needs the same shape
// GetCurrentCollectionSelection returns, keyed by an explicit id list
// instead of a home_sort_order filter. Mirrors internal/vault/catalogs.go's
// GetCatalogsByIDs, and like it reads no sharing state.
func (db *DB) GetCollectionsByIDs(ctx context.Context, ids []uuid.UUID) ([]CollectionWithFolders, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	collections, err := selectLeanCollections(ctx, db.conn, "col.id IN (SELECT value FROM json_each(?))", idsJSON(ids))
	if err != nil {
		return nil, err
	}
	return assembleCollectionTree(ctx, db.conn, collections, leanCatalogsByIDs)
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
	input = input.normalized()
	if err := input.validateCreate(); err != nil {
		return CollectionWithFolders{}, err
	}

	var created CollectionWithFolders
	var allCatalogIDs []uuid.UUID
	err := db.inTx(ctx, func(tx *sql.Tx) error {
		var err error
		created, allCatalogIDs, err = createCollectionTx(ctx, tx, profileID, input)
		return err
	})
	if err != nil {
		return CollectionWithFolders{}, err
	}

	catalogs, err := db.GetCatalogsByIDs(ctx, dedupeUUIDs(allCatalogIDs))
	if err != nil {
		return CollectionWithFolders{}, err
	}

	created.Catalogs = jsonwire.OrEmpty(catalogs)
	return created, nil
}

// createCollectionTx inserts form as a new collection owned by profileID —
// the row, its folders and their catalog refs, with any New entry created as
// a catalog scoped to it — after checking every existing catalog it
// references is one of profileID's listed catalogs.
//
// It is the one collection create: a save, an import, a subscribe, a fork
// and a Duplicate all go through it. The caller validates form first, runs
// this inside tx and commits. Returns the new collection with its folders,
// Catalogs unset, and the id of every catalog those folders reference,
// repeats included.
func createCollectionTx(ctx context.Context, tx *sql.Tx, profileID uuid.UUID, form CollectionForm) (CollectionWithFolders, []uuid.UUID, error) {
	if err := validateFolderRefs(ctx, tx, profileID, nil, existingFolderRefIDs(form.Folders)); err != nil {
		return CollectionWithFolders{}, nil, err
	}

	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339)
	c := Collection{
		ID:               uuid.New(),
		Title:            form.Title,
		OwnerID:          profileID,
		ViewMode:         form.ViewMode,
		ShowAllTab:       form.ShowAllTab,
		BackdropImageURL: form.BackdropImageURL,
		FocusGlowEnabled: form.FocusGlowEnabled,
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO collections (id, title, owner_id, view_mode, show_all_tab, backdrop_image_url,
		                          focus_glow_enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, c.ID.String(), c.Title, c.OwnerID.String(), c.ViewMode, c.ShowAllTab, c.BackdropImageURL,
		c.FocusGlowEnabled, nowStr, nowStr); err != nil {
		return CollectionWithFolders{}, nil, fmt.Errorf("inserting collection: %w", err)
	}

	folders, allCatalogIDs, err := insertFolders(ctx, tx, profileID, c.ID, form.Folders)
	if err != nil {
		return CollectionWithFolders{}, nil, err
	}
	return CollectionWithFolders{Collection: c, Folders: folders}, allCatalogIDs, nil
}

// updateCollectionRow writes the collection's own columns. pin_to_top is not
// one of them: only push writes it. Returns ErrCollectionNotFound if the row
// is no longer there.
func updateCollectionRow(ctx context.Context, tx *sql.Tx, profileID, collectionID uuid.UUID, input CollectionForm, nowStr string) error {
	result, err := tx.ExecContext(ctx, `
		UPDATE collections
		SET title = ?, view_mode = ?, show_all_tab = ?, backdrop_image_url = ?, focus_glow_enabled = ?,
		    updated_at = ?
		WHERE id = ? AND owner_id = ?
	`, input.Title, input.ViewMode, input.ShowAllTab, input.BackdropImageURL, input.FocusGlowEnabled, nowStr,
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

// updateCollectionRowAndEdits writes the collection's own columns and then
// every catalog edit input carries: the rows a save rewrites ahead of its
// folder set. The edits land before that rewrite
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

// applyCatalogEdit writes one ScopedCatalogEdit: the new name and recipe
// and, for MoveToLibrary, a cleared collection_id, which makes the catalog
// listed. An edit that changes nothing is skipped, so the row's updated_at
// stays put.
func applyCatalogEdit(ctx context.Context, tx *sql.Tx, profileID, collectionID uuid.UUID, e ScopedCatalogEdit, nowStr string) error {
	stored, err := loadEditedCatalog(ctx, tx, profileID, collectionID, e)
	if err != nil {
		return err
	}
	if e.changesNothing(stored) {
		return nil
	}
	return writeCatalogEdit(ctx, tx, profileID, e, nowStr)
}

// writeCatalogEdit stores e's recipe, unless it is stored already, and
// writes e over its catalog; see applyCatalogEdit. A catalog moved to the
// library loses its sub_key, which only a row inside a subscribed collection
// carries.
func writeCatalogEdit(ctx context.Context, tx *sql.Tx, profileID uuid.UUID, e ScopedCatalogEdit, nowStr string) error {
	hash, err := ensureRecipe(ctx, tx, e.Type, e.Provider, e.Params, nowStr)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE catalogs
		SET name = ?, recipe_hash = ?, updated_at = ?,
		    collection_id = CASE WHEN ?4 THEN NULL ELSE collection_id END,
		    sub_key = CASE WHEN ?4 THEN NULL ELSE sub_key END
		WHERE id = ?5 AND owner_id = ?6
	`, e.Name, hash, nowStr, e.MoveToLibrary, e.ID.String(), profileID.String()); err != nil {
		return fmt.Errorf("updating edited catalog: %w", err)
	}
	return nil
}

// storedRecipe is the part of a catalog row a ScopedCatalogEdit rewrites,
// read back to tell a real edit from one that changes nothing.
type storedRecipe struct {
	name, recipeHash string
}

// loadEditedCatalog reads the row e rewrites, confirming it is owned by
// profileID, scoped to collectionID, and of the type and provider e names.
// Anything else is ErrInvalidInput: the id and both fields came from the
// client.
func loadEditedCatalog(ctx context.Context, tx *sql.Tx, profileID, collectionID uuid.UUID, e ScopedCatalogEdit) (storedRecipe, error) {
	var s storedRecipe
	var catalogType, catalogProvider string
	err := tx.QueryRowContext(ctx, `
		SELECT r.type, r.provider, c.name, c.recipe_hash FROM `+catalogsWithRecipes+`
		WHERE c.id = ? AND c.owner_id = ? AND c.collection_id = ?
	`, e.ID.String(), profileID.String(), collectionID.String()).Scan(&catalogType, &catalogProvider, &s.name, &s.recipeHash)
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

// changesNothing reports whether e would leave stored exactly as it is: the
// same name and the same recipe, and no move to the library.
func (e ScopedCatalogEdit) changesNothing(stored storedRecipe) bool {
	return !e.MoveToLibrary && e.Name == stored.name && e.recipeHash() == stored.recipeHash
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

// updateCollectionTx writes form over collectionID inside tx: the row's own
// columns, form's catalog edits, and then its folder set, which drops the folders form leaves out and deletes any scoped catalog
// no folder references any more. The caller validates form first and
// commits; the row's own update answers ErrCollectionNotFound, before
// anything else is written, when collectionID isn't profileID's. An editor
// save and a subscription's Update both write through it.
func updateCollectionTx(ctx context.Context, tx *sql.Tx, profileID, collectionID uuid.UUID, form CollectionForm, nowStr string) error {
	if err := updateCollectionRowAndEdits(ctx, tx, profileID, collectionID, form, nowStr); err != nil {
		return err
	}
	if err := deleteRemovedFolders(ctx, tx, collectionID, form.Folders); err != nil {
		return err
	}
	return writeFolderSet(ctx, tx, profileID, collectionID, form.Folders)
}

// UpdateUserCollection validates input and replaces the collection
// identified by collectionID (title, settings, and its full folder set),
// provided it's owned by profileID (ErrCollectionNotFound otherwise).
// Saving a subscribed copy detaches it (saveCollectionTx).
func (db *DB) UpdateUserCollection(ctx context.Context, profileID uuid.UUID, collectionID uuid.UUID, input CollectionForm) (CollectionWithFolders, error) {
	input = input.normalized()
	if err := input.Validate(); err != nil {
		return CollectionWithFolders{}, err
	}
	err := db.inTx(ctx, func(tx *sql.Tx) error {
		return saveCollectionTx(ctx, tx, profileID, collectionID, input)
	})
	if err != nil {
		return CollectionWithFolders{}, err
	}
	return ownCollection(ctx, db.conn, profileID, collectionID)
}

// saveCollectionTx is UpdateUserCollection's write, inside tx: input is
// written over profileID's collection through the update core, whose first
// write answers ErrCollectionNotFound for a collection profileID doesn't
// own, and then the collection is detached: a save makes a subscribed copy
// the profile's own. Update shares the core but never detaches, so a copy
// it writes keeps its subscription and keys.
func saveCollectionTx(ctx context.Context, tx *sql.Tx, profileID, collectionID uuid.UUID, input CollectionForm) error {
	if err := updateCollectionTx(ctx, tx, profileID, collectionID, input, time.Now().UTC().Format(time.RFC3339)); err != nil {
		return err
	}
	return detachTx(ctx, tx, kindCollection, collectionID)
}

// DeleteUserCollection deletes the collection identified by collectionID,
// provided it's owned by profileID (ErrCollectionNotFound otherwise) and off
// Home: one on Home, which Nuvio holds, is ErrConflict with its reason as
// its message (collectionDeleteBlocker).
func (db *DB) DeleteUserCollection(ctx context.Context, profileID uuid.UUID, collectionID uuid.UUID) error {
	return db.inTx(ctx, func(tx *sql.Tx) error {
		return deleteOffHomeCollection(ctx, tx, profileID, collectionID)
	})
}

// deleteOffHomeCollection deletes collectionID inside tx, once
// collectionDeleteBlocker has found it profileID's and off Home.
func deleteOffHomeCollection(ctx context.Context, tx *sql.Tx, profileID, collectionID uuid.UUID) error {
	if err := collectionDeleteBlocker(ctx, tx, profileID, collectionID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM collections WHERE id = ? AND owner_id = ?
	`, collectionID.String(), profileID.String()); err != nil {
		return fmt.Errorf("deleting collection: %w", err)
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
	collections, err := db.queryCollections(ctx, "col.owner_id = ? AND col.home_sort_order IS NOT NULL", profileID.String())
	if err != nil {
		return nil, err
	}

	slices.SortFunc(collections, compareCollectionsByHomeSortOrder)

	return assembleCollectionTree(ctx, db.conn, collections, catalogsByIDs)
}

// saveCollectionSelectionTx resets this profile's collection selection to
// exactly input, in order, writes each included collection's pin_to_top from
// its entry, and stamps pushed_hash on every collection it includes with the
// hash of what pushCollections sent for it — this is push's only caller, so
// every collection reaching this point is, by definition, being pushed right
// now. Every owned collection's home_sort_order is cleared first, then each
// incoming entry is set in turn; a collection left out keeps its pin_to_top,
// which a later push putting it back on Home starts from. A 0-rows-affected
// update (an id that isn't owned) is ErrInvalidInput naming the id, the same
// pattern as saveCatalogSelectionTx.
//
// hashes is keyed by collection id, built by pushCollections over the exact
// bytes it sent — never the row as it stands now, which is what keeps a Save
// landing between push's read and this write from being mistaken for pushed:
// the row then hashes to something else, so it still needs a push. A missing
// entry (the collection vanished between push's read and this write) leaves
// pushed_hash untouched rather than guessing.
//
// Takes a caller-supplied transaction — see saveCatalogSelectionTx in
// catalogs.go for why, and for why there is no exported single-selection
// wrapper.
func saveCollectionSelectionTx(ctx context.Context, tx *sql.Tx, profileID uuid.UUID, input CollectionSelectionForm, hashes map[uuid.UUID]string) error {
	if _, err := tx.ExecContext(ctx, `
		UPDATE collections SET home_sort_order = NULL WHERE owner_id = ?
	`, profileID.String()); err != nil {
		return fmt.Errorf("clearing collection home selection: %w", err)
	}

	for i, entry := range input.Collections {
		id := entry.CollectionID
		var pushedHash any
		if h, ok := hashes[id]; ok {
			pushedHash = h
		}
		result, err := tx.ExecContext(ctx, `
			UPDATE collections
			SET home_sort_order = ?, pin_to_top = ?, pushed_hash = COALESCE(?, pushed_hash)
			WHERE id = ? AND owner_id = ?
		`, i, entry.PinToTop, pushedHash, id.String(), profileID.String())
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
