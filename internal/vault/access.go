package vault

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/google/uuid"
)

// validateFolderRefs confirms every catalog ID may be referenced by a
// folder in a collection this profile owns, under the closed-graph rule:
// owned by profileID, and either listed (collection_id IS NULL) or already
// scoped to this same collection. A nil collectionID means the collection
// doesn't exist yet (CreateUserCollection has no id to compare against), so
// only listed catalogs qualify. Returns an ErrInvalidInput naming the first
// id that doesn't. Must run inside the caller's transaction.
func validateFolderRefs(ctx context.Context, tx *sql.Tx, profileID uuid.UUID, collectionID *uuid.UUID, ids []uuid.UUID) error {
	unique := dedupeUUIDs(ids)
	usable, err := usableCatalogIDs(ctx, tx, profileID, collectionID, unique)
	if err != nil {
		return err
	}
	return refuseUnusable(unique, usable)
}

// usableCatalogIDs is those of ids validateFolderRefs admits, read through tx.
func usableCatalogIDs(ctx context.Context, tx *sql.Tx, profileID uuid.UUID, collectionID *uuid.UUID, ids []uuid.UUID) ([]uuid.UUID, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	return queryUUIDs(ctx, tx, "catalog id", `
		SELECT id FROM catalogs
		WHERE id IN (SELECT value FROM json_each(?)) AND owner_id = ?
		  AND (collection_id IS NULL OR collection_id = ?)
	`, idsJSON(ids), profileID.String(), nullableUUIDString(collectionID))
}

// refuseUnusable is the ErrInvalidInput naming the first of ids usable lacks,
// or nil when it has them all.
func refuseUnusable(ids, usable []uuid.UUID) error {
	found := make(map[uuid.UUID]bool, len(usable))
	for _, id := range usable {
		found[id] = true
	}
	for _, id := range ids {
		if !found[id] {
			return fmt.Errorf("%w: catalog %s is not usable in this collection's folders", ErrInvalidInput, id)
		}
	}
	return nil
}

// errSubscribedCopy is the ErrInvalidInput for writing or publishing a
// subscribed copy of kind: only its publisher changes or publishes it.
func errSubscribedCopy(kind string) error {
	return fmt.Errorf("%w: this %s is from Community, so only its publisher can change or publish it; duplicate it to make a version of your own", ErrInvalidInput, kind)
}

// refuseSubscribedCopy is errSubscribedCopy when id, a catalog or a collection
// as kind names it, is one of profileID's subscribed copies. Every content
// write other than Update runs it inside its transaction, ahead of the write.
func refuseSubscribedCopy(ctx context.Context, q dbtx, profileID uuid.UUID, kind string, id uuid.UUID) error {
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
