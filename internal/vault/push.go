package vault

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/google/uuid"
)

// ValidateSelectionAccess confirms every catalog and collection id in a
// pending push is one this profile may reference, without writing
// anything. Push calls this before attempting Nuvio: its local write
// happens last (see internal/api/push.go), so without this check a bad id
// would reach a third-party API before anything caught it.
func (db *DB) ValidateSelectionAccess(ctx context.Context, profileID uuid.UUID, catalogIDs, collectionIDs []uuid.UUID) error {
	// ReadOnly opens a deferred transaction, one snapshot for both checks
	// without the write lock _txlock=immediate gives every other one.
	tx, err := db.conn.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return fmt.Errorf("starting transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // read-only; never committed

	if err := validateCatalogAccess(ctx, tx, profileID, catalogIDs); err != nil {
		return err
	}
	return validateCollectionAccess(ctx, tx, profileID, collectionIDs)
}
