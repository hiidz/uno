package vault

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/google/uuid"
)

// queryRower is the common subset of *sql.DB and *sql.Tx the single-row
// lookups below need, so callers can run them either inside a transaction or
// straight against the pool.
type queryRower interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
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

	args := append([]any{idsJSON(unique), profileID.String()}, q.extraArgs...)

	got, err := queryUUIDs(ctx, tx, q.label+" id", fmt.Sprintf(`
		SELECT id FROM %s
		WHERE id IN (SELECT value FROM json_each(?)) AND owner_id = ?%s
	`, q.table, q.extraWhere), args...)
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

// errSubscribedCopy is the ErrInvalidInput for writing or publishing a
// subscribed copy of kind: only its publisher changes or publishes it.
func errSubscribedCopy(kind string) error {
	return fmt.Errorf("%w: this %s is from Community, so only its publisher can change or publish it; duplicate it to make a version of your own", ErrInvalidInput, kind)
}

// refuseSubscribedCopy is errSubscribedCopy when id, a catalog or a collection
// as kind names it, is one of profileID's subscribed copies. Every content
// write other than Update runs it inside its transaction, ahead of the write.
func refuseSubscribedCopy(ctx context.Context, q queryRower, profileID uuid.UUID, kind string, id uuid.UUID) error {
	var subscribed bool
	err := q.QueryRowContext(ctx, `
		SELECT EXISTS (SELECT 1 FROM subscriptions WHERE (catalog_id = ?1 OR collection_id = ?1) AND subscriber_id = ?2)
	`, id.String(), profileID.String()).Scan(&subscribed)
	if err != nil {
		return fmt.Errorf("checking subscription: %w", err)
	}
	if subscribed {
		return errSubscribedCopy(kind)
	}
	return nil
}
