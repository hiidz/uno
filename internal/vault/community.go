// Community: the live publications of other profiles, listed in one call as
// light rows, which the SPA searches, filters and sorts itself. A
// publication's snapshot comes from its detail.

package vault

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// CommunityItem is one publication as Community lists it: what it is, how
// big, how many subscribe, when it was published and last updated, whether
// the caller subscribes and has an update waiting, the names of the catalogs
// it holds, a collection's folders in order as their tiles show them, and for
// a catalog its recipe. Its publisher is never on the wire.
type CommunityItem struct {
	ID              uuid.UUID         `json:"id"`
	Kind            string            `json:"kind"`
	Title           string            `json:"title"`
	CatalogCount    int               `json:"catalog_count"`
	FolderCount     int               `json:"folder_count"`
	SubscriberCount int               `json:"subscriber_count"`
	PublishedAt     time.Time         `json:"published_at"`
	UpdatedAt       time.Time         `json:"updated_at"`
	Subscribed      bool              `json:"subscribed"`
	UpdateAvailable bool              `json:"update_available"`
	CatalogNames    []string          `json:"catalog_names"`
	Folders         []CommunityFolder `json:"folders"`
	Catalog         *BundleCatalog    `json:"catalog"`
}

// CommunityFolder is one folder of a listed collection, as much as its tile
// shows: its title, tile shape and cover.
type CommunityFolder struct {
	Title         string `json:"title"`
	TileShape     string `json:"tile_shape"`
	CoverEmoji    string `json:"cover_emoji"`
	CoverImageURL string `json:"cover_image_url"`
}

// PublicationDetail is one publication with its snapshot.
type PublicationDetail struct {
	CommunityItem
	Snapshot Snapshot `json:"snapshot"`
}

// communityItemColumns are the columns scanCommunityItem reads, from
// publications as p and the caller's subscriptions as s.
const communityItemColumns = `p.id, p.kind, p.title, p.catalog_count, p.folder_count, p.subscriber_count,
	p.published_at, p.updated_at, p.snapshot,
	s.id IS NOT NULL, coalesce(s.subscribed_hash <> p.content_hash, 0)`

// ListCommunity is every publication that isn't profileID's, newest first.
// Two publications of the same content are both listed.
func (db *DB) ListCommunity(ctx context.Context, profileID uuid.UUID) ([]CommunityItem, error) {
	rows, err := db.conn.QueryContext(ctx, `
		SELECT `+communityItemColumns+`
		FROM publications p
		LEFT JOIN subscriptions s ON s.publication_id = p.id AND s.subscriber_id = :me
		WHERE p.publisher_id <> :me
		ORDER BY p.published_at DESC, p.id DESC
	`, sql.Named("me", profileID.String()))
	if err != nil {
		return nil, fmt.Errorf("querying community: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanCommunityItems(rows)
}

// rowScanner is what *sql.Rows and *sql.Row have in common.
type rowScanner interface {
	Scan(dest ...any) error
}

// scanCommunityItems reads every row as a CommunityItem.
func scanCommunityItems(rows *sql.Rows) ([]CommunityItem, error) {
	items := []CommunityItem{}
	for rows.Next() {
		item, _, err := scanCommunityItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating community rows: %w", err)
	}
	return items, nil
}

// scanCommunityItem reads communityItemColumns and returns the item with the
// snapshot it was read from.
func scanCommunityItem(row rowScanner) (CommunityItem, Snapshot, error) {
	var item CommunityItem
	var id, publishedAt, updatedAt, raw string
	if err := row.Scan(&id, &item.Kind, &item.Title, &item.CatalogCount, &item.FolderCount,
		&item.SubscriberCount, &publishedAt, &updatedAt, &raw, &item.Subscribed, &item.UpdateAvailable); err != nil {
		return CommunityItem{}, Snapshot{}, err
	}
	var p rowParser
	item.ID = p.uuid(id, "publication id")
	item.PublishedAt = p.timestamp(publishedAt, "publication published_at")
	item.UpdatedAt = p.timestamp(updatedAt, "publication updated_at")
	if p.err != nil {
		return CommunityItem{}, Snapshot{}, p.err
	}
	return item.withSnapshot(raw)
}

// withSnapshot is item with what its row shows of raw, its publication's
// snapshot, and that snapshot decoded.
func (item CommunityItem) withSnapshot(raw string) (CommunityItem, Snapshot, error) {
	s, err := decodeSnapshot(raw)
	if err != nil {
		return CommunityItem{}, Snapshot{}, err
	}
	item.CatalogNames, item.Catalog = s.listed(item.Kind)
	item.Folders = s.listedFolders()
	return item, s, nil
}

// listedFolders is s's collection's folders in order as their tiles show
// them: empty for a catalog's snapshot, never nil.
func (s Snapshot) listedFolders() []CommunityFolder {
	folders := []CommunityFolder{}
	if s.Collection == nil {
		return folders
	}
	for _, f := range s.Collection.Folders {
		folders = append(folders, CommunityFolder{
			Title: f.Title, TileShape: f.TileShape, CoverEmoji: f.CoverEmoji, CoverImageURL: f.CoverImageURL,
		})
	}
	return folders
}

// listed is what a Community row shows of s, a publication of kind: the
// names of its catalogs, and a catalog publication's one catalog, nil for a
// collection's.
func (s Snapshot) listed(kind string) ([]string, *BundleCatalog) {
	names := make([]string, len(s.Catalogs))
	for i, c := range s.Catalogs {
		names[i] = c.Name
	}
	if kind != kindCatalog {
		return names, nil
	}
	return names, &s.Catalogs[0]
}

// GetPublication is publicationID with its snapshot, for profileID, or
// ErrPublicationNotFound when there is no such publication.
func (db *DB) GetPublication(ctx context.Context, profileID, publicationID uuid.UUID) (PublicationDetail, error) {
	row := db.conn.QueryRowContext(ctx, `
		SELECT `+communityItemColumns+`
		FROM publications p
		LEFT JOIN subscriptions s ON s.publication_id = p.id AND s.subscriber_id = :me
		WHERE p.id = :id
	`, sql.Named("me", profileID.String()), sql.Named("id", publicationID.String()))
	item, snapshot, err := scanCommunityItem(row)
	if errors.Is(err, sql.ErrNoRows) {
		return PublicationDetail{}, ErrPublicationNotFound
	}
	if err != nil {
		return PublicationDetail{}, fmt.Errorf("loading publication: %w", err)
	}
	return PublicationDetail{CommunityItem: item, Snapshot: snapshot}, nil
}
