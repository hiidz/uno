// What Nuvio may still hold can't be deleted: a catalog or collection on
// Home, or a catalog a collection on Home uses, stays until a push has taken
// it off. Every refusal is an ErrConflict whose message is its reason alone.

package vault

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"

	"github.com/google/uuid"
)

// conflictReason is an ErrConflict whose message is its reason alone: a
// sentence the SPA shows as it stands, and words for the same reason itself.
type conflictReason string

func (r conflictReason) Error() string { return string(r) }

func (r conflictReason) Unwrap() error { return ErrConflict }

const (
	// errTakeOffHome refuses deleting a row that is on Home, a catalog's
	// Discover-only row included.
	errTakeOffHome conflictReason = "Take it off Home and push first."
	// errPushFirst refuses deleting a catalog while a collection on Home
	// needs a push: Nuvio still holds that collection as it was last pushed,
	// which may use the catalog.
	errPushFirst conflictReason = "Push first: Nuvio may still show it in a collection."
)

// catalogDeleteBlocker is why profileID's catalog catalogID can't be deleted
// yet, read through tx, or nil when nothing on Home holds it (catalogHeldBy).
// A catalog that isn't profileID's is ErrCatalogNotFound, the delete's own
// answer to it.
func catalogDeleteBlocker(ctx context.Context, tx *sql.Tx, profileID, catalogID uuid.UUID) error {
	c, err := ownCatalog(ctx, tx, profileID, catalogID)
	if err != nil {
		return err
	}
	trees, err := homeCollections(ctx, tx, profileID)
	if err != nil {
		return err
	}
	return catalogHeldBy(c, trees)
}

// homeCollections is profileID's collections on Home, in Home order, each
// with its tree and NeedsPush, read through q.
func homeCollections(ctx context.Context, q querier, profileID uuid.UUID) ([]CollectionWithFolders, error) {
	collections, err := selectLeanCollections(ctx, q,
		"col.owner_id = ? AND col.home_sort_order IS NOT NULL ORDER BY col.home_sort_order", profileID.String())
	if err != nil {
		return nil, err
	}
	return assembleCollectionTree(ctx, q, collections, leanCatalogsByIDs)
}

// catalogHeldBy is why listed catalog c can't be deleted while trees are its
// profile's collections on Home, in Home order: c has a Home row of its own
// (errTakeOffHome), or else as collectionsHold says. A catalog inside a
// collection is nil here: its delete is refused on its own terms, since it
// goes through its collection's save.
func catalogHeldBy(c Catalog, trees []CollectionWithFolders) error {
	switch {
	case c.CollectionID != nil:
		return nil
	case c.HomeSortOrder != nil:
		return errTakeOffHome
	}
	return collectionsHold(trees, c.ID)
}

// collectionsHold is why catalogID can't be deleted while trees are the
// collections on Home, in Home order: the first of them that uses it, by
// name, or else any of them needing a push (errPushFirst). The pushed hash
// can't say which catalogs a collection's last pushed version used, so one
// collection needing a push holds every catalog.
func collectionsHold(trees []CollectionWithFolders, catalogID uuid.UUID) error {
	if i := slices.IndexFunc(trees, usesCatalog(catalogID)); i >= 0 {
		return conflictReason(fmt.Sprintf("Remove it from “%s” and push first.", trees[i].Title))
	}
	if slices.ContainsFunc(trees, treeNeedsPush) {
		return errPushFirst
	}
	return nil
}

// usesCatalog reports whether a tree's folders reference catalogID.
func usesCatalog(catalogID uuid.UUID) func(CollectionWithFolders) bool {
	return func(tree CollectionWithFolders) bool {
		return slices.ContainsFunc(tree.Catalogs, func(c Catalog) bool { return c.ID == catalogID })
	}
}

// treeNeedsPush reports tree's NeedsPush.
func treeNeedsPush(tree CollectionWithFolders) bool {
	return tree.NeedsPush
}

// collectionDeleteBlocker is why profileID's collection collectionID can't
// be deleted yet, read through tx: it is on Home (errTakeOffHome). One that
// isn't profileID's is ErrCollectionNotFound.
func collectionDeleteBlocker(ctx context.Context, tx *sql.Tx, profileID, collectionID uuid.UUID) error {
	var onHome bool
	err := tx.QueryRowContext(ctx, `
		SELECT home_sort_order IS NOT NULL FROM collections WHERE id = ? AND owner_id = ?
	`, collectionID.String(), profileID.String()).Scan(&onHome)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return ErrCollectionNotFound
	case err != nil:
		return fmt.Errorf("loading collection: %w", err)
	case onHome:
		return errTakeOffHome
	}
	return nil
}
