package vault

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

// queryRower is the common subset of *sql.DB and *sql.Tx the scope-checking
// helpers below need, so callers can run them either inside a transaction
// (UpdateUserCatalog) or straight against the pool (CreateUserCatalog).
type queryRower interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// requireOwnedCollection returns ErrInvalidInput unless collectionID is
// owned by profileID.
func requireOwnedCollection(ctx context.Context, q queryRower, profileID, collectionID uuid.UUID) error {
	var exists int
	err := q.QueryRowContext(ctx, `
		SELECT 1 FROM collections WHERE id = ? AND owner_id = ?
	`, collectionID.String(), profileID.String()).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: collection %s is not owned by this profile", ErrInvalidInput, collectionID)
	}
	if err != nil {
		return fmt.Errorf("checking collection ownership: %w", err)
	}
	return nil
}

// requireNotOnHome returns ErrInvalidInput if catalogID is on profileID's
// home-screen catalog selection (catalogs.home_sort_order IS NOT NULL) — a
// precondition for scoping a catalog to a collection.
func requireNotOnHome(ctx context.Context, q queryRower, profileID, catalogID uuid.UUID) error {
	var homeSortOrder sql.NullInt64
	err := q.QueryRowContext(ctx, `
		SELECT home_sort_order FROM catalogs WHERE id = ? AND owner_id = ?
	`, catalogID.String(), profileID.String()).Scan(&homeSortOrder)
	if errors.Is(err, sql.ErrNoRows) {
		// The caller already loaded this row to get here; a catalog that no
		// longer exists can't be "on home" either.
		return nil
	}
	if err != nil {
		return fmt.Errorf("checking home selection: %w", err)
	}
	if homeSortOrder.Valid {
		return fmt.Errorf("%w: catalog %s is on the home screen and cannot be scoped to a collection", ErrInvalidInput, catalogID)
	}
	return nil
}

// requireFolderRefsWithinCollection returns ErrInvalidInput if any folder
// referencing catalogID belongs to a collection other than collectionID —
// a precondition for scoping a catalog to that collection.
func requireFolderRefsWithinCollection(ctx context.Context, q queryRower, catalogID, collectionID uuid.UUID) error {
	var exists int
	err := q.QueryRowContext(ctx, `
		SELECT 1 FROM folder_catalogs fc
		JOIN folders f ON f.id = fc.folder_id
		WHERE fc.catalog_id = ? AND f.collection_id != ?
		LIMIT 1
	`, catalogID.String(), collectionID.String()).Scan(&exists)
	if err == nil {
		return fmt.Errorf("%w: catalog %s is referenced by a folder outside the target collection", ErrInvalidInput, catalogID)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("checking folder references: %w", err)
	}
	return nil
}

// validateFolderRefs confirms every catalog ID may be referenced by a
// folder in a collection this profile owns, under the closed-graph rule:
// owned by profileID, and either listed (collection_id IS NULL) or already
// scoped to this same collection. A nil collectionID means the collection
// doesn't exist yet (CreateUserCollection has no id to compare against), so
// only listed catalogs qualify. Must run inside the caller's transaction.
func validateFolderRefs(ctx context.Context, tx *sql.Tx, profileID uuid.UUID, collectionID *uuid.UUID, ids []uuid.UUID) error {
	unique := dedupeUUIDs(ids)
	if len(unique) == 0 {
		return nil
	}

	placeholders, args := buildInClause(unique)
	args = append(args, profileID.String())

	query := fmt.Sprintf(`
		SELECT id FROM catalogs
		WHERE id IN (%s) AND owner_id = ?
	`, placeholders)
	if collectionID != nil {
		query += ` AND (collection_id IS NULL OR collection_id = ?)`
		args = append(args, collectionID.String())
	} else {
		query += ` AND collection_id IS NULL`
	}

	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("validating folder catalog refs: %w", err)
	}
	defer rows.Close()

	found := make(map[uuid.UUID]bool, len(unique))
	for rows.Next() {
		var idStr string
		if err := rows.Scan(&idStr); err != nil {
			return fmt.Errorf("scanning catalog id: %w", err)
		}
		id, err := parseUUID(idStr, "catalog id")
		if err != nil {
			return err
		}
		found[id] = true
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterating catalog ids: %w", err)
	}

	for _, id := range unique {
		if !found[id] {
			return fmt.Errorf("%w: catalog %s is not usable in this collection's folders", ErrInvalidInput, id)
		}
	}
	return nil
}

// validateAccess confirms every id in the given table is owned by
// profileID — the closed graph has no cross-owner reference, so there is no
// "or public" branch. label names the table in the singular (e.g.
// "catalog") and is pluralized to get the table name. extraWhere, if
// non-empty, is ANDed onto the ownership check (e.g. " AND collection_id IS
// NULL" to also require "listed"). Must run inside the caller's transaction.
func validateAccess(ctx context.Context, tx *sql.Tx, label, extraWhere string, profileID uuid.UUID, ids []uuid.UUID) error {
	if len(ids) == 0 {
		return nil
	}
	table := label + "s"

	unique := dedupeUUIDs(ids)

	placeholders, args := buildInClause(unique)
	args = append(args, profileID.String())

	rows, err := tx.QueryContext(ctx, fmt.Sprintf(`
		SELECT id FROM %s
		WHERE id IN (%s) AND owner_id = ?%s
	`, table, placeholders, extraWhere), args...)
	if err != nil {
		return fmt.Errorf("validating %s access: %w", label, err)
	}
	defer rows.Close()

	found := make(map[uuid.UUID]bool, len(unique))
	for rows.Next() {
		var idStr string
		if err := rows.Scan(&idStr); err != nil {
			return fmt.Errorf("scanning %s id: %w", label, err)
		}
		id, err := parseUUID(idStr, label+" id")
		if err != nil {
			return err
		}
		found[id] = true
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterating %s ids: %w", label, err)
	}

	for _, id := range unique {
		if !found[id] {
			return fmt.Errorf("%w: %s %s is not accessible to this profile", ErrInvalidInput, label, id)
		}
	}
	return nil
}

// validateCatalogAccess confirms every catalog ID is owned by profileID and
// listed — the rule for a home selection. A scoped catalog is never
// home-selectable (the schema's own CHECK forbids collection_id and
// home_sort_order both being set), so this pre-check has to reject one
// before it ever reaches SaveSelectionsForPush's write, not just before a
// third-party API call: without it, a scoped id would pass validation, push
// successfully to Nuvio, and only then hit 0 rows affected on the local
// write's `AND collection_id IS NULL`, forcing a compensating revert after
// Nuvio already succeeded. Must run inside the caller's transaction.
func validateCatalogAccess(ctx context.Context, tx *sql.Tx, profileID uuid.UUID, catalogIDs []uuid.UUID) error {
	return validateAccess(ctx, tx, "catalog", " AND collection_id IS NULL", profileID, catalogIDs)
}

func validateCollectionAccess(ctx context.Context, tx *sql.Tx, profileID uuid.UUID, collectionIDs []uuid.UUID) error {
	return validateAccess(ctx, tx, "collection", "", profileID, collectionIDs)
}

// takenSourceIDs returns the set of taken_from ids that a row owned by
// profileID in the given table (catalogs or collections) already points at
// — the community list's "taken" flag, one query shared by
// GetCommunityCatalogs and GetCommunityCollections. table is always an
// internal literal, never client input.
func (db *DB) takenSourceIDs(ctx context.Context, table string, profileID uuid.UUID) (map[uuid.UUID]bool, error) {
	rows, err := db.conn.QueryContext(ctx, fmt.Sprintf(`
		SELECT taken_from FROM %s WHERE owner_id = ? AND taken_from IS NOT NULL
	`, table), profileID.String())
	if err != nil {
		return nil, fmt.Errorf("querying taken %s: %w", table, err)
	}
	defer rows.Close()

	taken := map[uuid.UUID]bool{}
	for rows.Next() {
		var idStr string
		if err := rows.Scan(&idStr); err != nil {
			return nil, fmt.Errorf("scanning taken_from id: %w", err)
		}
		id, err := parseUUID(idStr, "taken_from id")
		if err != nil {
			return nil, err
		}
		taken[id] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating taken %s: %w", table, err)
	}
	return taken, nil
}
