package vault

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/google/uuid"
)

// queryCatalogs runs a SELECT over catalogs with the given WHERE clause and
// args, parsing the result rows.
func (db *DB) queryCatalogs(ctx context.Context, where string, args ...any) ([]Catalog, error) {
	rows, err := db.conn.QueryContext(ctx, `
		SELECT id, type, name, provider, params, owner_id, is_public, is_default
		FROM catalogs
		WHERE `+where, args...)
	if err != nil {
		return nil, fmt.Errorf("querying catalogs: %w", err)
	}
	defer rows.Close()

	catalogs, err := parseCatalogs(rows)
	if err != nil {
		return nil, fmt.Errorf("parsing catalog rows: %w", err)
	}

	return catalogs, nil
}

// GetUserCatalogs returns the catalogs owned by profileID.
func (db *DB) GetUserCatalogs(ctx context.Context, profileID uuid.UUID) ([]Catalog, error) {
	return db.queryCatalogs(ctx, "owner_id = ?", profileID.String())
}

// GetCommunityCatalogs returns every catalog marked public, regardless of
// owner.
func (db *DB) GetCommunityCatalogs(ctx context.Context) ([]Catalog, error) {
	return db.queryCatalogs(ctx, "is_public = TRUE")
}

// GetCatalogsByIDs batch-loads catalogs by id, no ownership check — push
// uses this to resolve a folder's catalog_ids (already access-checked at
// selection time) into Type/Provider for building catalogSources.
func (db *DB) GetCatalogsByIDs(ctx context.Context, ids []uuid.UUID) ([]Catalog, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	placeholders, args := buildInClause(ids)
	return db.queryCatalogs(ctx, fmt.Sprintf("id IN (%s)", placeholders), args...)
}

// CreateUserCatalog validates input and inserts a new catalog owned by
// profileID.
func (db *DB) CreateUserCatalog(ctx context.Context, profileID uuid.UUID, input CatalogForm) (Catalog, error) {
	if err := input.Validate(); err != nil {
		return Catalog{}, err
	}

	c := Catalog{
		ID:        uuid.New(),
		Type:      input.Type,
		Name:      input.Name,
		Provider:  input.Provider,
		Params:    input.Params,
		OwnerID:   profileID,
		IsPublic:  input.IsPublic,
		IsDefault: false,
	}

	_, err := db.conn.ExecContext(ctx, `
		INSERT INTO catalogs (id, type, name, provider, params, owner_id, is_public, is_default)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, c.ID.String(), c.Type, c.Name, c.Provider, c.Params, c.OwnerID.String(), c.IsPublic, c.IsDefault)
	if err != nil {
		return Catalog{}, fmt.Errorf("inserting catalog: %w", err)
	}

	return c, nil
}

// UpdateUserCatalog validates input and updates the catalog identified by
// catalogID, provided it's owned by profileID. Returns ErrCatalogNotFound
// if no such row exists (including one owned by another profile).
func (db *DB) UpdateUserCatalog(ctx context.Context, profileID uuid.UUID, catalogID uuid.UUID, input CatalogForm) (Catalog, error) {
	if err := input.Validate(); err != nil {
		return Catalog{}, err
	}

	result, err := db.conn.ExecContext(ctx, `
		UPDATE catalogs
		SET type = ?, name = ?, provider = ?, params = ?, is_public = ?
		WHERE id = ? AND owner_id = ?
	`, input.Type, input.Name, input.Provider, input.Params, input.IsPublic,
		catalogID.String(), profileID.String())
	if err != nil {
		return Catalog{}, fmt.Errorf("updating catalog: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return Catalog{}, fmt.Errorf("checking rows affected: %w", err)
	}
	if rows == 0 {
		return Catalog{}, ErrCatalogNotFound
	}

	return Catalog{
		ID:        catalogID,
		Type:      input.Type,
		Name:      input.Name,
		Provider:  input.Provider,
		Params:    input.Params,
		OwnerID:   profileID,
		IsPublic:  input.IsPublic,
		IsDefault: false, // not returned by UPDATE; not on the wire anyway, see models.go
	}, nil
}

// DeleteUserCatalog deletes the catalog identified by catalogID, provided
// it's owned by profileID. Returns ErrCatalogNotFound otherwise.
func (db *DB) DeleteUserCatalog(ctx context.Context, profileID uuid.UUID, catalogID uuid.UUID) error {
	result, err := db.conn.ExecContext(ctx, `
		DELETE FROM catalogs WHERE id = ? AND owner_id = ?
	`, catalogID.String(), profileID.String())
	if err != nil {
		return fmt.Errorf("deleting catalog: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if rows == 0 {
		return ErrCatalogNotFound
	}

	return nil
}

// catalogSelectionRow is one profile_catalogs row: which catalog, in what
// order, with what show_in_home flag — everything except the catalog's own
// columns, which come from queryCatalogs/parseCatalogs instead of a second
// hand-rolled scan.
type catalogSelectionRow struct {
	catalogID  uuid.UUID
	showInHome bool
}

// GetCurrentCatalogSelection returns profileID's active catalog selection,
// in sort order, joined with each catalog's own columns.
func (db *DB) GetCurrentCatalogSelection(ctx context.Context, profileID uuid.UUID) ([]SelectedCatalog, error) {
	rows, err := db.conn.QueryContext(ctx, `
		SELECT catalog_id, show_in_home
		FROM profile_catalogs
		WHERE profile_id = ?
		ORDER BY sort_order
	`, profileID.String())
	if err != nil {
		return nil, fmt.Errorf("querying catalog selection: %w", err)
	}

	var selection []catalogSelectionRow
	for rows.Next() {
		var idStr string
		var showInHome bool
		if err := rows.Scan(&idStr, &showInHome); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scanning catalog selection row: %w", err)
		}
		id, err := parseUUID(idStr, "catalog id")
		if err != nil {
			rows.Close()
			return nil, err
		}
		selection = append(selection, catalogSelectionRow{catalogID: id, showInHome: showInHome})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("iterating catalog selection rows: %w", err)
	}
	rows.Close()

	if len(selection) == 0 {
		return []SelectedCatalog{}, nil
	}

	ids := make([]uuid.UUID, len(selection))
	for i, s := range selection {
		ids[i] = s.catalogID
	}
	placeholders, args := buildInClause(ids)
	catalogs, err := db.queryCatalogs(ctx, fmt.Sprintf("id IN (%s)", placeholders), args...)
	if err != nil {
		return nil, err
	}

	byID := make(map[uuid.UUID]Catalog, len(catalogs))
	for _, c := range catalogs {
		byID[c.ID] = c
	}

	out := make([]SelectedCatalog, len(selection))
	for i, s := range selection {
		out[i] = SelectedCatalog{Catalog: byID[s.catalogID], ShowInHome: s.showInHome}
	}
	return out, nil
}

// saveCatalogSelectionTx resets this profile's catalog selection to exactly
// input, in order, after checking it may reference every incoming catalog.
//
// Takes a caller-supplied transaction rather than opening its own: its only
// caller is SaveSelectionsForPush (push.go), which needs both selection
// writes to commit or roll back together.
func saveCatalogSelectionTx(ctx context.Context, tx *sql.Tx, profileID uuid.UUID, input CatalogSelectionForm) error {
	incomingIDs := make([]uuid.UUID, len(input.Catalogs))
	for i, sc := range input.Catalogs {
		incomingIDs[i] = sc.CatalogID
	}
	if err := validateCatalogAccess(ctx, tx, profileID, incomingIDs); err != nil {
		return err
	}

	return syncSelection(ctx, tx, "profile_catalogs", "catalog_id", profileID, incomingIDs, func(i int, id uuid.UUID) error {
		sc := input.Catalogs[i]
		_, err := tx.ExecContext(ctx, `
			INSERT INTO profile_catalogs (profile_id, catalog_id, show_in_home, sort_order)
			VALUES (?, ?, ?, ?)
			ON CONFLICT(profile_id, catalog_id) DO UPDATE SET
				show_in_home = excluded.show_in_home,
				sort_order = excluded.sort_order
		`, profileID.String(), sc.CatalogID.String(), sc.ShowInHome, i)
		if err != nil {
			return fmt.Errorf("saving catalog selection: %w", err)
		}
		return nil
	})
}
