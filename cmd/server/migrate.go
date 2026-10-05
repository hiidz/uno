// The v6→v7 migration, `uno migrate --db <path>`: run once against a vault at
// schema version 6, then deleted. Version 7 numbers a profile's Home catalogs
// and collections in one list, where version 6 numbered each apart and showed
// them in bands. In one transaction it renumbers every profile's Home in the
// order its bands showed it, gives each push record's Home those positions,
// and checks the result against a fresh v7 database.

package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"

	"github.com/hiidz/uno/internal/vault"
)

// homePositions is every on-Home catalog and collection with its v7
// position: per profile, in the order version 6's bands showed them —
// pinned collections, catalogs (Discover only among them), then the other
// collections — each band in its home_sort_order.
const homePositions = `
CREATE TEMP TABLE home_positions AS
SELECT kind, id, row_number() OVER (PARTITION BY owner_id ORDER BY band, pos) - 1 AS position FROM (
    SELECT 'collection' AS kind, id, owner_id, 0 AS band, home_sort_order AS pos
    FROM collections WHERE home_sort_order IS NOT NULL AND pin_to_top = 1
    UNION ALL
    SELECT 'catalog', id, owner_id, 1, home_sort_order FROM catalogs WHERE home_sort_order IS NOT NULL
    UNION ALL
    SELECT 'collection', id, owner_id, 2, home_sort_order
    FROM collections WHERE home_sort_order IS NOT NULL AND pin_to_top = 0
);
UPDATE catalogs SET home_sort_order =
    (SELECT position FROM home_positions WHERE kind = 'catalog' AND home_positions.id = catalogs.id)
WHERE home_sort_order IS NOT NULL;
UPDATE collections SET home_sort_order =
    (SELECT position FROM home_positions WHERE kind = 'collection' AND home_positions.id = collections.id)
WHERE home_sort_order IS NOT NULL;
DROP TABLE home_positions;
`

// runMigrate is the migrate subcommand: it parses args and migrates the
// database --db names, reporting to out.
func runMigrate(ctx context.Context, args []string, out io.Writer) error {
	flags := flag.NewFlagSet("migrate", flag.ContinueOnError)
	flags.SetOutput(out)
	path := flags.String("db", "", "path to the vault.db to migrate from schema version 6 to 7")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *path == "" {
		return errors.New("migrate: --db is required")
	}
	if _, err := os.Stat(*path); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	return migrateToV7(ctx, *path, out)
}

// migrateToV7 migrates the v6 database at path to v7 in one transaction on
// one connection, printing row counts before and after.
func migrateToV7(ctx context.Context, path string, out io.Writer) error {
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
	return migrateConn(ctx, conn, out)
}

// migrateConn migrates the database conn holds, which must be at version 6,
// printing row counts before and after to out.
func migrateConn(ctx context.Context, conn *sql.Conn, out io.Writer) error {
	if err := requireVersion(ctx, conn, 6); err != nil {
		return err
	}
	before, err := rowCounts(ctx, conn)
	if err != nil {
		return err
	}
	records, err := migrateInTx(ctx, conn)
	if err != nil {
		return err
	}
	return reportMigration(ctx, conn, out, before, records)
}

// reportMigration writes each table's row count before and after, then the
// outcome, to out.
func reportMigration(ctx context.Context, conn *sql.Conn, out io.Writer, before map[string]int, records int) error {
	after, err := rowCounts(ctx, conn)
	if err != nil {
		return err
	}
	printCounts(out, before, after)
	_, err = fmt.Fprintf(out, "migrated to schema version 7: Home renumbered as one list; %d push records given positions; structure matches a fresh v7 database\n", records)
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
// records it rewrote.
func migrateInTx(ctx context.Context, conn *sql.Conn) (int, error) {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("starting transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // no-op once Commit succeeds

	records, err := migrateTx(ctx, tx)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("committing: %w", err)
	}
	return records, nil
}

// migrateTx renumbers every Home, positions every push record, stamps
// version 7 and checks the structure against a fresh v7 database, through
// tx. Returns how many push records it rewrote.
func migrateTx(ctx context.Context, tx *sql.Tx) (int, error) {
	if _, err := tx.ExecContext(ctx, homePositions+`PRAGMA user_version = 7;`); err != nil {
		return 0, fmt.Errorf("renumbering Home: %w", err)
	}
	records, err := positionPushRecords(ctx, tx)
	if err != nil {
		return 0, err
	}
	return records, requireFreshStructure(ctx, tx)
}

// positionPushRecords gives every push record's Home entries positions in
// the order version 6's bands showed them (positionHome), through tx, leaving
// the rest of each record as it was. Returns how many records it rewrote.
func positionPushRecords(ctx context.Context, tx *sql.Tx) (int, error) {
	profiles, err := queryStrings(ctx, tx, `SELECT profile_id FROM push_records ORDER BY profile_id`)
	if err != nil {
		return 0, err
	}
	for _, id := range profiles {
		if err := positionPushRecord(ctx, tx, id); err != nil {
			return 0, err
		}
	}
	return len(profiles), nil
}

// positionPushRecord rewrites profileID's push record with positions on its
// Home entries, through tx.
func positionPushRecord(ctx context.Context, tx *sql.Tx, profileID string) error {
	record, err := readPushRecord(ctx, tx, profileID)
	if err != nil {
		return err
	}
	positionHome(&record.Home)
	return writePushRecord(ctx, tx, profileID, record)
}

// readPushRecord is profileID's push record, read through tx.
func readPushRecord(ctx context.Context, tx *sql.Tx, profileID string) (vault.PushRecord, error) {
	var raw string
	if err := tx.QueryRowContext(ctx, `SELECT record FROM push_records WHERE profile_id = ?`, profileID).Scan(&raw); err != nil {
		return vault.PushRecord{}, fmt.Errorf("reading push record of %s: %w", profileID, err)
	}
	var record vault.PushRecord
	if err := json.Unmarshal([]byte(raw), &record); err != nil {
		return vault.PushRecord{}, fmt.Errorf("decoding push record of %s: %w", profileID, err)
	}
	return record, nil
}

// writePushRecord replaces profileID's push record with record through tx,
// leaving its stamp and time as they were.
func writePushRecord(ctx context.Context, tx *sql.Tx, profileID string, record vault.PushRecord) error {
	encoded, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("encoding push record of %s: %w", profileID, err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE push_records SET record = ? WHERE profile_id = ?`, string(encoded), profileID); err != nil {
		return fmt.Errorf("writing push record of %s: %w", profileID, err)
	}
	return nil
}

// positionHome numbers home's entries in the order version 6's bands showed
// them: pinned collections, catalogs, then the other collections, each band
// in the order it lists them.
func positionHome(home *vault.PushedHome) {
	next := numberCollections(home, true, 0)
	for i := range home.Catalogs {
		home.Catalogs[i].Position = next
		next++
	}
	numberCollections(home, false, next)
}

// numberCollections numbers home's collections pinned or not, as pinned says,
// in the order it lists them, from next, and returns the number after the
// last.
func numberCollections(home *vault.PushedHome, pinned bool, next int) int {
	for i := range home.Collections {
		if home.Collections[i].PinToTop == pinned {
			home.Collections[i].Position = next
			next++
		}
	}
	return next
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
// v7 database's, made by vault.InitDB in a temporary directory.
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
		return fmt.Errorf("refusing to migrate: the structure differs from a fresh v7 database: has %v, lacks %v", extra, missing)
	}
	return nil
}

// freshStructure is the structure of a fresh v7 database.
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
