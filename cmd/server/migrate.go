// The v5→v6 migration, `uno migrate --db <path>`: run once against a vault at
// schema version 5, then deleted. In one transaction it renames the sharing
// columns and status, replaces the sharing triggers, refuses while any
// collection on Home needs a push, backfills each profile's push record, drops
// collections.pushed_hash, and checks the result against a fresh v6 database.

package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/google/uuid"

	"github.com/hiidz/uno/internal/vault"
)

// renameToV6 is the migration's structural half, run first: the four
// triggers that touch publications dropped, publications rebuilt (SQLite
// can't alter a CHECK) with publisher_id and 'unpublished', the subscription
// columns renamed, three triggers recreated, and push_records created.
const renameToV6 = `
DROP TRIGGER publications_withdraw_on_source_delete;
DROP TRIGGER publications_withdraw_on_scope;
DROP TRIGGER subscriptions_count_on_insert;
DROP TRIGGER subscriptions_count_on_delete;

CREATE TABLE publications_v6 (
    id               TEXT    PRIMARY KEY,         -- UUID, kept across republishes
    publisher_id     TEXT    NOT NULL REFERENCES profiles(id),
    kind             TEXT    NOT NULL CHECK (kind IN ('catalog', 'collection')),
    catalog_id       TEXT    REFERENCES catalogs(id) ON DELETE SET NULL,    -- the source; NULL once deleted
    collection_id    TEXT    REFERENCES collections(id) ON DELETE SET NULL, -- the source; NULL once deleted
    title            TEXT    NOT NULL,
    snapshot         TEXT    NOT NULL,            -- JSON, format uno-publication
    content_hash     TEXT    NOT NULL,            -- sha256 hex of snapshot
    catalog_count    INTEGER NOT NULL,
    folder_count     INTEGER NOT NULL,
    subscriber_count INTEGER NOT NULL DEFAULT 0,
    status           TEXT    NOT NULL CHECK (status IN ('live', 'unpublished')),
    published_at     TEXT    NOT NULL,            -- RFC3339 UTC, when first published
    updated_at       TEXT    NOT NULL,            -- RFC3339 UTC
    CHECK (kind = 'catalog' OR catalog_id IS NULL),
    CHECK (kind = 'collection' OR collection_id IS NULL)
);
INSERT INTO publications_v6 (id, publisher_id, kind, catalog_id, collection_id, title, snapshot, content_hash,
                             catalog_count, folder_count, subscriber_count, status, published_at, updated_at)
SELECT id, owner_id, kind, catalog_id, collection_id, title, snapshot, content_hash,
       catalog_count, folder_count, subscriber_count,
       CASE status WHEN 'withdrawn' THEN 'unpublished' ELSE status END, published_at, updated_at
FROM publications;
DROP TABLE publications;
ALTER TABLE publications_v6 RENAME TO publications;
CREATE UNIQUE INDEX publications_by_catalog ON publications (catalog_id) WHERE catalog_id IS NOT NULL;
CREATE UNIQUE INDEX publications_by_collection ON publications (collection_id) WHERE collection_id IS NOT NULL;

ALTER TABLE subscriptions RENAME COLUMN owner_id TO subscriber_id;
ALTER TABLE subscriptions RENAME COLUMN taken_hash TO subscribed_hash;

CREATE TRIGGER publications_unpublish_on_source_delete AFTER UPDATE OF catalog_id, collection_id ON publications
WHEN NEW.catalog_id IS NULL AND NEW.collection_id IS NULL AND NEW.status = 'live'
BEGIN
    UPDATE publications SET status = 'unpublished', updated_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
    WHERE id = NEW.id;
END;

CREATE TRIGGER subscriptions_count_on_insert AFTER INSERT ON subscriptions
BEGIN
    UPDATE publications SET subscriber_count = subscriber_count + 1 WHERE id = NEW.publication_id;
END;

CREATE TRIGGER subscriptions_count_on_delete AFTER DELETE ON subscriptions
BEGIN
    UPDATE publications SET subscriber_count = subscriber_count - 1 WHERE id = OLD.publication_id;
END;

CREATE TABLE push_records (
    profile_id         TEXT PRIMARY KEY REFERENCES profiles(id),
    nuvio_profile_uuid TEXT NOT NULL, -- the Nuvio profile it was pushed to: profiles.nuvio_profile_uuid then
    record             TEXT NOT NULL, -- JSON, vault.PushRecord
    pushed_at          TEXT NOT NULL  -- RFC3339 UTC
);
`

// runMigrate is the migrate subcommand: it parses args and migrates the
// database --db names, reporting to out.
func runMigrate(ctx context.Context, args []string, out io.Writer) error {
	flags := flag.NewFlagSet("migrate", flag.ContinueOnError)
	flags.SetOutput(out)
	path := flags.String("db", "", "path to the vault.db to migrate from schema version 5 to 6")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *path == "" {
		return errors.New("migrate: --db is required")
	}
	if _, err := os.Stat(*path); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	return migrateToV6(ctx, *path, out)
}

// migrateToV6 migrates the v5 database at path to v6 in one transaction on
// one connection with foreign keys off, printing row counts before and after.
func migrateToV6(ctx context.Context, path string, out io.Writer) error {
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)")
	if err != nil {
		return fmt.Errorf("opening %s: %w", path, err)
	}
	defer func() { _ = db.Close() }()
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("opening %s: %w", path, err)
	}
	defer func() { _ = conn.Close() }()

	if err := requireVersion(ctx, conn, 5); err != nil {
		return err
	}
	// Outside the transaction: inside one, SQLite ignores it.
	if _, err := conn.ExecContext(ctx, `PRAGMA foreign_keys = OFF`); err != nil {
		return fmt.Errorf("turning foreign keys off: %w", err)
	}
	before, err := rowCounts(ctx, conn)
	if err != nil {
		return err
	}
	records, err := migrateInTx(ctx, conn)
	if err != nil {
		return err
	}
	after, err := rowCounts(ctx, conn)
	if err != nil {
		return err
	}
	printCounts(out, before, after)
	_, err = fmt.Fprintf(out, "migrated to schema version 6: %d push records backfilled; structure matches a fresh v6 database\n", records)
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
// commits only when every step and check passes. Returns how many push
// records it wrote.
func migrateInTx(ctx context.Context, conn *sql.Conn) (int, error) {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("starting transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // no-op once Commit succeeds

	if _, err := tx.ExecContext(ctx, renameToV6); err != nil {
		return 0, fmt.Errorf("renaming to v6: %w", err)
	}
	records, err := backfillPushRecords(ctx, tx)
	if err != nil {
		return 0, err
	}
	if err := finishV6(ctx, tx); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("committing: %w", err)
	}
	return records, nil
}

// finishV6 drops collections.pushed_hash, checks foreign keys, stamps
// version 6 and checks the structure against a fresh v6 database, through tx.
func finishV6(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, `ALTER TABLE collections DROP COLUMN pushed_hash`); err != nil {
		return fmt.Errorf("dropping collections.pushed_hash: %w", err)
	}
	if err := requireNoForeignKeyProblems(ctx, tx); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `PRAGMA user_version = 6`); err != nil {
		return fmt.Errorf("stamping version 6: %w", err)
	}
	return requireFreshStructure(ctx, tx)
}

// queryRower is what *sql.Conn, *sql.Tx and *sql.DB share for a one-row read.
type queryRower interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// querier is what *sql.Conn, *sql.Tx and *sql.DB share for a many-row read.
type querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// pushedCollection is a pushed collection's id and title, read from the
// bytes push sends.
type pushedCollection struct {
	ID    uuid.UUID `json:"id"`
	Title string    `json:"title"`
}

// backfillPushRecords writes each profile's push record, built from the Home
// its last push stored (vault.StoredPushRecord), through tx. It first refuses,
// writing nothing, while any collection on Home needs a push by v5's rule: the
// hash of what push would send for it now differs from collections.pushed_hash.
// Returns how many records it wrote.
func backfillPushRecords(ctx context.Context, tx *sql.Tx) (int, error) {
	profiles, err := queryStrings(ctx, tx, `SELECT id FROM profiles ORDER BY id`)
	if err != nil {
		return 0, err
	}
	pushedHashes, err := pushedHashesOnHome(ctx, tx)
	if err != nil {
		return 0, err
	}
	records := make([]vault.PushRecord, len(profiles))
	var waiting []string
	for i, id := range profiles {
		if records[i], err = vault.StoredPushRecord(ctx, tx, uuid.MustParse(id)); err != nil {
			return 0, err
		}
		if waiting, err = appendWaiting(waiting, records[i], pushedHashes); err != nil {
			return 0, err
		}
	}
	if len(waiting) > 0 {
		return 0, fmt.Errorf("refusing to migrate: %d collections on Home need a push (%s); push every profile, then migrate",
			len(waiting), strings.Join(waiting, ", "))
	}
	for i, id := range profiles {
		if err := vault.WritePushRecord(ctx, tx, uuid.MustParse(id), records[i]); err != nil {
			return 0, err
		}
	}
	return len(profiles), nil
}

// pushedHashesOnHome is collections.pushed_hash of every collection on Home,
// by id, "" where push never stored one.
func pushedHashesOnHome(ctx context.Context, q querier) (map[uuid.UUID]string, error) {
	rows, err := q.QueryContext(ctx, `SELECT id, coalesce(pushed_hash, '') FROM collections WHERE home_sort_order IS NOT NULL`)
	if err != nil {
		return nil, fmt.Errorf("reading pushed hashes: %w", err)
	}
	defer func() { _ = rows.Close() }()
	hashes := map[uuid.UUID]string{}
	for rows.Next() {
		var id, hash string
		if err := rows.Scan(&id, &hash); err != nil {
			return nil, fmt.Errorf("reading pushed hashes: %w", err)
		}
		hashes[uuid.MustParse(id)] = hash
	}
	return hashes, rows.Err()
}

// appendWaiting is waiting with each collection record holds whose bytes
// don't hash to its pushed hash appended, as `"Title" (id)`.
func appendWaiting(waiting []string, record vault.PushRecord, pushedHashes map[uuid.UUID]string) ([]string, error) {
	for _, raw := range record.Collections {
		var c pushedCollection
		if err := json.Unmarshal(raw, &c); err != nil {
			return nil, fmt.Errorf("decoding a pushed collection: %w", err)
		}
		sum := sha256.Sum256(raw)
		if hex.EncodeToString(sum[:]) != pushedHashes[c.ID] {
			waiting = append(waiting, fmt.Sprintf("%q (%s)", c.Title, c.ID))
		}
	}
	return waiting, nil
}

// requireNoForeignKeyProblems refuses when PRAGMA foreign_key_check reports
// any row.
func requireNoForeignKeyProblems(ctx context.Context, q querier) error {
	rows, err := q.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		return fmt.Errorf("checking foreign keys: %w", err)
	}
	defer func() { _ = rows.Close() }()
	if rows.Next() {
		return errors.New("refusing to migrate: PRAGMA foreign_key_check reports a broken reference")
	}
	return rows.Err()
}

// requireFreshStructure refuses unless the structure tx sees matches a fresh
// v6 database's, made by vault.InitDB in a temporary directory.
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
		return fmt.Errorf("refusing to migrate: the structure differs from a fresh v6 database: has %v, lacks %v", extra, missing)
	}
	return nil
}

// freshStructure is the structure of a fresh v6 database.
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
// and table.
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
	slices.Sort(lines)
	return lines, nil
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
