// Reading a collection's folders and folder_catalogs rows in bulk, and
// assembling them into the nested shape the API returns. The same assembly
// is what the copy path in collection_copy.go extracts a source tree from.

package vault

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/hiidz/uno/internal/jsonwire"
)

// assembleCollectionTree fetches folders, folder_catalogs and catalogs for
// the given collections through q and zips everything into the nested
// response shape. Order of the input collections slice is preserved.
func assembleCollectionTree(ctx context.Context, q querier, collections []Collection) ([]CollectionWithFolders, error) {
	if len(collections) == 0 {
		return []CollectionWithFolders{}, nil
	}

	collectionIDs := make([]uuid.UUID, len(collections))
	for i, c := range collections {
		collectionIDs[i] = c.ID
	}

	folders, err := loadFoldersByCollections(ctx, q, collectionIDs)
	if err != nil {
		return nil, err
	}

	folderIDs := make([]uuid.UUID, len(folders))
	for i, f := range folders {
		folderIDs[i] = f.ID
	}

	refsByFolder, err := loadFolderRefs(ctx, q, folderIDs)
	if err != nil {
		return nil, err
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
		catalogs, err := catalogsByIDs(ctx, q, dedupeUUIDs(allCatalogIDs))
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

// loadFoldersByCollections reads the folders of the given collections in one
// query, ordered by collection and then by sort_order within each — the order
// assembleCollectionTree's grouping relies on.
func loadFoldersByCollections(ctx context.Context, q querier, collectionIDs []uuid.UUID) ([]Folder, error) {
	placeholders, args := buildInClause(collectionIDs)
	rows, err := q.QueryContext(ctx, fmt.Sprintf(`
		SELECT id, collection_id, title, sort_order, tile_shape, hide_title, cover_emoji, cover_image_url,
		       focus_gif_url, focus_gif_enabled, hero_backdrop_url, hero_video_url, title_logo_url
		FROM folders
		WHERE collection_id IN (%s)
		ORDER BY collection_id, sort_order
	`, placeholders), args...)
	if err != nil {
		return nil, fmt.Errorf("querying folders: %w", err)
	}
	defer func() { _ = rows.Close() }()

	folders, err := parseFolders(rows)
	if err != nil {
		return nil, fmt.Errorf("parsing folder rows: %w", err)
	}
	return folders, nil
}

// loadFolderRefs reads the catalog refs of the given folders in one query,
// grouped by folder and, within each folder, in sort_order — the order
// rewriteFolderCatalogRefs writes as the ref's position in its folder. A
// folder with no refs has no entry, which reads back from the map as a nil
// slice.
func loadFolderRefs(ctx context.Context, q querier, folderIDs []uuid.UUID) (map[uuid.UUID][]FolderRef, error) {
	refsByFolder := make(map[uuid.UUID][]FolderRef, len(folderIDs))
	if len(folderIDs) == 0 {
		return refsByFolder, nil
	}

	placeholders, args := buildInClause(folderIDs)
	rows, err := q.QueryContext(ctx, fmt.Sprintf(`
		SELECT folder_id, catalog_id, sort_order, genre
		FROM folder_catalogs
		WHERE folder_id IN (%s)
		ORDER BY folder_id, sort_order
	`, placeholders), args...)
	if err != nil {
		return nil, fmt.Errorf("querying folder catalogs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	folderCatalogs, err := parseFolderCatalogs(rows)
	if err != nil {
		return nil, fmt.Errorf("parsing folder catalog rows: %w", err)
	}
	for _, fc := range folderCatalogs {
		refsByFolder[fc.FolderID] = append(refsByFolder[fc.FolderID], FolderRef{CatalogID: fc.CatalogID, Genre: fc.Genre})
	}
	return refsByFolder, nil
}
