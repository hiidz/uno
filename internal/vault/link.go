// A taken copy's link to its original: Update, which rewrites a linked copy
// with its original's current content, and the writes that set or clear the
// link columns (taken_from, taken_hash).
//
// Update compares three hashes: the original's now, the copy's now, and the
// copy's taken_hash, the original's hash when the copy was last in step with
// it. A copy equal to its original is left alone and its taken_hash
// rewritten, which also settles a copy whose stored hash predates a change
// to what the hash covers. A copy that no longer matches its taken_hash was
// changed some way a save didn't unlink, so Update unlinks it and reports
// ErrConflict rather than overwrite it. Anything else is rewritten.

package vault

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// UpdateTakenCatalog brings profileID's linked copy of sourceID, a public
// catalog owned by someone else, up to date with it: the copy takes the
// original's name, params and stored fingerprint, and its taken_hash moves
// to match. Returns the copy, unchanged when it already matches. Returns
// ErrCatalogNotFound if sourceID isn't public and someone else's, or
// profileID holds no linked copy of it, and ErrConflict, after unlinking
// the copy, if the copy no longer matches its taken_hash. validateParams is
// required; see loadCommunityCatalog.
func (db *DB) UpdateTakenCatalog(ctx context.Context, profileID, sourceID uuid.UUID, validateParams CatalogParamsValidator) (Catalog, error) {
	original, err := db.loadCommunityCatalog(ctx, profileID, sourceID, validateParams)
	if err != nil {
		return Catalog{}, err
	}
	return runTakenUpdate(ctx, db, func(tx *sql.Tx) (Catalog, error) {
		return updateTakenCatalogTx(ctx, tx, profileID, original)
	})
}

// updateTakenCatalogTx is UpdateTakenCatalog's write, inside tx.
func updateTakenCatalogTx(ctx context.Context, tx *sql.Tx, profileID uuid.UUID, original Catalog) (Catalog, error) {
	linkedCopy, err := linkedCatalogCopy(ctx, tx, profileID, original.ID)
	if err != nil {
		return Catalog{}, err
	}
	originalHash := catalogHash(original.Name, original.Fingerprint)
	switch catalogHash(linkedCopy.Name, linkedCopy.Fingerprint) {
	case originalHash:
		linkedCopy.TakenHash = originalHash
		return linkedCopy, writeCatalogTakenHash(ctx, tx, linkedCopy.ID, originalHash)
	case linkedCopy.TakenHash:
		return rewriteTakenCatalog(ctx, tx, linkedCopy, original, originalHash)
	default:
		return Catalog{}, conflictAfterUnlink(unlinkCatalog(ctx, tx, linkedCopy.ID))
	}
}

// linkedCatalogCopy reads profileID's linked copy of sourceID through tx.
// Returns ErrCatalogNotFound if there is none.
func linkedCatalogCopy(ctx context.Context, tx *sql.Tx, profileID, sourceID uuid.UUID) (Catalog, error) {
	copies, err := selectCatalogs(ctx, tx, "owner_id = ? AND taken_from = ? AND collection_id IS NULL", profileID.String(), sourceID.String())
	if err != nil {
		return Catalog{}, err
	}
	if len(copies) == 0 {
		return Catalog{}, ErrCatalogNotFound
	}
	return copies[0], nil
}

// rewriteTakenCatalog writes original's name, params and stored fingerprint
// over linkedCopy, with taken_hash set to originalHash. A catalog's type never
// changes, so an original whose type differs from its copy's is refused
// rather than written.
func rewriteTakenCatalog(ctx context.Context, tx *sql.Tx, linkedCopy, original Catalog, originalHash string) (Catalog, error) {
	if original.Type != linkedCopy.Type {
		return Catalog{}, fmt.Errorf("%w: the original's type no longer matches your copy's", ErrInvalidInput)
	}
	now := time.Now().UTC()
	if _, err := tx.ExecContext(ctx, `
		UPDATE catalogs SET name = ?, params = ?, fingerprint = ?, taken_hash = ?, updated_at = ?
		WHERE id = ?
	`, original.Name, original.Params, original.Fingerprint, originalHash, now.Format(time.RFC3339), linkedCopy.ID.String()); err != nil {
		return Catalog{}, fmt.Errorf("updating taken catalog: %w", err)
	}
	linkedCopy.Name, linkedCopy.Params, linkedCopy.Fingerprint = original.Name, original.Params, original.Fingerprint
	linkedCopy.TakenHash, linkedCopy.UpdatedAt = originalHash, now
	return linkedCopy, nil
}

// UpdateTakenCollection brings profileID's linked copy of sourceID, a public
// collection owned by someone else, up to date with it, through the same
// update core a collection save uses:
//
//   - the collection's own fields come from the original, except is_public
//     and pin_to_top, which stay the copy's; home_sort_order and
//     pushed_version are left as they are, and version goes up by one, so
//     Home shows the update as an unpushed change;
//   - the original's folders are written over the copy's by position,
//     keeping the id of each of the copy's folders that has a counterpart;
//     the copy's folders past the original's last are removed;
//   - a catalog of the copy's whose taken_from is one of the original's
//     catalogs keeps its id and takes that catalog's name, params and stored
//     fingerprint, as a catalog edit; any other of the original's catalogs
//     becomes a new scoped catalog linked to it, and a copy catalog no folder
//     references any more is removed.
//
// Returns the copy as stored, unchanged when it already matches. Returns
// ErrCollectionNotFound if sourceID isn't public and someone else's, or
// profileID holds no linked copy of it, and ErrConflict, after unlinking the
// copy, if the copy no longer matches its taken_hash. validateParams is
// required; see loadCommunityCollection.
func (db *DB) UpdateTakenCollection(ctx context.Context, profileID, sourceID uuid.UUID, validateParams CatalogParamsValidator) (CollectionWithFolders, error) {
	original, err := db.loadCommunityCollection(ctx, profileID, sourceID, validateParams)
	if err != nil {
		return CollectionWithFolders{}, err
	}
	copyID, err := runTakenUpdate(ctx, db, func(tx *sql.Tx) (uuid.UUID, error) {
		return updateTakenCollectionTx(ctx, tx, profileID, sourceID, original)
	})
	if err != nil {
		return CollectionWithFolders{}, err
	}
	return db.reloadCollection(ctx, copyID)
}

// takenOriginal is the original an Update reads before its transaction: its
// bundle form, every catalog it references in its own list, and that form's
// hash.
type takenOriginal struct {
	collection BundleCollection
	hash       string
}

// loadCommunityCollection reads sourceID, which must be public and not owned
// by profileID (ErrCollectionNotFound otherwise), as an Update's original,
// and runs validateParams over every recipe in it. It reads through the
// pool, before any transaction opens: validateParams reaches TMDB, and
// holding SQLite's write lock across that would stall every other writer.
// validateParams is required — a nil validator is a programming error, not
// "skip the check".
func (db *DB) loadCommunityCollection(ctx context.Context, profileID, sourceID uuid.UUID, validateParams CatalogParamsValidator) (takenOriginal, error) {
	if validateParams == nil {
		return takenOriginal{}, errors.New("vault: reading a community collection requires a params validator")
	}
	spec := communitySpec(profileID, validateParams)
	source, err := db.loadCopySource(ctx, sourceID, spec)
	if err != nil {
		return takenOriginal{}, err
	}
	original := takenOriginal{collection: source.Collections[0]}
	original.hash, err = bundleCollectionHash(original.collection)
	if err != nil {
		return takenOriginal{}, err
	}
	return original, spec.checkParams(original.collection.Catalogs)
}

// updateTakenCollectionTx is UpdateTakenCollection's write, inside tx.
// Returns the copy's id.
func updateTakenCollectionTx(ctx context.Context, tx *sql.Tx, profileID, sourceID uuid.UUID, original takenOriginal) (uuid.UUID, error) {
	linkedCopy, copyHash, err := linkedCollectionCopy(ctx, tx, profileID, sourceID)
	if err != nil {
		return uuid.Nil, err
	}
	switch copyHash {
	case original.hash:
		return linkedCopy.ID, writeCollectionTakenHash(ctx, tx, linkedCopy.ID, original.hash)
	case linkedCopy.TakenHash:
		return linkedCopy.ID, rewriteTakenCollection(ctx, tx, profileID, linkedCopy, original.collection)
	default:
		return uuid.Nil, conflictAfterUnlink(unlinkCollection(ctx, tx, linkedCopy.ID))
	}
}

// linkedCollectionCopy reads profileID's linked copy of sourceID and its
// hash through tx. Returns ErrCollectionNotFound if there is none.
func linkedCollectionCopy(ctx context.Context, tx *sql.Tx, profileID, sourceID uuid.UUID) (CollectionWithFolders, string, error) {
	tree, err := selectTree(ctx, tx, "owner_id = ? AND taken_from = ?", profileID.String(), sourceID.String())
	if err != nil {
		return CollectionWithFolders{}, "", err
	}
	hash, err := collectionHash(tree)
	return tree, hash, err
}

// rewriteTakenCollection writes original over linkedCopy through the update
// core (see UpdateTakenCollection), then sets taken_hash from the copy as
// stored.
func rewriteTakenCollection(ctx context.Context, tx *sql.Tx, profileID uuid.UUID, linkedCopy CollectionWithFolders, original BundleCollection) error {
	form := updateFormFromBundle(original, linkedCopy)
	if err := form.Validate(); err != nil {
		return err
	}
	if _, _, err := updateCollectionTx(ctx, tx, profileID, linkedCopy.ID, form, time.Now().UTC().Format(time.RFC3339)); err != nil {
		return err
	}
	return setCollectionTakenHash(ctx, tx, linkedCopy.ID)
}

// updateFormFromBundle builds the CollectionForm that writes original over
// linkedCopy; see UpdateTakenCollection. A catalog of original's that the copy
// has a counterpart for is passed to collectionFormFromBundle as a top-level
// key, so its refs become CatalogID refs to the counterpart, and is carried
// as a catalog edit; the rest stay in original's own list and become New
// entries linked to their source.
func updateFormFromBundle(original BundleCollection, linkedCopy CollectionWithFolders) CollectionForm {
	counterparts, edits, unmatched := matchCopyCatalogs(original.Catalogs, copyCatalogsBySource(linkedCopy))
	original.Catalogs = unmatched
	form := collectionFormFromBundle(original, counterparts, true)
	form.IsPublic = linkedCopy.IsPublic
	form.PinToTop = linkedCopy.PinToTop
	form.CatalogEdits = edits
	for i := range min(len(form.Folders), len(linkedCopy.Folders)) {
		form.Folders[i].ID = &linkedCopy.Folders[i].ID
	}
	return form
}

// copyCatalogsBySource maps the taken_from of each catalog scoped to
// linkedCopy to that catalog's id. A listed catalog the copy references is
// never a counterpart: Update only writes catalogs inside the collection.
func copyCatalogsBySource(linkedCopy CollectionWithFolders) map[uuid.UUID]uuid.UUID {
	bySource := make(map[uuid.UUID]uuid.UUID, len(linkedCopy.Catalogs))
	for _, c := range linkedCopy.Catalogs {
		if c.TakenFrom != nil && c.CollectionID != nil {
			bySource[*c.TakenFrom] = c.ID
		}
	}
	return bySource
}

// matchCopyCatalogs splits catalogs, an original's own list, by whether
// bySource names a counterpart for each one's SourceID. A matched catalog
// yields its key's counterpart id and a catalog edit writing it over that
// counterpart; the rest are returned as unmatched, in order.
func matchCopyCatalogs(catalogs []BundleCatalog, bySource map[uuid.UUID]uuid.UUID) (map[string]uuid.UUID, []ScopedCatalogEdit, []BundleCatalog) {
	counterparts := map[string]uuid.UUID{}
	edits := []ScopedCatalogEdit{}
	unmatched := []BundleCatalog{}
	for _, c := range catalogs {
		id, ok := bySource[*c.SourceID]
		if !ok {
			unmatched = append(unmatched, c)
			continue
		}
		counterparts[c.Key] = id
		edits = append(edits, ScopedCatalogEdit{
			ID: id, Type: c.Type, Provider: c.Provider, Name: c.Name, Params: string(c.Params), Fingerprint: c.Fingerprint,
		})
	}
	return counterparts, edits, unmatched
}

// runTakenUpdate runs write in a new transaction and commits it unless write
// fails. An ErrConflict from write is committed too, with whatever write
// stored before returning it, and then returned: the unlink an Update makes
// when the copy no longer matches its taken_hash.
func runTakenUpdate[T any](ctx context.Context, db *DB, write func(*sql.Tx) (T, error)) (T, error) {
	var zero T
	tx, err := db.conn.BeginTx(ctx, nil)
	if err != nil {
		return zero, fmt.Errorf("starting transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // no-op once Commit succeeds

	result, err := write(tx)
	if err != nil && !errors.Is(err, ErrConflict) {
		return zero, err
	}
	if commitErr := tx.Commit(); commitErr != nil {
		return zero, fmt.Errorf("committing transaction: %w", commitErr)
	}
	return result, err
}

// conflictAfterUnlink is the ErrConflict an Update returns once it has
// unlinked a copy that no longer matches its taken_hash, or unlinkErr if the
// unlink failed.
func conflictAfterUnlink(unlinkErr error) error {
	if unlinkErr != nil {
		return unlinkErr
	}
	return fmt.Errorf("%w: your copy has been edited, so it is no longer linked", ErrConflict)
}

// reloadCollection reads collection id's tree back through the pool after a
// write has committed.
func (db *DB) reloadCollection(ctx context.Context, id uuid.UUID) (CollectionWithFolders, error) {
	return selectTree(ctx, db.conn, "id = ?", id.String())
}

// setCollectionTakenHash sets collection id's taken_hash to the hash of its
// tree as stored in tx: the state a linked copy is in step with its original.
func setCollectionTakenHash(ctx context.Context, tx *sql.Tx, id uuid.UUID) error {
	hash, err := storedCollectionHash(ctx, tx, id)
	if err != nil {
		return err
	}
	return writeCollectionTakenHash(ctx, tx, id, hash)
}

// writeCollectionTakenHash sets collection id's taken_hash to hash.
func writeCollectionTakenHash(ctx context.Context, e execer, id uuid.UUID, hash string) error {
	if _, err := e.ExecContext(ctx, `UPDATE collections SET taken_hash = ? WHERE id = ?`, hash, id.String()); err != nil {
		return fmt.Errorf("setting collection taken_hash: %w", err)
	}
	return nil
}

// writeCatalogTakenHash sets catalog id's taken_hash to hash.
func writeCatalogTakenHash(ctx context.Context, e execer, id uuid.UUID, hash string) error {
	if _, err := e.ExecContext(ctx, `UPDATE catalogs SET taken_hash = ? WHERE id = ?`, hash, id.String()); err != nil {
		return fmt.Errorf("setting catalog taken_hash: %w", err)
	}
	return nil
}

// unlinkCollection clears collection id's link to its original.
func unlinkCollection(ctx context.Context, e execer, id uuid.UUID) error {
	if _, err := e.ExecContext(ctx, `UPDATE collections SET taken_from = NULL, taken_hash = NULL WHERE id = ?`, id.String()); err != nil {
		return fmt.Errorf("unlinking collection: %w", err)
	}
	return nil
}

// unlinkCatalog clears catalog id's link to its original.
func unlinkCatalog(ctx context.Context, e execer, id uuid.UUID) error {
	if _, err := e.ExecContext(ctx, `UPDATE catalogs SET taken_from = NULL, taken_hash = NULL WHERE id = ?`, id.String()); err != nil {
		return fmt.Errorf("unlinking catalog: %w", err)
	}
	return nil
}
