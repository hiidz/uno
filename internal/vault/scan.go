package vault

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// dbtx is the common subset of *sql.DB and *sql.Tx the vault's query helpers
// need, so callers can run them either inside a transaction or straight
// against the pool.
type dbtx interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// queryUUIDs runs a query whose rows are a single TEXT column holding a UUID
// and returns them in row order. label names one such value (e.g. "folder
// id") and appears in every error this can return.
func queryUUIDs(ctx context.Context, q dbtx, label, query string, args ...any) ([]uuid.UUID, error) {
	strs, err := queryStrings(ctx, q, label, query, args...)
	if err != nil {
		return nil, err
	}
	var ids []uuid.UUID
	for _, s := range strs {
		id, err := parseUUID(s, label)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// queryStrings runs a query whose rows are a single TEXT column and returns
// them in row order. label names one such value and appears in every error
// this can return.
func queryStrings(ctx context.Context, q dbtx, label, query string, args ...any) ([]string, error) {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying %s list: %w", label, err)
	}
	defer func() { _ = rows.Close() }()

	var values []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, fmt.Errorf("scanning %s: %w", label, err)
		}
		values = append(values, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating %s list: %w", label, err)
	}
	return values, nil
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

// nullableInt converts a nullable INTEGER column into a *int, nil when the
// column is NULL — used for catalogs.home_sort_order/collections.home_sort_order.
func nullableInt(n sql.NullInt64) *int {
	if !n.Valid {
		return nil
	}
	v := int(n.Int64)
	return &v
}

// showsInHome is a catalog's show_in_home as a read gives it: the stored flag
// while homeSortOrder places the catalog on Home, and false off Home, where
// push leaves the flag as it last was.
func showsInHome(homeSortOrder sql.NullInt64, showInHome int) bool {
	return homeSortOrder.Valid && showInHome != 0
}

// rowParser parses the text columns of one scanned row, keeping the first
// error so a scanner checks once, after every column.
type rowParser struct{ err error }

// uuid parses s as field's UUID.
func (p *rowParser) uuid(s, field string) uuid.UUID {
	if p.err != nil {
		return uuid.Nil
	}
	id, err := parseUUID(s, field)
	p.err = err
	return id
}

// nullableUUID parses s as field's UUID, nil when s is NULL.
func (p *rowParser) nullableUUID(s sql.NullString, field string) *uuid.UUID {
	if !s.Valid {
		return nil
	}
	id := p.uuid(s.String, field)
	return &id
}

// timestamp parses s as field's RFC3339 time.
func (p *rowParser) timestamp(s, field string) time.Time {
	if p.err != nil {
		return time.Time{}
	}
	t, err := parseTimestamp(s, field)
	p.err = err
	return t
}

// scanCatalog reads one catalog row: the catalog columns (catalogColumns),
// its recipe's type, provider and params among them and its sharing columns
// last, in the order every catalog SELECT in this package lists them,
// followed by extraDests — destinations for any further columns the caller's
// own query appended (see GetPublishedCatalogs' ordering columns).
func scanCatalog(rows *sql.Rows, extraDests ...any) (Catalog, error) {
	var c Catalog
	var idStr, ownerIDStr, createdAtStr, updatedAtStr string
	var showInHome int
	var collectionIDStr, subKey sql.NullString
	var homeSortOrder sql.NullInt64
	var sharing sharingScan

	dests := append([]any{
		&idStr, &c.Type, &c.Name, &c.Provider, &c.Params, &ownerIDStr,
		&collectionIDStr, &homeSortOrder, &showInHome, &c.RecipeHash, &subKey,
		&c.PublisherUnpublished, &createdAtStr, &updatedAtStr,
	}, sharing.dests()...)
	if err := rows.Scan(append(dests, extraDests...)...); err != nil {
		return Catalog{}, fmt.Errorf("scanning catalog row: %w", err)
	}

	var p rowParser
	c.ID = p.uuid(idStr, "catalog id")
	c.OwnerID = p.uuid(ownerIDStr, "owner id")
	c.CollectionID = p.nullableUUID(collectionIDStr, "collection id")
	c.CreatedAt = p.timestamp(createdAtStr, "catalog created_at")
	c.UpdatedAt = p.timestamp(updatedAtStr, "catalog updated_at")
	c.Publication, c.Subscription = sharing.states(&p)
	if p.err != nil {
		return Catalog{}, p.err
	}
	c.HomeSortOrder = nullableInt(homeSortOrder)
	c.ShowInHome = showsInHome(homeSortOrder, showInHome)
	c.SubKey = subKey.String
	c.markChangedSincePublish()
	return c, nil
}

// sharingScan is a row's sharing columns as scanned (sharingColumns): its
// own publication's id and content hash, and the publication its
// subscription names, with whether an update is available. A row without
// one scans NULLs for it.
type sharingScan struct {
	publicationID, contentHash sql.NullString
	subscribedTo               sql.NullString
	updateAvailable            sql.NullBool
}

// dests are the destinations for s's columns, in sharingColumns' order.
func (s *sharingScan) dests() []any {
	return []any{&s.publicationID, &s.contentHash, &s.subscribedTo, &s.updateAvailable}
}

// states is the row's publication and subscription, each nil when the row
// has none, parsed through p.
func (s sharingScan) states(p *rowParser) (*PublicationState, *SubscriptionState) {
	var publication *PublicationState
	if id := p.nullableUUID(s.publicationID, "publication id"); id != nil {
		publication = &PublicationState{ID: *id, contentHash: s.contentHash.String}
	}
	var subscription *SubscriptionState
	if id := p.nullableUUID(s.subscribedTo, "subscription's publication id"); id != nil {
		subscription = &SubscriptionState{PublicationID: *id, UpdateAvailable: s.updateAvailable.Bool}
	}
	return publication, subscription
}

// markChangedSincePublish sets c's ChangedSincePublish when c no longer
// snapshots to what its publication holds.
func (c *Catalog) markChangedSincePublish() {
	if c.Publication != nil {
		c.Publication.ChangedSincePublish = catalogSnapshot(c.Publication.ID, *c).contentHash() != c.Publication.contentHash
	}
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
		c, err := scanCollection(rows)
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

// scanCollection reads one collection row: the columns selectCollections
// lists, its sharing columns last.
func scanCollection(rows *sql.Rows) (Collection, error) {
	var c Collection
	var idStr, ownerIDStr, createdAtStr, updatedAtStr string
	var pinToTop, showAllTab, focusGlowEnabled int
	var homeSortOrder sql.NullInt64
	var sharing sharingScan

	if err := rows.Scan(append([]any{&idStr, &c.Title, &ownerIDStr,
		&pinToTop, &c.ViewMode, &showAllTab, &c.BackdropImageURL, &focusGlowEnabled,
		&homeSortOrder, &c.PublisherUnpublished, &createdAtStr, &updatedAtStr}, sharing.dests()...)...); err != nil {
		return Collection{}, fmt.Errorf("scanning collection row: %w", err)
	}

	var p rowParser
	c.ID = p.uuid(idStr, "collection id")
	c.OwnerID = p.uuid(ownerIDStr, "owner id")
	c.CreatedAt = p.timestamp(createdAtStr, "collection created_at")
	c.UpdatedAt = p.timestamp(updatedAtStr, "collection updated_at")
	c.Publication, c.Subscription = sharing.states(&p)
	c.PinToTop = pinToTop != 0
	c.ShowAllTab = showAllTab != 0
	c.FocusGlowEnabled = focusGlowEnabled != 0
	c.HomeSortOrder = nullableInt(homeSortOrder)
	return c, p.err
}

func parseFolders(rows *sql.Rows) ([]Folder, error) {
	folders := []Folder{}
	for rows.Next() {
		var f Folder
		var idStr, collectionIDStr string
		var hideTitle, focusGIFEnabled int
		var subKey sql.NullString

		if err := rows.Scan(&idStr, &collectionIDStr, &f.Title, &f.SortOrder,
			&f.TileShape, &hideTitle, &f.CoverEmoji, &f.CoverImageURL,
			&f.FocusGIFURL, &focusGIFEnabled, &f.HeroBackdropURL, &f.HeroVideoURL, &f.TitleLogoURL, &subKey); err != nil {
			return nil, fmt.Errorf("scanning folder row: %w", err)
		}

		var p rowParser
		f.ID = p.uuid(idStr, "folder id")
		f.CollectionID = p.uuid(collectionIDStr, "collection id")
		if p.err != nil {
			return nil, p.err
		}
		f.HideTitle = hideTitle != 0
		f.FocusGIFEnabled = focusGIFEnabled != 0
		f.SubKey = subKey.String

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
