package vault

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/google/uuid"
)

// validateAccess confirms every id is one this profile is allowed to
// reference: either it owns the row in the given table, or the row is
// public. label names the table in the singular (e.g. "catalog") and is
// pluralized to get the table name — must run inside the caller's
// transaction.
func validateAccess(ctx context.Context, tx *sql.Tx, label string, profileID uuid.UUID, ids []uuid.UUID) error {
	if len(ids) == 0 {
		return nil
	}
	table := label + "s"

	seen := make(map[uuid.UUID]bool, len(ids))
	unique := make([]uuid.UUID, 0, len(ids))
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			unique = append(unique, id)
		}
	}

	placeholders, args := buildInClause(unique)
	args = append(args, profileID.String())

	rows, err := tx.QueryContext(ctx, fmt.Sprintf(`
		SELECT id FROM %s
		WHERE id IN (%s) AND (owner_id = ? OR is_public = 1)
	`, table, placeholders), args...)
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

// validateCatalogAccess confirms every catalog ID is one this profile is
// allowed to reference in a folder: either it owns the catalog, or the
// catalog is public. Must run inside the caller's transaction.
func validateCatalogAccess(ctx context.Context, tx *sql.Tx, profileID uuid.UUID, catalogIDs []uuid.UUID) error {
	return validateAccess(ctx, tx, "catalog", profileID, catalogIDs)
}

func validateCollectionAccess(ctx context.Context, tx *sql.Tx, profileID uuid.UUID, collectionIDs []uuid.UUID) error {
	return validateAccess(ctx, tx, "collection", profileID, collectionIDs)
}

// syncSelection resets a profile's join-table selection (profile_catalogs or
// profile_collections) to exactly ids, in order: rows for ids no longer
// present are deleted, and upsert is called for each id in ids with its
// index (which callers use as sort_order). Must run inside the caller's
// transaction; the caller is responsible for access-checking ids first.
func syncSelection(ctx context.Context, tx *sql.Tx, table, idColumn string, profileID uuid.UUID, ids []uuid.UUID, upsert func(i int, id uuid.UUID) error) error {
	rows, err := tx.QueryContext(ctx, fmt.Sprintf(`SELECT %s FROM %s WHERE profile_id = ?`, idColumn, table), profileID.String())
	if err != nil {
		return fmt.Errorf("querying existing selection: %w", err)
	}
	existing := make(map[uuid.UUID]bool)
	for rows.Next() {
		var idStr string
		if err := rows.Scan(&idStr); err != nil {
			rows.Close()
			return fmt.Errorf("scanning %s: %w", idColumn, err)
		}
		id, err := parseUUID(idStr, idColumn)
		if err != nil {
			rows.Close()
			return err
		}
		existing[id] = true
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterating existing selection: %w", err)
	}
	rows.Close()

	incoming := make(map[uuid.UUID]bool, len(ids))
	for _, id := range ids {
		incoming[id] = true
	}

	var toDelete []uuid.UUID
	for id := range existing {
		if !incoming[id] {
			toDelete = append(toDelete, id)
		}
	}
	if len(toDelete) > 0 {
		placeholders, args := buildInClause(toDelete)
		args = append([]any{profileID.String()}, args...)
		_, err = tx.ExecContext(ctx, fmt.Sprintf(`
			DELETE FROM %s WHERE profile_id = ? AND %s IN (%s)
		`, table, idColumn, placeholders), args...)
		if err != nil {
			return fmt.Errorf("removing dropped selections: %w", err)
		}
	}

	for i, id := range ids {
		if err := upsert(i, id); err != nil {
			return err
		}
	}

	return nil
}
