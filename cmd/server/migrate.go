// The v8→v9 migration, `uno migrate --db <path>`: run once against a vault at
// schema version 8, then deleted. Version 9 stores each catalog's type,
// provider and params on its own row, with no recipes table, requires a
// collection's view mode to be TABBED_GRID or ROWS and a folder's tile shape
// to be set, with no default. In one transaction on one connection with
// foreign keys off, it rewrites the view modes and tile shapes version 9
// refuses, rebuilds catalogs and folders as version 9 declares them, drops
// recipes, and checks the result: every row it keeps is the row it read,
// every catalog's recipe hashes as before, the foreign keys hold, and the
// structure matches a fresh v9 database. `--report` prints what it would
// rewrite and what stays over the folder and catalog caps, and changes
// nothing.

package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"

	"github.com/hiidz/uno/internal/vault"
)

// rebuildTables is the migration's DDL, run after the value rewrites:
// catalogs rebuilt with its recipe's columns from the recipes row it named,
// folders rebuilt without a tile shape default, each table, index and
// trigger written exactly as schema.sql writes it, and recipes dropped.
// publications_release_subscribers is dropped first and created again last:
// a rename re-reads every trigger, and this one names both rebuilt tables.
// Nothing here deletes a publication, so it never fires.
const rebuildTables = `
DROP TRIGGER publications_release_subscribers;
DROP TRIGGER recipes_drop_unused_on_delete;
DROP TRIGGER recipes_drop_unused_on_repoint;
CREATE TABLE catalogs_v9 (
    id              TEXT    PRIMARY KEY,         -- UUID, permanent once selected
    name            TEXT    NOT NULL,
    type            TEXT    NOT NULL,            -- Stremio's word: movie | series
    provider        TEXT    NOT NULL,            -- tmdb, for now
    params          TEXT    NOT NULL,            -- canonical JSON: known keys, no zero values, keys sorted
    owner_id        TEXT    NOT NULL REFERENCES profiles(id),
    collection_id   TEXT    REFERENCES collections(id) ON DELETE CASCADE, -- NULL = listed
    home_sort_order INTEGER,                     -- place on Home, numbered with collections.home_sort_order; NULL = not on Home
    show_in_home    INTEGER NOT NULL DEFAULT 1,  -- whether the home row appears when on the TV
    sub_key         TEXT,                        -- in a subscribed collection: its key in the snapshot
    created_at      TEXT    NOT NULL,            -- RFC3339 UTC
    updated_at      TEXT    NOT NULL,            -- RFC3339 UTC
    unpublished_at  TEXT,                        -- RFC3339 UTC: when the publication it was added from was unpublished; NULL once acknowledged
    CHECK (collection_id IS NULL OR home_sort_order IS NULL)
);
INSERT INTO catalogs_v9 (id, name, type, provider, params, owner_id, collection_id, home_sort_order, show_in_home,
                         sub_key, created_at, updated_at, unpublished_at)
SELECT c.id, c.name, r.type, r.provider, r.params, c.owner_id, c.collection_id, c.home_sort_order, c.show_in_home,
       c.sub_key, c.created_at, c.updated_at, c.unpublished_at
FROM catalogs c JOIN recipes r ON r.hash = c.recipe_hash;
DROP TABLE catalogs;
ALTER TABLE catalogs_v9 RENAME TO catalogs;
CREATE INDEX catalogs_by_owner ON catalogs (owner_id);
CREATE INDEX catalogs_by_collection ON catalogs (collection_id);
CREATE TABLE folders_v9 (
    id                TEXT    PRIMARY KEY,
    collection_id     TEXT    NOT NULL REFERENCES collections(id) ON DELETE CASCADE,
    title             TEXT    NOT NULL,
    sort_order        INTEGER NOT NULL,
    tile_shape        TEXT    NOT NULL,           -- POSTER | LANDSCAPE | SQUARE
    hide_title        INTEGER NOT NULL DEFAULT 0,
    cover_emoji       TEXT    NOT NULL DEFAULT '',
    cover_image_url   TEXT    NOT NULL DEFAULT '',
    focus_gif_url     TEXT    NOT NULL DEFAULT '',
    focus_gif_enabled INTEGER NOT NULL DEFAULT 1,
    hero_video_url    TEXT    NOT NULL DEFAULT '',
    hero_backdrop_url TEXT    NOT NULL DEFAULT '',
    title_logo_url    TEXT    NOT NULL DEFAULT '',
    sub_key           TEXT                        -- in a subscribed collection: its key in the snapshot
);
INSERT INTO folders_v9 (id, collection_id, title, sort_order, tile_shape, hide_title, cover_emoji, cover_image_url,
                        focus_gif_url, focus_gif_enabled, hero_video_url, hero_backdrop_url, title_logo_url, sub_key)
SELECT id, collection_id, title, sort_order, tile_shape, hide_title, cover_emoji, cover_image_url,
       focus_gif_url, focus_gif_enabled, hero_video_url, hero_backdrop_url, title_logo_url, sub_key
FROM folders;
DROP TABLE folders;
ALTER TABLE folders_v9 RENAME TO folders;
CREATE INDEX folders_by_collection ON folders (collection_id, sort_order);
DROP TABLE recipes;
CREATE TRIGGER publications_release_subscribers BEFORE DELETE ON publications
BEGIN
    UPDATE catalogs SET unpublished_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
    WHERE id IN (SELECT catalog_id FROM subscriptions WHERE publication_id = OLD.id);
    UPDATE collections SET unpublished_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
    WHERE id IN (SELECT collection_id FROM subscriptions WHERE publication_id = OLD.id);
    UPDATE catalogs SET sub_key = NULL
    WHERE collection_id IN (SELECT collection_id FROM subscriptions WHERE publication_id = OLD.id);
    UPDATE folders SET sub_key = NULL
    WHERE collection_id IN (SELECT collection_id FROM subscriptions WHERE publication_id = OLD.id);
END;
PRAGMA user_version = 9;
`

// rewriteValues gives every collection version 9 would refuse for its view
// mode TABBED_GRID, what Nuvio shows for FOLLOW_LAYOUT or none, and every
// folder with no tile shape POSTER, what Nuvio shows for none.
const rewriteValues = `
UPDATE collections SET view_mode = 'TABBED_GRID' WHERE view_mode IN ('FOLLOW_LAYOUT', '');
UPDATE folders SET tile_shape = 'POSTER' WHERE tile_shape = '';
`

// preserved are queries that read the same rows in a v8 and a v9 database,
// each row as one line: everything the migration keeps, with a view mode or
// tile shape it rewrites read as what it becomes. Their answers before and
// after must match, so the rewrites change nothing else. The released marks,
// the snapshot keys, the subscriptions and the publications with their
// subscriber counts are all here.
var preserved = []string{
	`SELECT id || '|' || name || '|' || owner_id || '|' || coalesce(collection_id, '-') || '|' ||
	        coalesce(home_sort_order, '-') || '|' || show_in_home || '|' || coalesce(sub_key, '-') || '|' ||
	        created_at || '|' || updated_at || '|' || coalesce(unpublished_at, '-') FROM catalogs ORDER BY id`,
	`SELECT id || '|' || title || '|' || owner_id || '|' || pin_to_top || '|' ||
	        CASE WHEN view_mode IN ('FOLLOW_LAYOUT', '') THEN 'TABBED_GRID' ELSE view_mode END || '|' || show_all_tab || '|' ||
	        backdrop_image_url || '|' || focus_glow_enabled || '|' || coalesce(home_sort_order, '-') || '|' ||
	        created_at || '|' || updated_at || '|' || coalesce(unpublished_at, '-') FROM collections ORDER BY id`,
	`SELECT id || '|' || collection_id || '|' || title || '|' || sort_order || '|' ||
	        CASE WHEN tile_shape = '' THEN 'POSTER' ELSE tile_shape END || '|' || hide_title || '|' || cover_emoji || '|' ||
	        cover_image_url || '|' || focus_gif_url || '|' || focus_gif_enabled || '|' || hero_video_url || '|' ||
	        hero_backdrop_url || '|' || title_logo_url || '|' || coalesce(sub_key, '-') FROM folders ORDER BY id`,
	`SELECT folder_id || '|' || catalog_id || '|' || sort_order || '|' || genre FROM folder_catalogs ORDER BY 1`,
	`SELECT id || '|' || publisher_id || '|' || kind || '|' || coalesce(catalog_id, '-') || '|' ||
	        coalesce(collection_id, '-') || '|' || title || '|' || content_hash || '|' || catalog_count || '|' ||
	        folder_count || '|' || subscriber_count || '|' || published_at || '|' || updated_at || '|' || snapshot
	 FROM publications ORDER BY id`,
	`SELECT id || '|' || subscriber_id || '|' || publication_id || '|' || coalesce(catalog_id, '-') || '|' ||
	        coalesce(collection_id, '-') || '|' || subscribed_hash || '|' || created_at FROM subscriptions ORDER BY id`,
	`SELECT profile_id || '|' || nuvio_profile_uuid || '|' || pushed_at || '|' || record FROM push_records ORDER BY 1`,
	`SELECT id || '|' || token || '|' || nuvio_user_id || '|' || nuvio_profile_index || '|' || nuvio_profile_uuid
	 FROM profiles ORDER BY id`,
	`SELECT nuvio_user_id || '|' || hex(tmdb_key_ciphertext) || '|' || tmdb_key_last4 || '|' || updated_at
	 FROM accounts ORDER BY 1`,
}

// findings are the report's queries: the rows the migration rewrites, the
// rows it refuses to migrate, and the collections and publications version
// 9's caps (10 folders, 20 catalogs a folder) would refuse to save or copy,
// which it leaves as they are. Each row is one line.
var findings = []struct {
	label, query string
	refuses      bool
}{
	{"collections whose view mode becomes TABBED_GRID", `
		SELECT id || '  ' || quote(view_mode) || '  ' || title FROM collections
		WHERE view_mode IN ('FOLLOW_LAYOUT', '') ORDER BY title, id`, false},
	{"folders whose empty tile shape becomes POSTER", `
		SELECT f.id || '  ' || c.title || ' / ' || f.title FROM folders f JOIN collections c ON c.id = f.collection_id
		WHERE f.tile_shape = '' ORDER BY c.title, f.sort_order`, false},
	{"collections with a view mode no version allows", `
		SELECT id || '  ' || quote(view_mode) || '  ' || title FROM collections
		WHERE view_mode NOT IN ('TABBED_GRID', 'ROWS', 'FOLLOW_LAYOUT', '') ORDER BY title, id`, true},
	{"folders with a tile shape no version allows", `
		SELECT id || '  ' || quote(tile_shape) || '  ' || title FROM folders
		WHERE tile_shape NOT IN ('POSTER', 'LANDSCAPE', 'SQUARE', '') ORDER BY title, id`, true},
	{"publications whose snapshot holds a view mode or tile shape version 9 refuses", `
		SELECT p.id || '  ' || p.title FROM publications p
		WHERE p.kind = 'collection' AND (
		    coalesce(json_extract(p.snapshot, '$.collection.view_mode'), '') NOT IN ('TABBED_GRID', 'ROWS')
		    OR EXISTS (SELECT 1 FROM json_each(p.snapshot, '$.collection.folders') f
		               WHERE coalesce(json_extract(f.value, '$.tile_shape'), '') NOT IN ('POSTER', 'LANDSCAPE', 'SQUARE')))
		ORDER BY p.title, p.id`, true},
	{"collections over the caps (stay readable and pushed; a save is refused until trimmed)", `
		SELECT c.id || '  ' || c.title || '  folders=' || (SELECT count(*) FROM folders f WHERE f.collection_id = c.id) ||
		       ' most catalogs in a folder=' || coalesce((SELECT max(n) FROM (SELECT count(*) n FROM folder_catalogs fc
		           JOIN folders f ON f.id = fc.folder_id WHERE f.collection_id = c.id GROUP BY fc.folder_id)), 0)
		FROM collections c
		WHERE (SELECT count(*) FROM folders f WHERE f.collection_id = c.id) > 10
		   OR EXISTS (SELECT 1 FROM folder_catalogs fc JOIN folders f ON f.id = fc.folder_id
		              WHERE f.collection_id = c.id GROUP BY fc.folder_id HAVING count(*) > 20)
		ORDER BY c.title, c.id`, false},
	{"publications over the caps (can't be added, duplicated or updated until republished trimmed)", `
		SELECT p.id || '  ' || p.title || '  folders=' || json_array_length(p.snapshot, '$.collection.folders') FROM publications p
		WHERE p.kind = 'collection' AND (
		    json_array_length(p.snapshot, '$.collection.folders') > 10
		    OR EXISTS (SELECT 1 FROM json_each(p.snapshot, '$.collection.folders') f
		               WHERE json_array_length(f.value, '$.refs') > 20))
		ORDER BY p.title, p.id`, false},
}

// runMigrate is the migrate subcommand: it parses args and migrates the
// database --db names, or with --report only reports on it, writing to out.
func runMigrate(ctx context.Context, args []string, out io.Writer) error {
	flags := flag.NewFlagSet("migrate", flag.ContinueOnError)
	flags.SetOutput(out)
	path := flags.String("db", "", "path to the vault.db to migrate from schema version 8 to 9")
	reportOnly := flags.Bool("report", false, "print what the migration would rewrite and leave over the caps, and change nothing")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *path == "" {
		return errors.New("migrate: --db is required")
	}
	if _, err := os.Stat(*path); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	return migrateToV9(ctx, *path, *reportOnly, out)
}

// migrateToV9 reports on the v8 database at path and, unless reportOnly,
// migrates it to v9 in one transaction on one connection with foreign keys
// off, printing row counts before and after.
func migrateToV9(ctx context.Context, path string, reportOnly bool, out io.Writer) error {
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)&_pragma=foreign_keys(0)")
	if err != nil {
		return fmt.Errorf("opening %s: %w", path, err)
	}
	defer func() { _ = db.Close() }()
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("opening %s: %w", path, err)
	}
	defer func() { _ = conn.Close() }()
	return migrateConn(ctx, conn, reportOnly, out)
}

// migrateConn reports on the database conn holds, which must be at version
// 8, and, unless reportOnly or the report finds rows it refuses, migrates it.
func migrateConn(ctx context.Context, conn *sql.Conn, reportOnly bool, out io.Writer) error {
	if err := requireVersion(ctx, conn, 8); err != nil {
		return err
	}
	refused, err := report(ctx, conn, out)
	if err != nil || reportOnly {
		return err
	}
	if refused > 0 {
		return fmt.Errorf("refusing to migrate: %d rows version 9 can't hold (listed above); nothing was changed", refused)
	}
	before, err := rowCounts(ctx, conn)
	if err != nil {
		return err
	}
	if err := migrateInTx(ctx, conn); err != nil {
		return err
	}
	after, err := rowCounts(ctx, conn)
	if err != nil {
		return err
	}
	printCounts(out, before, after)
	_, err = fmt.Fprintln(out, "migrated to schema version 9: every kept row reads as before, every catalog's recipe hashes as before, and the structure matches a fresh v9 database")
	return err
}

// report writes each finding's rows to out and returns how many rows the
// findings that refuse the migration hold.
func report(ctx context.Context, q querier, out io.Writer) (int, error) {
	refused := 0
	for _, f := range findings {
		rows, err := queryStrings(ctx, q, f.query)
		if err != nil {
			return 0, fmt.Errorf("reporting %s: %w", f.label, err)
		}
		_, _ = fmt.Fprintf(out, "%s: %d\n", f.label, len(rows))
		for _, row := range rows {
			_, _ = fmt.Fprintf(out, "  %s\n", row)
		}
		if f.refuses {
			refused += len(rows)
		}
	}
	return refused, nil
}

// requireVersion refuses a database whose user_version isn't want.
func requireVersion(ctx context.Context, q queryRower, want int) error {
	var version int
	if err := q.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&version); err != nil {
		return fmt.Errorf("reading the schema version: %w", err)
	}
	if version != want {
		return fmt.Errorf("the database is at schema version %d; migrate only runs on version %d", version, want)
	}
	return nil
}

// migrateInTx runs the whole migration in one transaction on conn and
// commits only when every step and check passes.
func migrateInTx(ctx context.Context, conn *sql.Conn) error {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("starting transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // no-op once Commit succeeds

	if err := migrateTx(ctx, tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing: %w", err)
	}
	return nil
}

// migrateTx rewrites the refused values, rebuilds catalogs and folders,
// drops recipes and stamps version 9 through tx, then checks the kept rows,
// the recipe hashes, the foreign keys and the structure.
func migrateTx(ctx context.Context, tx *sql.Tx) error {
	kept, err := answers(ctx, tx)
	if err != nil {
		return err
	}
	hashes, err := queryStrings(ctx, tx, `SELECT id || ' ' || recipe_hash FROM catalogs ORDER BY id`)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, rewriteValues); err != nil {
		return fmt.Errorf("rewriting view modes and tile shapes: %w", err)
	}
	if _, err := tx.ExecContext(ctx, rebuildTables); err != nil {
		return fmt.Errorf("rebuilding catalogs and folders: %w", err)
	}
	if err := requireKept(ctx, tx, kept); err != nil {
		return err
	}
	if err := requireRecipeHashes(ctx, tx, hashes); err != nil {
		return err
	}
	if err := requireForeignKeys(ctx, tx); err != nil {
		return err
	}
	return requireFreshStructure(ctx, tx)
}

// answers is every preserved query's answer through q, in order.
func answers(ctx context.Context, q querier) ([][]string, error) {
	var all [][]string
	for _, query := range preserved {
		rows, err := queryStrings(ctx, q, query)
		if err != nil {
			return nil, err
		}
		all = append(all, rows)
	}
	return all, nil
}

// requireKept refuses unless every preserved query answers what it answered
// before the migration.
func requireKept(ctx context.Context, q querier, before [][]string) error {
	after, err := answers(ctx, q)
	if err != nil {
		return err
	}
	for i := range before {
		if extra, missing := diffLines(after[i], before[i]); len(extra)+len(missing) > 0 {
			return fmt.Errorf("refusing to migrate: rows changed: has %v, lacks %v", extra, missing)
		}
	}
	return nil
}

// requireRecipeHashes refuses unless every catalog's type, provider and
// params hash to the recipe_hash it held before, listed in hashes as
// "id hash" lines.
func requireRecipeHashes(ctx context.Context, q querier, hashes []string) error {
	rows, err := q.QueryContext(ctx, `SELECT id, type, provider, params FROM catalogs ORDER BY id`)
	if err != nil {
		return fmt.Errorf("reading catalogs: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var got []string
	for rows.Next() {
		var id, catalogType, catalogProvider, params string
		if err := rows.Scan(&id, &catalogType, &catalogProvider, &params); err != nil {
			return fmt.Errorf("scanning catalog: %w", err)
		}
		got = append(got, id+" "+vault.RecipeHash(catalogType, catalogProvider, params))
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if extra, missing := diffLines(got, hashes); len(extra)+len(missing) > 0 {
		return fmt.Errorf("refusing to migrate: recipes changed: has %v, lacks %v", extra, missing)
	}
	return nil
}

// requireForeignKeys refuses while any row's foreign key names a row that
// isn't there.
func requireForeignKeys(ctx context.Context, tx *sql.Tx) error {
	broken, err := queryStrings(ctx, tx, `SELECT "table" || ' rowid ' || coalesce(rowid, '-') || ' -> ' || parent FROM pragma_foreign_key_check`)
	if err != nil {
		return err
	}
	if len(broken) > 0 {
		return fmt.Errorf("refusing to migrate: foreign keys broken: %v", broken)
	}
	return nil
}

// queryRower is what *sql.Conn, *sql.Tx and *sql.DB share for a one-row read.
type queryRower interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// querier is what *sql.Conn, *sql.Tx and *sql.DB share for a many-row read.
type querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// requireFreshStructure refuses unless the structure tx sees matches a fresh
// v9 database's, made by vault.InitDB in a temporary directory, and the
// tables and trigger the migration wrote read exactly as that database's.
func requireFreshStructure(ctx context.Context, tx *sql.Tx) error {
	want, err := freshStructure(ctx)
	if err != nil {
		return err
	}
	got, err := structureOf(ctx, tx)
	if err != nil {
		return err
	}
	if extra, missing := diffLines(got, want); len(extra)+len(missing) > 0 {
		return fmt.Errorf("refusing to migrate: the structure differs from a fresh v9 database: has %v, lacks %v", extra, missing)
	}
	return nil
}

// freshStructure is the structure of a fresh v9 database.
func freshStructure(ctx context.Context) ([]string, error) {
	dir, err := os.MkdirTemp("", "uno-migrate-")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	path := filepath.Join(dir, "fresh.db")
	fresh, err := vault.InitDB(path)
	if err != nil {
		return nil, err
	}
	if err := fresh.Close(); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = db.Close() }()
	return structureOf(ctx, db)
}

// structureOf is a database's structure as sorted lines, from PRAGMAs rather
// than sqlite_master's text, which differs in whitespace and quoting: every
// table's columns (table_xinfo), indexes with their columns (index_list,
// index_xinfo) and foreign keys (foreign_key_list), and every trigger's name
// and table; and the stored SQL of each table and trigger the migration
// writes (rebuiltSQL).
func structureOf(ctx context.Context, q querier) ([]string, error) {
	tables, err := queryStrings(ctx, q, `SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'`)
	if err != nil {
		return nil, err
	}
	lines, err := queryStrings(ctx, q, `SELECT 'trigger ' || name || ' on ' || tbl_name FROM sqlite_master WHERE type = 'trigger'`)
	if err != nil {
		return nil, err
	}
	for _, table := range tables {
		tableLines, err := tableStructure(ctx, q, table)
		if err != nil {
			return nil, err
		}
		lines = append(lines, tableLines...)
	}
	rebuilt, err := rebuiltSQL(ctx, q)
	if err != nil {
		return nil, err
	}
	lines = append(lines, rebuilt...)
	slices.Sort(lines)
	return lines, nil
}

// rebuiltSQL is the stored SQL of each table and trigger the migration
// writes, as lines, with the quotes a RENAME puts around a table's name in
// its CREATE TABLE taken off.
func rebuiltSQL(ctx context.Context, q querier) ([]string, error) {
	return queryStrings(ctx, q, `
		SELECT name || ' sql ' || replace(sql, 'CREATE TABLE "' || name || '"', 'CREATE TABLE ' || name)
		FROM sqlite_master
		WHERE name IN ('catalogs', 'folders', 'publications_release_subscribers')`)
}

// tableStructure is one table's columns, indexes and foreign keys as lines.
func tableStructure(ctx context.Context, q querier, table string) ([]string, error) {
	var lines []string
	for _, query := range []string{
		`SELECT ?1 || ' column ' || cid || ' ' || name || ' ' || type || ' notnull=' || "notnull" || ' default=' ||
		        coalesce(dflt_value, '-') || ' pk=' || pk || ' hidden=' || hidden FROM pragma_table_xinfo(?1)`,
		`SELECT ?1 || ' index ' || CASE WHEN il.name LIKE 'sqlite_autoindex_%' THEN 'auto' ELSE il.name END ||
		        ' unique=' || il."unique" || ' origin=' || il.origin || ' partial=' || il.partial || ' columns=' ||
		        (SELECT group_concat(coalesce(x.name, '-') || ':' || x."desc" || ':' || x.coll || ':' || x.key, ',')
		         FROM (SELECT * FROM pragma_index_xinfo(il.name) ORDER BY seqno) x)
		 FROM pragma_index_list(?1) il`,
		`SELECT ?1 || ' foreign key ' || id || '.' || seq || ' ' || "from" || ' -> ' || "table" || '(' || coalesce("to", '-') ||
		        ') update=' || on_update || ' delete=' || on_delete FROM pragma_foreign_key_list(?1)`,
	} {
		got, err := queryStrings(ctx, q, query, table)
		if err != nil {
			return nil, err
		}
		lines = append(lines, got...)
	}
	return lines, nil
}

// diffLines is the lines only got has and the lines only want has.
func diffLines(got, want []string) (extra, missing []string) {
	for _, line := range got {
		if !slices.Contains(want, line) {
			extra = append(extra, line)
		}
	}
	for _, line := range want {
		if !slices.Contains(got, line) {
			missing = append(missing, line)
		}
	}
	return extra, missing
}

// rowCounts is every table's row count, by name.
func rowCounts(ctx context.Context, q interface {
	querier
	queryRower
}) (map[string]int, error) {
	tables, err := queryStrings(ctx, q, `SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'`)
	if err != nil {
		return nil, err
	}
	counts := map[string]int{}
	for _, table := range tables {
		var n int
		if err := q.QueryRowContext(ctx, `SELECT count(*) FROM "`+table+`"`).Scan(&n); err != nil {
			return nil, fmt.Errorf("counting %s: %w", table, err)
		}
		counts[table] = n
	}
	return counts, nil
}

// printCounts writes each table's row count before and after to out, by
// table name.
func printCounts(out io.Writer, before, after map[string]int) {
	var tables []string
	for table := range before {
		tables = append(tables, table)
	}
	for table := range after {
		if _, ok := before[table]; !ok {
			tables = append(tables, table)
		}
	}
	slices.Sort(tables)
	_, _ = fmt.Fprintf(out, "%-16s %8s %8s\n", "table", "before", "after")
	for _, table := range tables {
		_, _ = fmt.Fprintf(out, "%-16s %8s %8s\n", table, countOf(before, table), countOf(after, table))
	}
}

// countOf is counts[table] as text, "-" when the table isn't there.
func countOf(counts map[string]int, table string) string {
	if n, ok := counts[table]; ok {
		return fmt.Sprint(n)
	}
	return "-"
}

// queryStrings runs a query whose rows are one text column, in row order.
func queryStrings(ctx context.Context, q querier, query string, args ...any) ([]string, error) {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var values []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, fmt.Errorf("scanning: %w", err)
		}
		values = append(values, s)
	}
	return values, rows.Err()
}
