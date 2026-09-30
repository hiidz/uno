package migrations

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"slices"
	"strings"
	"testing"
)

// runPushedHashes runs migration 5 in a transaction on d and commits it when
// it succeeds.
func runPushedHashes(t *testing.T, d *sql.DB) ([]string, error) {
	t.Helper()
	tx, err := d.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	notes, err := pushedHashes(context.Background(), tx)
	if err != nil {
		return notes, err
	}
	return notes, tx.Commit()
}

// weekendPushJSON is the push JSON of the fixture's first Weekend, as push
// sends it: the emoji as is, an empty genre and empty URLs left out, and the
// sources in each folder's order.
const weekendPushJSON = `{"id":"cccccccc-0000-4000-8000-000000000001","title":"Weekend",` +
	`"backdropImageUrl":"https://image.tmdb.org/t/p/original/weekend.jpg","pinToTop":true,"focusGlowEnabled":true,` +
	`"viewMode":"TABBED_GRID","showAllTab":true,"folders":[` +
	`{"id":"ffffffff-0000-4000-8000-000000000001","title":"Action","coverEmoji":"💥","focusGifEnabled":true,` +
	`"tileShape":"POSTER","hideTitle":false,"catalogSources":[` +
	`{"addonId":"hiidz.uno.catalog","type":"movie","catalogId":"tmdb-cacacaca-0000-4000-8000-000000000001"},` +
	`{"addonId":"hiidz.uno.catalog","type":"movie","catalogId":"tmdb-cacacaca-0000-4000-8000-000000000005","genre":"Action"}]},` +
	`{"id":"ffffffff-0000-4000-8000-000000000002","title":"Hidden Gems","focusGifEnabled":true,` +
	`"tileShape":"LANDSCAPE","hideTitle":false,"catalogSources":[` +
	`{"addonId":"hiidz.uno.catalog","type":"movie","catalogId":"tmdb-cacacaca-0000-4000-8000-000000000003","genre":"Drama"},` +
	`{"addonId":"hiidz.uno.catalog","type":"movie","catalogId":"tmdb-cacacaca-0000-4000-8000-000000000001","genre":"Comedy"}]}]}`

// Over the prod-shaped fixture, migration 5 hashes the two collections whose
// version equals their pushed_version, leaves the other four pending, drops
// both version columns and indexes folder refs by catalog. A backfilled hash
// is the sha256 of the exact push JSON.
func TestPushedHashesOverTheProdShapedFixture(t *testing.T) {
	d := openMigrated(t)
	d.SetMaxOpenConns(1)
	if _, err := runAccounts(t, d); err != nil {
		t.Fatalf("accounts: %v", err)
	}
	notes, err := runPushedHashes(t, d)
	if err != nil {
		t.Fatalf("pushed_hash: %v", err)
	}
	want := []string{"backfilled 2 push hashes; left 4 collections pending (changed since their last push, or never pushed)"}
	if !slices.Equal(notes, want) {
		t.Errorf("notes = %q, want %q", notes, want)
	}

	got := queryStrings(t, d, `SELECT id || ' ' || coalesce(pushed_hash, 'NULL') FROM collections ORDER BY id`)
	sum := sha256.Sum256([]byte(weekendPushJSON))
	if got[0] != collectionID("01")+" "+hex.EncodeToString(sum[:]) {
		t.Errorf("Weekend = %q, want the hash of %s", got[0], weekendPushJSON)
	}
	for i, pending := range []bool{false, true, true, false, true, true} {
		if strings.HasSuffix(got[i], " NULL") != pending {
			t.Errorf("collection %d = %q, want pending %t", i+1, got[i], pending)
		}
	}

	columns := queryStrings(t, d, `SELECT name FROM pragma_table_info('collections') WHERE name IN ('version', 'pushed_version', 'pushed_hash')`)
	if !slices.Equal(columns, []string{"pushed_hash"}) {
		t.Errorf("columns = %q, want only pushed_hash", columns)
	}
	index := queryStrings(t, d, `SELECT sql FROM sqlite_master WHERE name = 'folder_catalogs_by_catalog'`)
	if !slices.Equal(index, []string{"CREATE INDEX folder_catalogs_by_catalog ON folder_catalogs (catalog_id)"}) {
		t.Errorf("index = %q, want folder_catalogs_by_catalog", index)
	}
}

// A collection with no folders pushes "folders":[], and a folder with no
// catalogs "catalogSources":[]; & and < are escaped as json.Marshal
// escapes them.
func TestSentHashWritesEmptyListsAndEscapes(t *testing.T) {
	d := openTestDB(t, `
		CREATE TABLE collections (id TEXT, title TEXT, backdrop_image_url TEXT, pin_to_top INTEGER,
		                          focus_glow_enabled INTEGER, view_mode TEXT, show_all_tab INTEGER);
		CREATE TABLE folders (id TEXT, collection_id TEXT, title TEXT, sort_order INTEGER, cover_image_url TEXT,
		                      cover_emoji TEXT, focus_gif_url TEXT, focus_gif_enabled INTEGER, hero_backdrop_url TEXT,
		                      hero_video_url TEXT, title_logo_url TEXT, tile_shape TEXT, hide_title INTEGER);
		CREATE TABLE folder_catalogs (folder_id TEXT, catalog_id TEXT, sort_order INTEGER, genre TEXT);
		CREATE TABLE catalogs (id TEXT, recipe_hash TEXT);
		CREATE TABLE recipes (hash TEXT, type TEXT, provider TEXT);
		INSERT INTO collections VALUES ('bare', 'A & <B>', '', 0, 0, 'ROWS', 0), ('empty', 'E', '', 0, 1, 'ROWS', 0);
		INSERT INTO folders VALUES ('f', 'empty', 'F', 0, '', '', '', 0, '', '', '', 'POSTER', 0);
	`)
	tx, err := d.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	for id, raw := range map[string]string{
		"bare":  `{"id":"bare","title":"A \u0026 \u003cB\u003e","pinToTop":false,"focusGlowEnabled":false,"viewMode":"ROWS","showAllTab":false,"folders":[]}`,
		"empty": `{"id":"empty","title":"E","pinToTop":false,"focusGlowEnabled":true,"viewMode":"ROWS","showAllTab":false,"folders":[{"id":"f","title":"F","focusGifEnabled":false,"tileShape":"POSTER","hideTitle":false,"catalogSources":[]}]}`,
	} {
		sum := sha256.Sum256([]byte(raw))
		if got, err := sentHash(context.Background(), tx, id); err != nil || got != hex.EncodeToString(sum[:]) {
			t.Errorf("sentHash(%s) = %q, %v; want the hash of %s", id, got, err, raw)
		}
	}
}

// Migration 5 fails, naming its step, on a database it can't carry through:
// one whose collections already have pushed_hash, lack the version columns,
// or hold an in-step collection whose folders or refs can't be read or
// whose hash can't be stored, and one without folder_catalogs to index.
func TestPushedHashesFailsLoudly(t *testing.T) {
	const inStep = `CREATE TABLE collections (id TEXT, title TEXT, backdrop_image_url TEXT, pin_to_top INTEGER,
		focus_glow_enabled INTEGER, view_mode TEXT, show_all_tab INTEGER, version INTEGER, pushed_version INTEGER);
		INSERT INTO collections VALUES ('c1', 'T', '', 0, 1, 'ROWS', 0, 1, 1);`
	const folders = `CREATE TABLE folders (id TEXT, collection_id TEXT, title TEXT, sort_order INTEGER,
		cover_image_url TEXT, cover_emoji TEXT, focus_gif_url TEXT, focus_gif_enabled INTEGER, hero_backdrop_url TEXT,
		hero_video_url TEXT, title_logo_url TEXT, tile_shape TEXT, hide_title INTEGER);`
	for _, tc := range []struct{ name, setup, want string }{
		{"pushed_hash already there", `CREATE TABLE collections (id TEXT, pushed_hash TEXT)`, "adding collections.pushed_hash"},
		{"no version columns", `CREATE TABLE collections (id TEXT)`, "reading pushed collections"},
		{"no folders table", inStep, "reading collection c1's folders"},
		{"no folder_catalogs table", inStep + folders +
			`INSERT INTO folders VALUES ('f1', 'c1', 'F', 0, '', '', '', 1, '', '', '', 'POSTER', 0);`, "reading folder f1's catalogs"},
		{"a hash that can't be stored", inStep + folders +
			`CREATE TRIGGER no_updates BEFORE UPDATE ON collections BEGIN SELECT RAISE(ABORT, 'read only'); END;`,
			"setting collection c1's pushed_hash"},
		{"nothing to index", `CREATE TABLE collections (id TEXT, version INTEGER, pushed_version INTEGER)`,
			"dropping collections' version columns"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := openTestDB(t, tc.setup)
			if _, err := runPushedHashes(t, d); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want one naming %q", err, tc.want)
			}
		})
	}
}
