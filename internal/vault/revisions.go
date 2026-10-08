// Revisions: what an editor's save and a push are checked against. Each
// content write raises its row's revision in its own statement (writeCatalog,
// updateCollectionRow, writeCatalogEdit, writeCatalogCopy), and each push
// raises its profile's home_revision (SavePush). An editor's save carries the
// revision it was built from, and is refused as ErrStale when the row is at
// another; push compares the home_revision it was built from itself
// (HomeRevision).

package vault

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

// refuseStale is ErrStale when id, a row of table owned by profileID, is at a
// revision other than revision. A row that isn't profileID's passes: the
// write after it answers its own not-found. table is one of this package's
// literals, never client input.
func refuseStale(ctx context.Context, tx *sql.Tx, table string, profileID, id uuid.UUID, revision int64) error {
	var stale bool
	err := tx.QueryRowContext(ctx, `
		SELECT EXISTS (SELECT 1 FROM `+table+` WHERE id = ? AND owner_id = ? AND revision <> ?)
	`, id.String(), profileID.String(), revision).Scan(&stale)
	if err != nil {
		return fmt.Errorf("checking the %s revision: %w", table, err)
	}
	if stale {
		return ErrStale
	}
	return nil
}

// writeCatalogAt is writeCatalog for a catalog editor's save built from
// catalogID at revision: ErrStale, writing nothing, when the row is at
// another.
func writeCatalogAt(ctx context.Context, tx *sql.Tx, profileID, catalogID uuid.UUID, revision int64, input CatalogForm) error {
	if err := refuseStale(ctx, tx, "catalogs", profileID, catalogID, revision); err != nil {
		return err
	}
	return writeCatalog(ctx, tx, profileID, catalogID, input)
}

// updateCollectionTxAt is updateCollectionTx for a collection editor's save
// built from collectionID at revision: ErrStale, writing nothing, when the
// row is at another. The collection's revision also guards the catalogs
// scoped to it, which only a save or an Update of the collection writes.
func updateCollectionTxAt(ctx context.Context, tx *sql.Tx, profileID, collectionID uuid.UUID, revision int64, form CollectionForm, nowStr string) error {
	if err := refuseStale(ctx, tx, "collections", profileID, collectionID, revision); err != nil {
		return err
	}
	return updateCollectionTx(ctx, tx, profileID, collectionID, form, nowStr)
}

// HomeRevision is profileID's home_revision as it stands now: what push
// compares the revision its Home was built from with, once it holds the
// profile's push lock. A profile that isn't there is ErrProfileNotFound.
func (db *DB) HomeRevision(ctx context.Context, profileID uuid.UUID) (int64, error) {
	return homeRevision(ctx, db.conn, profileID)
}

// homeRevision is HomeRevision through q.
func homeRevision(ctx context.Context, q dbtx, profileID uuid.UUID) (int64, error) {
	var revision int64
	err := q.QueryRowContext(ctx, `SELECT home_revision FROM profiles WHERE id = ?`, profileID.String()).Scan(&revision)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrProfileNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("reading the home revision: %w", err)
	}
	return revision, nil
}

// pendingAndHomeRevision is profileID's pending list and home_revision, read
// through q: GetLibrary reads both in the snapshot it reads the rows' Home
// positions in, so a tab never pairs rows from before a push with a revision
// from after it.
func pendingAndHomeRevision(ctx context.Context, q dbtx, profileID uuid.UUID) ([]PendingChange, int64, error) {
	pending, err := pendingPush(ctx, q, profileID)
	if err != nil {
		return nil, 0, err
	}
	revision, err := homeRevision(ctx, q, profileID)
	return pending, revision, err
}

// writePushedHome writes record as profileID's push record (WritePushRecord)
// and raises its home_revision by one, through tx, returning the new
// revision: the one a push answers, which the tab's next push is built from.
func writePushedHome(ctx context.Context, tx *sql.Tx, profileID uuid.UUID, record PushRecord) (int64, error) {
	if err := WritePushRecord(ctx, tx, profileID, record); err != nil {
		return 0, err
	}
	var revision int64
	if err := tx.QueryRowContext(ctx, `
		UPDATE profiles SET home_revision = home_revision + 1 WHERE id = ? RETURNING home_revision
	`, profileID.String()).Scan(&revision); err != nil {
		return 0, fmt.Errorf("raising the home revision: %w", err)
	}
	return revision, nil
}
