package migrations

import (
	"context"
	"database/sql"
	"slices"
	"strings"
	"testing"
)

// runAccounts runs migration 4 in a transaction on d and commits it when it
// succeeds.
func runAccounts(t *testing.T, d *sql.DB) ([]string, error) {
	t.Helper()
	tx, err := d.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	notes, err := accounts(context.Background(), tx)
	if err != nil {
		return notes, err
	}
	return notes, tx.Commit()
}

// Migration 4 adds an empty accounts table with its four columns, and leaves
// every profile as it was.
func TestAccountsMigration(t *testing.T) {
	d := openMigrated(t)
	d.SetMaxOpenConns(1)
	before := queryStrings(t, d, `SELECT id || token || nuvio_user_id FROM profiles ORDER BY id`)

	notes, err := runAccounts(t, d)
	if err != nil {
		t.Fatalf("accounts: %v", err)
	}
	if want := []string{"added accounts, which holds each account's own TMDB key, sealed"}; !slices.Equal(notes, want) {
		t.Errorf("notes = %q, want %q", notes, want)
	}
	if got := queryStrings(t, d, `SELECT count(*) FROM accounts`); !slices.Equal(got, []string{"0"}) {
		t.Errorf("accounts = %q, want none", got)
	}
	got := queryStrings(t, d, `SELECT name FROM pragma_table_info('accounts') ORDER BY cid`)
	if want := []string{"nuvio_user_id", "tmdb_key_ciphertext", "tmdb_key_last4", "updated_at"}; !slices.Equal(got, want) {
		t.Errorf("columns = %q, want %q", got, want)
	}
	if after := queryStrings(t, d, `SELECT id || token || nuvio_user_id FROM profiles ORDER BY id`); !slices.Equal(after, before) {
		t.Errorf("profiles = %q, want %q", after, before)
	}
	if _, err := d.Exec(`INSERT INTO accounts VALUES ('a', x'00', '1234', 't'), ('a', x'01', '5678', 't')`); err == nil {
		t.Error("one account took two rows; want the primary key to refuse it")
	}
}

// Migration 4 fails, naming its step, when accounts already exists.
func TestAccountsMigrationFailsLoudly(t *testing.T) {
	d := openTestDB(t, `CREATE TABLE accounts (id TEXT)`)
	if _, err := runAccounts(t, d); err == nil || !strings.Contains(err.Error(), "creating accounts") {
		t.Fatalf("err = %v, want one naming the step", err)
	}
}
