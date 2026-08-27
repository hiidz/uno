package vault

import (
	"database/sql"
	"fmt"
)

func parseCatalogs(rows *sql.Rows) ([]Catalog, error) {
	catalogs := []Catalog{}
	for rows.Next() {
		var c Catalog
		var idStr, ownerIDStr string
		var isPublic, isDefault int

		if err := rows.Scan(&idStr, &c.Type, &c.Name, &c.Provider,
			&c.Params, &ownerIDStr, &isPublic, &isDefault); err != nil {
			return nil, fmt.Errorf("scanning catalog row: %w", err)
		}

		id, err := parseUUID(idStr, "catalog id")
		if err != nil {
			return nil, err
		}
		c.ID = id

		c.OwnerID, err = parseUUID(ownerIDStr, "owner id")
		if err != nil {
			return nil, err
		}
		c.IsPublic = isPublic != 0
		c.IsDefault = isDefault != 0

		catalogs = append(catalogs, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating catalog rows: %w", err)
	}
	return catalogs, nil
}

func parseCollections(rows *sql.Rows) ([]Collection, error) {
	collections := []Collection{}
	for rows.Next() {
		var c Collection
		var idStr, ownerIDStr string
		var isPublic, isDefault, pinToTop, showAllTab int

		if err := rows.Scan(&idStr, &c.Title, &ownerIDStr, &isPublic, &isDefault,
			&pinToTop, &c.ViewMode, &showAllTab, &c.BackdropImageURL); err != nil {
			return nil, fmt.Errorf("scanning collection row: %w", err)
		}

		id, err := parseUUID(idStr, "collection id")
		if err != nil {
			return nil, err
		}
		c.ID = id

		c.OwnerID, err = parseUUID(ownerIDStr, "owner id")
		if err != nil {
			return nil, err
		}
		c.IsPublic = isPublic != 0
		c.IsDefault = isDefault != 0
		c.PinToTop = pinToTop != 0
		c.ShowAllTab = showAllTab != 0

		collections = append(collections, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating collection rows: %w", err)
	}
	return collections, nil
}

func parseFolders(rows *sql.Rows) ([]Folder, error) {
	folders := []Folder{}
	for rows.Next() {
		var f Folder
		var idStr, collectionIDStr string
		var hideTitle int

		if err := rows.Scan(&idStr, &collectionIDStr, &f.Title, &f.SortOrder,
			&f.TileShape, &hideTitle, &f.CoverEmoji, &f.CoverImageURL); err != nil {
			return nil, fmt.Errorf("scanning folder row: %w", err)
		}

		id, err := parseUUID(idStr, "folder id")
		if err != nil {
			return nil, err
		}
		f.ID = id

		f.CollectionID, err = parseUUID(collectionIDStr, "collection id")
		if err != nil {
			return nil, err
		}
		f.HideTitle = hideTitle != 0

		folders = append(folders, f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating folder rows: %w", err)
	}
	return folders, nil
}

func parseFolderCatalogs(rows *sql.Rows) ([]FolderCatalog, error) {
	refs := []FolderCatalog{}
	for rows.Next() {
		var fc FolderCatalog
		var folderIDStr, catalogIDStr string

		if err := rows.Scan(&folderIDStr, &catalogIDStr, &fc.SortOrder); err != nil {
			return nil, fmt.Errorf("scanning folder_catalog row: %w", err)
		}

		id, err := parseUUID(folderIDStr, "folder id")
		if err != nil {
			return nil, err
		}
		fc.FolderID = id

		fc.CatalogID, err = parseUUID(catalogIDStr, "catalog id")
		if err != nil {
			return nil, err
		}

		refs = append(refs, fc)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating folder_catalog rows: %w", err)
	}
	return refs, nil
}
