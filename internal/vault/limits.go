package vault

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/google/uuid"
)

// maxListedCatalogsPerProfile and maxCollectionsPerProfile bound how many
// listed catalogs and how many collections one profile holds. The builder reads
// the whole library in one unpaged call, so the bound keeps that read, and an
// import's 4 MiB bundle, from growing without limit. A profile already past
// them stays readable, pushed and served; only a write that adds a row is
// refused.
const (
	maxListedCatalogsPerProfile = 200
	maxCollectionsPerProfile    = 50
)

// checkCatalogAdd is ErrInvalidInput when c is a listed catalog and its owner
// already holds maxListedCatalogsPerProfile of them. A catalog scoped to a
// collection is never refused. Run inside the transaction that inserts the
// catalog, it counts the rows that transaction has added, so an import that
// would go over is refused whole.
func checkCatalogAdd(ctx context.Context, tx *sql.Tx, c Catalog) error {
	if c.CollectionID != nil {
		return nil
	}
	return checkRoom(ctx, tx, `SELECT COUNT(*) FROM catalogs WHERE owner_id = ? AND collection_id IS NULL`,
		c.OwnerID, maxListedCatalogsPerProfile, "catalogs")
}

// checkCollectionAdd is ErrInvalidInput when profileID already holds
// maxCollectionsPerProfile collections, and else the check of form's folder
// refs (validateFolderRefs): everything createCollectionTx settles before its
// first write.
func checkCollectionAdd(ctx context.Context, tx *sql.Tx, profileID uuid.UUID, form CollectionForm) error {
	if err := checkRoom(ctx, tx, `SELECT COUNT(*) FROM collections WHERE owner_id = ?`,
		profileID, maxCollectionsPerProfile, "collections"); err != nil {
		return err
	}
	return validateFolderRefs(ctx, tx, profileID, nil, existingFolderRefIDs(form.Folders))
}

// checkRoom runs count, a COUNT(*) of profileID's rows, and refuses once it
// reaches limit.
func checkRoom(ctx context.Context, tx *sql.Tx, count string, profileID uuid.UUID, limit int, noun string) error {
	var held int
	if err := tx.QueryRowContext(ctx, count, profileID.String()).Scan(&held); err != nil {
		return fmt.Errorf("counting %s: %w", noun, err)
	}
	if held >= limit {
		return fmt.Errorf("%w: a profile holds at most %d %s; delete one to add another", ErrInvalidInput, limit, noun)
	}
	return nil
}
