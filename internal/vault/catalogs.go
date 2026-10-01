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
)

// queryCatalogs is selectCatalogs against the pool.
func (db *DB) queryCatalogs(ctx context.Context, where string, args ...any) ([]Catalog, error) {
	return selectCatalogs(ctx, db.conn, where, args...)
}

// baseCatalogColumns are a catalog's own columns and its recipe's, the ones
// scanCatalog reads first.
const baseCatalogColumns = `c.id, r.type, c.name, r.provider, r.params, c.owner_id,
	c.collection_id, c.home_sort_order, c.show_in_home, c.recipe_hash, c.sub_key,
	c.created_at, c.updated_at`

// catalogColumns are the columns scanCatalog reads, in its order, from
// catalogRows: the base columns, then the sharing state.
const catalogColumns = baseCatalogColumns + `, ` + sharingColumns

// leanCatalogColumns are the columns scanCatalog reads from
// catalogsWithRecipes alone: the base columns, then NULL for the sharing
// state, which scans as none.
const leanCatalogColumns = baseCatalogColumns + `, ` + noSharingColumns

// sharingColumns are a row's sharing columns, which sharingScan reads: the
// row's own publication (p) and, when it is a subscribed copy, the
// publication its subscription (s) names (sp), with whether that one has
// an update and whether it is withdrawn.
const sharingColumns = `p.id, p.status, p.content_hash,
	s.publication_id, sp.status = 'live' AND s.taken_hash <> sp.content_hash, sp.status = 'withdrawn'`

// noSharingColumns stand in for sharingColumns in a read that doesn't join
// the sharing tables.
const noSharingColumns = `NULL, NULL, NULL, NULL, NULL, NULL`

// catalogsWithRecipes is every catalog joined to its recipe, which holds its
// type, provider and params.
const catalogsWithRecipes = `catalogs c JOIN recipes r ON r.hash = c.recipe_hash`

// catalogRows is catalogsWithRecipes with each catalog's publication and
// subscription joined in: what a read of the owner's own catalogs selects
// catalogColumns from.
const catalogRows = catalogsWithRecipes + `
	LEFT JOIN publications p ON p.catalog_id = c.id
	LEFT JOIN subscriptions s ON s.catalog_id = c.id
	LEFT JOIN publications sp ON sp.id = s.publication_id`

// selectCatalogs runs a SELECT over catalogRows through q with the given
// WHERE clause and args, parsing the result rows, each with its sharing
// state. where is built from this package's own literals — never from
// client input, which reaches the query only as a bound arg — and names
// every column through its table's alias, c for catalogs and r for recipes,
// since the joined tables share column names.
func selectCatalogs(ctx context.Context, q querier, where string, args ...any) ([]Catalog, error) {
	return queryCatalogRows(ctx, q, `SELECT `+catalogColumns+` FROM `+catalogRows+` WHERE `+where, args...)
}

// selectLeanCatalogs is selectCatalogs without the sharing state, which
// saves the joins and the changed-since-publish hash: the read for push, the
// addon and the catalogs of a collection tree, none of which shows it.
func selectLeanCatalogs(ctx context.Context, q querier, where string, args ...any) ([]Catalog, error) {
	return queryCatalogRows(ctx, q, `SELECT `+leanCatalogColumns+` FROM `+catalogsWithRecipes+` WHERE `+where, args...)
}

// queryCatalogRows runs query, built by selectCatalogs or
// selectLeanCatalogs from internal literals, through q and parses its rows.
func queryCatalogRows(ctx context.Context, q querier, query string, args ...any) ([]Catalog, error) {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying catalogs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return parseCatalogs(rows)
}

// GetUserCatalogs returns the listed catalogs owned by profileID — catalogs
// scoped to a collection are excluded; they're reached through the owning
// collection's own response instead.
func (db *DB) GetUserCatalogs(ctx context.Context, profileID uuid.UUID) ([]Catalog, error) {
	return db.queryCatalogs(ctx, "c.owner_id = ? AND c.collection_id IS NULL", profileID.String())
}

// ownCatalog reads catalog id, which must be owned by profileID
// (ErrCatalogNotFound otherwise), through q.
func ownCatalog(ctx context.Context, q querier, profileID, id uuid.UUID) (Catalog, error) {
	catalogs, err := selectCatalogs(ctx, q, "c.id = ? AND c.owner_id = ?", id.String(), profileID.String())
	if err != nil {
		return Catalog{}, err
	}
	if len(catalogs) == 0 {
		return Catalog{}, ErrCatalogNotFound
	}
	return catalogs[0], nil
}

// compareByHomeSortOrder orders catalogs by HomeSortOrder, nil-safe: a nil
// order (not currently selected) sorts after every non-nil one rather than
// panicking, so this doesn't depend on the caller's WHERE clause having
// filtered them out.
func compareByHomeSortOrder(a, b Catalog) int {
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

// CatalogParamsValidator checks a catalog recipe against its provider. This
// package is the leaf of the dependency graph and cannot reach
// internal/provider, so the check is passed in by the caller that can
// (api.validateCatalogParams): a publish runs it over every recipe it
// shares.
type CatalogParamsValidator func(catalogType, catalogProvider, params string) error

// execer is the common subset of *sql.DB and *sql.Tx a single write needs —
// the ExecContext counterpart to scan.go's querier.
type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// insertCatalog writes c as a new catalogs row, storing its recipe first, and
// returns c with its RecipeHash. It is the one catalog INSERT in this
// package: a catalog save, a subscribe or fork of a catalog and a collection
// save's New entries all go through it, each inside a transaction, so a
// recipe it stores never outlives a failed insert. home_sort_order and
// show_in_home are left to their column defaults, since no catalog is born
// on the home screen. An empty SubKey is stored as NULL.
func insertCatalog(ctx context.Context, tx *sql.Tx, c Catalog) (Catalog, error) {
	hash, err := ensureRecipe(ctx, tx, c.Type, c.Provider, c.Params, c.CreatedAt.Format(time.RFC3339))
	if err != nil {
		return Catalog{}, err
	}
	c.RecipeHash = hash
	return c, insertCatalogRow(ctx, tx, c)
}

// insertCatalogRow writes c's catalogs row, pointing at the recipe
// c.RecipeHash names; see insertCatalog.
func insertCatalogRow(ctx context.Context, tx *sql.Tx, c Catalog) error {
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO catalogs (id, name, recipe_hash, owner_id, collection_id, sub_key, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, c.ID.String(), c.Name, c.RecipeHash, c.OwnerID.String(), nullableUUIDString(c.CollectionID),
		nullableString(c.SubKey), c.CreatedAt.Format(time.RFC3339), c.UpdatedAt.Format(time.RFC3339)); err != nil {
		return fmt.Errorf("inserting catalog: %w", err)
	}
	return nil
}

// GetCatalogsByIDs batch-loads catalogs by id, no ownership check — push
// uses this to resolve a folder's catalog_ids (already access-checked at
// selection time) into Type/Provider for building catalogSources. It reads
// no sharing state (selectLeanCatalogs).
func (db *DB) GetCatalogsByIDs(ctx context.Context, ids []uuid.UUID) ([]Catalog, error) {
	return leanCatalogsByIDs(ctx, db.conn, ids)
}

// catalogReader reads the catalogs ids names through q, as a collection
// tree holds them: catalogsByIDs for an owner's read, leanCatalogsByIDs for
// push's.
type catalogReader func(ctx context.Context, q querier, ids []uuid.UUID) ([]Catalog, error)

// catalogsByIDs reads the catalogs ids names through q, each with its
// sharing state.
func catalogsByIDs(ctx context.Context, q querier, ids []uuid.UUID) ([]Catalog, error) {
	return selectCatalogs(ctx, q, "c.id IN (SELECT value FROM json_each(?))", idsJSON(ids))
}

// leanCatalogsByIDs is GetCatalogsByIDs through q.
func leanCatalogsByIDs(ctx context.Context, q querier, ids []uuid.UUID) ([]Catalog, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	return selectLeanCatalogs(ctx, q, "c.id IN (SELECT value FROM json_each(?))", idsJSON(ids))
}

// CreateUserCatalog validates input and inserts a new catalog owned by
// profileID. If input.CollectionID is set, the catalog is scoped to that
// collection, which must be profileID's own and not a subscribed copy
// (ErrInvalidInput).
func (db *DB) CreateUserCatalog(ctx context.Context, profileID uuid.UUID, input CatalogForm) (Catalog, error) {
	input = input.normalized()
	if err := input.Validate(); err != nil {
		return Catalog{}, err
	}
	if input.CollectionID != nil {
		if err := requireOwnedCollection(ctx, db.conn, profileID, *input.CollectionID); err != nil {
			return Catalog{}, err
		}
	}

	now := time.Now().UTC()
	var created Catalog
	err := db.inTx(ctx, func(tx *sql.Tx) error {
		if err := refuseScopeCopy(ctx, tx, profileID, input.CollectionID); err != nil {
			return err
		}
		var err error
		created, err = insertCatalog(ctx, tx, Catalog{
			ID:           uuid.New(),
			Type:         input.Type,
			Name:         input.Name,
			Provider:     input.Provider,
			Params:       input.Params,
			OwnerID:      profileID,
			CollectionID: input.CollectionID,
			CreatedAt:    now,
			UpdatedAt:    now,
		})
		return err
	})
	if err != nil {
		return Catalog{}, err
	}
	return created, nil
}

// refuseScopeCopy refuses a catalog written into collectionID, when there is
// one, if that collection is a subscribed copy of profileID's.
func refuseScopeCopy(ctx context.Context, tx *sql.Tx, profileID uuid.UUID, collectionID *uuid.UUID) error {
	if collectionID == nil {
		return nil
	}
	return refuseSubscribedCopy(ctx, tx, profileID, kindCollection, *collectionID)
}

// UpdateUserCatalog validates input and updates the listed catalog
// identified by catalogID, provided it's owned by profileID. Returns
// ErrCatalogNotFound if no such row exists (including one owned by another
// profile), and ErrInvalidInput for a catalog inside a collection
// (checkCatalogRewrite) and for a subscribed copy (refuseSubscribedCopy).
//
// The catalog's scope is not part of an update: input.CollectionID is not
// read, and a listed catalog stays listed. A scoped catalog is moved to the
// library by its collection's save (ScopedCatalogEdit.MoveToLibrary).
func (db *DB) UpdateUserCatalog(ctx context.Context, profileID uuid.UUID, catalogID uuid.UUID, input CatalogForm) (Catalog, error) {
	input = input.normalized()
	if err := input.Validate(); err != nil {
		return Catalog{}, err
	}
	err := db.inTx(ctx, func(tx *sql.Tx) error {
		return updateCatalogTx(ctx, tx, profileID, catalogID, input)
	})
	if err != nil {
		return Catalog{}, err
	}
	return ownCatalog(ctx, db.conn, profileID, catalogID)
}

// updateCatalogTx is UpdateUserCatalog's write, inside tx.
func updateCatalogTx(ctx context.Context, tx *sql.Tx, profileID, catalogID uuid.UUID, input CatalogForm) error {
	stored, err := loadCatalogForUpdate(ctx, tx, profileID, catalogID)
	if err != nil {
		return err
	}
	if err := refuseSubscribedCopy(ctx, tx, profileID, kindCatalog, catalogID); err != nil {
		return err
	}
	if err := checkCatalogRewrite(stored, input); err != nil {
		return err
	}
	return writeCatalog(ctx, tx, profileID, catalogID, input)
}

// writeCatalog stores input's recipe, unless it is stored already, and
// writes input's name and recipe over catalogID; see UpdateUserCatalog.
func writeCatalog(ctx context.Context, tx *sql.Tx, profileID, catalogID uuid.UUID, input CatalogForm) error {
	nowStr := time.Now().UTC().Format(time.RFC3339)
	hash, err := ensureRecipe(ctx, tx, input.Type, input.Provider, input.Params, nowStr)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE catalogs SET name = ?, recipe_hash = ?, updated_at = ?
		WHERE id = ? AND owner_id = ?
	`, input.Name, hash, nowStr, catalogID.String(), profileID.String()); err != nil {
		return fmt.Errorf("updating catalog: %w", err)
	}
	return nil
}

// storedCatalog is what UpdateUserCatalog reads of the row it rewrites: its
// type and its scope.
type storedCatalog struct {
	catalogType  string
	collectionID sql.NullString
}

// loadCatalogForUpdate reads catalogID's storedCatalog, confirming the row
// exists and is owned by profileID (ErrCatalogNotFound otherwise).
func loadCatalogForUpdate(ctx context.Context, tx *sql.Tx, profileID, catalogID uuid.UUID) (storedCatalog, error) {
	var s storedCatalog
	err := tx.QueryRowContext(ctx, `
		SELECT r.type, c.collection_id FROM `+catalogsWithRecipes+` WHERE c.id = ? AND c.owner_id = ?
	`, catalogID.String(), profileID.String()).Scan(&s.catalogType, &s.collectionID)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return storedCatalog{}, ErrCatalogNotFound
	case err != nil:
		return storedCatalog{}, fmt.Errorf("loading catalog: %w", err)
	}
	return s, nil
}

// checkCatalogRewrite refuses an UpdateUserCatalog the stored row can't take:
//
//   - A catalog inside a collection is written only through that
//     collection's save (CollectionForm.CatalogEdits), so an edit made in the
//     collection editor lands, or is discarded, with the rest of the
//     collection.
//   - A catalog's type is part of the pushed collections blob (each folder
//     source names its catalog's type), so changing it here would alter what
//     Nuvio should have with no save of any collection. The UI locks the
//     field once a catalog exists; this is the server enforcing it.
func checkCatalogRewrite(stored storedCatalog, input CatalogForm) error {
	switch {
	case stored.collectionID.Valid:
		return fmt.Errorf("%w: a catalog inside a collection is edited through the collection's save", ErrInvalidInput)
	case input.Type != stored.catalogType:
		return fmt.Errorf("%w: a catalog's type can't be changed", ErrInvalidInput)
	}
	return nil
}

// DeleteUserCatalog deletes the listed catalog identified by catalogID,
// provided it's owned by profileID. Returns ErrCatalogNotFound if no such row
// exists, and ErrInvalidInput for a catalog inside a collection, which is
// removed by dropping its last folder ref and saving the collection
// (deleteOrphanedScopedCatalogs). A catalog Nuvio may still hold is
// ErrConflict, with the reason as its message (catalogDeleteBlocker): one on
// Home, one a collection on Home uses, and any while a collection on Home
// needs a push. Deleting a subscribed copy removes its subscription, and
// deleting a published catalog withdraws its publication, both by cascade.
// The collections off Home whose folders used the catalog lose it from those
// folders by cascade too.
func (db *DB) DeleteUserCatalog(ctx context.Context, profileID uuid.UUID, catalogID uuid.UUID) error {
	var deleted bool
	err := db.inTx(ctx, func(tx *sql.Tx) (err error) {
		deleted, err = deleteUnheldCatalog(ctx, tx, profileID, catalogID)
		return err
	})
	if err != nil || deleted {
		return err
	}
	return db.catalogNotDeleted(ctx, profileID, catalogID)
}

// deleteUnheldCatalog is deleteListedCatalog, once catalogDeleteBlocker has
// found nothing on Home that holds catalogID.
func deleteUnheldCatalog(ctx context.Context, tx *sql.Tx, profileID, catalogID uuid.UUID) (bool, error) {
	if err := catalogDeleteBlocker(ctx, tx, profileID, catalogID); err != nil {
		return false, err
	}
	return deleteListedCatalog(ctx, tx, profileID, catalogID)
}

// deleteListedCatalog deletes catalogID when it is a listed catalog owned by
// profileID; otherwise it changes nothing and reports false.
func deleteListedCatalog(ctx context.Context, tx *sql.Tx, profileID, catalogID uuid.UUID) (bool, error) {
	result, err := tx.ExecContext(ctx, `
		DELETE FROM catalogs WHERE id = ? AND owner_id = ? AND collection_id IS NULL
	`, catalogID.String(), profileID.String())
	if err != nil {
		return false, fmt.Errorf("deleting catalog: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("checking rows affected: %w", err)
	}
	return rows > 0, nil
}

// catalogNotDeleted explains a DeleteUserCatalog that matched no row: the
// catalog is either not owned by profileID at all (ErrCatalogNotFound) or
// inside a collection (ErrInvalidInput).
func (db *DB) catalogNotDeleted(ctx context.Context, profileID, catalogID uuid.UUID) error {
	var exists int
	err := db.conn.QueryRowContext(ctx, `
		SELECT 1 FROM catalogs WHERE id = ? AND owner_id = ?
	`, catalogID.String(), profileID.String()).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrCatalogNotFound
	}
	if err != nil {
		return fmt.Errorf("checking catalog: %w", err)
	}
	return fmt.Errorf("%w: a catalog inside a collection is removed through the collection's save", ErrInvalidInput)
}

// GetCurrentCatalogSelection returns profileID's active catalog selection —
// every owned catalog with a non-nil home_sort_order — ordered by it.
func (db *DB) GetCurrentCatalogSelection(ctx context.Context, profileID uuid.UUID) ([]SelectedCatalog, error) {
	catalogs, err := db.queryCatalogs(ctx, "c.owner_id = ? AND c.home_sort_order IS NOT NULL", profileID.String())
	if err != nil {
		return nil, err
	}

	slices.SortFunc(catalogs, compareByHomeSortOrder)

	out := make([]SelectedCatalog, len(catalogs))
	for i, c := range catalogs {
		out[i] = SelectedCatalog{Catalog: c, ShowInHome: c.ShowInHome}
	}
	return out, nil
}

// GetPublishedCatalogs returns profileID's derived published catalog set —
// the union of listed catalogs on the home screen and every catalog
// referenced by a folder of a collection on the home screen. This is what
// the addon's manifest lists; unlike
// GetCurrentCatalogSelection (the pre-push validation/selection-editor
// view), it also surfaces folder-only catalogs so nothing a folder tile
// shows on the TV is missing from the manifest. Deduped by id: a catalog
// sitting in more than one folder would otherwise appear once per folder.
// The manifest route reads it on every request and shows no sharing state,
// so it reads none (leanCatalogColumns).
func (db *DB) GetPublishedCatalogs(ctx context.Context, profileID uuid.UUID) ([]SelectedCatalog, error) {
	rows, err := db.conn.QueryContext(ctx, `
		SELECT `+leanCatalogColumns+`,
		       c.show_in_home AS derived_show_in_home, 0 AS rank, c.home_sort_order AS o1, 0 AS o2, 0 AS o3
		FROM `+catalogsWithRecipes+`
		WHERE c.owner_id = ? AND c.home_sort_order IS NOT NULL
		UNION
		SELECT `+leanCatalogColumns+`,
		       0 AS derived_show_in_home, 1 AS rank, col.home_sort_order AS o1, f.sort_order AS o2, fc.sort_order AS o3
		FROM `+catalogsWithRecipes+`
		JOIN folder_catalogs fc ON fc.catalog_id = c.id
		JOIN folders f          ON f.id = fc.folder_id
		JOIN collections col    ON col.id = f.collection_id
		WHERE col.owner_id = ? AND col.home_sort_order IS NOT NULL AND c.home_sort_order IS NULL
		ORDER BY rank, o1, o2, o3
	`, profileID.String(), profileID.String())
	if err != nil {
		return nil, fmt.Errorf("querying published catalogs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := []SelectedCatalog{}
	seen := map[uuid.UUID]bool{}
	for rows.Next() {
		var derivedShowInHome, rank, o2, o3 int
		var o1 sql.NullInt64

		c, err := scanCatalog(rows, &derivedShowInHome, &rank, &o1, &o2, &o3)
		if err != nil {
			return nil, err
		}

		// The two SELECTs' WHERE clauses are mutually exclusive on
		// home_sort_order, so a catalog can't match both; seen instead
		// collapses duplicate folder-derived rows for a catalog sitting in
		// more than one folder.
		if seen[c.ID] {
			continue
		}
		seen[c.ID] = true

		out = append(out, SelectedCatalog{Catalog: c, ShowInHome: derivedShowInHome != 0})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating published catalog rows: %w", err)
	}

	return out, nil
}

// ServedCatalog is what an addon catalog route serves: a catalog's params,
// and the Nuvio account that owns its profile with that account's sealed TMDB
// key, nil when the account has saved none.
type ServedCatalog struct {
	Params    string
	Account   string
	SealedKey []byte
}

// ServedCatalog returns the catalog an addon catalog route names: catalogID
// owned by the profile whose token it is, with catalogType and
// catalogProvider, and on the TV — in GetPublishedCatalogs' set, checked for
// this one catalog: it has its own home row (Discover-only included), or a
// folder of one of the profile's on-home collections references it. It comes
// with its owner's account and sealed key. It is the route's one lookup and
// its access check: a catalog off the TV, another profile's catalog, an
// unknown token and a type or provider the catalog doesn't have are all
// ErrCatalogNotFound.
func (db *DB) ServedCatalog(ctx context.Context, token string, catalogID uuid.UUID, catalogType, catalogProvider string) (ServedCatalog, error) {
	var served ServedCatalog
	err := db.conn.QueryRowContext(ctx, `
		SELECT r.params, p.nuvio_user_id, a.tmdb_key_ciphertext
		FROM profiles p
		JOIN catalogs c ON c.owner_id = p.id
		JOIN recipes r  ON r.hash = c.recipe_hash
		LEFT JOIN accounts a ON a.nuvio_user_id = p.nuvio_user_id
		WHERE p.token = ? AND c.id = ? AND r.type = ? AND r.provider = ?
		  AND (c.home_sort_order IS NOT NULL OR EXISTS (
		      SELECT 1 FROM folder_catalogs fc
		      JOIN folders f       ON f.id = fc.folder_id
		      JOIN collections col ON col.id = f.collection_id
		      WHERE fc.catalog_id = c.id AND col.owner_id = p.id AND col.home_sort_order IS NOT NULL))
	`, token, catalogID.String(), catalogType, catalogProvider).Scan(&served.Params, &served.Account, &served.SealedKey)
	if errors.Is(err, sql.ErrNoRows) {
		return ServedCatalog{}, ErrCatalogNotFound
	}
	if err != nil {
		return ServedCatalog{}, fmt.Errorf("resolving served catalog: %w", err)
	}
	return served, nil
}

// saveCatalogSelectionTx resets this profile's catalog selection to exactly
// input, in order: every owned catalog's home_sort_order is cleared, then
// each incoming id is set in turn. A 0-rows-affected update (an id that
// isn't owned, or is scoped rather than listed) is ErrInvalidInput naming
// the id — this is the access check, not a separate query, since the same
// WHERE clause both selects and validates.
//
// Takes a caller-supplied transaction rather than opening its own: its only
// caller is SaveSelectionsForPush (push.go), which needs both selection
// writes to commit or roll back together.
func saveCatalogSelectionTx(ctx context.Context, tx *sql.Tx, profileID uuid.UUID, input CatalogSelectionForm) error {
	if _, err := tx.ExecContext(ctx, `
		UPDATE catalogs SET home_sort_order = NULL WHERE owner_id = ?
	`, profileID.String()); err != nil {
		return fmt.Errorf("clearing catalog home selection: %w", err)
	}

	for i, sc := range input.Catalogs {
		result, err := tx.ExecContext(ctx, `
			UPDATE catalogs
			SET home_sort_order = ?, show_in_home = ?
			WHERE id = ? AND owner_id = ? AND collection_id IS NULL
		`, i, sc.ShowInHome, sc.CatalogID.String(), profileID.String())
		if err != nil {
			return fmt.Errorf("saving catalog selection: %w", err)
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("checking rows affected: %w", err)
		}
		if rows == 0 {
			return fmt.Errorf("%w: catalog %s is not accessible to this profile", ErrInvalidInput, sc.CatalogID)
		}
	}

	return nil
}
