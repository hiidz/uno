// The v11→v12 migration, `uno migrate --db <path>`: run once against prod's
// vault at schema version 11, then deleted. Prod took version 11 before the
// release mark left it, so its vault still holds catalogs.unpublished_at,
// collections.unpublished_at and the release trigger that sets them; the
// migration drops both columns and writes the trigger as schema.sql has it, and
// a v11 vault without those columns is refused. Version 12 adds the revisions a save and a
// push are checked against, catalogs.revision, collections.revision and
// profiles.home_revision, each 1 for every row, and replaces accounts with
// account_keys, one sealed key per account and provider. The keys accounts
// holds are dropped, not carried over: each was sealed bound to its account
// alone, which a v12 key no longer opens under, so an owner on a per-account
// server enters the key again. In one transaction it applies these as
// schema.sql writes them, stamps version 12, and checks that the database's
// tables, columns, indexes and triggers are a fresh v12 database's.

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

// changes is the migration's DDL, each statement as schema.sql has it. The
// release trigger is dropped first, since it names the columns dropped after
// it, and each drop comes before the add that takes the column's place.
const changes = `
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
ALTER TABLE profiles ADD COLUMN home_revision INTEGER NOT NULL DEFAULT 1;
ALTER TABLE catalogs ADD COLUMN revision INTEGER NOT NULL DEFAULT 1;
ALTER TABLE collections ADD COLUMN revision INTEGER NOT NULL DEFAULT 1;
CREATE TABLE account_keys (
    nuvio_user_id  TEXT NOT NULL, -- Nuvio auth.users.id, as profiles.nuvio_user_id
    provider       TEXT NOT NULL, -- the provider the key is for, as catalogs.provider
    key_ciphertext BLOB NOT NULL, -- nonce || AES-GCM sealed key
    key_last4      TEXT NOT NULL, -- the key's last four characters, shown to its owner
    updated_at     TEXT NOT NULL,
    PRIMARY KEY (nuvio_user_id, provider)
);
DROP TABLE accounts;
PRAGMA user_version = 12;
`

// runMigrate is the migrate subcommand: it parses args and migrates the
// database --db names, writing what it did to out.
func runMigrate(ctx context.Context, args []string, out io.Writer) error {
	flags := flag.NewFlagSet("migrate", flag.ContinueOnError)
	flags.SetOutput(out)
	path := flags.String("db", "", "path to the vault.db to migrate from schema version 11 to 12")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *path == "" {
		return errors.New("migrate: --db is required")
	}
	if _, err := os.Stat(*path); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	if err := migrateToV12(ctx, *path); err != nil {
		return err
	}
	_, err := fmt.Fprintf(out, "migrated %s to schema version 12: dropped the release mark, added the revision columns, replaced accounts with account_keys (no key carried over)\n", *path)
	return err
}

// migrateToV12 migrates the v11 database at path to v12 in one transaction,
// committing only once the structure matches a fresh v12 database's. The
// transaction takes the write lock when it begins, as the vault's own do.
func migrateToV12(ctx context.Context, path string) error {
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

// migrateTx refuses a database not at version 11, applies the changes and
// stamps version 12 through tx, then checks the structure.
func migrateTx(ctx context.Context, tx *sql.Tx) error {
	var version int
	if err := tx.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&version); err != nil {
		return fmt.Errorf("reading schema version: %w", err)
	}
	if version != 11 {
		return fmt.Errorf("the database is at schema version %d; migrate only runs on version 11", version)
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

// requireFreshStructure refuses unless the structure tx sees is a fresh v12
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
		return fmt.Errorf("refusing to migrate: the structure differs from a fresh v12 database:\nhas  %q\nwant %q", got, want)
	}
	return nil
}

// freshStructure is the structure of a fresh v12 database.
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
// type, nullability, default and primary-key place; the table and the trigger
// the migration creates also carry their SQL, so the table's constraints and
// the trigger's body are compared too. Other rows' SQL is left out: a table a
// column was added to or dropped from stores its CREATE text in another form.
const structureQuery = `
	SELECT type || ' ' || name || ' on ' || tbl_name
	       || CASE WHEN name IN ('account_keys', 'publications_release_subscribers') THEN ': ' || sql ELSE '' END
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
