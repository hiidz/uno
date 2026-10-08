package vault

import (
	"context"

	"github.com/google/uuid"
)

// Library is everything the builder shows for a profile: its listed catalogs,
// its collections with their folders and catalogs, what a push of the Home
// as Uno stores it would change in Nuvio, and the home_revision its next push
// is built from.
type Library struct {
	Catalogs     []Catalog               `json:"catalogs"`
	Collections  []CollectionWithFolders `json:"collections"`
	Pending      []PendingChange         `json:"pending"`
	HomeRevision int64                   `json:"home_revision"`
}

// GetLibrary reads profileID's Library in one snapshot, so its pending list
// describes the very rows its catalogs and collections hold, and its
// home_revision the Home positions they carry.
func (db *DB) GetLibrary(ctx context.Context, profileID uuid.UUID) (lib Library, err error) {
	err = db.inReadTx(ctx, func(q dbtx) (err error) {
		if lib.Catalogs, err = userCatalogs(ctx, q, profileID); err != nil {
			return err
		}
		if lib.Collections, err = userCollections(ctx, q, profileID); err != nil {
			return err
		}
		lib.Pending, lib.HomeRevision, err = pendingAndHomeRevision(ctx, q, profileID)
		return err
	})
	return lib, err
}
