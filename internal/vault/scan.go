package vault

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// querier is the common subset of *sql.DB and *sql.Tx the multi-row query
// helpers below need, so callers can run them either inside a transaction or
// straight against the pool — the QueryContext counterpart to access.go's
// queryRower.
type querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// queryUUIDs runs a query whose rows are a single TEXT column holding a UUID
// and returns them in row order. label names one such value (e.g. "folder
// id") and appears in every error this can return.
func queryUUIDs(ctx context.Context, q querier, label, query string, args ...any) ([]uuid.UUID, error) {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying %s list: %w", label, err)
	}
	defer func() { _ = rows.Close() }()

	var ids []uuid.UUID
	for rows.Next() {
		var idStr string
		if err := rows.Scan(&idStr); err != nil {
			return nil, fmt.Errorf("scanning %s: %w", label, err)
		}
		id, err := parseUUID(idStr, label)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating %s list: %w", label, err)
	}
	return ids, nil
}

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

// scanCatalog reads one catalog row: the fourteen catalog columns, in the
// order every catalog SELECT in this package lists them, followed by
// extraDests — destinations for any further columns the caller's own query
// appended (see GetPublishedCatalogs' ordering columns).
func scanCatalog(rows *sql.Rows, extraDests ...any) (Catalog, error) {
	var c Catalog
	var idStr, ownerIDStr string
	var isPublic, showInHome int
	var collectionIDStr, takenFromStr sql.NullString
	var homeSortOrder sql.NullInt64
	var createdAtStr, updatedAtStr string

	dests := append([]any{
		&idStr, &c.Type, &c.Name, &c.Provider,
		&c.Params, &ownerIDStr, &isPublic,
		&collectionIDStr, &homeSortOrder, &showInHome, &takenFromStr, &c.Fingerprint,
		&createdAtStr, &updatedAtStr,
	}, extraDests...)
	if err := rows.Scan(dests...); err != nil {
		return Catalog{}, fmt.Errorf("scanning catalog row: %w", err)
	}

	id, err := parseUUID(idStr, "catalog id")
	if err != nil {
		return Catalog{}, err
	}
	c.ID = id

	c.OwnerID, err = parseUUID(ownerIDStr, "owner id")
	if err != nil {
		return Catalog{}, err
	}
	c.IsPublic = isPublic != 0

	c.CollectionID, err = parseNullableUUID(collectionIDStr, "collection id")
	if err != nil {
		return Catalog{}, err
	}
	c.HomeSortOrder = nullableInt(homeSortOrder)
	c.ShowInHome = showInHome != 0
	c.TakenFrom, err = parseNullableUUID(takenFromStr, "taken_from id")
	if err != nil {
		return Catalog{}, err
	}

	c.CreatedAt, err = parseTimestamp(createdAtStr, "catalog created_at")
	if err != nil {
		return Catalog{}, err
	}
	c.UpdatedAt, err = parseTimestamp(updatedAtStr, "catalog updated_at")
	if err != nil {
		return Catalog{}, err
	}

	return c, nil
}

func parseCatalogs(rows *sql.Rows) ([]Catalog, error) {
	catalogs := []Catalog{}
	for rows.Next() {
		c, err := scanCatalog(rows)
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
		var isPublic, pinToTop, showAllTab, focusGlowEnabled, version int
		var takenFromStr sql.NullString
		var homeSortOrder, pushedVersion sql.NullInt64
		var createdAtStr, updatedAtStr string

		if err := rows.Scan(&idStr, &c.Title, &ownerIDStr, &isPublic,
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
