// Duplicate of a collection the caller owns: the source tree is extracted
// into its bundle form and written back as a new collection through the
// same create core a collection save uses.

package vault

import (
	"context"
	"database/sql"

	"github.com/google/uuid"
)

// DuplicateCollection deep-copies a collection profileID already owns —
// its cosmetics, folders, and every catalog its folders reference — into a
// new collection, also owned by profileID. Every folder ref survives the
// copy: a listed source catalog stays a reference (the new collection's
// folders point at the same row), while each distinct scoped source catalog
// becomes one fresh scoped copy in the new collection, referenced wherever
// the source referenced it. The copy is unpublished and subscribed to
// nothing, even when its source is a subscribed copy, and its title gets a
// " (copy)" suffix (copyName), cut short to fit maxNameLen. Returns
// ErrCollectionNotFound if sourceID isn't owned by profileID.
//
// The source is checked as the form that writes the copy, by
// CollectionForm.Validate, the rules a collection save runs, since being
// stored is not evidence a row passes today's rules. The listed catalogs it
// references are not checked: they stay references to rows the caller already
// owns, and nothing is written from them.
func (db *DB) DuplicateCollection(ctx context.Context, profileID uuid.UUID, sourceID uuid.UUID) (CollectionWithFolders, error) {
	return db.copyCollection(ctx, profileID, sourceID)
}

// copyCollection is DuplicateCollection's copy: extract the source tree,
// check it as the form that writes the copy, write it, and read it back.
func (db *DB) copyCollection(ctx context.Context, profileID, sourceID uuid.UUID) (CollectionWithFolders, error) {
	source, err := ownCollection(ctx, db.conn, profileID, sourceID)
	if err != nil {
		return CollectionWithFolders{}, err
	}
	b := extractBundle(nil, []CollectionWithFolders{source}, false)
	form := collectionFormFromBundle(b.Collections[0], topSourceIDs(b.Catalogs), false)
	form.Title = copyName(form.Title)
	if err := form.Validate(); err != nil {
		return CollectionWithFolders{}, err
	}
	return db.createCollection(ctx, profileID, form)
}

// createCollection writes form as a new collection in a transaction of its
// own, then reads it back.
func (db *DB) createCollection(ctx context.Context, profileID uuid.UUID, form CollectionForm) (CollectionWithFolders, error) {
	var created CollectionWithFolders
	err := db.inTx(ctx, func(tx *sql.Tx) error {
		var err error
		created, _, err = createCollectionTx(ctx, tx, profileID, form)
		return err
	})
	if err != nil {
		return CollectionWithFolders{}, err
	}
	return ownCollection(ctx, db.conn, profileID, created.ID)
}

// topSourceIDs maps each top-level catalog's key to the row it was extracted
// from. A Duplicate leaves listed catalogs top-level, and they are the
// caller's own, so its refs to them point at those same rows.
func topSourceIDs(catalogs []BundleCatalog) map[string]uuid.UUID {
	ids := make(map[string]uuid.UUID, len(catalogs))
	for _, c := range catalogs {
		ids[c.Key] = *c.SourceID
	}
	return ids
}
