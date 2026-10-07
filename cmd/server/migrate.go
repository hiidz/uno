// The v9→v10 migration, `uno migrate --db <path>`: run once against a vault at
// schema version 9, then deleted. Version 10 adds the two indexes Community
// pages through. In one transaction it creates them as schema.sql writes them,
// stamps version 10, and checks that the database's tables, indexes and
// triggers are a fresh v10 database's. No row changes.

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

// addIndexes is the migration's DDL, each index exactly as schema.sql writes it.
const addIndexes = `
CREATE INDEX publications_by_kind_newest ON publications (kind, published_at, id);
CREATE INDEX publications_by_kind_title ON publications (kind, title COLLATE NOCASE, id);
PRAGMA user_version = 10;
`

// runMigrate is the migrate subcommand: it parses args and migrates the
// database --db names, writing what it did to out.
func runMigrate(ctx context.Context, args []string, out io.Writer) error {
	flags := flag.NewFlagSet("migrate", flag.ContinueOnError)
	flags.SetOutput(out)
	path := flags.String("db", "", "path to the vault.db to migrate from schema version 9 to 10")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *path == "" {
		return errors.New("migrate: --db is required")
	}
	if _, err := os.Stat(*path); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	if err := migrateToV10(ctx, *path); err != nil {
		return err
	}
	_, err := fmt.Fprintf(out, "migrated %s to schema version 10: added publications_by_kind_newest and publications_by_kind_title\n", *path)
	return err
}

// migrateToV10 migrates the v9 database at path to v10 in one transaction,
// committing only once the structure matches a fresh v10 database's.
func migrateToV10(ctx context.Context, path string) error {
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)")
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

// migrateTx refuses a database not at version 9, adds the indexes and stamps
// version 10 through tx, then checks the structure.
func migrateTx(ctx context.Context, tx *sql.Tx) error {
	var version int
	if err := tx.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&version); err != nil {
		return fmt.Errorf("reading schema version: %w", err)
	}
	if version != 9 {
		return fmt.Errorf("the database is at schema version %d; migrate only runs on version 9", version)
	}
	if _, err := tx.ExecContext(ctx, addIndexes); err != nil {
		return fmt.Errorf("adding indexes: %w", err)
	}
	return requireFreshStructure(ctx, tx)
}

// querier is what *sql.Tx and *sql.DB share for a many-row read.
type querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// requireFreshStructure refuses unless the structure tx sees is a fresh v10
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
		return fmt.Errorf("refusing to migrate: the structure differs from a fresh v10 database:\nhas  %q\nwant %q", got, want)
	}
	return nil
}

// freshStructure is the structure of a fresh v10 database.
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

// structureOf is every table, index and trigger in a database, each as its
// kind, name and table, sorted by name, with the SQL of the two indexes the
// migration writes. Other rows' SQL is left out: a table an earlier migration
// rebuilt and renamed stores its CREATE text in another form.
func structureOf(ctx context.Context, q querier) ([]string, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT type || ' ' || name || ' on ' || tbl_name
		       || CASE WHEN name LIKE 'publications\_by\_kind\_%' ESCAPE '\' THEN ': ' || sql ELSE '' END
		FROM sqlite_master WHERE name NOT LIKE 'sqlite\_%' ESCAPE '\' ORDER BY name`)
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
