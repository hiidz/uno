// Community: the live publications of other profiles, listed a page at a time
// as light rows, of one kind, searched and in one of two orders. A
// publication's snapshot comes from its detail.

package vault

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

// CommunityItem is one publication as Community lists it: what it is, how
// many subscribe, when it was published and last updated, whether
// the caller subscribes and has an update waiting, the names of the catalogs
// it holds, a collection's folders in order as their tiles show them, and for
// a catalog its recipe. Its publisher is never on the wire.
type CommunityItem struct {
	ID              uuid.UUID         `json:"id"`
	Kind            string            `json:"kind"`
	Title           string            `json:"title"`
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
const communityItemColumns = `p.id, p.kind, p.title, p.subscriber_count,
	p.published_at, p.updated_at, p.snapshot,
	s.id IS NOT NULL, coalesce(s.subscribed_hash <> p.content_hash, 0)`

// The Community list's page size and the bounds on its search: each word is
// one more pass over every row's catalog names, so the word count is what a
// search costs.
const (
	communityPageSize = 50
	maxSearchLen      = 200
	maxSearchWords    = 8
)

// CommunityQuery asks for one page of Community: publications of Kind
// (catalog or collection) in Sort's order (name or newest), where every word
// of Search is in the title or a catalog's name, whatever its ASCII case, and
// from after Cursor, the previous page's NextCursor, when it is set.
type CommunityQuery struct {
	Kind   string
	Sort   string
	Search string
	Cursor string
}

// CommunityPage is one page of Community, and the cursor that reads the next
// one: nil on the last page.
type CommunityPage struct {
	Items      []CommunityItem `json:"items"`
	NextCursor *string         `json:"next_cursor"`
}

// communityOrder is one of Community's orders: the column a cursor holds, the
// ORDER BY, and the condition that keeps the rows after a cursor. Each is
// written as its index declares it, so a page is a range of that index.
type communityOrder struct {
	key, orderBy, after string
}

var communityOrders = map[string]communityOrder{
	"newest": {"p.published_at", "p.published_at DESC, p.id DESC", "(p.published_at, p.id) < (:k, :id)"},
	"name":   {"p.title", "p.title COLLATE NOCASE, p.id", "(p.title COLLATE NOCASE, p.id) > (:k, :id)"},
}

// communityCursor is where a page ended: its order, and its last row's key
// column as stored and id.
type communityCursor struct {
	Sort string `json:"s"`
	Key  string `json:"k"`
	ID   string `json:"id"`
}

// communityPlan is a CommunityQuery checked: its order, its search words,
// each once whatever its case, and the cursor it reads after.
type communityPlan struct {
	kind  string
	sort  string
	order communityOrder
	words []string
	after *communityCursor
}

// ListCommunity is one page of the publications that aren't profileID's, as q
// asks, or ErrInvalidInput for a q with an unknown kind or sort, a search
// over its bounds, or a cursor that isn't one of q's sort's. Two publications
// of the same content are both listed.
func (db *DB) ListCommunity(ctx context.Context, profileID uuid.UUID, q CommunityQuery) (CommunityPage, error) {
	plan, err := q.plan()
	if err != nil {
		return CommunityPage{}, err
	}
	return db.communityPage(ctx, profileID, plan)
}

// communityPage reads the page plan asks for, for profileID.
func (db *DB) communityPage(ctx context.Context, profileID uuid.UUID, plan communityPlan) (CommunityPage, error) {
	rows, err := db.conn.QueryContext(ctx, plan.query(), plan.args(profileID)...)
	if err != nil {
		return CommunityPage{}, fmt.Errorf("querying community: %w", err)
	}
	defer func() { _ = rows.Close() }()
	items, keys, err := scanCommunityItems(rows)
	if err != nil {
		return CommunityPage{}, err
	}
	return plan.page(items, keys), nil
}

// plan checks q, refusing it with ErrInvalidInput.
func (q CommunityQuery) plan() (communityPlan, error) {
	order, err := q.order()
	if err != nil {
		return communityPlan{}, err
	}
	words, err := searchWords(q.Search)
	if err != nil {
		return communityPlan{}, err
	}
	after, err := decodeCommunityCursor(q.Sort, q.Cursor)
	if err != nil {
		return communityPlan{}, err
	}
	return communityPlan{kind: q.Kind, sort: q.Sort, order: order, words: words, after: after}, nil
}

// order is q's order, refusing an unknown sort or kind.
func (q CommunityQuery) order() (communityOrder, error) {
	if q.Kind != kindCatalog && q.Kind != kindCollection {
		return communityOrder{}, fmt.Errorf("%w: kind must be catalog or collection", ErrInvalidInput)
	}
	order, ok := communityOrders[q.Sort]
	if !ok {
		return communityOrder{}, fmt.Errorf("%w: sort must be name or newest", ErrInvalidInput)
	}
	return order, nil
}

// searchWords is search's words, each once whatever its case, refusing a search
// over maxSearchLen characters or maxSearchWords words.
func searchWords(search string) ([]string, error) {
	if utf8.RuneCountInString(search) > maxSearchLen {
		return nil, fmt.Errorf("%w: search is over %d characters", ErrInvalidInput, maxSearchLen)
	}
	words := []string{}
	seen := map[string]bool{}
	for _, word := range strings.Fields(search) {
		if key := strings.ToLower(word); !seen[key] {
			seen[key] = true
			words = append(words, word)
		}
	}
	if len(words) > maxSearchWords {
		return nil, fmt.Errorf("%w: search has over %d words", ErrInvalidInput, maxSearchWords)
	}
	return words, nil
}

// decodeCommunityCursor reads cursor, nil when it is empty, refusing one that
// is malformed or was made for an order other than sort.
func decodeCommunityCursor(sort, cursor string) (*communityCursor, error) {
	if cursor == "" {
		return nil, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	var c communityCursor
	if err == nil {
		err = json.Unmarshal(raw, &c)
	}
	if err != nil || c.Sort != sort {
		return nil, fmt.Errorf("%w: cursor is not one of this list's", ErrInvalidInput)
	}
	return &c, nil
}

// encode is c as a cursor: opaque to a client.
func (c communityCursor) encode() string {
	raw, _ := json.Marshal(c) // three strings always marshal
	return base64.RawURLEncoding.EncodeToString(raw)
}

// query is the plan's SELECT: one row past a page, which only tells whether
// another page follows.
func (p communityPlan) query() string {
	return `SELECT ` + communityItemColumns + `, ` + p.order.key + `
		FROM publications p
		LEFT JOIN subscriptions s ON s.publication_id = p.id AND s.subscriber_id = :me
		WHERE ` + strings.Join(p.conditions(), " AND ") + `
		ORDER BY ` + p.order.orderBy + `
		LIMIT :limit`
}

// conditions are the plan's WHERE terms: not the caller's, of its kind, every
// search word, and after its cursor.
func (p communityPlan) conditions() []string {
	where := []string{"p.publisher_id <> :me", "p.kind = :kind"}
	for i := range p.words {
		where = append(where, searchCondition(i))
	}
	if p.after != nil {
		where = append(where, p.order.after)
	}
	return where
}

// searchCondition is search word i's term: in the title or in one of the
// snapshot's catalog names.
func searchCondition(i int) string {
	w := ":w" + strconv.Itoa(i)
	return `(p.title LIKE ` + w + ` ESCAPE '\' OR EXISTS (SELECT 1 FROM json_each(p.snapshot, '$.catalogs') c
		WHERE json_extract(c.value, '$.name') LIKE ` + w + ` ESCAPE '\'))`
}

// args are query's parameters.
func (p communityPlan) args(profileID uuid.UUID) []any {
	args := []any{
		sql.Named("me", profileID.String()),
		sql.Named("kind", p.kind),
		sql.Named("limit", communityPageSize+1),
	}
	for i, word := range p.words {
		args = append(args, sql.Named("w"+strconv.Itoa(i), "%"+likeEscaper.Replace(word)+"%"))
	}
	if p.after != nil {
		args = append(args, sql.Named("k", p.after.Key), sql.Named("id", p.after.ID))
	}
	return args
}

// likeEscaper makes a search word match itself in a LIKE … ESCAPE '\'.
var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// page is items, read with one row past a page and keys their order's key
// columns, as a page: cut to the page size, with a cursor at its last row
// when the extra row says another page follows.
func (p communityPlan) page(items []CommunityItem, keys []string) CommunityPage {
	if len(items) <= communityPageSize {
		return CommunityPage{Items: items}
	}
	last := communityPageSize - 1
	next := communityCursor{Sort: p.sort, Key: keys[last], ID: items[last].ID.String()}.encode()
	return CommunityPage{Items: items[:communityPageSize], NextCursor: &next}
}

// rowScanner is what *sql.Rows and *sql.Row have in common.
type rowScanner interface {
	Scan(dest ...any) error
}

// keyedRow scans a row of communityItemColumns and one more column, its
// order's key, into key.
type keyedRow struct {
	rows *sql.Rows
	key  *string
}

func (r keyedRow) Scan(dest ...any) error {
	return r.rows.Scan(append(dest, r.key)...)
}

// scanCommunityItems reads every row as a CommunityItem, with its order's key
// as stored.
func scanCommunityItems(rows *sql.Rows) ([]CommunityItem, []string, error) {
	items, keys := []CommunityItem{}, []string{}
	for rows.Next() {
		var key string
		item, _, err := scanCommunityItem(keyedRow{rows, &key})
		if err != nil {
			return nil, nil, err
		}
		items, keys = append(items, item), append(keys, key)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("iterating community rows: %w", err)
	}
	return items, keys, nil
}

// scanCommunityItem reads communityItemColumns and returns the item with the
// snapshot it was read from.
func scanCommunityItem(row rowScanner) (CommunityItem, Snapshot, error) {
	var item CommunityItem
	var id, publishedAt, updatedAt, raw string
	if err := row.Scan(&id, &item.Kind, &item.Title, &item.SubscriberCount,
		&publishedAt, &updatedAt, &raw, &item.Subscribed, &item.UpdateAvailable); err != nil {
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
// ErrPublicationNotFound when there is no such publication. Unlike the list, a
// subscribe and a duplicate, it answers for profileID's own publication too: a
// publisher previews their page as everyone else sees it.
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
