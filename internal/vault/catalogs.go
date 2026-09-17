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

// queryCatalogs runs a SELECT over catalogs with the given WHERE clause and
// args, parsing the result rows.
func (db *DB) queryCatalogs(ctx context.Context, where string, args ...any) ([]Catalog, error) {
	rows, err := db.conn.QueryContext(ctx, `
		SELECT id, type, name, provider, params, owner_id, is_public, is_default,
		       collection_id, home_sort_order, show_in_home, taken_from, fingerprint,
		       created_at, updated_at
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

// GetUserCatalogs returns the listed catalogs owned by profileID — catalogs
// scoped to a collection are excluded; they're reached through the owning
// collection's own response instead.
func (db *DB) GetUserCatalogs(ctx context.Context, profileID uuid.UUID) ([]Catalog, error) {
	return db.queryCatalogs(ctx, "owner_id = ? AND collection_id IS NULL", profileID.String())
}

// GetCommunityCatalogs returns the community catalog list for profileID: a
// public catalog owned by someone else, collapsed to one row per fingerprint
// — oldest created_at wins, ties broken by the smallest id, both fully
// deterministic rather than left to the query's row order — sorted by name,
// then created_at, then id so equal names don't swap between requests, each
// flagged with whether profileID has already taken a copy of *any* row in
// its fingerprint group — not just the surviving one, since taking a newer
// duplicate still counts as taking it.
func (db *DB) GetCommunityCatalogs(ctx context.Context, profileID uuid.UUID) ([]CommunityCatalog, error) {
	catalogs, err := db.queryCatalogs(ctx, "is_public = TRUE AND owner_id != ?", profileID.String())
	if err != nil {
		return nil, err
	}

	taken, err := db.takenSourceIDs(ctx, "catalogs", profileID)
	if err != nil {
		return nil, err
	}

	type fingerprintGroup struct {
		survivor Catalog
		taken    bool
	}
	byFingerprint := make(map[string]fingerprintGroup, len(catalogs))
	for _, c := range catalogs {
		g, ok := byFingerprint[c.Fingerprint]
		if !ok {
			byFingerprint[c.Fingerprint] = fingerprintGroup{survivor: c, taken: taken[c.ID]}
			continue
		}
		if taken[c.ID] {
			g.taken = true
		}
		if isOlderCatalog(c, g.survivor) {
			g.survivor = c
		}
		byFingerprint[c.Fingerprint] = g
	}
	collapsed := make([]fingerprintGroup, 0, len(byFingerprint))
	for _, g := range byFingerprint {
		collapsed = append(collapsed, g)
	}
	sort.Slice(collapsed, func(i, j int) bool {
		a, b := collapsed[i].survivor, collapsed[j].survivor
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return isOlderCatalog(a, b)
	})

	out := make([]CommunityCatalog, len(collapsed))
	for i, g := range collapsed {
		out[i] = CommunityCatalog{Catalog: g.survivor, Taken: g.taken}
	}
	return out, nil
}

// isOlderCatalog orders two catalogs by created_at, then by id — the
// deterministic tie-break GetCommunityCatalogs uses both for the
// fingerprint-collapse survivor and for the final list's stable ordering.
func isOlderCatalog(a, b Catalog) bool {
	if !a.CreatedAt.Equal(b.CreatedAt) {
		return a.CreatedAt.Before(b.CreatedAt)
	}
	return a.ID.String() < b.ID.String()
}

// TakeCatalog deep-copies a public catalog owned by someone else into a new
// listed catalog owned by profileID, with fresh ids and taken_from set to
// the source so the copy is unaffected by later changes to the source.
// Returns ErrCatalogNotFound if sourceID isn't public or is already owned by
// profileID.
func (db *DB) TakeCatalog(ctx context.Context, profileID uuid.UUID, sourceID uuid.UUID) (Catalog, error) {
	source, err := db.queryCatalogs(ctx, "id = ? AND is_public = TRUE AND owner_id != ?", sourceID.String(), profileID.String())
	if err != nil {
		return Catalog{}, err
	}
	if len(source) == 0 {
		return Catalog{}, ErrCatalogNotFound
	}

	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339)
	c := Catalog{
		ID:          uuid.New(),
		Type:        source[0].Type,
		Name:        source[0].Name,
		Provider:    source[0].Provider,
		Params:      source[0].Params,
		OwnerID:     profileID,
		TakenFrom:   &source[0].ID,
		Fingerprint: source[0].Fingerprint,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	_, err = db.conn.ExecContext(ctx, `
		INSERT INTO catalogs (id, type, name, provider, params, owner_id, is_public, is_default,
		                       taken_from, fingerprint, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, c.ID.String(), c.Type, c.Name, c.Provider, c.Params, c.OwnerID.String(), c.IsPublic, c.IsDefault,
		c.TakenFrom.String(), c.Fingerprint, nowStr, nowStr)
	if err != nil {
		return Catalog{}, fmt.Errorf("inserting taken catalog: %w", err)
	}

	return c, nil
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
// profileID. If input.CollectionID is set, the catalog is scoped to that
// collection (must be owned by profileID, and may not be public).
func (db *DB) CreateUserCatalog(ctx context.Context, profileID uuid.UUID, input CatalogForm) (Catalog, error) {
	if err := input.Validate(); err != nil {
		return Catalog{}, err
	}

	if input.CollectionID != nil {
		if input.IsPublic {
			return Catalog{}, fmt.Errorf("%w: a catalog scoped to a collection cannot be public", ErrInvalidInput)
		}
		if err := requireOwnedCollection(ctx, db.conn, profileID, *input.CollectionID); err != nil {
			return Catalog{}, err
		}
	}

	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339)
	c := Catalog{
		ID:           uuid.New(),
		Type:         input.Type,
		Name:         input.Name,
		Provider:     input.Provider,
		Params:       input.Params,
		OwnerID:      profileID,
		IsPublic:     input.IsPublic,
		IsDefault:    false,
		CollectionID: input.CollectionID,
		Fingerprint:  input.Fingerprint,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	_, err := db.conn.ExecContext(ctx, `
		INSERT INTO catalogs (id, type, name, provider, params, owner_id, is_public, is_default,
		                       collection_id, fingerprint, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, c.ID.String(), c.Type, c.Name, c.Provider, c.Params, c.OwnerID.String(), c.IsPublic, c.IsDefault,
		nullableUUIDString(c.CollectionID), c.Fingerprint, nowStr, nowStr)
	if err != nil {
		return Catalog{}, fmt.Errorf("inserting catalog: %w", err)
	}

	return c, nil
}

// UpdateUserCatalog validates input and updates the catalog identified by
// catalogID, provided it's owned by profileID. Returns ErrCatalogNotFound
// if no such row exists (including one owned by another profile).
//
// input.CollectionID governs scope: setting it demotes the catalog into
// that collection (it must be owned by profileID, the catalog must not be
// on the home screen, and every existing folder ref to it must already be
// inside the target collection); clearing it promotes the catalog back to
// listed, always allowed.
func (db *DB) UpdateUserCatalog(ctx context.Context, profileID uuid.UUID, catalogID uuid.UUID, input CatalogForm) (Catalog, error) {
	if err := input.Validate(); err != nil {
		return Catalog{}, err
	}

	tx, err := db.conn.BeginTx(ctx, nil)
	if err != nil {
		return Catalog{}, fmt.Errorf("starting transaction: %w", err)
	}
	defer tx.Rollback() // no-op once Commit succeeds

	var createdAtStr, existingType string
	var homeSortOrder sql.NullInt64
	var showInHome int
	err = tx.QueryRowContext(ctx, `
		SELECT created_at, home_sort_order, show_in_home, type FROM catalogs WHERE id = ? AND owner_id = ?
	`, catalogID.String(), profileID.String()).Scan(&createdAtStr, &homeSortOrder, &showInHome, &existingType)
	if errors.Is(err, sql.ErrNoRows) {
		return Catalog{}, ErrCatalogNotFound
	}
	if err != nil {
		return Catalog{}, fmt.Errorf("loading catalog: %w", err)
	}

	// A catalog's type is part of the pushed collections blob (each folder
	// source names its catalog's type), so changing it here would alter what
	// Nuvio should have without bumping any collection's version — the UI
	// already locks the field once a catalog exists, but that's a client
	// convention, not something this write path enforced on its own.
	if input.Type != existingType {
		return Catalog{}, fmt.Errorf("%w: a catalog's type can't be changed", ErrInvalidInput)
	}

	if input.CollectionID != nil {
		if input.IsPublic {
			return Catalog{}, fmt.Errorf("%w: a catalog scoped to a collection cannot be public", ErrInvalidInput)
		}
		if err := requireOwnedCollection(ctx, tx, profileID, *input.CollectionID); err != nil {
			return Catalog{}, err
		}
		if err := requireNotOnHome(ctx, tx, profileID, catalogID); err != nil {
			return Catalog{}, err
		}
		if err := requireFolderRefsWithinCollection(ctx, tx, catalogID, *input.CollectionID); err != nil {
			return Catalog{}, err
		}
	}

	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339)

	result, err := tx.ExecContext(ctx, `
		UPDATE catalogs
		SET type = ?, name = ?, provider = ?, params = ?, is_public = ?,
		    collection_id = ?, fingerprint = ?, updated_at = ?
		WHERE id = ? AND owner_id = ?
	`, input.Type, input.Name, input.Provider, input.Params, input.IsPublic,
		nullableUUIDString(input.CollectionID), input.Fingerprint, nowStr,
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

	if err := tx.Commit(); err != nil {
		return Catalog{}, fmt.Errorf("committing transaction: %w", err)
	}

	createdAt, err := parseTimestamp(createdAtStr, "catalog created_at")
	if err != nil {
		return Catalog{}, err
	}

	return Catalog{
		ID:            catalogID,
		Type:          input.Type,
		Name:          input.Name,
		Provider:      input.Provider,
		Params:        input.Params,
		OwnerID:       profileID,
		IsPublic:      input.IsPublic,
		IsDefault:     false, // not returned by UPDATE; not on the wire anyway, see models.go
		CollectionID:  input.CollectionID,
		HomeSortOrder: nullableInt(homeSortOrder), // unchanged by this update, read back for an accurate response
		ShowInHome:    showInHome != 0,
		Fingerprint:   input.Fingerprint,
		CreatedAt:     createdAt,
		UpdatedAt:     now,
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

// GetCurrentCatalogSelection returns profileID's active catalog selection —
// every owned catalog with a non-nil home_sort_order — ordered by it.
func (db *DB) GetCurrentCatalogSelection(ctx context.Context, profileID uuid.UUID) ([]SelectedCatalog, error) {
	catalogs, err := db.queryCatalogs(ctx, "owner_id = ? AND home_sort_order IS NOT NULL", profileID.String())
	if err != nil {
		return nil, err
	}

	sort.Slice(catalogs, func(i, j int) bool {
		return *catalogs[i].HomeSortOrder < *catalogs[j].HomeSortOrder
	})

	out := make([]SelectedCatalog, len(catalogs))
	for i, c := range catalogs {
		out[i] = SelectedCatalog{Catalog: c, ShowInHome: c.ShowInHome}
	}
	return out, nil
}

// GetPublishedCatalogs returns profileID's derived published catalog set —
// the union of listed catalogs on the home screen and every catalog
// referenced by a folder of a collection on the home screen. This is what
// the addon server publishes; unlike
// GetCurrentCatalogSelection (the pre-push validation/selection-editor
// view), it also surfaces folder-only catalogs so nothing a folder tile
// shows on the TV is missing from the manifest. Deduped by id: a catalog on
// both home and in a folder appears once, keeping its home ShowInHome.
func (db *DB) GetPublishedCatalogs(ctx context.Context, profileID uuid.UUID) ([]SelectedCatalog, error) {
	rows, err := db.conn.QueryContext(ctx, `
		SELECT c.id, c.type, c.name, c.provider, c.params, c.owner_id, c.is_public, c.is_default,
		       c.collection_id, c.home_sort_order, c.show_in_home, c.taken_from, c.fingerprint,
		       c.created_at, c.updated_at,
		       c.show_in_home AS derived_show_in_home, 0 AS rank, c.home_sort_order AS o1, 0 AS o2, 0 AS o3
		FROM catalogs c
		WHERE c.owner_id = ? AND c.home_sort_order IS NOT NULL
		UNION
		SELECT c.id, c.type, c.name, c.provider, c.params, c.owner_id, c.is_public, c.is_default,
		       c.collection_id, c.home_sort_order, c.show_in_home, c.taken_from, c.fingerprint,
		       c.created_at, c.updated_at,
		       0 AS derived_show_in_home, 1 AS rank, col.home_sort_order AS o1, f.sort_order AS o2, fc.sort_order AS o3
		FROM catalogs c
		JOIN folder_catalogs fc ON fc.catalog_id = c.id
		JOIN folders f          ON f.id = fc.folder_id
		JOIN collections col    ON col.id = f.collection_id
		WHERE col.owner_id = ? AND col.home_sort_order IS NOT NULL AND c.home_sort_order IS NULL
		ORDER BY rank, o1, o2, o3
	`, profileID.String(), profileID.String())
	if err != nil {
		return nil, fmt.Errorf("querying published catalogs: %w", err)
	}
	defer rows.Close()

	out := []SelectedCatalog{}
	seen := map[uuid.UUID]bool{}
	for rows.Next() {
		var c Catalog
		var idStr, ownerIDStr string
		var isPublic, isDefault, showInHome, derivedShowInHome, rank, o2, o3 int
		var collectionIDStr, takenFromStr sql.NullString
		var homeSortOrder, o1 sql.NullInt64
		var createdAtStr, updatedAtStr string

		if err := rows.Scan(&idStr, &c.Type, &c.Name, &c.Provider, &c.Params, &ownerIDStr,
			&isPublic, &isDefault, &collectionIDStr, &homeSortOrder, &showInHome, &takenFromStr,
			&c.Fingerprint, &createdAtStr, &updatedAtStr,
			&derivedShowInHome, &rank, &o1, &o2, &o3); err != nil {
			return nil, fmt.Errorf("scanning published catalog row: %w", err)
		}

		id, err := parseUUID(idStr, "catalog id")
		if err != nil {
			return nil, err
		}
		// First row wins: SQL orders home rows (rank 0) before folder-derived
		// ones (rank 1), so a catalog on both home and in a folder keeps its
		// home ShowInHome rather than the folder-derived false.
		if seen[id] {
			continue
		}
		seen[id] = true
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

		out = append(out, SelectedCatalog{Catalog: c, ShowInHome: derivedShowInHome != 0})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating published catalog rows: %w", err)
	}

	return out, nil
}

// saveCatalogSelectionTx resets this profile's catalog selection to exactly
// input, in order: every owned catalog's home_sort_order is cleared, then
// each incoming id is set in turn. A 0-rows-affected update (an id that
// isn't owned, or is scoped rather than listed) is ErrInvalidInput naming
// the id — this is the access check, not a separate query, since the same
// WHERE clause both selects and validates.
//
// Takes a caller-supplied transaction rather than opening its own: its only
// caller is SaveSelectionsForPush (push.go), which needs both selection
// writes to commit or roll back together.
func saveCatalogSelectionTx(ctx context.Context, tx *sql.Tx, profileID uuid.UUID, input CatalogSelectionForm) error {
	if _, err := tx.ExecContext(ctx, `
		UPDATE catalogs SET home_sort_order = NULL WHERE owner_id = ?
	`, profileID.String()); err != nil {
		return fmt.Errorf("clearing catalog home selection: %w", err)
	}

	for i, sc := range input.Catalogs {
		result, err := tx.ExecContext(ctx, `
			UPDATE catalogs
			SET home_sort_order = ?, show_in_home = ?
			WHERE id = ? AND owner_id = ? AND collection_id IS NULL
		`, i, sc.ShowInHome, sc.CatalogID.String(), profileID.String())
		if err != nil {
			return fmt.Errorf("saving catalog selection: %w", err)
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("checking rows affected: %w", err)
		}
		if rows == 0 {
			return fmt.Errorf("%w: catalog %s is not accessible to this profile", ErrInvalidInput, sc.CatalogID)
		}
	}

	return nil
}
