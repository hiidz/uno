// Package vault owns Uno's local SQLite database: schema, and the
// catalogs, collections, and profiles stored against it.
package vault

import (
	"context"
	"database/sql"
	"fmt"

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

// InitDB opens (creating if necessary) the SQLite database at path, applies
// the schema, and returns a ready-to-use DB.
func InitDB(path string) (*DB, error) {
	dsn := "file:" + path +
		"?_pragma=journal_mode(WAL)" +
		"&_pragma=busy_timeout(5000)" +
		"&_pragma=foreign_keys(on)"

	d, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Create tables
	if _, err := d.Exec(schema); err != nil {
		return nil, fmt.Errorf("failed to create schema: %w", err)
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
