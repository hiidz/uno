package vault

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/google/uuid"
)

// queryCollections runs a SELECT over collections with the given WHERE
// clause and args, parsing the result rows.
func (db *DB) queryCollections(ctx context.Context, where string, args ...any) ([]Collection, error) {
	rows, err := db.conn.QueryContext(ctx, `
		SELECT id, title, owner_id, is_public, is_default, pin_to_top, view_mode, show_all_tab, backdrop_image_url
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

// GetCommunityCollections returns every collection marked public,
// regardless of owner, each with its folders assembled.
func (db *DB) GetCommunityCollections(ctx context.Context) ([]CollectionWithFolders, error) {
	collections, err := db.queryCollections(ctx, "is_public = TRUE")
	if err != nil {
		return nil, err
	}
	return db.assembleCollectionTree(ctx, collections)
}

// GetCollectionsByIDs batch-loads collections (with folders) by id, no
// ownership check and no ordering guarantee — push uses this to resolve the
// pending collection selection straight from the request body rather than
// reading profile_collections back, so it needs the same shape
// GetCurrentCollectionSelection returns, keyed by an explicit id list
// instead of a profile join. Mirrors internal/vault/catalogs.go's
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
	if err := validateCatalogAccess(ctx, tx, profileID, allCatalogIDs); err != nil {
		return CollectionWithFolders{}, err
	}

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
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO collections (id, title, owner_id, is_public, is_default, pin_to_top, view_mode, show_all_tab, backdrop_image_url)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, c.ID.String(), c.Title, c.OwnerID.String(), c.IsPublic, c.IsDefault, c.PinToTop, c.ViewMode, c.ShowAllTab, c.BackdropImageURL)
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

	return CollectionWithFolders{Collection: c, Folders: folders}, nil
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

	result, err := tx.ExecContext(ctx, `
		UPDATE collections
		SET title = ?, is_public = ?, pin_to_top = ?, view_mode = ?, show_all_tab = ?, backdrop_image_url = ?
		WHERE id = ? AND owner_id = ?
	`, input.Title, input.IsPublic, input.PinToTop, input.ViewMode, input.ShowAllTab, input.BackdropImageURL,
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
	if err := validateCatalogAccess(ctx, tx, profileID, allCatalogIDs); err != nil {
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

	if err := tx.Commit(); err != nil {
		return CollectionWithFolders{}, fmt.Errorf("committing transaction: %w", err)
	}

	c := Collection{
		ID: collectionID, Title: input.Title, OwnerID: profileID,
		IsPublic: input.IsPublic, IsDefault: false, // not returned by UPDATE
		PinToTop: input.PinToTop, ViewMode: input.ViewMode,
		ShowAllTab: input.ShowAllTab, BackdropImageURL: input.BackdropImageURL,
	}

	return CollectionWithFolders{Collection: c, Folders: folders}, nil
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
// selection, in sort order, each with its folders assembled.
func (db *DB) GetCurrentCollectionSelection(ctx context.Context, profileID uuid.UUID) ([]CollectionWithFolders, error) {
	rows, err := db.conn.QueryContext(ctx, `
		SELECT c.id, c.title, c.owner_id, c.is_public, c.is_default, c.pin_to_top, c.view_mode, c.show_all_tab, c.backdrop_image_url
		FROM collections c
		JOIN profile_collections pc ON pc.collection_id = c.id
		WHERE pc.profile_id = ?
		ORDER BY pc.sort_order
	`, profileID.String())
	if err != nil {
		return nil, fmt.Errorf("querying collection selection: %w", err)
	}

	collections, err := parseCollections(rows)
	rows.Close()
	if err != nil {
		return nil, fmt.Errorf("parsing collection rows: %w", err)
	}

	return db.assembleCollectionTree(ctx, collections)
}

// saveCollectionSelectionTx resets this profile's collection selection to
// exactly input, in order, after checking it may reference every incoming
// collection. Takes a caller-supplied transaction — see
// saveCatalogSelectionTx in catalogs.go for why, and for why there is no
// exported single-selection wrapper.
func saveCollectionSelectionTx(ctx context.Context, tx *sql.Tx, profileID uuid.UUID, input CollectionSelectionForm) error {
	if err := validateCollectionAccess(ctx, tx, profileID, input.CollectionIDs); err != nil {
		return err
	}

	return syncSelection(ctx, tx, "profile_collections", "collection_id", profileID, input.CollectionIDs, func(i int, id uuid.UUID) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO profile_collections (profile_id, collection_id, sort_order)
			VALUES (?, ?, ?)
			ON CONFLICT(profile_id, collection_id) DO UPDATE SET
				sort_order = excluded.sort_order
		`, profileID.String(), id.String(), i)
		if err != nil {
			return fmt.Errorf("saving collection selection: %w", err)
		}
		return nil
	})
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
	for _, f := range folders {
		foldersByCollection[f.CollectionID] = append(foldersByCollection[f.CollectionID], FolderWithCatalogs{
			Folder:     f,
			CatalogIDs: orEmpty(catalogIDsByFolder[f.ID]),
		})
	}

	result := make([]CollectionWithFolders, len(collections))
	for i, c := range collections {
		result[i] = CollectionWithFolders{
			Collection: c,
			Folders:    orEmpty(foldersByCollection[c.ID]),
		}
	}

	return result, nil
}
