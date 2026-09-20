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

// ownedIDsQuery describes one closed-graph ownership check for
// requireOwnedIDs: which table the ids must exist in, how to name them in
// errors, and an optional fragment ANDed onto the ownership test. The closed
// graph has no cross-owner reference, so there is never an "or public"
// branch.
//
// table and extraWhere are always internal literals, never client input.
type ownedIDsQuery struct {
	table string
	// label names one row in the singular, e.g. "catalog".
	label string
	// extraWhere is ANDed onto "owner_id = ?" (e.g. " AND collection_id IS
	// NULL" to also require "listed"); extraArgs fill its placeholders and
	// are appended after profileID.
	extraWhere string
	extraArgs  []any
	// rejection completes the message for an id that didn't come back, after
	// "<label> <id> ".
	rejection string
}

// requireOwnedIDs confirms every id in ids satisfies q, returning an
// ErrInvalidInput naming the first that doesn't. Must run inside the
// caller's transaction.
func requireOwnedIDs(ctx context.Context, tx *sql.Tx, profileID uuid.UUID, ids []uuid.UUID, q ownedIDsQuery) error {
	unique := dedupeUUIDs(ids)
	if len(unique) == 0 {
		return nil
	}

	placeholders, args := buildInClause(unique)
	args = append(args, profileID.String())
	args = append(args, q.extraArgs...)

	got, err := queryUUIDs(ctx, tx, q.label+" id", fmt.Sprintf(`
		SELECT id FROM %s
		WHERE id IN (%s) AND owner_id = ?%s
	`, q.table, placeholders, q.extraWhere), args...)
	if err != nil {
		return err
	}

	found := make(map[uuid.UUID]bool, len(got))
	for _, id := range got {
		found[id] = true
	}

	for _, id := range unique {
		if !found[id] {
			return fmt.Errorf("%w: %s %s %s", ErrInvalidInput, q.label, id, q.rejection)
		}
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
	q := ownedIDsQuery{
		table:      "catalogs",
		label:      "catalog",
		extraWhere: ` AND collection_id IS NULL`,
		rejection:  "is not usable in this collection's folders",
	}
	if collectionID != nil {
		q.extraWhere = ` AND (collection_id IS NULL OR collection_id = ?)`
		q.extraArgs = []any{collectionID.String()}
	}
	return requireOwnedIDs(ctx, tx, profileID, ids, q)
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
	return requireOwnedIDs(ctx, tx, profileID, catalogIDs, ownedIDsQuery{
		table:      "catalogs",
		label:      "catalog",
		extraWhere: " AND collection_id IS NULL",
		rejection:  "is not accessible to this profile",
	})
}

func validateCollectionAccess(ctx context.Context, tx *sql.Tx, profileID uuid.UUID, collectionIDs []uuid.UUID) error {
	return requireOwnedIDs(ctx, tx, profileID, collectionIDs, ownedIDsQuery{
		table:     "collections",
		label:     "collection",
		rejection: "is not accessible to this profile",
	})
}

// takenSourceIDs returns the set of taken_from ids that a row owned by
// profileID in the given table (catalogs or collections) already points at
// — the community list's "taken" flag, one query shared by
// GetCommunityCatalogs and GetCommunityCollections. table is always an
// internal literal, never client input.
func (db *DB) takenSourceIDs(ctx context.Context, table string, profileID uuid.UUID) (map[uuid.UUID]bool, error) {
	ids, err := queryUUIDs(ctx, db.conn, "taken_from id", fmt.Sprintf(`
		SELECT taken_from FROM %s WHERE owner_id = ? AND taken_from IS NOT NULL
	`, table), profileID.String())
	if err != nil {
		return nil, err
	}

	taken := make(map[uuid.UUID]bool, len(ids))
	for _, id := range ids {
		taken[id] = true
	}
	return taken, nil
}
