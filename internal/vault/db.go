// Package vault owns Uno's local SQLite database: its schema (schema.sql),
// and the catalogs, collections, and profiles stored against it.
package vault

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"log"
	"os"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schema string

// schemaVersion is the PRAGMA user_version a database holding schema.sql's
// schema carries.
const schemaVersion = 14

// migrations[v] is the SQL that takes a database from schema version v to
// v+1. A database below schemaVersion runs every step from its version up, so
// each step stays here for good: a server can skip any number of releases.
var migrations = map[int]string{
	12: `
CREATE TABLE blocked_publishers (
    nuvio_user_id TEXT PRIMARY KEY,
    blocked_at    TEXT NOT NULL,
    title         TEXT NOT NULL
);`,
	// 14 changes no table. It holds recipes keyed by with_origin_country,
	// with_cast, with_crew or tmdb_list, which a version-13 Uno would read
	// without them and serve broader titles under the row's name.
	13: `SELECT 1;`,
}

// DB is a handle to Uno's SQLite database.
type DB struct {
	conn *sql.DB
	// sharesMedia reports whether Community shares a media address
	// (WithholdCommunityMedia, ShareCommunityMediaFrom); nil shares every one.
	sharesMedia func(raw string) bool
	// mediaSites is the sites ShareCommunityMediaFrom shares media from.
	mediaSites []string
}

// Close closes the underlying database connection.
func (db *DB) Close() error {
	return db.conn.Close()
}

// InitDB opens (creating if necessary) the SQLite database at path, creates
// the schema in it when it is empty or upgrades it when it is older, and
// returns a ready-to-use DB. A database at a newer schema version, or at one
// with no upgrade path, is an error.
func InitDB(path string) (*DB, error) {
	// _txlock=immediate: every transaction takes the write lock when it
	// begins, waiting out busy_timeout for it. A deferred one that reads
	// before it writes, as most writes here do, can't take the lock later once
	// another write has committed, and fails at once with SQLITE_BUSY. The
	// pool's reads run outside transactions and are unaffected.
	dsn := "file:" + path +
		"?_pragma=journal_mode(WAL)" +
		"&_pragma=busy_timeout(5000)" +
		"&_pragma=foreign_keys(on)" +
		"&_txlock=immediate"

	d, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}
	db := &DB{conn: d}
	if err := db.ensureSchema(context.Background(), path); err != nil {
		_ = d.Close()
		return nil, err
	}
	return db, nil
}

// ensureSchema upgrades a database below schemaVersion, and otherwise creates
// the schema in an empty one or accepts one already at schemaVersion.
func (db *DB) ensureSchema(ctx context.Context, path string) error {
	version, err := readVersion(ctx, db.conn)
	if err != nil {
		return err
	}
	if version > 0 && version < schemaVersion {
		return db.upgrade(ctx, path, version, schemaVersion, migrations)
	}
	return db.inTx(ctx, func(tx *sql.Tx) error { return createSchema(ctx, tx) })
}

// createSchema creates the schema and stamps schemaVersion in a database at
// user_version 0, and fails on any version but schemaVersion.
func createSchema(ctx context.Context, tx *sql.Tx) error {
	version, err := readVersion(ctx, tx)
	if err != nil {
		return err
	}
	if version == schemaVersion {
		return nil
	}
	if version != 0 {
		return fmt.Errorf("the database is at schema version %d; this build uses schema version %d", version, schemaVersion)
	}
	if _, err := tx.ExecContext(ctx, schema+fmt.Sprintf("\nPRAGMA user_version = %d;", schemaVersion)); err != nil {
		return fmt.Errorf("creating the schema: %w", err)
	}
	return nil
}

// readVersion reads the database's PRAGMA user_version.
func readVersion(ctx context.Context, q dbtx) (int, error) {
	var version int
	if err := q.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&version); err != nil {
		return 0, fmt.Errorf("reading the schema version: %w", err)
	}
	return version, nil
}

// upgrade takes the database at path from schema version from to version to,
// through steps[from] to steps[to-1]. It backs the database up to
// path.v<from>.bak first, and changes nothing when a step is missing.
func (db *DB) upgrade(ctx context.Context, path string, from, to int, steps map[int]string) error {
	for v := from; v < to; v++ {
		if _, ok := steps[v]; !ok {
			return fmt.Errorf("the database is at schema version %d; this build uses schema version %d and has no upgrade from version %d", from, to, v)
		}
	}
	backup := fmt.Sprintf("%s.v%d.bak", path, from)
	log.Printf("Upgrading the database from schema version %d to %d, backup at %s", from, to, backup)
	if err := db.backup(ctx, backup); err != nil {
		return err
	}
	return db.migrate(ctx, from, to, steps)
}

// backup writes a copy of the database to path with VACUUM INTO, through a
// temporary file renamed over any copy already there.
func (db *DB) backup(ctx context.Context, path string) error {
	tmp := path + ".tmp"
	_ = os.Remove(tmp) // left by an interrupted backup; VACUUM INTO refuses an existing file
	if _, err := db.conn.ExecContext(ctx, `VACUUM INTO ?`, tmp); err != nil {
		return fmt.Errorf("backing the database up before upgrading it: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("backing the database up before upgrading it: %w", err)
	}
	return nil
}

// migrate runs the steps from version from to version to and stamps to, in
// one transaction on one connection with foreign keys off: a step that
// rebuilds a table drops the old one, and with foreign keys on the drop runs
// its ON DELETE actions. Foreign keys are back on before the connection
// returns to the pool.
func (db *DB) migrate(ctx context.Context, from, to int, steps map[int]string) error {
	conn, err := db.conn.Conn(ctx)
	if err != nil {
		return fmt.Errorf("upgrading the database: %w", err)
	}
	defer func() { _ = conn.Close() }()
	if _, err := conn.ExecContext(ctx, `PRAGMA foreign_keys = OFF`); err != nil {
		return fmt.Errorf("upgrading the database: %w", err)
	}
	err = inTxOn(ctx, conn, func(tx *sql.Tx) error { return runSteps(ctx, tx, from, to, steps) })
	_, onErr := conn.ExecContext(ctx, `PRAGMA foreign_keys = ON`)
	return errors.Join(err, onErr)
}

// runSteps runs steps[from] to steps[to-1] in tx, confirms every foreign key
// still holds, and stamps version to. It refuses a database no longer at
// version from.
func runSteps(ctx context.Context, tx *sql.Tx, from, to int, steps map[int]string) error {
	if err := requireVersion(ctx, tx, from); err != nil {
		return err
	}
	if err := applySteps(ctx, tx, from, to, steps); err != nil {
		return err
	}
	if err := checkForeignKeys(ctx, tx); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, fmt.Sprintf(`PRAGMA user_version = %d`, to)); err != nil {
		return fmt.Errorf("stamping schema version %d: %w", to, err)
	}
	return nil
}

// requireVersion fails unless the database is at schema version want, as it
// was when the upgrade began.
func requireVersion(ctx context.Context, tx *sql.Tx, want int) error {
	version, err := readVersion(ctx, tx)
	if err != nil {
		return err
	}
	if version != want {
		return fmt.Errorf("the database's schema version changed from %d to %d during the upgrade", want, version)
	}
	return nil
}

// applySteps runs steps[from] to steps[to-1] in tx, in order.
func applySteps(ctx context.Context, tx *sql.Tx, from, to int, steps map[int]string) error {
	for v := from; v < to; v++ {
		if _, err := tx.ExecContext(ctx, steps[v]); err != nil {
			return fmt.Errorf("upgrading the database from schema version %d: %w", v, err)
		}
	}
	return nil
}

// checkForeignKeys fails when any row refers to a row that doesn't exist.
func checkForeignKeys(ctx context.Context, tx *sql.Tx) error {
	var table string
	switch err := tx.QueryRowContext(ctx, `SELECT "table" FROM pragma_foreign_key_check`).Scan(&table); {
	case err == nil:
		return fmt.Errorf("the upgrade left rows in %s pointing at rows that don't exist", table)
	case !errors.Is(err, sql.ErrNoRows):
		return fmt.Errorf("checking foreign keys after the upgrade: %w", err)
	}
	return nil
}

// inReadTx runs read inside one read-only transaction, so every query it makes
// sees the one snapshot the first of them took. A read that assembles a tree
// or compares several tables goes through it; a single query doesn't need to.
func (db *DB) inReadTx(ctx context.Context, read func(q dbtx) error) error {
	tx, err := db.conn.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return fmt.Errorf("starting read transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // read-only; never committed
	return read(tx)
}

// inTx runs write inside one transaction, committing only when it succeeds.
func (db *DB) inTx(ctx context.Context, write func(*sql.Tx) error) error {
	return inTxOn(ctx, db.conn, write)
}

// txBeginner is what inTxOn begins a transaction on: the pool, or one
// connection taken from it.
type txBeginner interface {
	BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error)
}

// inTxOn runs write inside one transaction begun on b, committing only when
// it succeeds.
func inTxOn(ctx context.Context, b txBeginner, write func(*sql.Tx) error) error {
	tx, err := b.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("starting transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // no-op once Commit succeeds

	if err := write(tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}
	return nil
}
