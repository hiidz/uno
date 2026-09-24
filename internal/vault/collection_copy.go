// Take and Duplicate of a whole collection: the source tree is extracted into
// its bundle form and written back as a new collection through the same
// create core a collection save uses.

package vault

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

// TakeCollection deep-copies a public collection owned by someone else —
// its cosmetics, folders, and every catalog its folders reference — into a
// new collection owned by profileID, with fresh ids throughout so the copy
// is unaffected by later changes to the source. Every copied catalog is
// scoped to the new collection, even if the source catalog was listed; a
// source catalog referenced by two folders becomes one scoped copy
// referenced twice. The new collection's taken_from is the source, and each
// catalog copy's is the catalog it was copied from. Returns
// ErrCollectionNotFound if sourceID isn't public or is already owned by
// profileID.
//
// validateParams re-checks every catalog recipe the copy would carry over,
// for the same reason [DB.TakeCatalog] re-checks the one it copies: the
// recipe is someone else's input, validated when they wrote it rather than
// when this profile takes it, and a collection take is the other way the
// very same listed row crosses the owner boundary. One rejected recipe
// fails the whole take — a half-copied collection is not a collection. It
// is required; a nil validator is a programming error, not "skip the
// check". [DB.DuplicateCollection] passes none because it copies rows this
// profile already owns.
func (db *DB) TakeCollection(ctx context.Context, profileID uuid.UUID, sourceID uuid.UUID, validateParams CatalogParamsValidator) (CollectionWithFolders, error) {
	if validateParams == nil {
		return CollectionWithFolders{}, errors.New("vault: TakeCollection requires a params validator")
	}

	return db.copyCollection(ctx, profileID, sourceID, copyCollectionSpec{
		sourceWhere:    " AND is_public = TRUE AND owner_id != ?",
		sourceArgs:     []any{profileID.String()},
		takenFrom:      &sourceID,
		scopeAll:       true,
		validateParams: validateParams,
		verb:           "taken",
	})
}

// DuplicateCollection deep-copies a collection profileID already owns —
// its cosmetics, folders, and every catalog its folders reference — into a
// new collection, also owned by profileID. Every folder ref survives the
// copy: a listed source catalog stays a reference (the new collection's
// folders point at the same row), while each distinct scoped source catalog
// becomes one fresh scoped copy in the new collection, referenced wherever
// the source referenced it. taken_from is left nil throughout: this is not a
// take, it's a fresh copy of your own data. The new collection is private
// and its title gets a " (copy)" suffix. Returns ErrCollectionNotFound if
// sourceID isn't owned by profileID.
func (db *DB) DuplicateCollection(ctx context.Context, profileID uuid.UUID, sourceID uuid.UUID) (CollectionWithFolders, error) {
	return db.copyCollection(ctx, profileID, sourceID, copyCollectionSpec{
		sourceWhere: " AND owner_id = ?",
		sourceArgs:  []any{profileID.String()},
		titleSuffix: " (copy)",
		verb:        "duplicated",
	})
}

// copyCollectionSpec describes one collection copy for copyCollection: which
// source rows qualify, what the copy is titled, what it stamps as taken_from,
// and where its catalogs go.
//
// sourceWhere is ANDed onto the source lookup's "id = ?" and sourceArgs fill
// its placeholders; both are internal literals, never client input. scopeAll
// makes every catalog the source references a scoped copy in the new
// collection; without it a listed catalog stays a reference (extractBundle).
// takenFrom, when set, is the new collection's taken_from, and each catalog
// copy's taken_from is then the catalog it was copied from. verb names the
// operation in the post-insert reload's error message.
//
// validateParams re-checks the recipes the copy writes as new rows before
// anything is written, and is nil for a copy that stays within one profile —
// see [DB.TakeCollection] for why only a take needs it.
type copyCollectionSpec struct {
	sourceWhere    string
	sourceArgs     []any
	titleSuffix    string
	takenFrom      *uuid.UUID
	scopeAll       bool
	validateParams CatalogParamsValidator
	verb           string
}

// copyCollection runs one collection copy end to end: extract the source
// tree, check it, write the copy, commit, and read the result back in the
// nested shape the handler returns. Returns ErrCollectionNotFound if
// spec.sourceWhere excludes sourceID.
//
// The source is checked as the form that writes the copy, by
// CollectionForm.Validate, the rules a collection save runs. A Take copies
// rows this profile never authored, straight into its own push to Nuvio, so
// being already stored is not evidence a row was ever checked — rows written
// before a given check existed reach here too. The title is checked with its
// suffix, so a title already at maxNameLen is refused rather than copied into
// a row the collection editor's own save would then refuse.
//
// The load and the checks run against the pool, before the transaction
// opens. spec.validateParams reaches TMDB, and holding SQLite's write lock
// across a network call would stall every other writer for the length of
// it — the same read-then-validate-then-insert order [DB.TakeCatalog] uses.
// The copy is a snapshot either way, so a source edit landing in the gap
// only means copying the slightly older tree.
func (db *DB) copyCollection(ctx context.Context, profileID, sourceID uuid.UUID, spec copyCollectionSpec) (CollectionWithFolders, error) {
	source, err := db.loadCopySource(ctx, sourceID, spec)
	if err != nil {
		return CollectionWithFolders{}, err
	}
	form := collectionFormFromBundle(source.Collections[0], topSourceIDs(source.Catalogs), spec.takenFrom != nil)
	form.Title += spec.titleSuffix
	if err := form.Validate(); err != nil {
		return CollectionWithFolders{}, err
	}
	if err := spec.checkParams(source.Collections[0].Catalogs); err != nil {
		return CollectionWithFolders{}, err
	}

	tx, err := db.conn.BeginTx(ctx, nil)
	if err != nil {
		return CollectionWithFolders{}, fmt.Errorf("starting transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // no-op once Commit succeeds

	created, _, err := createCollectionTx(ctx, tx, profileID, form, spec.takenFrom)
	if err != nil {
		return CollectionWithFolders{}, err
	}

	if err := tx.Commit(); err != nil {
		return CollectionWithFolders{}, fmt.Errorf("committing transaction: %w", err)
	}

	trees, err := db.GetCollectionsByIDs(ctx, []uuid.UUID{created.ID})
	if err != nil {
		return CollectionWithFolders{}, err
	}
	if len(trees) == 0 {
		return CollectionWithFolders{}, fmt.Errorf("loading %s collection %s: not found after insert", spec.verb, created.ID)
	}
	return trees[0], nil
}

// loadCopySource reads sourceID's tree through the pool, subject to
// spec.sourceWhere, and extracts it as a one-collection Bundle. Returns
// ErrCollectionNotFound if spec.sourceWhere excludes sourceID.
//
// A tree's refs and the catalogs they name are read in separate queries, so a
// delete landing between them leaves a ref with no catalog. extractBundle
// drops that ref: folder_catalogs.catalog_id is a live foreign key, and
// carrying it through would fail the whole copy.
func (db *DB) loadCopySource(ctx context.Context, sourceID uuid.UUID, spec copyCollectionSpec) (Bundle, error) {
	args := append([]any{sourceID.String()}, spec.sourceArgs...)
	collections, err := db.queryCollections(ctx, "id = ?"+spec.sourceWhere, args...)
	if err != nil {
		return Bundle{}, err
	}
	if len(collections) == 0 {
		return Bundle{}, ErrCollectionNotFound
	}
	trees, err := assembleCollectionTree(ctx, db.conn, collections)
	if err != nil {
		return Bundle{}, err
	}
	return extractBundle(nil, trees, spec.scopeAll), nil
}

// topSourceIDs maps each top-level catalog's key to the row it was extracted
// from. A copy that leaves listed catalogs top-level is a Duplicate, whose
// listed catalogs are the caller's own, so its refs to them point at those
// same rows.
func topSourceIDs(catalogs []BundleCatalog) map[string]uuid.UUID {
	ids := make(map[string]uuid.UUID, len(catalogs))
	for _, c := range catalogs {
		ids[c.Key] = *c.SourceID
	}
	return ids
}

// checkParams runs spec.validateParams over catalogs, the ones a copy writes
// as new rows. A copy with no validator checks nothing.
func (spec copyCollectionSpec) checkParams(catalogs []BundleCatalog) error {
	if spec.validateParams == nil {
		return nil
	}
	for _, c := range catalogs {
		if err := spec.validateParams(c.Type, c.Provider, string(c.Params)); err != nil {
			return err
		}
	}
	return nil
}
