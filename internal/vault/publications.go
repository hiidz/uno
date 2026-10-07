// Publications: a publisher's catalog or collection put in Community as a
// frozen snapshot. Publishing freezes the source's current content; the
// source's later edits stay private until it is published again, which keeps
// the publication's id. Unpublishing deletes a publication, and its
// subscribers keep their copies as their own.

package vault

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// The kinds of publication.
const (
	kindCatalog    = "catalog"
	kindCollection = "collection"
)

// publication is one publications row a publish writes: the source it
// publishes, the snapshot it holds, and whether it republishes an existing
// publication, whose id it then keeps.
type publication struct {
	id          uuid.UUID
	existing    bool
	publisherID uuid.UUID
	kind        string
	sourceID    uuid.UUID
	title       string
	snapshot    Snapshot
	raw         string
	contentHash string
}

// newPublication is the publication of source as snapshot s.
func newPublication(kind string, id uuid.UUID, existing bool, publisherID, sourceID uuid.UUID, title string, s Snapshot) (publication, error) {
	raw, hash, err := s.encode()
	if err != nil {
		return publication{}, err
	}
	return publication{
		id: id, existing: existing, publisherID: publisherID, kind: kind, sourceID: sourceID, title: title,
		snapshot: s, raw: raw, contentHash: hash,
	}, nil
}

// publicationID is the id a publish of a row whose publication is state
// uses: its existing publication's, or a new one for a first publish.
func publicationID(state *PublicationState) (uuid.UUID, bool) {
	if state == nil {
		return uuid.New(), false
	}
	return state.ID, true
}

// PublishCatalog publishes profileID's listed catalog catalogID, or
// publishes an update to it: its content now becomes the publication's
// snapshot, under the publication's existing id when it has one. The catalog
// must be listed and not a subscribed copy (ErrInvalidInput). Returns the
// catalog, with its publication.
func (db *DB) PublishCatalog(ctx context.Context, profileID, catalogID uuid.UUID) (Catalog, error) {
	load := func(tx *sql.Tx) (publication, error) {
		c, err := publishableCatalog(ctx, tx, profileID, catalogID)
		if err != nil {
			return publication{}, err
		}
		id, existing := publicationID(c.Publication)
		return newPublication(kindCatalog, id, existing, profileID, c.ID, c.Name, catalogSnapshot(id, c))
	}
	if err := db.publish(ctx, load); err != nil {
		return Catalog{}, err
	}
	return ownCatalog(ctx, db.conn, profileID, catalogID)
}

// publishableCatalog reads profileID's catalog catalogID through q,
// refusing one inside a collection, which is published with its collection,
// and a subscribed copy.
func publishableCatalog(ctx context.Context, q dbtx, profileID, catalogID uuid.UUID) (Catalog, error) {
	c, err := ownCatalog(ctx, q, profileID, catalogID)
	switch {
	case err != nil:
		return Catalog{}, err
	case c.CollectionID != nil:
		return Catalog{}, fmt.Errorf("%w: a catalog inside a collection is published with its collection", ErrInvalidInput)
	case c.Subscription != nil:
		return Catalog{}, errSubscribedCopy("catalog")
	}
	return c, nil
}

// PublishCollection publishes profileID's collection collectionID, or
// publishes an update to it, as PublishCatalog does a catalog. The snapshot holds
// every catalog its folders reference, private library catalogs included:
// publishing is the consent to make them publicly readable as part of the
// collection. Community never lists them on their own. The collection must not be a
// subscribed copy (ErrInvalidInput). Returns the collection, with its
// publication.
func (db *DB) PublishCollection(ctx context.Context, profileID, collectionID uuid.UUID) (CollectionWithFolders, error) {
	load := func(tx *sql.Tx) (publication, error) {
		tree, err := publishableCollection(ctx, tx, profileID, collectionID)
		if err != nil {
			return publication{}, err
		}
		id, existing := publicationID(tree.Publication)
		return newPublication(kindCollection, id, existing, profileID, tree.ID, tree.Title, collectionSnapshot(id, tree))
	}
	if err := db.publish(ctx, load); err != nil {
		return CollectionWithFolders{}, err
	}
	return ownCollection(ctx, db.conn, profileID, collectionID)
}

// publishableCollection reads profileID's collection collectionID through
// q, refusing a subscribed copy.
func publishableCollection(ctx context.Context, q dbtx, profileID, collectionID uuid.UUID) (CollectionWithFolders, error) {
	tree, err := ownCollection(ctx, q, profileID, collectionID)
	if err != nil {
		return CollectionWithFolders{}, err
	}
	if tree.Subscription != nil {
		return CollectionWithFolders{}, errSubscribedCopy("collection")
	}
	return tree, nil
}

// publish runs one publish in one transaction: load the source as its
// publication, refuse a snapshot the form validators its copies are written
// through would refuse, and write it. Every recipe in it was checked against
// TMDB when its row was written, so a publish makes no TMDB call.
func (db *DB) publish(ctx context.Context, load func(*sql.Tx) (publication, error)) error {
	return db.inTx(ctx, func(tx *sql.Tx) error {
		p, err := load(tx)
		if err != nil {
			return err
		}
		if err := p.snapshot.validate(); err != nil {
			return err
		}
		return writePublication(ctx, tx, p, time.Now().UTC().Format(time.RFC3339))
	})
}

// writePublication stores p at now: publishing an update rewrites its
// existing row, and a first publish inserts one.
func writePublication(ctx context.Context, tx *sql.Tx, p publication, now string) error {
	if p.existing {
		return updatePublication(ctx, tx, p, now)
	}
	return insertPublication(ctx, tx, p, now)
}

// insertPublication inserts p as a new publication, published at now.
func insertPublication(ctx context.Context, tx *sql.Tx, p publication, now string) error {
	catalogID, collectionID := sourceColumns(p.kind, p.sourceID)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO publications (id, publisher_id, kind, catalog_id, collection_id, title, snapshot, content_hash,
		                          published_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, p.id.String(), p.publisherID.String(), p.kind, catalogID, collectionID, p.title, p.raw, p.contentHash,
		now, now); err != nil {
		return fmt.Errorf("inserting publication: %w", err)
	}
	return nil
}

// updatePublication rewrites p's existing row with p's snapshot.
// published_at stays the first publish's.
func updatePublication(ctx context.Context, tx *sql.Tx, p publication, now string) error {
	if _, err := tx.ExecContext(ctx, `
		UPDATE publications
		SET title = ?, snapshot = ?, content_hash = ?, updated_at = ?
		WHERE id = ?
	`, p.title, p.raw, p.contentHash, now, p.id.String()); err != nil {
		return fmt.Errorf("updating publication: %w", err)
	}
	return nil
}

// sourceColumns is the catalog_id and collection_id a row of kind naming id
// stores: id in its kind's column, NULL in the other.
func sourceColumns(kind string, id uuid.UUID) (any, any) {
	if kind == kindCatalog {
		return id.String(), nil
	}
	return nil, id.String()
}

// UnpublishCatalog unpublishes profileID's catalog catalogID, if it is
// published: its publication is deleted, so Community stops listing it, and
// its subscribers keep their copies as their own, marked unpublished
// (publications_release_subscribers). Publishing the catalog again is a new
// publication. Returns the catalog, or ErrCatalogNotFound if it isn't
// profileID's.
func (db *DB) UnpublishCatalog(ctx context.Context, profileID, catalogID uuid.UUID) (Catalog, error) {
	if err := unpublish(ctx, db.conn, profileID, catalogID); err != nil {
		return Catalog{}, err
	}
	return ownCatalog(ctx, db.conn, profileID, catalogID)
}

// UnpublishCollection is UnpublishCatalog for a collection.
func (db *DB) UnpublishCollection(ctx context.Context, profileID, collectionID uuid.UUID) (CollectionWithFolders, error) {
	if err := unpublish(ctx, db.conn, profileID, collectionID); err != nil {
		return CollectionWithFolders{}, err
	}
	return ownCollection(ctx, db.conn, profileID, collectionID)
}

// unpublish deletes profileID's publication of sourceID, a catalog or a
// collection, if there is one.
func unpublish(ctx context.Context, e dbtx, profileID, sourceID uuid.UUID) error {
	if _, err := e.ExecContext(ctx, `
		DELETE FROM publications WHERE (catalog_id = ?1 OR collection_id = ?1) AND publisher_id = ?2
	`, sourceID.String(), profileID.String()); err != nil {
		return fmt.Errorf("unpublishing publication: %w", err)
	}
	return nil
}
