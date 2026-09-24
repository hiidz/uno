package vault

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
)

// queryCatalogs runs a SELECT over catalogs with the given WHERE clause and
// args, parsing the result rows. where is built from this package's own
// literals and buildInClause placeholders — never from client input, which
// reaches the query only as a bound arg.
//
//nolint:gosec // G202: see above — the concatenated where is an internal literal.
func (db *DB) queryCatalogs(ctx context.Context, where string, args ...any) ([]Catalog, error) {
	rows, err := db.conn.QueryContext(ctx, `
		SELECT id, type, name, provider, params, owner_id, is_public,
		       collection_id, home_sort_order, show_in_home, taken_from, fingerprint,
		       created_at, updated_at
		FROM catalogs
		WHERE `+where, args...)
	if err != nil {
		return nil, fmt.Errorf("querying catalogs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return parseCatalogs(rows)
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
	slices.SortFunc(collapsed, func(x, y fingerprintGroup) int {
		a, b := x.survivor, y.survivor
		if c := cmp.Compare(a.Name, b.Name); c != 0 {
			return c
		}
		return compareCreatedThenID(a.CreatedAt, b.CreatedAt, a.ID, b.ID)
	})

	out := make([]CommunityCatalog, len(collapsed))
	for i, g := range collapsed {
		out[i] = CommunityCatalog{Catalog: g.survivor, Taken: g.taken}
	}
	return out, nil
}

// compareByHomeSortOrder orders catalogs by HomeSortOrder, nil-safe: a nil
// order (not currently selected) sorts after every non-nil one rather than
// panicking, so this doesn't depend on the caller's WHERE clause having
// filtered them out.
func compareByHomeSortOrder(a, b Catalog) int {
	switch {
	case a.HomeSortOrder == nil && b.HomeSortOrder == nil:
		return 0
	case a.HomeSortOrder == nil:
		return 1
	case b.HomeSortOrder == nil:
		return -1
	default:
		return cmp.Compare(*a.HomeSortOrder, *b.HomeSortOrder)
	}
}

// isOlderCatalog reads compareCreatedThenID as the "a comes first" test
// GetCommunityCatalogs' fingerprint collapse asks of a candidate survivor.
func isOlderCatalog(a, b Catalog) bool {
	return compareCreatedThenID(a.CreatedAt, b.CreatedAt, a.ID, b.ID) < 0
}

// CatalogParamsValidator re-checks a catalog recipe that is about to be
// copied out of another profile's row. This package is the leaf of the
// dependency graph and cannot reach internal/provider, so the check is
// passed in by the caller that can (api.validateCatalogParams).
type CatalogParamsValidator func(catalogType, catalogProvider, params string) error

// validateSourceCatalog re-checks a catalog row about to be copied against
// the length bounds CatalogForm.Validate applies to a save — the
// single-catalog half of what validateSourceCollection does for a whole
// tree, so the same listed row is bounded whichever door it is taken
// through. Its type and provider are left to the caller's params validator,
// which owns the recipe.
func validateSourceCatalog(source Catalog) error {
	problems := appendProblem(nil, lengthProblem("name", source.Name, maxNameLen))
	problems = appendProblem(problems, lengthProblem("params", source.Params, maxParamsLen))
	if len(problems) == 0 {
		return nil
	}
	return fmt.Errorf("%w: source catalog: %s", ErrInvalidInput, strings.Join(problems, "; "))
}

// TakeCatalog deep-copies a public catalog owned by someone else into a new
// listed catalog owned by profileID, with fresh ids and taken_from set to
// the source so the copy is unaffected by later changes to the source.
// Returns ErrCatalogNotFound if sourceID isn't public or is already owned by
// profileID, and ErrInvalidInput if the source row's name or params are
// past the bounds a save enforces.
//
// validateParams re-runs the create/update params check against the source
// row before anything is copied: the recipe is someone else's input, and it
// was validated when they wrote it, not when this profile takes it. It is
// required — a nil validator is a programming error, not "skip the check".
func (db *DB) TakeCatalog(ctx context.Context, profileID uuid.UUID, sourceID uuid.UUID, validateParams CatalogParamsValidator) (Catalog, error) {
	if validateParams == nil {
		return Catalog{}, errors.New("vault: TakeCatalog requires a params validator")
	}

	source, err := db.queryCatalogs(ctx, "id = ? AND is_public = TRUE AND owner_id != ?", sourceID.String(), profileID.String())
	if err != nil {
		return Catalog{}, err
	}
	if len(source) == 0 {
		return Catalog{}, ErrCatalogNotFound
	}
	if err := validateSourceCatalog(source[0]); err != nil {
		return Catalog{}, err
	}
	if err := validateParams(source[0].Type, source[0].Provider, source[0].Params); err != nil {
		return Catalog{}, err
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
		INSERT INTO catalogs (id, type, name, provider, params, owner_id, is_public,
		                       taken_from, fingerprint, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, c.ID.String(), c.Type, c.Name, c.Provider, c.Params, c.OwnerID.String(), c.IsPublic,
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
		CollectionID: input.CollectionID,
		Fingerprint:  input.Fingerprint,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	_, err := db.conn.ExecContext(ctx, `
		INSERT INTO catalogs (id, type, name, provider, params, owner_id, is_public,
		                       collection_id, fingerprint, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, c.ID.String(), c.Type, c.Name, c.Provider, c.Params, c.OwnerID.String(), c.IsPublic,
		nullableUUIDString(c.CollectionID), c.Fingerprint, nowStr, nowStr)
	if err != nil {
		return Catalog{}, fmt.Errorf("inserting catalog: %w", err)
	}

	return c, nil
}

// UpdateUserCatalog validates input and updates the listed catalog
// identified by catalogID, provided it's owned by profileID. Returns
// ErrCatalogNotFound if no such row exists (including one owned by another
// profile), and ErrInvalidInput for a catalog inside a collection — see
// checkCatalogRewrite.
//
// input.CollectionID set demotes the catalog into that collection: it must be
// owned by profileID, the catalog must not be on the home screen, and every
// existing folder ref to it must already be inside the target collection.
// The way back to listed is the collection's own save
// (ScopedCatalogEdit.MoveToLibrary).
func (db *DB) UpdateUserCatalog(ctx context.Context, profileID uuid.UUID, catalogID uuid.UUID, input CatalogForm) (Catalog, error) {
	if err := input.Validate(); err != nil {
		return Catalog{}, err
	}

	tx, err := db.conn.BeginTx(ctx, nil)
	if err != nil {
		return Catalog{}, fmt.Errorf("starting transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // no-op once Commit succeeds

	var createdAtStr, existingType string
	var homeSortOrder sql.NullInt64
	var showInHome int
	var existingCollectionID sql.NullString
	err = tx.QueryRowContext(ctx, `
		SELECT created_at, home_sort_order, show_in_home, type, collection_id FROM catalogs WHERE id = ? AND owner_id = ?
	`, catalogID.String(), profileID.String()).Scan(&createdAtStr, &homeSortOrder, &showInHome, &existingType, &existingCollectionID)
	if errors.Is(err, sql.ErrNoRows) {
		return Catalog{}, ErrCatalogNotFound
	}
	if err != nil {
		return Catalog{}, fmt.Errorf("loading catalog: %w", err)
	}

	if err := checkCatalogRewrite(existingType, existingCollectionID, input); err != nil {
		return Catalog{}, err
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
		CollectionID:  input.CollectionID,
		HomeSortOrder: nullableInt(homeSortOrder), // unchanged by this update, read back for an accurate response
		ShowInHome:    showInHome != 0,
		Fingerprint:   input.Fingerprint,
		CreatedAt:     createdAt,
		UpdatedAt:     now,
	}, nil
}

// checkCatalogRewrite refuses an UpdateUserCatalog the stored row can't take:
//
//   - A catalog inside a collection (existingCollectionID set) is written only
//     through that collection's save (CollectionForm.CatalogEdits), so an edit
//     made in the collection editor lands, or is discarded, with the rest of
//     the collection. Moving a listed catalog into a collection is still this
//     method's job: the row being written is listed until the write lands.
//   - A catalog's type is part of the pushed collections blob (each folder
//     source names its catalog's type), so changing it here would alter what
//     Nuvio should have without bumping any collection's version. The UI
//     locks the field once a catalog exists; this is the server enforcing it.
func checkCatalogRewrite(existingType string, existingCollectionID sql.NullString, input CatalogForm) error {
	if existingCollectionID.Valid {
		return fmt.Errorf("%w: a catalog inside a collection is edited through the collection's save", ErrInvalidInput)
	}
	if input.Type != existingType {
		return fmt.Errorf("%w: a catalog's type can't be changed", ErrInvalidInput)
	}
	return nil
}

// DeleteUserCatalog deletes the listed catalog identified by catalogID,
// provided it's owned by profileID. Returns ErrCatalogNotFound if no such row
// exists, and ErrInvalidInput for a catalog inside a collection, which is
// removed by dropping its last folder ref and saving the collection
// (deleteOrphanedScopedCatalogs).
func (db *DB) DeleteUserCatalog(ctx context.Context, profileID uuid.UUID, catalogID uuid.UUID) error {
	result, err := db.conn.ExecContext(ctx, `
		DELETE FROM catalogs WHERE id = ? AND owner_id = ? AND collection_id IS NULL
	`, catalogID.String(), profileID.String())
	if err != nil {
		return fmt.Errorf("deleting catalog: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if rows == 0 {
		return db.catalogNotDeleted(ctx, profileID, catalogID)
	}

	return nil
}

// catalogNotDeleted explains a DeleteUserCatalog that matched no row: the
// catalog is either not owned by profileID at all (ErrCatalogNotFound) or
// inside a collection (ErrInvalidInput).
func (db *DB) catalogNotDeleted(ctx context.Context, profileID, catalogID uuid.UUID) error {
	var exists int
	err := db.conn.QueryRowContext(ctx, `
		SELECT 1 FROM catalogs WHERE id = ? AND owner_id = ?
	`, catalogID.String(), profileID.String()).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrCatalogNotFound
	}
	if err != nil {
		return fmt.Errorf("checking catalog: %w", err)
	}
	return fmt.Errorf("%w: a catalog inside a collection is removed through the collection's save", ErrInvalidInput)
}

// GetCurrentCatalogSelection returns profileID's active catalog selection —
// every owned catalog with a non-nil home_sort_order — ordered by it.
func (db *DB) GetCurrentCatalogSelection(ctx context.Context, profileID uuid.UUID) ([]SelectedCatalog, error) {
	catalogs, err := db.queryCatalogs(ctx, "owner_id = ? AND home_sort_order IS NOT NULL", profileID.String())
	if err != nil {
		return nil, err
	}

	slices.SortFunc(catalogs, compareByHomeSortOrder)

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
// shows on the TV is missing from the manifest. Deduped by id: a catalog
// sitting in more than one folder would otherwise appear once per folder.
func (db *DB) GetPublishedCatalogs(ctx context.Context, profileID uuid.UUID) ([]SelectedCatalog, error) {
	rows, err := db.conn.QueryContext(ctx, `
		SELECT c.id, c.type, c.name, c.provider, c.params, c.owner_id, c.is_public,
		       c.collection_id, c.home_sort_order, c.show_in_home, c.taken_from, c.fingerprint,
		       c.created_at, c.updated_at,
		       c.show_in_home AS derived_show_in_home, 0 AS rank, c.home_sort_order AS o1, 0 AS o2, 0 AS o3
		FROM catalogs c
		WHERE c.owner_id = ? AND c.home_sort_order IS NOT NULL
		UNION
		SELECT c.id, c.type, c.name, c.provider, c.params, c.owner_id, c.is_public,
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
	defer func() { _ = rows.Close() }()

	out := []SelectedCatalog{}
	seen := map[uuid.UUID]bool{}
	for rows.Next() {
		var derivedShowInHome, rank, o2, o3 int
		var o1 sql.NullInt64

		c, err := scanCatalog(rows, &derivedShowInHome, &rank, &o1, &o2, &o3)
		if err != nil {
			return nil, err
		}

		// The two SELECTs' WHERE clauses are mutually exclusive on
		// home_sort_order, so a catalog can't match both; seen instead
		// collapses duplicate folder-derived rows for a catalog sitting in
		// more than one folder.
		if seen[c.ID] {
			continue
		}
		seen[c.ID] = true

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
