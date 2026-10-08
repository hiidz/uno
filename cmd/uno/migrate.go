// The v10→v11 migration, `uno migrate --db <path>`: run once against a vault at
// schema version 10, then deleted. Version 11 adds the index a publication's
// subscribers are read by, drops publications.catalog_count and folder_count,
// which nothing reads, and drops the release mark, catalogs.unpublished_at and
// collections.unpublished_at, with the trigger that set it. In one transaction
// it applies these as schema.sql writes them, stamps version 11, and checks
// that the database's tables, columns, indexes and triggers are a fresh v11
// database's. No row is rewritten.

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

// changes is the migration's DDL, each statement as schema.sql has it.
const changes = `
CREATE INDEX subscriptions_by_publication ON subscriptions (publication_id);
ALTER TABLE publications DROP COLUMN catalog_count;
ALTER TABLE publications DROP COLUMN folder_count;
DROP TRIGGER publications_release_subscribers;
ALTER TABLE catalogs DROP COLUMN unpublished_at;
ALTER TABLE collections DROP COLUMN unpublished_at;
CREATE TRIGGER publications_release_subscribers BEFORE DELETE ON publications
BEGIN
    UPDATE catalogs SET sub_key = NULL
    WHERE collection_id IN (SELECT collection_id FROM subscriptions WHERE publication_id = OLD.id);
    UPDATE folders SET sub_key = NULL
    WHERE collection_id IN (SELECT collection_id FROM subscriptions WHERE publication_id = OLD.id);
END;
PRAGMA user_version = 11;
`

// runMigrate is the migrate subcommand: it parses args and migrates the
// database --db names, writing what it did to out.
func runMigrate(ctx context.Context, args []string, out io.Writer) error {
	flags := flag.NewFlagSet("migrate", flag.ContinueOnError)
	flags.SetOutput(out)
	path := flags.String("db", "", "path to the vault.db to migrate from schema version 10 to 11")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *path == "" {
		return errors.New("migrate: --db is required")
	}
	if _, err := os.Stat(*path); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	if err := migrateToV11(ctx, *path); err != nil {
		return err
	}
	_, err := fmt.Fprintf(out, "migrated %s to schema version 11: added subscriptions_by_publication, dropped publications.catalog_count and folder_count and the release mark\n", *path)
	return err
}

// migrateToV11 migrates the v10 database at path to v11 in one transaction,
// committing only once the structure matches a fresh v11 database's. The
// transaction takes the write lock when it begins, as the vault's own do.
func migrateToV11(ctx context.Context, path string) error {
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)&_txlock=immediate")
	if err != nil {
		return fmt.Errorf("opening %s: %w", path, err)
	}
	defer func() { _ = db.Close() }()
	tx, err := db.BeginTx(ctx, nil)
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

// migrateTx refuses a database not at version 10, applies the changes and
// stamps version 11 through tx, then checks the structure.
func migrateTx(ctx context.Context, tx *sql.Tx) error {
	var version int
	if err := tx.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&version); err != nil {
		return fmt.Errorf("reading schema version: %w", err)
	}
	if version != 10 {
		return fmt.Errorf("the database is at schema version %d; migrate only runs on version 10", version)
	}
	if _, err := tx.ExecContext(ctx, changes); err != nil {
		return fmt.Errorf("applying the changes: %w", err)
	}
	return requireFreshStructure(ctx, tx)
}

// querier is what *sql.Tx and *sql.DB share for a many-row read.
type querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// requireFreshStructure refuses unless the structure tx sees is a fresh v11
// database's, made by vault.InitDB in a temporary directory.
func requireFreshStructure(ctx context.Context, tx *sql.Tx) error {
	want, err := freshStructure(ctx)
	if err != nil {
		return err
	}
	got, err := structureOf(ctx, tx)
	if err != nil {
		return err
	}
	if !slices.Equal(got, want) {
		return fmt.Errorf("refusing to migrate: the structure differs from a fresh v11 database:\nhas  %q\nwant %q", got, want)
	}
	return nil
}

// freshStructure is the structure of a fresh v11 database.
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

// structureQuery lists every table, index and trigger in a database as its
// kind, name and table, then every column of every table in order with its
// type, nullability, default and primary-key place; the index and the trigger
// the migration writes also carry their SQL. Other rows' SQL is left out: a table
// whose column was dropped stores its CREATE text in another form.
const structureQuery = `
	SELECT type || ' ' || name || ' on ' || tbl_name
	       || CASE WHEN name IN ('subscriptions_by_publication', 'publications_release_subscribers') THEN ': ' || sql ELSE '' END
	FROM sqlite_master WHERE name NOT LIKE 'sqlite\_%' ESCAPE '\'
	UNION ALL
	SELECT 'column ' || m.name || ' ' || c.cid || ' ' || c.name || ' ' || c.type
	       || ' notnull=' || c."notnull" || ' default=' || coalesce(c.dflt_value, 'NULL') || ' pk=' || c.pk
	FROM sqlite_master m, pragma_table_xinfo(m.name) c
	WHERE m.type = 'table' AND m.name NOT LIKE 'sqlite\_%' ESCAPE '\'
	ORDER BY 1`

// structureOf is the structure structureQuery lists for a database, sorted.
func structureOf(ctx context.Context, q querier) ([]string, error) {
	rows, err := q.QueryContext(ctx, structureQuery)
	if err != nil {
		return nil, fmt.Errorf("reading the structure: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var lines []string
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			return nil, err
		}
		lines = append(lines, line)
	}
	return lines, rows.Err()
}
