// Package vault owns Uno's local SQLite database: its schema (schema.sql),
// and the catalogs, collections, and profiles stored against it.
package vault

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schema string

// schemaVersion is the PRAGMA user_version a database holding schema.sql's
// schema carries.
const schemaVersion = 5

// DB is a handle to Uno's SQLite database.
type DB struct {
	conn *sql.DB
}

// Close closes the underlying database connection.
func (db *DB) Close() error {
	return db.conn.Close()
}

// InitDB opens (creating if necessary) the SQLite database at path, creates
// the schema in it when it is empty, and returns a ready-to-use DB. A
// database at any other schema version is an error.
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
	if err := db.ensureSchema(context.Background()); err != nil {
		_ = d.Close()
		return nil, err
	}
	return db, nil
}

// ensureSchema creates the schema and stamps schemaVersion in a database at
// user_version 0, in one transaction, and fails on any version but
// schemaVersion.
func (db *DB) ensureSchema(ctx context.Context) error {
	return db.inTx(ctx, func(tx *sql.Tx) error {
		var version int
		if err := tx.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&version); err != nil {
			return fmt.Errorf("reading the schema version: %w", err)
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
	})
}

// inTx runs write inside one transaction, committing only when it succeeds.
func (db *DB) inTx(ctx context.Context, write func(*sql.Tx) error) error {
	tx, err := db.conn.BeginTx(ctx, nil)
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
