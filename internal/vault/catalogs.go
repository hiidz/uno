package vault

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// queryCatalogs is selectCatalogs against the pool.
func (db *DB) queryCatalogs(ctx context.Context, where string, args ...any) ([]Catalog, error) {
	return selectCatalogs(ctx, db.conn, where, args...)
}

// baseCatalogColumns are a catalog's own columns, the ones scanCatalog reads
// first.
const baseCatalogColumns = `c.id, c.type, c.name, c.provider, c.params, c.owner_id,
	c.collection_id, c.home_sort_order, c.show_in_home, c.sub_key,
	c.created_at, c.updated_at, c.revision`

// catalogColumns are the columns scanCatalog reads, in its order, from
// catalogRows: the base columns, then the sharing state.
const catalogColumns = baseCatalogColumns + `, ` + sharingColumns

// leanCatalogColumns are the columns scanCatalog reads from catalogs alone:
// the base columns, then NULL for the sharing state, which scans as none.
const leanCatalogColumns = baseCatalogColumns + `, ` + noSharingColumns

// sharingColumns are a row's sharing columns, which sharingScan reads: the
// row's own publication (p) and, when it is a subscribed copy, the
// publication its subscription (s) names (sp), with whether that one has
// an update.
const sharingColumns = `p.id, p.content_hash,
	s.publication_id, s.subscribed_hash <> sp.content_hash`

// noSharingColumns stand in for sharingColumns in a read that doesn't join
// the sharing tables.
const noSharingColumns = `NULL, NULL, NULL, NULL`

// catalogRows is every catalog, as c, with its publication and subscription
// joined in: what a read of the owner's own catalogs selects catalogColumns
// from.
const catalogRows = `catalogs c
	LEFT JOIN publications p ON p.catalog_id = c.id
	LEFT JOIN subscriptions s ON s.catalog_id = c.id
	LEFT JOIN publications sp ON sp.id = s.publication_id`

// selectCatalogs runs a SELECT over catalogRows through q with the given
// WHERE clause and args, parsing the result rows, each with its sharing
// state. where is built from this package's own literals — never from
// client input, which reaches the query only as a bound arg — and names
// every column through c, since the joined tables share column names.
func selectCatalogs(ctx context.Context, q dbtx, where string, args ...any) ([]Catalog, error) {
	return queryCatalogRows(ctx, q, `SELECT `+catalogColumns+` FROM `+catalogRows+` WHERE `+where, args...)
}

// selectLeanCatalogs is selectCatalogs without the sharing state, which
// saves the joins and the changed-since-publish hash: the read for push, the
// addon and the catalogs of a collection tree, none of which shows it.
func selectLeanCatalogs(ctx context.Context, q dbtx, where string, args ...any) ([]Catalog, error) {
	return queryCatalogRows(ctx, q, `SELECT `+leanCatalogColumns+` FROM catalogs c WHERE `+where, args...)
}

// queryCatalogRows runs query, built by selectCatalogs or
// selectLeanCatalogs from internal literals, through q and parses its rows.
func queryCatalogRows(ctx context.Context, q dbtx, query string, args ...any) ([]Catalog, error) {
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
	return userCatalogs(ctx, db.conn, profileID)
}

// userCatalogs is GetUserCatalogs through q.
func userCatalogs(ctx context.Context, q dbtx, profileID uuid.UUID) ([]Catalog, error) {
	return selectCatalogs(ctx, q, "c.owner_id = ? AND c.collection_id IS NULL", profileID.String())
}

// ownCatalog reads catalog id, which must be owned by profileID
// (ErrCatalogNotFound otherwise), through q.
func ownCatalog(ctx context.Context, q dbtx, profileID, id uuid.UUID) (Catalog, error) {
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

// insertCatalog writes c as a new catalogs row and returns c with its
// RecipeHash and the revision a new row starts at, 1. It is the one catalog
// INSERT in this package: a catalog save, a subscribe or duplicate of a
// catalog and a collection save's New entries all go through it.
// home_sort_order and show_in_home are left to their column defaults, since
// no catalog is born on the home screen. An empty SubKey is stored as NULL. A
// listed catalog (no CollectionID) is refused once its owner holds
// maxListedCatalogsPerProfile.
func insertCatalog(ctx context.Context, tx *sql.Tx, c Catalog) (Catalog, error) {
	if err := checkCatalogAdd(ctx, tx, c); err != nil {
		return Catalog{}, err
	}
	return insertCatalogRow(ctx, tx, c)
}

// insertCatalogRow is insertCatalog's INSERT.
func insertCatalogRow(ctx context.Context, tx *sql.Tx, c Catalog) (Catalog, error) {
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO catalogs (id, name, type, provider, params, owner_id, collection_id, sub_key, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, c.ID.String(), c.Name, c.Type, c.Provider, c.Params, c.OwnerID.String(), nullableUUIDString(c.CollectionID),
		nullableString(c.SubKey), utcTimestamp(c.CreatedAt), utcTimestamp(c.UpdatedAt)); err != nil {
		return Catalog{}, fmt.Errorf("inserting catalog: %w", err)
	}
	c.RecipeHash = RecipeHash(c.Type, c.Provider, c.Params)
	c.Revision = 1
	return c, nil
}

// GetCatalogsByIDs batch-loads catalogs by id, no ownership check. It reads
// no sharing state (selectLeanCatalogs).
func (db *DB) GetCatalogsByIDs(ctx context.Context, ids []uuid.UUID) ([]Catalog, error) {
	return leanCatalogsByIDs(ctx, db.conn, ids)
}

// catalogReader reads the catalogs ids names through q, as a collection
// tree holds them: catalogsByIDs for an owner's read, leanCatalogsByIDs for
// push's.
type catalogReader func(ctx context.Context, q dbtx, ids []uuid.UUID) ([]Catalog, error)

// catalogsByIDs reads the catalogs ids names through q, each with its
// sharing state.
func catalogsByIDs(ctx context.Context, q dbtx, ids []uuid.UUID) ([]Catalog, error) {
	return selectCatalogs(ctx, q, "c.id IN (SELECT value FROM json_each(?))", idsJSON(ids))
}

// leanCatalogsByIDs is GetCatalogsByIDs through q.
func leanCatalogsByIDs(ctx context.Context, q dbtx, ids []uuid.UUID) ([]Catalog, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	return selectLeanCatalogs(ctx, q, "c.id IN (SELECT value FROM json_each(?))", idsJSON(ids))
}

// CreateUserCatalog validates input and inserts it as a new listed catalog
// owned by profileID. A catalog scoped to a collection is made only by that
// collection's save, from a folder's New entry (resolveFolderCatalogRef).
func (db *DB) CreateUserCatalog(ctx context.Context, profileID uuid.UUID, input CatalogForm) (Catalog, error) {
	input = input.normalized()
	if err := input.Validate(); err != nil {
		return Catalog{}, err
	}

	now := time.Now().UTC()
	var created Catalog
	err := db.inTx(ctx, func(tx *sql.Tx) error {
		var err error
		created, err = insertCatalog(ctx, tx, Catalog{
			ID:        uuid.New(),
			Type:      input.Type,
			Name:      input.Name,
			Provider:  input.Provider,
			Params:    input.Params,
			OwnerID:   profileID,
			CreatedAt: now,
			UpdatedAt: now,
		})
		return err
	})
	if err != nil {
		return Catalog{}, err
	}
	return created, nil
}

// DuplicateCatalog copies the listed catalog catalogID, which profileID owns,
// as a new listed catalog of theirs: the same type, provider and params, its
// name given a " (copy)" suffix (copyName), unpublished and subscribed to
// nothing even when the source is a subscribed copy. The stored recipe is
// checked by the form validators only, with no TMDB call, so a recipe TMDB
// has since outgrown doesn't block a copy. Returns ErrCatalogNotFound for a
// catalog that isn't one of profileID's listed ones.
func (db *DB) DuplicateCatalog(ctx context.Context, profileID, catalogID uuid.UUID) (Catalog, error) {
	source, err := ownCatalog(ctx, db.conn, profileID, catalogID)
	if err != nil {
		return Catalog{}, err
	}
	if source.CollectionID != nil {
		return Catalog{}, ErrCatalogNotFound
	}
	return db.CreateUserCatalog(ctx, profileID, CatalogForm{
		Type: source.Type, Name: copyName(source.Name), Provider: source.Provider, Params: source.Params,
	})
}

// UpdateUserCatalog validates input and updates the listed catalog
// identified by catalogID, provided it's owned by profileID and still at
// revision, the one the editor's form was built from. Returns
// ErrCatalogNotFound if no such row exists (including one owned by another
// profile), ErrInvalidInput for a catalog inside a collection
// (checkCatalogRewrite) and for a subscribed copy (refuseSubscribedCopy), and
// ErrStale for a catalog at another revision.
func (db *DB) UpdateUserCatalog(ctx context.Context, profileID uuid.UUID, catalogID uuid.UUID, revision int64, input CatalogForm) (Catalog, error) {
	input = input.normalized()
	if err := input.Validate(); err != nil {
		return Catalog{}, err
	}
	err := db.inTx(ctx, func(tx *sql.Tx) error {
		return updateCatalogTx(ctx, tx, profileID, catalogID, revision, input)
	})
	if err != nil {
		return Catalog{}, err
	}
	return ownCatalog(ctx, db.conn, profileID, catalogID)
}

// updateCatalogTx is UpdateUserCatalog's write, inside tx.
func updateCatalogTx(ctx context.Context, tx *sql.Tx, profileID, catalogID uuid.UUID, revision int64, input CatalogForm) error {
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
	return writeCatalogAt(ctx, tx, profileID, catalogID, revision, input)
}

// writeCatalog writes input's name and params over catalogID, raising its
// revision; see UpdateUserCatalog. Its type and provider never change
// (checkCatalogRewrite).
func writeCatalog(ctx context.Context, tx *sql.Tx, profileID, catalogID uuid.UUID, input CatalogForm) error {
	nowStr := utcTimestamp(time.Now())
	if _, err := tx.ExecContext(ctx, `
		UPDATE catalogs SET name = ?, params = ?, updated_at = ?, revision = revision + 1
		WHERE id = ? AND owner_id = ?
	`, input.Name, input.Params, nowStr, catalogID.String(), profileID.String()); err != nil {
		return fmt.Errorf("updating catalog: %w", err)
	}
	return nil
}

// catalogKind is what never changes about a catalog once it exists: its type
// and provider. A folder source in the pushed collections blob names both, so
// a change would alter what Nuvio should have with no save of any collection.
type catalogKind struct {
	catalogType, provider string
}

// sameKind reports whether input is of the kind stored is: the one rule every
// write that rewrites a catalog's recipe holds to, whether it refuses a change
// (an edit, an Update of a catalog copy) or replaces the catalog (an Update of
// a collection copy).
func sameKind(stored, input catalogKind) bool {
	return stored == input
}

// storedCatalog is what UpdateUserCatalog reads of the row it rewrites: its
// kind and its scope.
type storedCatalog struct {
	kind         catalogKind
	collectionID sql.NullString
}

// loadCatalogForUpdate reads catalogID's storedCatalog, confirming the row
// exists and is owned by profileID (ErrCatalogNotFound otherwise).
func loadCatalogForUpdate(ctx context.Context, tx *sql.Tx, profileID, catalogID uuid.UUID) (storedCatalog, error) {
	var s storedCatalog
	err := tx.QueryRowContext(ctx, `
		SELECT type, provider, collection_id FROM catalogs WHERE id = ? AND owner_id = ?
	`, catalogID.String(), profileID.String()).Scan(&s.kind.catalogType, &s.kind.provider, &s.collectionID)
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
//   - A catalog's kind is part of the pushed collections blob (each folder
//     source names its catalog's type and provider), so changing it here would
//     alter what Nuvio should have with no save of any collection. The UI
//     locks the type once a catalog exists; this is the server enforcing it.
func checkCatalogRewrite(stored storedCatalog, input CatalogForm) error {
	switch {
	case stored.collectionID.Valid:
		return fmt.Errorf("%w: a catalog inside a collection is edited through the collection's save", ErrInvalidInput)
	case !sameKind(stored.kind, catalogKind{input.Type, input.Provider}):
		return fmt.Errorf("%w: a catalog's type and provider can't be changed", ErrInvalidInput)
	}
	return nil
}

// DeleteUserCatalog deletes the listed catalog identified by catalogID,
// provided it's owned by profileID. Returns ErrCatalogNotFound if no such row
// exists, and ErrInvalidInput for a catalog inside a collection, which is
// removed by dropping its last folder ref and saving the collection
// (deleteOrphanedScopedCatalogs). It is allowed at any time, Home or not:
// Nuvio keeps what the last push put there, served from the push record,
// until the next push drops it. Deleting a subscribed copy removes its
// subscription, and deleting a published catalog unpublishes it, both by
// cascade; its subscribers keep their copies as their own
// (publications_release_subscribers). Every collection whose folders used the
// catalog loses it from those folders by cascade too.
func (db *DB) DeleteUserCatalog(ctx context.Context, profileID uuid.UUID, catalogID uuid.UUID) error {
	var deleted bool
	err := db.inTx(ctx, func(tx *sql.Tx) (err error) {
		deleted, err = deleteListedCatalog(ctx, tx, profileID, catalogID)
		return err
	})
	if err != nil || deleted {
		return err
	}
	return db.catalogNotDeleted(ctx, profileID, catalogID)
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

// saveCatalogSelectionTx resets this profile's catalog selection to exactly
// entries: every owned catalog's home_sort_order is cleared, then each
// entry's is set to its Position. A 0-rows-affected update (an id that isn't
// owned, or is scoped rather than listed) is ErrInvalidInput naming the id,
// backing up BuildPushRecord, which refuses such an id before Nuvio is
// contacted.
//
// Takes a caller-supplied transaction rather than opening its own: its only
// caller is SavePush (pushrecord.go), which needs both selection writes and
// the push record to commit or roll back together.
func saveCatalogSelectionTx(ctx context.Context, tx *sql.Tx, profileID uuid.UUID, entries []SelectedCatalogInput) error {
	if _, err := tx.ExecContext(ctx, `
		UPDATE catalogs SET home_sort_order = NULL WHERE owner_id = ?
	`, profileID.String()); err != nil {
		return fmt.Errorf("clearing catalog home selection: %w", err)
	}

	for _, sc := range entries {
		result, err := tx.ExecContext(ctx, `
			UPDATE catalogs
			SET home_sort_order = ?, show_in_home = ?
			WHERE id = ? AND owner_id = ? AND collection_id IS NULL
		`, sc.Position, sc.ShowInHome, sc.CatalogID.String(), profileID.String())
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
