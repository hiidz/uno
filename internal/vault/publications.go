// Publications: a publisher's catalog or collection put in Community as a
// frozen snapshot. Publishing freezes the source's current content; the
// source's later edits stay private until it is published again, which keeps
// the publication's id. Unpublishing takes a publication out of Community,
// and its subscribers keep their copies.

package vault

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// The kinds of publication, and the status of one Community lists.
const (
	kindCatalog    = "catalog"
	kindCollection = "collection"
	statusLive     = "live"
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
// uses: its existing publication's, or newID for a first publish.
func publicationID(state *PublicationState, newID uuid.UUID) (uuid.UUID, bool) {
	if state == nil {
		return newID, false
	}
	return state.ID, true
}

// check runs the checks a publish runs: the form validators the snapshot's
// copies are written through, then validateParams over every distinct
// recipe it publishes, once each.
func (p publication) check(validateParams CatalogParamsValidator) error {
	if err := p.snapshot.validate(); err != nil {
		return err
	}
	for _, c := range distinctRecipes(p.snapshot.Catalogs) {
		if err := validateParams(c.Type, c.Provider, string(c.Params)); err != nil {
			return err
		}
	}
	return nil
}

// distinctRecipes is catalogs with every catalog whose recipe an earlier one
// already asks for left out, in order.
func distinctRecipes(catalogs []BundleCatalog) []BundleCatalog {
	seen := map[string]bool{}
	var distinct []BundleCatalog
	for _, c := range catalogs {
		hash := RecipeHash(c.Type, c.Provider, string(c.Params))
		if !seen[hash] {
			seen[hash] = true
			distinct = append(distinct, c)
		}
	}
	return distinct
}

// PublishCatalog publishes profileID's listed catalog catalogID, or
// publishes it again: its content now becomes the publication's snapshot,
// under the publication's existing id when it has one, and the publication
// is live. The catalog must be listed and not a subscribed copy
// (ErrInvalidInput), and its recipe passes validateParams, which is
// required. Returns the catalog, with its publication.
func (db *DB) PublishCatalog(ctx context.Context, profileID, catalogID uuid.UUID, validateParams CatalogParamsValidator) (Catalog, error) {
	newID := uuid.New()
	load := func(q querier) (publication, error) {
		c, err := publishableCatalog(ctx, q, profileID, catalogID)
		if err != nil {
			return publication{}, err
		}
		id, existing := publicationID(c.Publication, newID)
		return newPublication(kindCatalog, id, existing, profileID, c.ID, c.Name, catalogSnapshot(id, c))
	}
	if err := db.publish(ctx, load, validateParams); err != nil {
		return Catalog{}, err
	}
	return ownCatalog(ctx, db.conn, profileID, catalogID)
}

// publishableCatalog reads profileID's catalog catalogID through q,
// refusing one inside a collection, which is published with its collection,
// and a subscribed copy.
func publishableCatalog(ctx context.Context, q querier, profileID, catalogID uuid.UUID) (Catalog, error) {
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
// publishes it again, as PublishCatalog does a catalog. The snapshot holds
// every catalog its folders reference, private library catalogs included:
// publishing is the consent to publish them. The collection must not be a
// subscribed copy (ErrInvalidInput), and every recipe it publishes passes
// validateParams, which is required. Returns the collection, with its
// publication.
func (db *DB) PublishCollection(ctx context.Context, profileID, collectionID uuid.UUID, validateParams CatalogParamsValidator) (CollectionWithFolders, error) {
	newID := uuid.New()
	load := func(q querier) (publication, error) {
		tree, err := publishableCollection(ctx, q, profileID, collectionID)
		if err != nil {
			return publication{}, err
		}
		id, existing := publicationID(tree.Publication, newID)
		return newPublication(kindCollection, id, existing, profileID, tree.ID, tree.Title, collectionSnapshot(id, tree))
	}
	if err := db.publish(ctx, load, validateParams); err != nil {
		return CollectionWithFolders{}, err
	}
	return ownCollection(ctx, db.conn, profileID, collectionID)
}

// publishableCollection reads profileID's collection collectionID through
// q, refusing a subscribed copy.
func publishableCollection(ctx context.Context, q querier, profileID, collectionID uuid.UUID) (CollectionWithFolders, error) {
	tree, err := ownCollection(ctx, q, profileID, collectionID)
	if err != nil {
		return CollectionWithFolders{}, err
	}
	if tree.Subscription != nil {
		return CollectionWithFolders{}, errSubscribedCopy("collection")
	}
	return tree, nil
}

// publish runs one publish: load the source as its publication through the
// pool, check it, then load it again inside the write transaction and write
// it if it is still the same. validateParams reaches TMDB, so it runs
// before the transaction opens, and a source edited meanwhile is refused
// with ErrConflict rather than published unchecked.
func (db *DB) publish(ctx context.Context, load func(querier) (publication, error), validateParams CatalogParamsValidator) error {
	if validateParams == nil {
		return errors.New("vault: publishing requires a params validator")
	}
	checked, err := load(db.conn)
	if err != nil {
		return err
	}
	if err := checked.check(validateParams); err != nil {
		return err
	}
	return db.inTx(ctx, func(tx *sql.Tx) error {
		current, err := load(tx)
		if err != nil {
			return err
		}
		if current.contentHash != checked.contentHash || current.id != checked.id {
			return fmt.Errorf("%w: it changed while being published; publish it again", ErrConflict)
		}
		return writePublication(ctx, tx, current, time.Now().UTC().Format(time.RFC3339))
	})
}

// writePublication stores p, live, at now: a republish rewrites its
// existing row, and a first publish inserts one.
func writePublication(ctx context.Context, tx *sql.Tx, p publication, now string) error {
	if p.existing {
		return updatePublication(ctx, tx, p, now)
	}
	return insertPublication(ctx, tx, p, now)
}

// insertPublication inserts p as a new live publication, published at now.
func insertPublication(ctx context.Context, tx *sql.Tx, p publication, now string) error {
	catalogID, collectionID := sourceColumns(p.kind, p.sourceID)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO publications (id, publisher_id, kind, catalog_id, collection_id, title, snapshot, content_hash,
		                          catalog_count, folder_count, status, published_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'live', ?, ?)
	`, p.id.String(), p.publisherID.String(), p.kind, catalogID, collectionID, p.title, p.raw, p.contentHash,
		len(p.snapshot.Catalogs), p.snapshot.folderCount(), now, now); err != nil {
		return fmt.Errorf("inserting publication: %w", err)
	}
	return nil
}

// updatePublication rewrites p's existing row with p's snapshot, live again
// if it was unpublished. published_at stays the first publish's.
func updatePublication(ctx context.Context, tx *sql.Tx, p publication, now string) error {
	if _, err := tx.ExecContext(ctx, `
		UPDATE publications
		SET title = ?, snapshot = ?, content_hash = ?,
		    catalog_count = ?, folder_count = ?, status = 'live', updated_at = ?
		WHERE id = ?
	`, p.title, p.raw, p.contentHash,
		len(p.snapshot.Catalogs), p.snapshot.folderCount(), now, p.id.String()); err != nil {
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

// UnpublishCatalog unpublishes the publication of profileID's catalog
// catalogID, if it has a live one: Community stops listing it, and its
// subscribers keep their copies, marked unpublished. Publishing the catalog
// again revives the same publication. Returns the catalog, or
// ErrCatalogNotFound if it isn't profileID's.
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

// unpublish unpublishes profileID's live publication of sourceID, a catalog
// or a collection, if there is one.
func unpublish(ctx context.Context, e execer, profileID, sourceID uuid.UUID) error {
	if _, err := e.ExecContext(ctx, `
		UPDATE publications SET status = 'unpublished', updated_at = ?1
		WHERE (catalog_id = ?2 OR collection_id = ?2) AND publisher_id = ?3 AND status = 'live'
	`, time.Now().UTC().Format(time.RFC3339), sourceID.String(), profileID.String()); err != nil {
		return fmt.Errorf("unpublishing publication: %w", err)
	}
	return nil
}
