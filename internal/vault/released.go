// Released copies: a subscribed copy whose publication ended becomes its
// subscriber's own, marked unpublished_at (publications_release_subscribers),
// and stays marked until the subscriber acknowledges the release, which the
// builder asks them to once.

package vault

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/google/uuid"
)

// ReleasedCopy is one of a profile's rows released and not yet acknowledged:
// its kind ("catalog" or "collection"), its id, and its name, a collection's
// being its title.
type ReleasedCopy struct {
	Kind string    `json:"kind"`
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

// ReleasedCopies is every row of profileID's released and not yet
// acknowledged, oldest release first.
func (db *DB) ReleasedCopies(ctx context.Context, profileID uuid.UUID) ([]ReleasedCopy, error) {
	rows, err := db.conn.QueryContext(ctx, `
		SELECT 'catalog', id, name, unpublished_at FROM catalogs WHERE owner_id = :me AND unpublished_at IS NOT NULL
		UNION ALL
		SELECT 'collection', id, title, unpublished_at FROM collections WHERE owner_id = :me AND unpublished_at IS NOT NULL
		ORDER BY 4, 1, 2
	`, sql.Named("me", profileID.String()))
	if err != nil {
		return nil, fmt.Errorf("querying released copies: %w", err)
	}
	defer func() { _ = rows.Close() }()

	copies := []ReleasedCopy{}
	for rows.Next() {
		var c ReleasedCopy
		var id, releasedAt string
		if err := rows.Scan(&c.Kind, &id, &c.Name, &releasedAt); err != nil {
			return nil, fmt.Errorf("scanning released copy: %w", err)
		}
		if c.ID, err = parseUUID(id, c.Kind+" id"); err != nil {
			return nil, err
		}
		copies = append(copies, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating released copies: %w", err)
	}
	return copies, nil
}

// AcknowledgeReleasedCatalog clears the release mark on profileID's catalog
// catalogID, marked or not, and returns the released copies still
// unacknowledged. Returns ErrCatalogNotFound if the catalog isn't
// profileID's.
func (db *DB) AcknowledgeReleasedCatalog(ctx context.Context, profileID, catalogID uuid.UUID) ([]ReleasedCopy, error) {
	return db.acknowledgeReleased(ctx, `UPDATE catalogs SET unpublished_at = NULL WHERE id = ? AND owner_id = ?`,
		profileID, catalogID, ErrCatalogNotFound)
}

// AcknowledgeReleasedCollection is AcknowledgeReleasedCatalog for a
// collection, ErrCollectionNotFound when it isn't profileID's.
func (db *DB) AcknowledgeReleasedCollection(ctx context.Context, profileID, collectionID uuid.UUID) ([]ReleasedCopy, error) {
	return db.acknowledgeReleased(ctx, `UPDATE collections SET unpublished_at = NULL WHERE id = ? AND owner_id = ?`,
		profileID, collectionID, ErrCollectionNotFound)
}

// acknowledgeReleased runs clear, an UPDATE of id owned by profileID, and
// answers notFound when it matches no row, else ReleasedCopies.
func (db *DB) acknowledgeReleased(ctx context.Context, clear string, profileID, id uuid.UUID, notFound error) ([]ReleasedCopy, error) {
	result, err := db.conn.ExecContext(ctx, clear, id.String(), profileID.String())
	if err != nil {
		return nil, fmt.Errorf("acknowledging release: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("checking rows affected: %w", err)
	}
	if rows == 0 {
		return nil, notFound
	}
	return db.ReleasedCopies(ctx, profileID)
}
