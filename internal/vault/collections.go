package vault

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

// queryCollections runs a SELECT over collections with the given WHERE
// clause and args, parsing the result rows.
func (db *DB) queryCollections(ctx context.Context, where string, args ...any) ([]Collection, error) {
	rows, err := db.conn.QueryContext(ctx, `
		SELECT id, title, owner_id, is_public, is_default, pin_to_top, view_mode, show_all_tab, backdrop_image_url,
		       focus_glow_enabled, home_sort_order, version, pushed_version, taken_from, created_at, updated_at
		FROM collections
		WHERE `+where, args...)
	if err != nil {
		return nil, fmt.Errorf("querying collections: %w", err)
	}
	defer rows.Close()

	collections, err := parseCollections(rows)
	if err != nil {
		return nil, fmt.Errorf("parsing collection rows: %w", err)
	}

	return collections, nil
}

// GetUserCollections returns the collections owned by profileID, each with
// its folders assembled.
func (db *DB) GetUserCollections(ctx context.Context, profileID uuid.UUID) ([]CollectionWithFolders, error) {
	collections, err := db.queryCollections(ctx, "owner_id = ?", profileID.String())
	if err != nil {
		return nil, err
	}
	return db.assembleCollectionTree(ctx, collections)
}

// GetCommunityCollections returns the community collection list for
// profileID: every collection marked public and owned by someone else, each
// with its folders assembled, sorted by title, then created_at, then id so
// equal titles don't swap between requests (no fingerprint collapse —
// that's a catalog-only concept), flagged with whether profileID has
// already taken a copy.
func (db *DB) GetCommunityCollections(ctx context.Context, profileID uuid.UUID) ([]CommunityCollection, error) {
	collections, err := db.queryCollections(ctx, "is_public = TRUE AND owner_id != ?", profileID.String())
	if err != nil {
		return nil, err
	}
	sort.Slice(collections, func(i, j int) bool {
		a, b := collections[i], collections[j]
		if a.Title != b.Title {
			return a.Title < b.Title
		}
		if !a.CreatedAt.Equal(b.CreatedAt) {
			return a.CreatedAt.Before(b.CreatedAt)
		}
		return a.ID.String() < b.ID.String()
	})

	trees, err := db.assembleCollectionTree(ctx, collections)
	if err != nil {
		return nil, err
	}

	taken, err := db.takenSourceIDs(ctx, "collections", profileID)
	if err != nil {
		return nil, err
	}

	out := make([]CommunityCollection, len(trees))
	for i, c := range trees {
		out[i] = CommunityCollection{CollectionWithFolders: c, Taken: taken[c.ID]}
	}
	return out, nil
}

// sourceFolder is one folder of a collection tree being copied — loaded by
// loadSourceCollectionTree, shared by TakeCollection and DuplicateCollection.
type sourceFolder struct {
	id   uuid.UUID
	data FolderData
	refs []FolderRef // source catalog ids, in order
}

// loadSourceCollectionTree loads sourceID's cosmetics (subject to
// whereExtra, which distinguishes a Take's "public and not mine" from a
// Duplicate's "mine") plus its folders and each folder's ordered catalog
// refs, all inside tx so the read is part of the same transaction the copy
// commits in. Returns ErrCollectionNotFound if whereExtra excludes sourceID.
func loadSourceCollectionTree(ctx context.Context, tx *sql.Tx, sourceID uuid.UUID, whereExtra string, whereArgs ...any) (title, viewMode, backdropImageURL string, pinToTop, showAllTab, focusGlowEnabled int, folders []sourceFolder, err error) {
	args := append([]any{sourceID.String()}, whereArgs...)
	err = tx.QueryRowContext(ctx, `
		SELECT title, pin_to_top, view_mode, show_all_tab, backdrop_image_url, focus_glow_enabled
		FROM collections WHERE id = ?`+whereExtra, args...,
	).Scan(&title, &pinToTop, &viewMode, &showAllTab, &backdropImageURL, &focusGlowEnabled)
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrCollectionNotFound
		return
	}
	if err != nil {
		err = fmt.Errorf("loading source collection: %w", err)
		return
	}

	folderRows, ferr := tx.QueryContext(ctx, `
		SELECT id, title, tile_shape, hide_title, cover_emoji, cover_image_url,
		       focus_gif_url, focus_gif_enabled, hero_backdrop_url, hero_video_url, title_logo_url
		FROM folders WHERE collection_id = ? ORDER BY sort_order
	`, sourceID.String())
	if ferr != nil {
		err = fmt.Errorf("querying source folders: %w", ferr)
		return
	}
	for folderRows.Next() {
		var idStr string
		var fd FolderData
		var hideTitleInt, focusGIFEnabledInt int
		if serr := folderRows.Scan(&idStr, &fd.Title, &fd.TileShape, &hideTitleInt, &fd.CoverEmoji, &fd.CoverImageURL,
			&fd.FocusGIFURL, &focusGIFEnabledInt, &fd.HeroBackdropURL, &fd.HeroVideoURL, &fd.TitleLogoURL); serr != nil {
			folderRows.Close()
			err = fmt.Errorf("scanning source folder: %w", serr)
			return
		}
		id, perr := parseUUID(idStr, "folder id")
		if perr != nil {
			folderRows.Close()
			err = perr
			return
		}
		fd.HideTitle = hideTitleInt != 0
		fd.FocusGIFEnabled = focusGIFEnabledInt != 0
		folders = append(folders, sourceFolder{id: id, data: fd})
	}
	if ferr := folderRows.Err(); ferr != nil {
		folderRows.Close()
		err = fmt.Errorf("iterating source folders: %w", ferr)
		return
	}
	folderRows.Close()

	for i := range folders {
		catRows, cerr := tx.QueryContext(ctx, `
			SELECT catalog_id, genre FROM folder_catalogs WHERE folder_id = ? ORDER BY sort_order
		`, folders[i].id.String())
		if cerr != nil {
			err = fmt.Errorf("querying source folder catalogs: %w", cerr)
			return
		}
		for catRows.Next() {
			var catIDStr, genre string
			if serr := catRows.Scan(&catIDStr, &genre); serr != nil {
				catRows.Close()
				err = fmt.Errorf("scanning source folder catalog: %w", serr)
				return
			}
			catID, perr := parseUUID(catIDStr, "catalog id")
			if perr != nil {
				catRows.Close()
				err = perr
				return
			}
			folders[i].refs = append(folders[i].refs, FolderRef{CatalogID: catID, Genre: genre})
		}
		if cerr := catRows.Err(); cerr != nil {
			catRows.Close()
			err = fmt.Errorf("iterating source folder catalogs: %w", cerr)
			return
		}
		catRows.Close()
	}

	return
}

// copyCollectionTree inserts a new collection owned by profileID — titled
// title, cosmetics from the loadSourceCollectionTree call that produced
// pinToTop/viewMode/showAllTab/backdropImageURL/focusGlowEnabled and folders — plus the
// folders themselves, refs remapped through an old-id→new-id map built as
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
// inside tx; the caller commits.
func copyCollectionTree(ctx context.Context, tx *sql.Tx, profileID uuid.UUID, title, viewMode, backdropImageURL string, pinToTop, showAllTab, focusGlowEnabled int, folders []sourceFolder, takenFrom *uuid.UUID, copyListedRefs bool) (uuid.UUID, error) {
	var distinctCatalogIDs []uuid.UUID
	seenCatalog := map[uuid.UUID]bool{}
	for _, f := range folders {
		for _, ref := range f.refs {
			if !seenCatalog[ref.CatalogID] {
				seenCatalog[ref.CatalogID] = true
				distinctCatalogIDs = append(distinctCatalogIDs, ref.CatalogID)
			}
		}
	}

	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339)

	newCollectionID := uuid.New()
	_, err := tx.ExecContext(ctx, `
		INSERT INTO collections (id, title, owner_id, is_public, is_default, pin_to_top, view_mode, show_all_tab,
		                          backdrop_image_url, focus_glow_enabled, taken_from, created_at, updated_at)
		VALUES (?, ?, ?, 0, 0, ?, ?, ?, ?, ?, ?, ?, ?)
	`, newCollectionID.String(), title, profileID.String(), pinToTop, viewMode, showAllTab, backdropImageURL,
		focusGlowEnabled, nullableUUIDString(takenFrom), nowStr, nowStr)
	if err != nil {
		return uuid.Nil, fmt.Errorf("inserting copied collection: %w", err)
	}

	idMap := make(map[uuid.UUID]uuid.UUID, len(distinctCatalogIDs))
	if len(distinctCatalogIDs) > 0 {
		placeholders, args := buildInClause(distinctCatalogIDs)
		catRows, err := tx.QueryContext(ctx, fmt.Sprintf(`
			SELECT id, type, name, provider, params, fingerprint, collection_id FROM catalogs WHERE id IN (%s)
		`, placeholders), args...)
		if err != nil {
			return uuid.Nil, fmt.Errorf("querying source catalogs: %w", err)
		}
		type sourceCatalog struct {
			id, typ, name, provider, params, fingerprint string
			collectionID                                 sql.NullString
		}
		var sourceCatalogs []sourceCatalog
		for catRows.Next() {
			var sc sourceCatalog
			if err := catRows.Scan(&sc.id, &sc.typ, &sc.name, &sc.provider, &sc.params, &sc.fingerprint, &sc.collectionID); err != nil {
				catRows.Close()
				return uuid.Nil, fmt.Errorf("scanning source catalog: %w", err)
			}
			sourceCatalogs = append(sourceCatalogs, sc)
		}
		if err := catRows.Err(); err != nil {
			catRows.Close()
			return uuid.Nil, fmt.Errorf("iterating source catalogs: %w", err)
		}
		catRows.Close()

		for _, sc := range sourceCatalogs {
			oldID, err := parseUUID(sc.id, "catalog id")
			if err != nil {
				return uuid.Nil, err
			}

			// A listed source catalog, when copyListedRefs is false, stays a
			// reference — no new row, the new collection's folders point at
			// the same id the caller already owns.
			if !copyListedRefs && !sc.collectionID.Valid {
				idMap[oldID] = oldID
				continue
			}

			newID := uuid.New()
			idMap[oldID] = newID
			// Each catalog copy's own taken_from points at the catalog it was
			// copied from — but only when this is a Take (takenFrom != nil).
			// A Duplicate copies your own data, so its scoped catalog copies
			// get no taken_from either, matching the new collection row.
			var catalogTakenFrom *uuid.UUID
			if takenFrom != nil {
				catalogTakenFrom = &oldID
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
	}

	for i, f := range folders {
		newFolder, err := insertFolder(ctx, tx, newCollectionID, i, f.data)
		if err != nil {
			return uuid.Nil, err
		}
		newRefs := make([]FolderRef, len(f.refs))
		for j, ref := range f.refs {
			newRefs[j] = FolderRef{CatalogID: idMap[ref.CatalogID], Genre: ref.Genre}
		}
		if err := replaceFolderCatalogRefs(ctx, tx, newFolder.ID, newRefs); err != nil {
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
func (db *DB) TakeCollection(ctx context.Context, profileID uuid.UUID, sourceID uuid.UUID) (CollectionWithFolders, error) {
	tx, err := db.conn.BeginTx(ctx, nil)
	if err != nil {
		return CollectionWithFolders{}, fmt.Errorf("starting transaction: %w", err)
	}
	defer tx.Rollback() // no-op once Commit succeeds

	title, viewMode, backdropImageURL, pinToTop, showAllTab, focusGlowEnabled, folders, err :=
		loadSourceCollectionTree(ctx, tx, sourceID, " AND is_public = TRUE AND owner_id != ?", profileID.String())
	if err != nil {
		return CollectionWithFolders{}, err
	}

	newCollectionID, err := copyCollectionTree(ctx, tx, profileID, title, viewMode, backdropImageURL, pinToTop, showAllTab, focusGlowEnabled, folders, &sourceID, true)
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
		return CollectionWithFolders{}, fmt.Errorf("loading taken collection %s: not found after insert", newCollectionID)
	}
	return trees[0], nil
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
	tx, err := db.conn.BeginTx(ctx, nil)
	if err != nil {
		return CollectionWithFolders{}, fmt.Errorf("starting transaction: %w", err)
	}
	defer tx.Rollback() // no-op once Commit succeeds

	// copyCollectionTree always inserts is_public = 0, so "born never public"
	// falls out of reusing it rather than needing its own line here.
	title, viewMode, backdropImageURL, pinToTop, showAllTab, focusGlowEnabled, folders, err :=
		loadSourceCollectionTree(ctx, tx, sourceID, " AND owner_id = ?", profileID.String())
	if err != nil {
		return CollectionWithFolders{}, err
	}

	newCollectionID, err := copyCollectionTree(ctx, tx, profileID, title+" (copy)", viewMode, backdropImageURL, pinToTop, showAllTab, focusGlowEnabled, folders, nil, false)
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
		return CollectionWithFolders{}, fmt.Errorf("loading duplicated collection %s: not found after insert", newCollectionID)
	}
	return trees[0], nil
}

// GetCollectionsByIDs batch-loads collections (with folders) by id, no
// ownership check and no ordering guarantee — push uses this to resolve the
// pending collection selection straight from the request body rather than
// reading the persisted selection back, so it needs the same shape
// GetCurrentCollectionSelection returns, keyed by an explicit id list
// instead of a home_sort_order filter. Mirrors internal/vault/catalogs.go's
// GetCatalogsByIDs.
func (db *DB) GetCollectionsByIDs(ctx context.Context, ids []uuid.UUID) ([]CollectionWithFolders, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	placeholders, args := buildInClause(ids)
	collections, err := db.queryCollections(ctx, fmt.Sprintf("id IN (%s)", placeholders), args...)
	if err != nil {
		return nil, err
	}
	return db.assembleCollectionTree(ctx, collections)
}

// GetOwnedCollectionIDs lists just the IDs of collections this profile owns,
// skipping the folder-tree assembly GetUserCollections does — push's merge
// only needs identity, to know which entries in a pulled Nuvio blob are
// Uno's own and safe to replace or drop.
func (db *DB) GetOwnedCollectionIDs(ctx context.Context, profileID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := db.conn.QueryContext(ctx, `SELECT id FROM collections WHERE owner_id = ?`, profileID.String())
	if err != nil {
		return nil, fmt.Errorf("querying owned collection ids: %w", err)
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var idStr string
		if err := rows.Scan(&idStr); err != nil {
			return nil, fmt.Errorf("scanning collection id: %w", err)
		}
		id, err := parseUUID(idStr, "collection id")
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating collection ids: %w", err)
	}
	return ids, nil
}

// insertFolder inserts fd as a new folder under collectionID at sortOrder
// and returns the resulting row (with a freshly generated ID).
func insertFolder(ctx context.Context, tx *sql.Tx, collectionID uuid.UUID, sortOrder int, fd FolderData) (Folder, error) {
	f := Folder{
		ID:              uuid.New(),
		CollectionID:    collectionID,
		Title:           fd.Title,
		SortOrder:       sortOrder,
		TileShape:       fd.TileShape,
		HideTitle:       fd.HideTitle,
		CoverEmoji:      fd.CoverEmoji,
		CoverImageURL:   fd.CoverImageURL,
		FocusGIFURL:     fd.FocusGIFURL,
		FocusGIFEnabled: fd.FocusGIFEnabled,
		HeroBackdropURL: fd.HeroBackdropURL,
		HeroVideoURL:    fd.HeroVideoURL,
		TitleLogoURL:    fd.TitleLogoURL,
	}

	_, err := tx.ExecContext(ctx, `
		INSERT INTO folders (id, collection_id, title, sort_order, tile_shape, hide_title, cover_emoji, cover_image_url,
		                     focus_gif_url, focus_gif_enabled, hero_backdrop_url, hero_video_url, title_logo_url)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, f.ID.String(), f.CollectionID.String(), f.Title, f.SortOrder, f.TileShape, f.HideTitle, f.CoverEmoji, f.CoverImageURL,
		f.FocusGIFURL, f.FocusGIFEnabled, f.HeroBackdropURL, f.HeroVideoURL, f.TitleLogoURL)
	if err != nil {
		return Folder{}, fmt.Errorf("inserting folder: %w", err)
	}
	return f, nil
}

// replaceFolderCatalogRefs wipes and rewrites folder_catalogs for folderID,
// in order, from already-resolved refs. Used by the tree-copy path
// (copyCollectionTree, for Take/Duplicate), which has already minted or
// mapped every id it needs before calling this — unlike
// writeFolderCatalogRefs below, there is no inline-create case here.
func replaceFolderCatalogRefs(ctx context.Context, tx *sql.Tx, folderID uuid.UUID, refs []FolderRef) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM folder_catalogs WHERE folder_id = ?`, folderID.String()); err != nil {
		return fmt.Errorf("clearing folder catalog refs: %w", err)
	}
	for j, ref := range refs {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO folder_catalogs (folder_id, catalog_id, sort_order, genre)
			VALUES (?, ?, ?, ?)
		`, folderID.String(), ref.CatalogID.String(), j, ref.Genre); err != nil {
			return fmt.Errorf("inserting folder catalog ref: %w", err)
		}
	}
	return nil
}

// existingRefIDs returns the CatalogID of every ref in refs that references
// an existing catalog, skipping New entries — they aren't rows yet, so
// there's nothing for validateFolderRefs to check.
func existingRefIDs(refs []FolderCatalogRef) []uuid.UUID {
	var ids []uuid.UUID
	for _, ref := range refs {
		if ref.CatalogID != nil {
			ids = append(ids, *ref.CatalogID)
		}
	}
	return ids
}

// resolveFolderCatalogRef returns ref's catalog id, inserting a fresh
// catalog scoped to collectionID first when ref.New is set — owned by
// profileID, never public, never home-eligible, in the caller's own
// transaction. This is the one place a "copy into this collection"/"new
// inside this collection" catalog is ever written: atomic with the folder
// write that references it, so an edit discarded instead of saved never
// created one at all.
func resolveFolderCatalogRef(ctx context.Context, tx *sql.Tx, profileID, collectionID uuid.UUID, ref FolderCatalogRef) (uuid.UUID, error) {
	if ref.CatalogID != nil {
		return *ref.CatalogID, nil
	}

	spec := ref.New
	now := time.Now().UTC().Format(time.RFC3339)
	id := uuid.New()
	_, err := tx.ExecContext(ctx, `
		INSERT INTO catalogs (id, type, name, provider, params, owner_id, is_public, is_default,
		                       collection_id, fingerprint, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, id.String(), spec.Type, spec.Name, spec.Provider, spec.Params, profileID.String(), false, false,
		collectionID.String(), spec.Fingerprint, now, now)
	if err != nil {
		return uuid.Nil, fmt.Errorf("inserting scoped catalog: %w", err)
	}
	return id, nil
}

// writeFolderCatalogRefs wipes and rewrites folder_catalogs for folderID, in
// order, resolving each ref via resolveFolderCatalogRef first. Safe to call
// for a brand-new folder too — the delete is then a no-op. Refs are cheap
// and small compared to folders, so delete-and-reinsert beats diffing them.
// Returns the resolved refs in order, for the caller's response shape and
// final catalog fetch.
func writeFolderCatalogRefs(ctx context.Context, tx *sql.Tx, profileID, collectionID, folderID uuid.UUID, refs []FolderCatalogRef) ([]FolderRef, error) {
	if _, err := tx.ExecContext(ctx, `DELETE FROM folder_catalogs WHERE folder_id = ?`, folderID.String()); err != nil {
		return nil, fmt.Errorf("clearing folder catalog refs: %w", err)
	}
	resolved := make([]FolderRef, len(refs))
	for j, ref := range refs {
		catalogID, err := resolveFolderCatalogRef(ctx, tx, profileID, collectionID, ref)
		if err != nil {
			return nil, err
		}
		resolved[j] = FolderRef{CatalogID: catalogID, Genre: strings.TrimSpace(ref.Genre)}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO folder_catalogs (folder_id, catalog_id, sort_order, genre)
			VALUES (?, ?, ?, ?)
		`, folderID.String(), catalogID.String(), j, resolved[j].Genre); err != nil {
			return nil, fmt.Errorf("inserting folder catalog ref: %w", err)
		}
	}
	return resolved, nil
}

// CreateUserCollection validates input, checks profileID has access to
// every referenced catalog, and inserts the collection with its folders in
// one transaction.
func (db *DB) CreateUserCollection(ctx context.Context, profileID uuid.UUID, input CollectionForm) (CollectionWithFolders, error) {
	if err := input.Validate(); err != nil {
		return CollectionWithFolders{}, err
	}

	tx, err := db.conn.BeginTx(ctx, nil)
	if err != nil {
		return CollectionWithFolders{}, fmt.Errorf("starting transaction: %w", err)
	}
	defer tx.Rollback() // no-op once Commit succeeds

	var existingCatalogIDs []uuid.UUID
	for _, fd := range input.Folders {
		existingCatalogIDs = append(existingCatalogIDs, existingRefIDs(fd.Catalogs)...)
	}
	if err := validateFolderRefs(ctx, tx, profileID, nil, existingCatalogIDs); err != nil {
		return CollectionWithFolders{}, err
	}

	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339)
	c := Collection{
		ID:               uuid.New(),
		Title:            input.Title,
		OwnerID:          profileID,
		IsPublic:         input.IsPublic,
		IsDefault:        false,
		PinToTop:         input.PinToTop,
		ViewMode:         input.ViewMode,
		ShowAllTab:       input.ShowAllTab,
		BackdropImageURL: input.BackdropImageURL,
		FocusGlowEnabled: input.FocusGlowEnabled,
		Version:          1,
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO collections (id, title, owner_id, is_public, is_default, pin_to_top, view_mode, show_all_tab, backdrop_image_url,
		                          focus_glow_enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, c.ID.String(), c.Title, c.OwnerID.String(), c.IsPublic, c.IsDefault, c.PinToTop, c.ViewMode, c.ShowAllTab, c.BackdropImageURL,
		c.FocusGlowEnabled, nowStr, nowStr)
	if err != nil {
		return CollectionWithFolders{}, fmt.Errorf("inserting collection: %w", err)
	}

	folders := make([]FolderWithCatalogs, len(input.Folders))
	var allCatalogIDs []uuid.UUID
	for i, fd := range input.Folders {
		f, err := insertFolder(ctx, tx, c.ID, i, fd)
		if err != nil {
			return CollectionWithFolders{}, err
		}
		refs, err := writeFolderCatalogRefs(ctx, tx, profileID, c.ID, f.ID, fd.Catalogs)
		if err != nil {
			return CollectionWithFolders{}, err
		}

		folders[i] = FolderWithCatalogs{Folder: f, Refs: refs}
		allCatalogIDs = append(allCatalogIDs, folders[i].CatalogIDs()...)
	}

	if err := tx.Commit(); err != nil {
		return CollectionWithFolders{}, fmt.Errorf("committing transaction: %w", err)
	}

	catalogs, err := db.GetCatalogsByIDs(ctx, dedupeUUIDs(allCatalogIDs))
	if err != nil {
		return CollectionWithFolders{}, err
	}

	return CollectionWithFolders{Collection: c, Folders: folders, Catalogs: orEmpty(catalogs)}, nil
}

// UpdateUserCollection validates input and replaces the collection
// identified by collectionID (title, settings, and its full folder set),
// provided it's owned by profileID.
func (db *DB) UpdateUserCollection(ctx context.Context, profileID uuid.UUID, collectionID uuid.UUID, input CollectionForm) (CollectionWithFolders, error) {
	if err := input.Validate(); err != nil {
		return CollectionWithFolders{}, err
	}

	tx, err := db.conn.BeginTx(ctx, nil)
	if err != nil {
		return CollectionWithFolders{}, fmt.Errorf("starting transaction: %w", err)
	}
	defer tx.Rollback() // no-op once Commit succeeds

	var createdAtStr string
	var homeSortOrder, pushedVersion sql.NullInt64
	var version int
	err = tx.QueryRowContext(ctx, `
		SELECT created_at, home_sort_order, version, pushed_version FROM collections WHERE id = ? AND owner_id = ?
	`, collectionID.String(), profileID.String()).Scan(&createdAtStr, &homeSortOrder, &version, &pushedVersion)
	if errors.Is(err, sql.ErrNoRows) {
		return CollectionWithFolders{}, ErrCollectionNotFound
	}
	if err != nil {
		return CollectionWithFolders{}, fmt.Errorf("loading collection: %w", err)
	}

	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339)

	result, err := tx.ExecContext(ctx, `
		UPDATE collections
		SET title = ?, is_public = ?, pin_to_top = ?, view_mode = ?, show_all_tab = ?, backdrop_image_url = ?, focus_glow_enabled = ?,
		    updated_at = ?, version = version + 1
		WHERE id = ? AND owner_id = ?
	`, input.Title, input.IsPublic, input.PinToTop, input.ViewMode, input.ShowAllTab, input.BackdropImageURL, input.FocusGlowEnabled, nowStr,
		collectionID.String(), profileID.String())
	if err != nil {
		return CollectionWithFolders{}, fmt.Errorf("updating collection: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return CollectionWithFolders{}, fmt.Errorf("checking rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return CollectionWithFolders{}, ErrCollectionNotFound
	}

	// Existing folders, so we know what the incoming payload is keeping vs dropping.
	rows, err := tx.QueryContext(ctx, `SELECT id FROM folders WHERE collection_id = ?`, collectionID.String())
	if err != nil {
		return CollectionWithFolders{}, fmt.Errorf("querying existing folders: %w", err)
	}
	existingFolderIDs := make(map[uuid.UUID]bool)
	for rows.Next() {
		var idStr string
		if err := rows.Scan(&idStr); err != nil {
			rows.Close()
			return CollectionWithFolders{}, fmt.Errorf("scanning folder id: %w", err)
		}
		id, err := parseUUID(idStr, "folder id")
		if err != nil {
			rows.Close()
			return CollectionWithFolders{}, fmt.Errorf("parsing folder id: %w", err)
		}
		existingFolderIDs[id] = true
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return CollectionWithFolders{}, fmt.Errorf("iterating folder ids: %w", err)
	}
	rows.Close()

	keepFolderIDs := make(map[uuid.UUID]bool, len(input.Folders))
	for _, fd := range input.Folders {
		if fd.ID == nil {
			continue
		}
		if !existingFolderIDs[*fd.ID] {
			return CollectionWithFolders{}, fmt.Errorf("%w: folder %s does not belong to this collection", ErrInvalidInput, *fd.ID)
		}
		keepFolderIDs[*fd.ID] = true
	}

	var toDelete []uuid.UUID
	for id := range existingFolderIDs {
		if !keepFolderIDs[id] {
			toDelete = append(toDelete, id)
		}
	}
	if len(toDelete) > 0 {
		placeholders, args := buildInClause(toDelete)
		// ON DELETE CASCADE on folder_catalogs.folder_id handles the refs.
		_, err = tx.ExecContext(ctx, fmt.Sprintf(`DELETE FROM folders WHERE id IN (%s)`, placeholders), args...)
		if err != nil {
			return CollectionWithFolders{}, fmt.Errorf("deleting removed folders: %w", err)
		}
	}

	// One validation query for every catalog ID across every folder, up
	// front — New entries are skipped, they aren't rows yet (existingRefIDs).
	var existingCatalogIDs []uuid.UUID
	for _, fd := range input.Folders {
		existingCatalogIDs = append(existingCatalogIDs, existingRefIDs(fd.Catalogs)...)
	}
	if err := validateFolderRefs(ctx, tx, profileID, &collectionID, existingCatalogIDs); err != nil {
		return CollectionWithFolders{}, err
	}

	folders := make([]FolderWithCatalogs, len(input.Folders))
	var allCatalogIDs []uuid.UUID
	for i, fd := range input.Folders {
		var f Folder
		if fd.ID != nil {
			f = Folder{
				ID: *fd.ID, CollectionID: collectionID, Title: fd.Title, SortOrder: i,
				TileShape: fd.TileShape, HideTitle: fd.HideTitle,
				CoverEmoji: fd.CoverEmoji, CoverImageURL: fd.CoverImageURL,
				FocusGIFURL: fd.FocusGIFURL, FocusGIFEnabled: fd.FocusGIFEnabled,
				HeroBackdropURL: fd.HeroBackdropURL, HeroVideoURL: fd.HeroVideoURL, TitleLogoURL: fd.TitleLogoURL,
			}
			_, err = tx.ExecContext(ctx, `
				UPDATE folders
				SET title = ?, sort_order = ?, tile_shape = ?, hide_title = ?, cover_emoji = ?, cover_image_url = ?,
				    focus_gif_url = ?, focus_gif_enabled = ?, hero_backdrop_url = ?, hero_video_url = ?, title_logo_url = ?
				WHERE id = ?
			`, f.Title, f.SortOrder, f.TileShape, f.HideTitle, f.CoverEmoji, f.CoverImageURL,
				f.FocusGIFURL, f.FocusGIFEnabled, f.HeroBackdropURL, f.HeroVideoURL, f.TitleLogoURL, f.ID.String())
			if err != nil {
				return CollectionWithFolders{}, fmt.Errorf("updating folder: %w", err)
			}
		} else {
			f, err = insertFolder(ctx, tx, collectionID, i, fd)
			if err != nil {
				return CollectionWithFolders{}, err
			}
		}

		refs, err := writeFolderCatalogRefs(ctx, tx, profileID, collectionID, f.ID, fd.Catalogs)
		if err != nil {
			return CollectionWithFolders{}, err
		}

		folders[i] = FolderWithCatalogs{Folder: f, Refs: refs}
		allCatalogIDs = append(allCatalogIDs, folders[i].CatalogIDs()...)
	}

	// A scoped catalog with no remaining folder reference in this
	// collection is deleted here, in the same transaction as the folder
	// rewrite above — this is what catches a scoped catalog created via
	// POST .../catalogs and never referenced (editor abandoned before Save).
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM catalogs
		WHERE collection_id = ?
		  AND id NOT IN (SELECT fc.catalog_id FROM folder_catalogs fc
		                 JOIN folders f ON f.id = fc.folder_id
		                 WHERE f.collection_id = ?)
	`, collectionID.String(), collectionID.String()); err != nil {
		return CollectionWithFolders{}, fmt.Errorf("deleting orphaned scoped catalogs: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return CollectionWithFolders{}, fmt.Errorf("committing transaction: %w", err)
	}

	createdAt, err := parseTimestamp(createdAtStr, "collection created_at")
	if err != nil {
		return CollectionWithFolders{}, err
	}

	c := Collection{
		ID: collectionID, Title: input.Title, OwnerID: profileID,
		IsPublic: input.IsPublic, IsDefault: false, // not returned by UPDATE
		PinToTop: input.PinToTop, ViewMode: input.ViewMode,
		ShowAllTab: input.ShowAllTab, BackdropImageURL: input.BackdropImageURL,
		// HomeSortOrder/PushedVersion are unchanged by this update, read back
		// above for an accurate response. Version is the freshly bumped value
		// the UPDATE above just wrote.
		HomeSortOrder: nullableInt(homeSortOrder), Version: version + 1, PushedVersion: nullableInt(pushedVersion),
		CreatedAt: createdAt, UpdatedAt: now,
	}

	catalogs, err := db.GetCatalogsByIDs(ctx, dedupeUUIDs(allCatalogIDs))
	if err != nil {
		return CollectionWithFolders{}, err
	}

	return CollectionWithFolders{Collection: c, Folders: folders, Catalogs: orEmpty(catalogs)}, nil
}

// DeleteUserCollection deletes the collection identified by collectionID,
// provided it's owned by profileID. Returns ErrCollectionNotFound
// otherwise.
func (db *DB) DeleteUserCollection(ctx context.Context, profileID uuid.UUID, collectionID uuid.UUID) error {
	result, err := db.conn.ExecContext(ctx, `
		DELETE FROM collections WHERE id = ? AND owner_id = ?
	`, collectionID.String(), profileID.String())
	if err != nil {
		return fmt.Errorf("deleting collection: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if rows == 0 {
		return ErrCollectionNotFound
	}

	return nil
}

// GetCurrentCollectionSelection returns profileID's active collection
// selection — every owned collection with a non-nil home_sort_order —
// ordered by it, each with its folders assembled.
func (db *DB) GetCurrentCollectionSelection(ctx context.Context, profileID uuid.UUID) ([]CollectionWithFolders, error) {
	collections, err := db.queryCollections(ctx, "owner_id = ? AND home_sort_order IS NOT NULL", profileID.String())
	if err != nil {
		return nil, err
	}

	sort.Slice(collections, func(i, j int) bool {
		return *collections[i].HomeSortOrder < *collections[j].HomeSortOrder
	})

	return db.assembleCollectionTree(ctx, collections)
}

// saveCollectionSelectionTx resets this profile's collection selection to
// exactly input, in order, and stamps pushed_version on every collection it
// includes with the version pushCollections read for it — this is push's
// only caller, so every collection reaching this point is, by definition,
// being pushed right now. Every owned collection's home_sort_order is
// cleared first, then each incoming id is set in turn; a 0-rows-affected
// update (an id that isn't owned) is ErrInvalidInput naming the id, the same
// pattern as saveCatalogSelectionTx.
//
// versions is keyed by collection id, built by pushCollections from the same
// read that fed Nuvio — never the row's current version and never a clock,
// which is what keeps a Save landing between that read and this write from
// being mistaken for pushed. A missing entry (the collection vanished
// between push's read and this write) leaves pushed_version untouched rather
// than guessing.
//
// Takes a caller-supplied transaction — see saveCatalogSelectionTx in
// catalogs.go for why, and for why there is no exported single-selection
// wrapper.
func saveCollectionSelectionTx(ctx context.Context, tx *sql.Tx, profileID uuid.UUID, input CollectionSelectionForm, versions map[uuid.UUID]int) error {
	if _, err := tx.ExecContext(ctx, `
		UPDATE collections SET home_sort_order = NULL WHERE owner_id = ?
	`, profileID.String()); err != nil {
		return fmt.Errorf("clearing collection home selection: %w", err)
	}

	for i, id := range input.CollectionIDs {
		var pushedVersion any
		if v, ok := versions[id]; ok {
			pushedVersion = v
		}
		result, err := tx.ExecContext(ctx, `
			UPDATE collections
			SET home_sort_order = ?, pushed_version = COALESCE(?, pushed_version)
			WHERE id = ? AND owner_id = ?
		`, i, pushedVersion, id.String(), profileID.String())
		if err != nil {
			return fmt.Errorf("saving collection selection: %w", err)
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("checking rows affected: %w", err)
		}
		if rows == 0 {
			return fmt.Errorf("%w: collection %s is not accessible to this profile", ErrInvalidInput, id)
		}
	}

	return nil
}

// assembleCollectionTree fetches folders and folder_catalogs for the given
// collections and zips everything into the nested response shape. Order of
// the input collections slice is preserved.
func (db *DB) assembleCollectionTree(ctx context.Context, collections []Collection) ([]CollectionWithFolders, error) {
	if len(collections) == 0 {
		return []CollectionWithFolders{}, nil
	}

	collectionIDs := make([]uuid.UUID, len(collections))
	for i, c := range collections {
		collectionIDs[i] = c.ID
	}

	placeholders, args := buildInClause(collectionIDs)
	rows, err := db.conn.QueryContext(ctx, fmt.Sprintf(`
		SELECT id, collection_id, title, sort_order, tile_shape, hide_title, cover_emoji, cover_image_url,
		       focus_gif_url, focus_gif_enabled, hero_backdrop_url, hero_video_url, title_logo_url
		FROM folders
		WHERE collection_id IN (%s)
		ORDER BY collection_id, sort_order
	`, placeholders), args...)
	if err != nil {
		return nil, fmt.Errorf("querying folders: %w", err)
	}
	folders, err := parseFolders(rows)
	rows.Close()
	if err != nil {
		return nil, fmt.Errorf("parsing folder rows: %w", err)
	}

	folderIDs := make([]uuid.UUID, len(folders))
	for i, f := range folders {
		folderIDs[i] = f.ID
	}

	var folderCatalogs []FolderCatalog
	if len(folderIDs) > 0 {
		placeholders, args = buildInClause(folderIDs)
		rows, err = db.conn.QueryContext(ctx, fmt.Sprintf(`
			SELECT folder_id, catalog_id, sort_order, genre
			FROM folder_catalogs
			WHERE folder_id IN (%s)
			ORDER BY folder_id, sort_order
		`, placeholders), args...)
		if err != nil {
			return nil, fmt.Errorf("querying folder catalogs: %w", err)
		}
		folderCatalogs, err = parseFolderCatalogs(rows)
		rows.Close()
		if err != nil {
			return nil, fmt.Errorf("parsing folder catalog rows: %w", err)
		}
	}

	refsByFolder := make(map[uuid.UUID][]FolderRef)
	for _, fc := range folderCatalogs {
		refsByFolder[fc.FolderID] = append(refsByFolder[fc.FolderID], FolderRef{CatalogID: fc.CatalogID, Genre: fc.Genre})
	}

	foldersByCollection := make(map[uuid.UUID][]FolderWithCatalogs)
	catalogIDsByCollection := make(map[uuid.UUID][]uuid.UUID)
	for _, f := range folders {
		folder := FolderWithCatalogs{Folder: f, Refs: orEmpty(refsByFolder[f.ID])}
		foldersByCollection[f.CollectionID] = append(foldersByCollection[f.CollectionID], folder)
		catalogIDsByCollection[f.CollectionID] = append(catalogIDsByCollection[f.CollectionID], folder.CatalogIDs()...)
	}

	var allCatalogIDs []uuid.UUID
	for _, ids := range catalogIDsByCollection {
		allCatalogIDs = append(allCatalogIDs, ids...)
	}
	catalogsByID := make(map[uuid.UUID]Catalog)
	if len(allCatalogIDs) > 0 {
		catalogs, err := db.GetCatalogsByIDs(ctx, dedupeUUIDs(allCatalogIDs))
		if err != nil {
			return nil, err
		}
		for _, cat := range catalogs {
			catalogsByID[cat.ID] = cat
		}
	}

	result := make([]CollectionWithFolders, len(collections))
	for i, c := range collections {
		ids := dedupeUUIDs(catalogIDsByCollection[c.ID])
		catalogs := make([]Catalog, 0, len(ids))
		for _, id := range ids {
			if cat, ok := catalogsByID[id]; ok {
				catalogs = append(catalogs, cat)
			}
		}
		result[i] = CollectionWithFolders{
			Collection: c,
			Folders:    orEmpty(foldersByCollection[c.ID]),
			Catalogs:   orEmpty(catalogs),
		}
	}

	return result, nil
}
