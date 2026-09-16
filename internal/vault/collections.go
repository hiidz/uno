package vault

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
)

// queryCollections runs a SELECT over collections with the given WHERE
// clause and args, parsing the result rows.
func (db *DB) queryCollections(ctx context.Context, where string, args ...any) ([]Collection, error) {
	rows, err := db.conn.QueryContext(ctx, `
		SELECT id, title, owner_id, is_public, is_default, pin_to_top, view_mode, show_all_tab, backdrop_image_url,
		       home_sort_order, pushed_at, taken_from, created_at, updated_at
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
// with its folders assembled, sorted by title (no fingerprint collapse —
// that's a catalog-only concept, §3.3), flagged with whether profileID has
// already taken a copy (§3.5).
func (db *DB) GetCommunityCollections(ctx context.Context, profileID uuid.UUID) ([]CommunityCollection, error) {
	collections, err := db.queryCollections(ctx, "is_public = TRUE AND owner_id != ?", profileID.String())
	if err != nil {
		return nil, err
	}
	sort.Slice(collections, func(i, j int) bool { return collections[i].Title < collections[j].Title })

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

// TakeCollection deep-copies a public collection owned by someone else —
// its cosmetics, folders, and every catalog its folders reference — into a
// new collection owned by profileID, per §3.5 of the sharing model plan.
// Every copied catalog is scoped to the new collection, even if the source
// catalog was listed; a source catalog referenced by two folders becomes one
// scoped copy referenced twice. Returns ErrCollectionNotFound if sourceID
// isn't public or is already owned by profileID.
func (db *DB) TakeCollection(ctx context.Context, profileID uuid.UUID, sourceID uuid.UUID) (CollectionWithFolders, error) {
	tx, err := db.conn.BeginTx(ctx, nil)
	if err != nil {
		return CollectionWithFolders{}, fmt.Errorf("starting transaction: %w", err)
	}
	defer tx.Rollback() // no-op once Commit succeeds

	var title, viewMode, backdropImageURL string
	var pinToTop, showAllTab int
	err = tx.QueryRowContext(ctx, `
		SELECT title, pin_to_top, view_mode, show_all_tab, backdrop_image_url
		FROM collections WHERE id = ? AND is_public = TRUE AND owner_id != ?
	`, sourceID.String(), profileID.String()).Scan(&title, &pinToTop, &viewMode, &showAllTab, &backdropImageURL)
	if errors.Is(err, sql.ErrNoRows) {
		return CollectionWithFolders{}, ErrCollectionNotFound
	}
	if err != nil {
		return CollectionWithFolders{}, fmt.Errorf("loading source collection: %w", err)
	}

	folderRows, err := tx.QueryContext(ctx, `
		SELECT id, title, tile_shape, hide_title, cover_emoji, cover_image_url
		FROM folders WHERE collection_id = ? ORDER BY sort_order
	`, sourceID.String())
	if err != nil {
		return CollectionWithFolders{}, fmt.Errorf("querying source folders: %w", err)
	}
	type sourceFolder struct {
		id         uuid.UUID
		data       FolderData
		catalogIDs []uuid.UUID
	}
	var sourceFolders []sourceFolder
	for folderRows.Next() {
		var idStr string
		var fd FolderData
		var hideTitleInt int
		if err := folderRows.Scan(&idStr, &fd.Title, &fd.TileShape, &hideTitleInt, &fd.CoverEmoji, &fd.CoverImageURL); err != nil {
			folderRows.Close()
			return CollectionWithFolders{}, fmt.Errorf("scanning source folder: %w", err)
		}
		id, err := parseUUID(idStr, "folder id")
		if err != nil {
			folderRows.Close()
			return CollectionWithFolders{}, err
		}
		fd.HideTitle = hideTitleInt != 0
		sourceFolders = append(sourceFolders, sourceFolder{id: id, data: fd})
	}
	if err := folderRows.Err(); err != nil {
		folderRows.Close()
		return CollectionWithFolders{}, fmt.Errorf("iterating source folders: %w", err)
	}
	folderRows.Close()

	for i := range sourceFolders {
		catRows, err := tx.QueryContext(ctx, `
			SELECT catalog_id FROM folder_catalogs WHERE folder_id = ? ORDER BY sort_order
		`, sourceFolders[i].id.String())
		if err != nil {
			return CollectionWithFolders{}, fmt.Errorf("querying source folder catalogs: %w", err)
		}
		for catRows.Next() {
			var catIDStr string
			if err := catRows.Scan(&catIDStr); err != nil {
				catRows.Close()
				return CollectionWithFolders{}, fmt.Errorf("scanning source folder catalog: %w", err)
			}
			catID, err := parseUUID(catIDStr, "catalog id")
			if err != nil {
				catRows.Close()
				return CollectionWithFolders{}, err
			}
			sourceFolders[i].catalogIDs = append(sourceFolders[i].catalogIDs, catID)
		}
		if err := catRows.Err(); err != nil {
			catRows.Close()
			return CollectionWithFolders{}, fmt.Errorf("iterating source folder catalogs: %w", err)
		}
		catRows.Close()
	}

	// Distinct source catalog ids across every folder, first-seen order —
	// this is what collapses two folders sharing a source catalog into one
	// scoped copy.
	var distinctCatalogIDs []uuid.UUID
	seenCatalog := map[uuid.UUID]bool{}
	for _, f := range sourceFolders {
		for _, id := range f.catalogIDs {
			if !seenCatalog[id] {
				seenCatalog[id] = true
				distinctCatalogIDs = append(distinctCatalogIDs, id)
			}
		}
	}

	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339)

	newCollectionID := uuid.New()
	_, err = tx.ExecContext(ctx, `
		INSERT INTO collections (id, title, owner_id, is_public, is_default, pin_to_top, view_mode, show_all_tab,
		                          backdrop_image_url, taken_from, created_at, updated_at)
		VALUES (?, ?, ?, 0, 0, ?, ?, ?, ?, ?, ?, ?)
	`, newCollectionID.String(), title, profileID.String(), pinToTop, viewMode, showAllTab, backdropImageURL,
		sourceID.String(), nowStr, nowStr)
	if err != nil {
		return CollectionWithFolders{}, fmt.Errorf("inserting taken collection: %w", err)
	}

	idMap := make(map[uuid.UUID]uuid.UUID, len(distinctCatalogIDs))
	if len(distinctCatalogIDs) > 0 {
		placeholders, args := buildInClause(distinctCatalogIDs)
		catRows, err := tx.QueryContext(ctx, fmt.Sprintf(`
			SELECT id, type, name, provider, params, fingerprint FROM catalogs WHERE id IN (%s)
		`, placeholders), args...)
		if err != nil {
			return CollectionWithFolders{}, fmt.Errorf("querying source catalogs: %w", err)
		}
		type sourceCatalog struct {
			id, typ, name, provider, params, fingerprint string
		}
		var sourceCatalogs []sourceCatalog
		for catRows.Next() {
			var sc sourceCatalog
			if err := catRows.Scan(&sc.id, &sc.typ, &sc.name, &sc.provider, &sc.params, &sc.fingerprint); err != nil {
				catRows.Close()
				return CollectionWithFolders{}, fmt.Errorf("scanning source catalog: %w", err)
			}
			sourceCatalogs = append(sourceCatalogs, sc)
		}
		if err := catRows.Err(); err != nil {
			catRows.Close()
			return CollectionWithFolders{}, fmt.Errorf("iterating source catalogs: %w", err)
		}
		catRows.Close()

		for _, sc := range sourceCatalogs {
			oldID, err := parseUUID(sc.id, "catalog id")
			if err != nil {
				return CollectionWithFolders{}, err
			}
			newID := uuid.New()
			idMap[oldID] = newID
			_, err = tx.ExecContext(ctx, `
				INSERT INTO catalogs (id, type, name, provider, params, owner_id, is_public, is_default,
				                       collection_id, taken_from, fingerprint, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, 0, 0, ?, ?, ?, ?, ?)
			`, newID.String(), sc.typ, sc.name, sc.provider, sc.params, profileID.String(),
				newCollectionID.String(), oldID.String(), sc.fingerprint, nowStr, nowStr)
			if err != nil {
				return CollectionWithFolders{}, fmt.Errorf("inserting taken scoped catalog: %w", err)
			}
		}
	}

	for i, f := range sourceFolders {
		newFolder, err := insertFolder(ctx, tx, newCollectionID, i, f.data)
		if err != nil {
			return CollectionWithFolders{}, err
		}
		newCatalogIDs := make([]uuid.UUID, len(f.catalogIDs))
		for j, oldID := range f.catalogIDs {
			newCatalogIDs[j] = idMap[oldID]
		}
		if err := replaceFolderCatalogRefs(ctx, tx, newFolder.ID, newCatalogIDs); err != nil {
			return CollectionWithFolders{}, err
		}
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
		ID:            uuid.New(),
		CollectionID:  collectionID,
		Title:         fd.Title,
		SortOrder:     sortOrder,
		TileShape:     fd.TileShape,
		HideTitle:     fd.HideTitle,
		CoverEmoji:    fd.CoverEmoji,
		CoverImageURL: fd.CoverImageURL,
	}

	_, err := tx.ExecContext(ctx, `
		INSERT INTO folders (id, collection_id, title, sort_order, tile_shape, hide_title, cover_emoji, cover_image_url)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, f.ID.String(), f.CollectionID.String(), f.Title, f.SortOrder, f.TileShape, f.HideTitle, f.CoverEmoji, f.CoverImageURL)
	if err != nil {
		return Folder{}, fmt.Errorf("inserting folder: %w", err)
	}
	return f, nil
}

// replaceFolderCatalogRefs wipes and rewrites folder_catalogs for folderID,
// in order. Safe to call for a brand-new folder too — the delete is then a
// no-op. Refs are cheap and small compared to folders, so delete-and-reinsert
// beats diffing them.
func replaceFolderCatalogRefs(ctx context.Context, tx *sql.Tx, folderID uuid.UUID, catalogIDs []uuid.UUID) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM folder_catalogs WHERE folder_id = ?`, folderID.String()); err != nil {
		return fmt.Errorf("clearing folder catalog refs: %w", err)
	}
	for j, catalogID := range catalogIDs {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO folder_catalogs (folder_id, catalog_id, sort_order)
			VALUES (?, ?, ?)
		`, folderID.String(), catalogID.String(), j); err != nil {
			return fmt.Errorf("inserting folder catalog ref: %w", err)
		}
	}
	return nil
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

	var allCatalogIDs []uuid.UUID
	for _, fd := range input.Folders {
		allCatalogIDs = append(allCatalogIDs, fd.CatalogIDs...)
	}
	if err := validateFolderRefs(ctx, tx, profileID, nil, allCatalogIDs); err != nil {
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
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO collections (id, title, owner_id, is_public, is_default, pin_to_top, view_mode, show_all_tab, backdrop_image_url,
		                          created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, c.ID.String(), c.Title, c.OwnerID.String(), c.IsPublic, c.IsDefault, c.PinToTop, c.ViewMode, c.ShowAllTab, c.BackdropImageURL,
		nowStr, nowStr)
	if err != nil {
		return CollectionWithFolders{}, fmt.Errorf("inserting collection: %w", err)
	}

	folders := make([]FolderWithCatalogs, len(input.Folders))
	for i, fd := range input.Folders {
		f, err := insertFolder(ctx, tx, c.ID, i, fd)
		if err != nil {
			return CollectionWithFolders{}, err
		}
		if err := replaceFolderCatalogRefs(ctx, tx, f.ID, fd.CatalogIDs); err != nil {
			return CollectionWithFolders{}, err
		}

		folders[i] = FolderWithCatalogs{Folder: f, CatalogIDs: orEmpty(fd.CatalogIDs)}
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
	var homeSortOrder sql.NullInt64
	var pushedAtStr sql.NullString
	err = tx.QueryRowContext(ctx, `
		SELECT created_at, home_sort_order, pushed_at FROM collections WHERE id = ? AND owner_id = ?
	`, collectionID.String(), profileID.String()).Scan(&createdAtStr, &homeSortOrder, &pushedAtStr)
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
		SET title = ?, is_public = ?, pin_to_top = ?, view_mode = ?, show_all_tab = ?, backdrop_image_url = ?, updated_at = ?
		WHERE id = ? AND owner_id = ?
	`, input.Title, input.IsPublic, input.PinToTop, input.ViewMode, input.ShowAllTab, input.BackdropImageURL, nowStr,
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

	// One validation query for every catalog ID across every folder, up front.
	var allCatalogIDs []uuid.UUID
	for _, fd := range input.Folders {
		allCatalogIDs = append(allCatalogIDs, fd.CatalogIDs...)
	}
	if err := validateFolderRefs(ctx, tx, profileID, &collectionID, allCatalogIDs); err != nil {
		return CollectionWithFolders{}, err
	}

	folders := make([]FolderWithCatalogs, len(input.Folders))
	for i, fd := range input.Folders {
		var f Folder
		if fd.ID != nil {
			f = Folder{
				ID: *fd.ID, CollectionID: collectionID, Title: fd.Title, SortOrder: i,
				TileShape: fd.TileShape, HideTitle: fd.HideTitle,
				CoverEmoji: fd.CoverEmoji, CoverImageURL: fd.CoverImageURL,
			}
			_, err = tx.ExecContext(ctx, `
				UPDATE folders
				SET title = ?, sort_order = ?, tile_shape = ?, hide_title = ?, cover_emoji = ?, cover_image_url = ?
				WHERE id = ?
			`, f.Title, f.SortOrder, f.TileShape, f.HideTitle, f.CoverEmoji, f.CoverImageURL, f.ID.String())
			if err != nil {
				return CollectionWithFolders{}, fmt.Errorf("updating folder: %w", err)
			}
		} else {
			f, err = insertFolder(ctx, tx, collectionID, i, fd)
			if err != nil {
				return CollectionWithFolders{}, err
			}
		}

		if err := replaceFolderCatalogRefs(ctx, tx, f.ID, fd.CatalogIDs); err != nil {
			return CollectionWithFolders{}, err
		}

		folders[i] = FolderWithCatalogs{Folder: f, CatalogIDs: orEmpty(fd.CatalogIDs)}
	}

	// §3.2: a scoped catalog with no remaining folder reference in this
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
	pushedAt, err := parseNullableTimestamp(pushedAtStr, "collection pushed_at")
	if err != nil {
		return CollectionWithFolders{}, err
	}

	c := Collection{
		ID: collectionID, Title: input.Title, OwnerID: profileID,
		IsPublic: input.IsPublic, IsDefault: false, // not returned by UPDATE
		PinToTop: input.PinToTop, ViewMode: input.ViewMode,
		ShowAllTab: input.ShowAllTab, BackdropImageURL: input.BackdropImageURL,
		// HomeSortOrder/PushedAt are unchanged by this update, read back above
		// for an accurate response.
		HomeSortOrder: nullableInt(homeSortOrder), PushedAt: pushedAt,
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
// exactly input, in order, and stamps pushed_at on every collection it
// includes — this is push's only caller, so every collection reaching this
// point is, by definition, being pushed right now. Every owned collection's
// home_sort_order is cleared first, then each incoming id is set in turn; a
// 0-rows-affected update (an id that isn't owned) is ErrInvalidInput naming
// the id, the same pattern as saveCatalogSelectionTx.
//
// Takes a caller-supplied transaction — see saveCatalogSelectionTx in
// catalogs.go for why, and for why there is no exported single-selection
// wrapper.
func saveCollectionSelectionTx(ctx context.Context, tx *sql.Tx, profileID uuid.UUID, input CollectionSelectionForm) error {
	if _, err := tx.ExecContext(ctx, `
		UPDATE collections SET home_sort_order = NULL WHERE owner_id = ?
	`, profileID.String()); err != nil {
		return fmt.Errorf("clearing collection home selection: %w", err)
	}

	nowStr := time.Now().UTC().Format(time.RFC3339)
	for i, id := range input.CollectionIDs {
		result, err := tx.ExecContext(ctx, `
			UPDATE collections
			SET home_sort_order = ?, pushed_at = ?
			WHERE id = ? AND owner_id = ?
		`, i, nowStr, id.String(), profileID.String())
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
		SELECT id, collection_id, title, sort_order, tile_shape, hide_title, cover_emoji, cover_image_url
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
			SELECT folder_id, catalog_id, sort_order
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

	catalogIDsByFolder := make(map[uuid.UUID][]uuid.UUID)
	for _, fc := range folderCatalogs {
		catalogIDsByFolder[fc.FolderID] = append(catalogIDsByFolder[fc.FolderID], fc.CatalogID)
	}

	foldersByCollection := make(map[uuid.UUID][]FolderWithCatalogs)
	catalogIDsByCollection := make(map[uuid.UUID][]uuid.UUID)
	for _, f := range folders {
		foldersByCollection[f.CollectionID] = append(foldersByCollection[f.CollectionID], FolderWithCatalogs{
			Folder:     f,
			CatalogIDs: orEmpty(catalogIDsByFolder[f.ID]),
		})
		catalogIDsByCollection[f.CollectionID] = append(catalogIDsByCollection[f.CollectionID], catalogIDsByFolder[f.ID]...)
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
