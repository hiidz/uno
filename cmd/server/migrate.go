// The v7→v8 migration, `uno migrate --db <path>`: run once against a vault at
// schema version 7, then deleted. Version 8 makes unpublishing one-way: a
// publication is live exactly while its row exists, and ending it releases
// its subscribers, whose copies become their own. In one transaction it
// releases the subscribers of every publication version 7 kept as
// unpublished and deletes those publications, adds the unpublished_at
// columns, rebuilds publications and subscriptions with their version 8
// foreign keys and triggers, and checks the result against a fresh v8
// database.

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

// releaseUnpublished releases the subscribers of every publication version 7
// kept as unpublished, as version 8's publications_release_subscribers does
// when a publication ends, then deletes those subscriptions and
// publications.
const releaseUnpublished = `
ALTER TABLE catalogs ADD COLUMN unpublished_at TEXT;
ALTER TABLE collections ADD COLUMN unpublished_at TEXT;
CREATE TEMP TABLE released AS
SELECT catalog_id, collection_id FROM subscriptions
WHERE publication_id IN (SELECT id FROM publications WHERE status = 'unpublished');
UPDATE catalogs SET unpublished_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
WHERE id IN (SELECT catalog_id FROM released);
UPDATE collections SET unpublished_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
WHERE id IN (SELECT collection_id FROM released);
UPDATE catalogs SET sub_key = NULL WHERE collection_id IN (SELECT collection_id FROM released);
UPDATE folders SET sub_key = NULL WHERE collection_id IN (SELECT collection_id FROM released);
DROP TABLE released;
DELETE FROM subscriptions WHERE publication_id IN (SELECT id FROM publications WHERE status = 'unpublished');
DELETE FROM publications WHERE status = 'unpublished';
`

// rebuildSharing rebuilds publications and subscriptions as version 8
// declares them, each table, index and trigger written exactly as schema.sql
// writes it, every publication's subscriber_count counted afresh, and stamps
// version 8. Foreign keys are off on the connection while it runs.
const rebuildSharing = `
DROP TRIGGER publications_unpublish_on_source_delete;
DROP TRIGGER subscriptions_count_on_insert;
DROP TRIGGER subscriptions_count_on_delete;
CREATE TABLE publications_v8 (
    id               TEXT    PRIMARY KEY,         -- UUID, kept when an update is published
    publisher_id     TEXT    NOT NULL REFERENCES profiles(id),
    kind             TEXT    NOT NULL CHECK (kind IN ('catalog', 'collection')),
    catalog_id       TEXT    REFERENCES catalogs(id) ON DELETE CASCADE,    -- the source, for a catalog
    collection_id    TEXT    REFERENCES collections(id) ON DELETE CASCADE, -- the source, for a collection
    title            TEXT    NOT NULL,
    snapshot         TEXT    NOT NULL,            -- JSON, format uno-publication
    content_hash     TEXT    NOT NULL,            -- sha256 hex of snapshot
    catalog_count    INTEGER NOT NULL,
    folder_count     INTEGER NOT NULL,
    subscriber_count INTEGER NOT NULL DEFAULT 0,
    published_at     TEXT    NOT NULL,            -- RFC3339 UTC, when first published
    updated_at       TEXT    NOT NULL,            -- RFC3339 UTC
    CHECK ((catalog_id IS NULL) <> (collection_id IS NULL)),
    CHECK (kind = 'catalog' OR catalog_id IS NULL),
    CHECK (kind = 'collection' OR collection_id IS NULL)
);
INSERT INTO publications_v8 (id, publisher_id, kind, catalog_id, collection_id, title, snapshot, content_hash,
                             catalog_count, folder_count, subscriber_count, published_at, updated_at)
SELECT id, publisher_id, kind, catalog_id, collection_id, title, snapshot, content_hash, catalog_count, folder_count,
       (SELECT count(*) FROM subscriptions s WHERE s.publication_id = p.id), published_at, updated_at
FROM publications p;
CREATE TABLE subscriptions_v8 (
    id             TEXT PRIMARY KEY,
    subscriber_id  TEXT NOT NULL REFERENCES profiles(id),
    publication_id TEXT NOT NULL REFERENCES publications(id) ON DELETE CASCADE,
    catalog_id     TEXT REFERENCES catalogs(id) ON DELETE CASCADE,    -- the copy, for a catalog
    collection_id  TEXT REFERENCES collections(id) ON DELETE CASCADE, -- the copy, for a collection
    subscribed_hash TEXT NOT NULL,               -- the content hash the copy was last written from
    created_at     TEXT NOT NULL,                -- RFC3339 UTC
    UNIQUE (subscriber_id, publication_id),
    CHECK ((catalog_id IS NULL) <> (collection_id IS NULL))
);
INSERT INTO subscriptions_v8 (id, subscriber_id, publication_id, catalog_id, collection_id, subscribed_hash, created_at)
SELECT id, subscriber_id, publication_id, catalog_id, collection_id, subscribed_hash, created_at FROM subscriptions;
DROP TABLE subscriptions;
DROP TABLE publications;
ALTER TABLE publications_v8 RENAME TO publications;
ALTER TABLE subscriptions_v8 RENAME TO subscriptions;
CREATE UNIQUE INDEX publications_by_catalog ON publications (catalog_id) WHERE catalog_id IS NOT NULL;
CREATE UNIQUE INDEX publications_by_collection ON publications (collection_id) WHERE collection_id IS NOT NULL;
CREATE UNIQUE INDEX subscriptions_by_catalog ON subscriptions (catalog_id) WHERE catalog_id IS NOT NULL;
CREATE UNIQUE INDEX subscriptions_by_collection ON subscriptions (collection_id) WHERE collection_id IS NOT NULL;
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
CREATE TRIGGER subscriptions_count_on_insert AFTER INSERT ON subscriptions
BEGIN
    UPDATE publications SET subscriber_count = subscriber_count + 1 WHERE id = NEW.publication_id;
END;
CREATE TRIGGER subscriptions_count_on_delete AFTER DELETE ON subscriptions
BEGIN
    UPDATE publications SET subscriber_count = subscriber_count - 1 WHERE id = OLD.publication_id;
END;
PRAGMA user_version = 8;
`

// runMigrate is the migrate subcommand: it parses args and migrates the
// database --db names, reporting to out.
func runMigrate(ctx context.Context, args []string, out io.Writer) error {
	flags := flag.NewFlagSet("migrate", flag.ContinueOnError)
	flags.SetOutput(out)
	path := flags.String("db", "", "path to the vault.db to migrate from schema version 7 to 8")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *path == "" {
		return errors.New("migrate: --db is required")
	}
	if _, err := os.Stat(*path); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	return migrateToV8(ctx, *path, out)
}

// migrateToV8 migrates the v7 database at path to v8 in one transaction on
// one connection with foreign keys off, printing row counts before and after.
func migrateToV8(ctx context.Context, path string, out io.Writer) error {
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
	return migrateConn(ctx, conn, out)
}

// migrateConn migrates the database conn holds, which must be at version 7,
// printing row counts before and after to out.
func migrateConn(ctx context.Context, conn *sql.Conn, out io.Writer) error {
	if err := requireVersion(ctx, conn, 7); err != nil {
		return err
	}
	before, err := rowCounts(ctx, conn)
	if err != nil {
		return err
	}
	ended, err := countEnded(ctx, conn)
	if err != nil {
		return err
	}
	if err := migrateInTx(ctx, conn); err != nil {
		return err
	}
	return reportMigration(ctx, conn, out, before, ended)
}

// endedCounts is how many publications version 7 kept as unpublished, and
// how many subscribed copies they still held.
type endedCounts struct{ publications, copies int }

// countEnded counts the unpublished publications and their subscriptions.
func countEnded(ctx context.Context, q queryRower) (endedCounts, error) {
	var ended endedCounts
	err := q.QueryRowContext(ctx, `
		SELECT (SELECT count(*) FROM publications WHERE status = 'unpublished'),
		       (SELECT count(*) FROM subscriptions WHERE publication_id IN
		            (SELECT id FROM publications WHERE status = 'unpublished'))
	`).Scan(&ended.publications, &ended.copies)
	if err != nil {
		return endedCounts{}, fmt.Errorf("counting unpublished publications: %w", err)
	}
	return ended, nil
}

// reportMigration writes each table's row count before and after, then the
// outcome, to out.
func reportMigration(ctx context.Context, conn *sql.Conn, out io.Writer, before map[string]int, ended endedCounts) error {
	after, err := rowCounts(ctx, conn)
	if err != nil {
		return err
	}
	printCounts(out, before, after)
	_, err = fmt.Fprintf(out, "migrated to schema version 8: %d unpublished publications deleted, their %d subscribed copies released as their subscribers' own; structure matches a fresh v8 database\n",
		ended.publications, ended.copies)
	return err
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

// migrateTx releases the unpublished publications' subscribers, rebuilds the
// sharing tables, stamps version 8, and checks the foreign keys and the
// structure against a fresh v8 database, through tx.
func migrateTx(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, releaseUnpublished); err != nil {
		return fmt.Errorf("releasing unpublished publications: %w", err)
	}
	if _, err := tx.ExecContext(ctx, rebuildSharing); err != nil {
		return fmt.Errorf("rebuilding publications and subscriptions: %w", err)
	}
	if err := requireForeignKeys(ctx, tx); err != nil {
		return err
	}
	return requireFreshStructure(ctx, tx)
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
// v8 database's, made by vault.InitDB in a temporary directory, and the
// tables and triggers the migration wrote read exactly as that database's.
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
		return fmt.Errorf("refusing to migrate: the structure differs from a fresh v8 database: has %v, lacks %v", extra, missing)
	}
	return nil
}

// freshStructure is the structure of a fresh v8 database.
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
		WHERE name IN ('publications', 'subscriptions', 'publications_release_subscribers',
		               'subscriptions_count_on_insert', 'subscriptions_count_on_delete')`)
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
