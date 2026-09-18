package vault

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// parseTimestamp parses an RFC3339 TEXT column into a time.Time, wrapping
// any error with the given field name.
func parseTimestamp(s, field string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("parsing %s: %w", field, err)
	}
	return t, nil
}

// parseNullableUUID parses a nullable TEXT column into a *uuid.UUID, nil
// when the column is NULL, wrapping any parse error with the given field
// name.
func parseNullableUUID(s sql.NullString, field string) (*uuid.UUID, error) {
	if !s.Valid {
		return nil, nil
	}
	id, err := parseUUID(s.String, field)
	if err != nil {
		return nil, err
	}
	return &id, nil
}

// nullableInt converts a nullable INTEGER column into a *int, nil when the
// column is NULL — used for catalogs.home_sort_order/collections.home_sort_order.
func nullableInt(n sql.NullInt64) *int {
	if !n.Valid {
		return nil
	}
	v := int(n.Int64)
	return &v
}

func parseCatalogs(rows *sql.Rows) ([]Catalog, error) {
	catalogs := []Catalog{}
	for rows.Next() {
		var c Catalog
		var idStr, ownerIDStr string
		var isPublic, isDefault, showInHome int
		var collectionIDStr, takenFromStr sql.NullString
		var homeSortOrder sql.NullInt64
		var createdAtStr, updatedAtStr string

		if err := rows.Scan(&idStr, &c.Type, &c.Name, &c.Provider,
			&c.Params, &ownerIDStr, &isPublic, &isDefault,
			&collectionIDStr, &homeSortOrder, &showInHome, &takenFromStr, &c.Fingerprint,
			&createdAtStr, &updatedAtStr); err != nil {
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

		c.CollectionID, err = parseNullableUUID(collectionIDStr, "collection id")
		if err != nil {
			return nil, err
		}
		c.HomeSortOrder = nullableInt(homeSortOrder)
		c.ShowInHome = showInHome != 0
		c.TakenFrom, err = parseNullableUUID(takenFromStr, "taken_from id")
		if err != nil {
			return nil, err
		}

		c.CreatedAt, err = parseTimestamp(createdAtStr, "catalog created_at")
		if err != nil {
			return nil, err
		}
		c.UpdatedAt, err = parseTimestamp(updatedAtStr, "catalog updated_at")
		if err != nil {
			return nil, err
		}

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
		var isPublic, isDefault, pinToTop, showAllTab, focusGlowEnabled, version int
		var takenFromStr sql.NullString
		var homeSortOrder, pushedVersion sql.NullInt64
		var createdAtStr, updatedAtStr string

		if err := rows.Scan(&idStr, &c.Title, &ownerIDStr, &isPublic, &isDefault,
			&pinToTop, &c.ViewMode, &showAllTab, &c.BackdropImageURL, &focusGlowEnabled,
			&homeSortOrder, &version, &pushedVersion, &takenFromStr, &createdAtStr, &updatedAtStr); err != nil {
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
		c.FocusGlowEnabled = focusGlowEnabled != 0

		c.TakenFrom, err = parseNullableUUID(takenFromStr, "taken_from id")
		if err != nil {
			return nil, err
		}
		c.HomeSortOrder = nullableInt(homeSortOrder)
		c.Version = version
		c.PushedVersion = nullableInt(pushedVersion)

		c.CreatedAt, err = parseTimestamp(createdAtStr, "collection created_at")
		if err != nil {
			return nil, err
		}
		c.UpdatedAt, err = parseTimestamp(updatedAtStr, "collection updated_at")
		if err != nil {
			return nil, err
		}

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
		var hideTitle, focusGIFEnabled int

		if err := rows.Scan(&idStr, &collectionIDStr, &f.Title, &f.SortOrder,
			&f.TileShape, &hideTitle, &f.CoverEmoji, &f.CoverImageURL,
			&f.FocusGIFURL, &focusGIFEnabled, &f.HeroBackdropURL, &f.HeroVideoURL, &f.TitleLogoURL); err != nil {
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
		f.FocusGIFEnabled = focusGIFEnabled != 0

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

		if err := rows.Scan(&folderIDStr, &catalogIDStr, &fc.SortOrder, &fc.Genre); err != nil {
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
