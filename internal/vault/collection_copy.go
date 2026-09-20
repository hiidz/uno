// Take and Duplicate: the machinery that copies one collection tree
// into a new one.

package vault

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// sourceFolder is one folder of a collection tree being copied — loaded by
// loadSourceCollectionTree, shared by TakeCollection and DuplicateCollection.
type sourceFolder struct {
	id   uuid.UUID
	data FolderData
	refs []FolderRef // source catalog ids, in order
}

// sourceCollection is a collection tree loaded for copying: the cosmetics a
// copy carries over, its folders in sort order, each with its own ordered
// catalog refs, and every distinct catalog those refs name. Produced by
// loadSourceCollectionTree, consumed by copyCollectionTree.
type sourceCollection struct {
	title            string
	viewMode         string
	backdropImageURL string
	pinToTop         bool
	showAllTab       bool
	focusGlowEnabled bool
	folders          []sourceFolder
	catalogs         []sourceCatalog // first-seen order across folders
}

// sourceCatalog is one catalog row a collection tree's folders reference,
// loaded whole so the copy can re-insert it. scoped records whether the
// source row was scoped to its own collection rather than listed — the
// distinction copyCollectionTree's copyListedRefs turns on.
type sourceCatalog struct {
	id                                       uuid.UUID
	typ, name, provider, params, fingerprint string
	scoped                                   bool
}

// sourceQuerier is the read side loadSourceCollectionTree needs: the
// collection row through QueryRowContext, its folders and catalogs through
// QueryContext. Both *sql.DB and *sql.Tx satisfy it, and copyCollection
// reads through the pool so the params check can run before the write
// transaction opens.
type sourceQuerier interface {
	queryRower
	querier
}

// loadSourceCollectionTree loads sourceID's cosmetics (subject to
// whereExtra, which distinguishes a Take's "public and not mine" from a
// Duplicate's "mine") plus its folders, each folder's ordered catalog refs,
// and the catalog rows those refs name. It reads through q rather than the
// copy's own transaction: everything a copy writes is derived here, so the
// whole read — and the validation copyCollection runs on its result — can
// finish before a write transaction opens. Returns ErrCollectionNotFound if
// whereExtra excludes sourceID.
func loadSourceCollectionTree(ctx context.Context, q sourceQuerier, sourceID uuid.UUID, whereExtra string, whereArgs ...any) (sourceCollection, error) {
	var src sourceCollection
	var pinToTop, showAllTab, focusGlowEnabled int

	args := append([]any{sourceID.String()}, whereArgs...)
	err := q.QueryRowContext(ctx, `
		SELECT title, pin_to_top, view_mode, show_all_tab, backdrop_image_url, focus_glow_enabled
		FROM collections WHERE id = ?`+whereExtra, args...,
	).Scan(&src.title, &pinToTop, &src.viewMode, &showAllTab, &src.backdropImageURL, &focusGlowEnabled)
	if errors.Is(err, sql.ErrNoRows) {
		return sourceCollection{}, ErrCollectionNotFound
	}
	if err != nil {
		return sourceCollection{}, fmt.Errorf("loading source collection: %w", err)
	}
	src.pinToTop = pinToTop != 0
	src.showAllTab = showAllTab != 0
	src.focusGlowEnabled = focusGlowEnabled != 0

	src.folders, err = loadSourceFolders(ctx, q, sourceID)
	if err != nil {
		return sourceCollection{}, err
	}
	for i := range src.folders {
		refs, err := loadSourceFolderRefs(ctx, q, src.folders[i].id)
		if err != nil {
			return sourceCollection{}, err
		}
		src.folders[i].refs = refs
	}

	src.catalogs, err = loadSourceCatalogs(ctx, q, distinctRefCatalogIDs(src.folders))
	if err != nil {
		return sourceCollection{}, err
	}

	if err := validateSourceMediaURLs(src); err != nil {
		return sourceCollection{}, err
	}

	return src, nil
}

// distinctRefCatalogIDs collects the catalog ids folders reference, deduped
// in first-seen order — the order that decides which single copy a catalog
// referenced by two folders collapses into.
func distinctRefCatalogIDs(folders []sourceFolder) []uuid.UUID {
	var ids []uuid.UUID
	seen := map[uuid.UUID]bool{}
	for _, f := range folders {
		for _, ref := range f.refs {
			if !seen[ref.CatalogID] {
				seen[ref.CatalogID] = true
				ids = append(ids, ref.CatalogID)
			}
		}
	}
	return ids
}

// loadSourceCatalogs reads the catalog rows a collection tree references,
// returned in ids order. An id with no row is skipped rather than being an
// error, so a ref left dangling by a concurrent delete drops out of the
// copy instead of failing it.
func loadSourceCatalogs(ctx context.Context, q querier, ids []uuid.UUID) ([]sourceCatalog, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	placeholders, args := buildInClause(ids)
	rows, err := q.QueryContext(ctx, fmt.Sprintf(`
		SELECT id, type, name, provider, params, fingerprint, collection_id FROM catalogs WHERE id IN (%s)
	`, placeholders), args...)
	if err != nil {
		return nil, fmt.Errorf("querying source catalogs: %w", err)
	}
	defer rows.Close()

	byID := make(map[uuid.UUID]sourceCatalog, len(ids))
	for rows.Next() {
		var sc sourceCatalog
		var idStr string
		var collectionID sql.NullString
		if err := rows.Scan(&idStr, &sc.typ, &sc.name, &sc.provider, &sc.params, &sc.fingerprint, &collectionID); err != nil {
			return nil, fmt.Errorf("scanning source catalog: %w", err)
		}
		sc.id, err = parseUUID(idStr, "catalog id")
		if err != nil {
			return nil, err
		}
		sc.scoped = collectionID.Valid
		byID[sc.id] = sc
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating source catalogs: %w", err)
	}

	catalogs := make([]sourceCatalog, 0, len(byID))
	for _, id := range ids {
		if sc, ok := byID[id]; ok {
			catalogs = append(catalogs, sc)
		}
	}
	return catalogs, nil
}

// validateSourceMediaURLs re-checks the media URLs of a collection tree
// about to be copied, using the same rules CollectionForm.Validate applies
// to a save. A Take copies a row this profile never authored, and those
// URLs go straight into the taker's own push to Nuvio, so being already
// stored is not evidence they were ever checked — rows written before that
// check existed reach here too.
func validateSourceMediaURLs(src sourceCollection) error {
	var problems []string
	if p := mediaURLProblem("backdrop image url", src.backdropImageURL); p != "" {
		problems = append(problems, p)
	}
	for i, f := range src.folders {
		problems = append(problems, folderMediaURLProblems(i, f.data)...)
	}

	if len(problems) == 0 {
		return nil
	}
	return fmt.Errorf("%w: source collection: %s", ErrInvalidInput, strings.Join(problems, "; "))
}

// loadSourceFolders reads collectionID's folders in sort order, without
// their catalog refs — loadSourceFolderRefs fills those in per folder, which
// is also what keeps each query's rows closed by its own defer.
func loadSourceFolders(ctx context.Context, q querier, collectionID uuid.UUID) ([]sourceFolder, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT id, title, tile_shape, hide_title, cover_emoji, cover_image_url,
		       focus_gif_url, focus_gif_enabled, hero_backdrop_url, hero_video_url, title_logo_url
		FROM folders WHERE collection_id = ? ORDER BY sort_order
	`, collectionID.String())
	if err != nil {
		return nil, fmt.Errorf("querying source folders: %w", err)
	}
	defer rows.Close()

	var folders []sourceFolder
	for rows.Next() {
		var idStr string
		var fd FolderData
		var hideTitle, focusGIFEnabled int
		if err := rows.Scan(&idStr, &fd.Title, &fd.TileShape, &hideTitle, &fd.CoverEmoji, &fd.CoverImageURL,
			&fd.FocusGIFURL, &focusGIFEnabled, &fd.HeroBackdropURL, &fd.HeroVideoURL, &fd.TitleLogoURL); err != nil {
			return nil, fmt.Errorf("scanning source folder: %w", err)
		}
		id, err := parseUUID(idStr, "folder id")
		if err != nil {
			return nil, err
		}
		fd.HideTitle = hideTitle != 0
		fd.FocusGIFEnabled = focusGIFEnabled != 0
		folders = append(folders, sourceFolder{id: id, data: fd})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating source folders: %w", err)
	}
	return folders, nil
}

// loadSourceFolderRefs reads one source folder's catalog refs, in order.
func loadSourceFolderRefs(ctx context.Context, q querier, folderID uuid.UUID) ([]FolderRef, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT catalog_id, genre FROM folder_catalogs WHERE folder_id = ? ORDER BY sort_order
	`, folderID.String())
	if err != nil {
		return nil, fmt.Errorf("querying source folder catalogs: %w", err)
	}
	defer rows.Close()

	var refs []FolderRef
	for rows.Next() {
		var catIDStr, genre string
		if err := rows.Scan(&catIDStr, &genre); err != nil {
			return nil, fmt.Errorf("scanning source folder catalog: %w", err)
		}
		catID, err := parseUUID(catIDStr, "catalog id")
		if err != nil {
			return nil, err
		}
		refs = append(refs, FolderRef{CatalogID: catID, Genre: genre})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating source folder catalogs: %w", err)
	}
	return refs, nil
}

// copyCollectionTree inserts a new collection owned by profileID — src's
// title and cosmetics — plus src's folders, refs remapped through an old-id→new-id map built as
// follows for each distinct catalog id folders reference (first-seen
// order, so a catalog referenced by two folders collapses into one copy
// referenced twice):
//
//   - copyListedRefs true (TakeCollection): every distinct catalog, listed
//     or scoped on the source side, becomes a fresh scoped copy in the new
//     collection — crossing the owner boundary means there is no existing
//     row of the taker's to point at instead.
//   - copyListedRefs false (DuplicateCollection): a listed source catalog
//     stays a reference — same id, same owner — while a catalog scoped to
//     the source collection still becomes a fresh scoped copy. Every
//     catalog a source collection's folders can reference is, by
//     validateFolderRefs, either listed or scoped to that same source
//     collection, so those two cases are exhaustive.
//
// takenFrom is stamped on the new collection row and, when copyListedRefs is
// also true, on every copied catalog row too: TakeCollection passes the
// source id for both; DuplicateCollection passes nil for both — a
// duplicate is a fresh fact, not a copy taken from someone else. Must run
// inside tx; the caller commits. Every row it writes comes from src, which
// loadSourceCollectionTree already read, so the transaction holds a write
// lock for inserts alone.
func copyCollectionTree(ctx context.Context, tx *sql.Tx, profileID uuid.UUID, src sourceCollection, takenFrom *uuid.UUID, copyListedRefs bool) (uuid.UUID, error) {
	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339)

	newCollectionID := uuid.New()
	_, err := tx.ExecContext(ctx, `
		INSERT INTO collections (id, title, owner_id, is_public, is_default, pin_to_top, view_mode, show_all_tab,
		                          backdrop_image_url, focus_glow_enabled, taken_from, created_at, updated_at)
		VALUES (?, ?, ?, 0, 0, ?, ?, ?, ?, ?, ?, ?, ?)
	`, newCollectionID.String(), src.title, profileID.String(), src.pinToTop, src.viewMode, src.showAllTab,
		src.backdropImageURL, src.focusGlowEnabled, nullableUUIDString(takenFrom), nowStr, nowStr)
	if err != nil {
		return uuid.Nil, fmt.Errorf("inserting copied collection: %w", err)
	}

	idMap := make(map[uuid.UUID]uuid.UUID, len(src.catalogs))
	for _, sc := range src.catalogs {
		// A listed source catalog, when copyListedRefs is false, stays a
		// reference — no new row, the new collection's folders point at
		// the same id the caller already owns.
		if !copyListedRefs && !sc.scoped {
			idMap[sc.id] = sc.id
			continue
		}

		newID := uuid.New()
		idMap[sc.id] = newID
		// Each catalog copy's own taken_from points at the catalog it was
		// copied from — but only when this is a Take (takenFrom != nil).
		// A Duplicate copies your own data, so its scoped catalog copies
		// get no taken_from either, matching the new collection row.
		var catalogTakenFrom *uuid.UUID
		if takenFrom != nil {
			catalogTakenFrom = &sc.id
		}
		_, err = tx.ExecContext(ctx, `
			INSERT INTO catalogs (id, type, name, provider, params, owner_id, is_public, is_default,
			                       collection_id, taken_from, fingerprint, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, 0, 0, ?, ?, ?, ?, ?)
		`, newID.String(), sc.typ, sc.name, sc.provider, sc.params, profileID.String(),
			newCollectionID.String(), nullableUUIDString(catalogTakenFrom), sc.fingerprint, nowStr, nowStr)
		if err != nil {
			return uuid.Nil, fmt.Errorf("inserting copied scoped catalog: %w", err)
		}
	}

	for i, f := range src.folders {
		newFolder, err := insertFolder(ctx, tx, newCollectionID, i, f.data)
		if err != nil {
			return uuid.Nil, err
		}
		// A ref whose catalog loadSourceCatalogs didn't find is dropped
		// rather than remapped: the refs and the catalog rows are read in
		// two queries, so a delete landing between them leaves a ref with
		// nothing to point at, and folder_catalogs.catalog_id is a live
		// foreign key — carrying it through would fail the whole copy.
		newRefs := make([]FolderRef, 0, len(f.refs))
		for _, ref := range f.refs {
			if newID, ok := idMap[ref.CatalogID]; ok {
				newRefs = append(newRefs, FolderRef{CatalogID: newID, Genre: ref.Genre})
			}
		}
		if err := rewriteFolderCatalogRefs(ctx, tx, newFolder.ID, newRefs); err != nil {
			return uuid.Nil, err
		}
	}

	return newCollectionID, nil
}

// TakeCollection deep-copies a public collection owned by someone else —
// its cosmetics, folders, and every catalog its folders reference — into a
// new collection owned by profileID, with fresh ids throughout so the copy
// is unaffected by later changes to the source. Every copied catalog is
// scoped to the new collection, even if the source
// catalog was listed; a source catalog referenced by two folders becomes one
// scoped copy referenced twice. Returns ErrCollectionNotFound if sourceID
// isn't public or is already owned by profileID.
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
		copyListedRefs: true,
		validateParams: validateParams,
		verb:           "taken",
	})
}

// DuplicateCollection deep-copies a collection profileID already owns —
// its cosmetics, folders, and every catalog its folders reference — into a
// new collection, also owned by profileID. Every folder ref survives the
// copy: a listed source catalog stays a reference (the new collection's
// folders point at the same row),
// while each distinct scoped source catalog becomes a fresh scoped copy in
// the new collection — the same one-copy-per-distinct-source-catalog rule
// TakeCollection uses, so a catalog referenced by two folders collapses into
// one new scoped copy referenced twice. taken_from is left nil throughout:
// this is not a take, it's a fresh copy of your own data. The new
// collection is never born public and its title gets a "(copy)" suffix, the
// same two rules the frontend used to apply client-side before this existed
// as a single atomic server operation. Returns ErrCollectionNotFound if
// sourceID isn't owned by profileID.
func (db *DB) DuplicateCollection(ctx context.Context, profileID uuid.UUID, sourceID uuid.UUID) (CollectionWithFolders, error) {
	// copyCollectionTree always inserts is_public = 0, so "born never public"
	// falls out of reusing it rather than needing its own line here.
	return db.copyCollection(ctx, profileID, sourceID, copyCollectionSpec{
		sourceWhere:    " AND owner_id = ?",
		sourceArgs:     []any{profileID.String()},
		titleSuffix:    " (copy)",
		copyListedRefs: false,
		verb:           "duplicated",
	})
}

// copyCollectionSpec describes one collection copy for copyCollection: which
// source rows qualify, what the copy is titled, what it stamps as taken_from,
// and whether its folders' listed catalog refs are copied or reused — see
// copyCollectionTree for what copyListedRefs decides.
//
// sourceWhere is ANDed onto the source lookup's "id = ?" and sourceArgs fill
// its placeholders; both are internal literals, never client input. verb
// names the operation in the post-insert reload's error message.
//
// validateParams re-checks the source recipes before anything is written,
// and is nil for a copy that stays within one profile — see
// [DB.TakeCollection] for why only a take needs it.
type copyCollectionSpec struct {
	sourceWhere    string
	sourceArgs     []any
	titleSuffix    string
	takenFrom      *uuid.UUID
	copyListedRefs bool
	validateParams CatalogParamsValidator
	verb           string
}

// copyCollection runs one collection copy end to end: load the source tree,
// check it, write the copy, commit, and read the result back in the nested
// shape the handler returns. Returns ErrCollectionNotFound if
// spec.sourceWhere excludes sourceID.
//
// The load and the checks run against the pool, before the transaction
// opens. spec.validateParams reaches TMDB, and holding SQLite's write lock
// across a network call would stall every other writer for the length of
// it — the same read-then-validate-then-insert order [DB.TakeCatalog] uses.
// The copy is a snapshot either way, so a source edit landing in the gap
// only means copying the slightly older tree.
func (db *DB) copyCollection(ctx context.Context, profileID, sourceID uuid.UUID, spec copyCollectionSpec) (CollectionWithFolders, error) {
	source, err := loadSourceCollectionTree(ctx, db.conn, sourceID, spec.sourceWhere, spec.sourceArgs...)
	if err != nil {
		return CollectionWithFolders{}, err
	}
	source.title += spec.titleSuffix

	if spec.validateParams != nil {
		for _, sc := range source.catalogs {
			if err := spec.validateParams(sc.typ, sc.provider, sc.params); err != nil {
				return CollectionWithFolders{}, err
			}
		}
	}

	tx, err := db.conn.BeginTx(ctx, nil)
	if err != nil {
		return CollectionWithFolders{}, fmt.Errorf("starting transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // no-op once Commit succeeds

	newCollectionID, err := copyCollectionTree(ctx, tx, profileID, source, spec.takenFrom, spec.copyListedRefs)
	if err != nil {
		return CollectionWithFolders{}, err
	}

	if err := tx.Commit(); err != nil {
		return CollectionWithFolders{}, fmt.Errorf("committing transaction: %w", err)
	}

	trees, err := db.GetCollectionsByIDs(ctx, []uuid.UUID{newCollectionID})
	if err != nil {
		return CollectionWithFolders{}, err
	}
	if len(trees) == 0 {
		return CollectionWithFolders{}, fmt.Errorf("loading %s collection %s: not found after insert", spec.verb, newCollectionID)
	}
	return trees[0], nil
}
