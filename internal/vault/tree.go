// Assembling flat collection, folder and folder_catalogs rows into the
// nested shape the API returns.

package vault

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/hiidz/uno/internal/jsonwire"
)

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
		folder := FolderWithCatalogs{Folder: f, Refs: jsonwire.OrEmpty(refsByFolder[f.ID])}
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
			Folders:    jsonwire.OrEmpty(foldersByCollection[c.ID]),
			Catalogs:   jsonwire.OrEmpty(catalogs),
		}
	}

	return result, nil
}
