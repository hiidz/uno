// What changed, for the rows a publication touches: what an Update would
// change in a row added from Community, and what publishing again would
// change in an own row. Each is the one comparison (diffSnapshots) between a
// row's snapshot and a publication's, under the same keys, and reads only
// what the vault stores: nothing here compares with what a push sent.

package vault

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

// UpdateChanges is what UpdateSubscription would change in profileID's copy
// of publicationID: the copy, as a snapshot under its own keys, against the
// publication's current one. Empty for a copy in step. ErrPublicationNotFound
// unless profileID subscribes to the publication and it is live.
func (db *DB) UpdateChanges(ctx context.Context, profileID, publicationID uuid.UUID) ([]SnapshotChange, error) {
	sub, err := loadSubscription(ctx, db.conn, profileID, publicationID)
	if err != nil {
		return nil, err
	}
	have, err := db.copyAsSnapshot(ctx, profileID, sub)
	if err != nil {
		return nil, err
	}
	return diffSnapshots(have, sub.pub.snapshot), nil
}

// copyAsSnapshot is sub's copy as a snapshot that compares with its
// publication's.
func (db *DB) copyAsSnapshot(ctx context.Context, profileID uuid.UUID, sub subscription) (Snapshot, error) {
	if sub.pub.kind == kindCatalog {
		c, err := ownCatalog(ctx, db.conn, profileID, sub.copyID)
		return copySnapshot(c, sub.pub.snapshot), err
	}
	tree, err := ownCollection(ctx, db.conn, profileID, sub.copyID)
	return copyTreeSnapshot(tree), err
}

// CatalogChangesSincePublish is what publishing profileID's catalog catalogID
// again would change in its publication: the catalog now against the
// snapshot it last published. Empty for a catalog that was never published.
// The catalog must be one that can be published (publishableCatalog).
func (db *DB) CatalogChangesSincePublish(ctx context.Context, profileID, catalogID uuid.UUID) ([]SnapshotChange, error) {
	c, err := publishableCatalog(ctx, db.conn, profileID, catalogID)
	if err != nil || c.Publication == nil {
		return []SnapshotChange{}, err
	}
	return db.changesSincePublish(ctx, c.Publication.ID, catalogSnapshot(c.Publication.ID, c))
}

// CollectionChangesSincePublish is CatalogChangesSincePublish for a
// collection. Its snapshot holds the catalogs its folders use, so a library
// catalog edited since shows as changed.
func (db *DB) CollectionChangesSincePublish(ctx context.Context, profileID, collectionID uuid.UUID) ([]SnapshotChange, error) {
	tree, err := publishableCollection(ctx, db.conn, profileID, collectionID)
	if err != nil || tree.Publication == nil {
		return []SnapshotChange{}, err
	}
	return db.changesSincePublish(ctx, tree.Publication.ID, collectionSnapshot(tree.Publication.ID, tree))
}

// changesSincePublish is now, a row's snapshot under publicationID's keys,
// against what publicationID last published.
func (db *DB) changesSincePublish(ctx context.Context, publicationID uuid.UUID, now Snapshot) ([]SnapshotChange, error) {
	var raw string
	err := db.conn.QueryRowContext(ctx, `SELECT snapshot FROM publications WHERE id = ?`, publicationID.String()).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrPublicationNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("loading publication: %w", err)
	}
	published, err := decodeSnapshot(raw)
	if err != nil {
		return nil, err
	}
	return diffSnapshots(published, now), nil
}
