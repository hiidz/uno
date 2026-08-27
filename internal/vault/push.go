package vault

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

// ValidateSelectionAccess confirms every catalog and collection id in a
// pending push is one this profile may reference, without writing
// anything. Push calls this before attempting Nuvio: its local write
// happens last (see internal/api/push.go), so without this check a bad id
// would reach a third-party API before anything caught it.
func (db *DB) ValidateSelectionAccess(ctx context.Context, profileID uuid.UUID, catalogIDs, collectionIDs []uuid.UUID) error {
	tx, err := db.conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("starting transaction: %w", err)
	}
	defer tx.Rollback() // read-only; never committed

	if err := validateCatalogAccess(ctx, tx, profileID, catalogIDs); err != nil {
		return err
	}
	return validateCollectionAccess(ctx, tx, profileID, collectionIDs)
}

// SaveSelectionsForPush writes both selections in one transaction — the
// local half of push, run only after both Nuvio calls have already
// succeeded (internal/api/push.go). The two writes share one transaction
// so they commit or roll back together.
func (db *DB) SaveSelectionsForPush(ctx context.Context, profileID uuid.UUID, catalogs CatalogSelectionForm, collections CollectionSelectionForm) error {
	tx, err := db.conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("starting transaction: %w", err)
	}
	defer tx.Rollback() // no-op once Commit succeeds

	if err := saveCatalogSelectionTx(ctx, tx, profileID, catalogs); err != nil {
		return err
	}
	if err := saveCollectionSelectionTx(ctx, tx, profileID, collections); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}
	return nil
}
