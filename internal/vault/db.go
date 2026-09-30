// Package vault owns Uno's local SQLite database: its schema, migrated by
// the migrations package, and the catalogs, collections, and profiles
// stored against it.
package vault

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/hiidz/uno/internal/vault/migrations"
	_ "modernc.org/sqlite"
)

// DB is a handle to Uno's SQLite database.
type DB struct {
	conn *sql.DB
}

// Close closes the underlying database connection.
func (db *DB) Close() error {
	return db.conn.Close()
}

// InitDB opens (creating if necessary) the SQLite database at path, migrates
// it to the latest schema version, and returns a ready-to-use DB. A database
// that can't be migrated, or is newer than this build knows, is an error.
func InitDB(path string) (*DB, error) {
	if err := migrate(context.Background(), path, migrations.All()); err != nil {
		return nil, fmt.Errorf("failed to migrate database: %w", err)
	}

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
	return &DB{conn: d}, nil
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
