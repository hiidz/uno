// Accounts: the TMDB key a Nuvio account saves for a server in per-account
// key mode. The vault stores the key only sealed; sealing and opening it is
// internal/tmdbkey's.

package vault

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrNoAccountKey is an account that has saved no TMDB key.
var ErrNoAccountKey = errors.New("vault: the account has no TMDB key")

// AccountKey is an account's stored TMDB key: sealed, and its last four
// characters, which are all its owner is ever shown of it.
type AccountKey struct {
	Sealed []byte
	Last4  string
}

// SetAccountKey stores account's sealed TMDB key and its last four
// characters, replacing any it had.
func (db *DB) SetAccountKey(ctx context.Context, account string, key AccountKey) error {
	if _, err := db.conn.ExecContext(ctx, `
		INSERT INTO accounts (nuvio_user_id, tmdb_key_ciphertext, tmdb_key_last4, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT (nuvio_user_id) DO UPDATE SET
			tmdb_key_ciphertext = excluded.tmdb_key_ciphertext,
			tmdb_key_last4      = excluded.tmdb_key_last4,
			updated_at          = excluded.updated_at
	`, account, key.Sealed, key.Last4, utcTimestamp(time.Now())); err != nil {
		return fmt.Errorf("storing the TMDB key: %w", err)
	}
	return nil
}

// AccountKey returns account's stored TMDB key, or ErrNoAccountKey.
func (db *DB) AccountKey(ctx context.Context, account string) (AccountKey, error) {
	var key AccountKey
	err := db.conn.QueryRowContext(ctx, `
		SELECT tmdb_key_ciphertext, tmdb_key_last4 FROM accounts WHERE nuvio_user_id = ?
	`, account).Scan(&key.Sealed, &key.Last4)
	if errors.Is(err, sql.ErrNoRows) {
		return AccountKey{}, ErrNoAccountKey
	}
	if err != nil {
		return AccountKey{}, fmt.Errorf("reading the TMDB key: %w", err)
	}
	return key, nil
}

// DeleteAccountKey removes account's stored TMDB key. Removing a key that
// isn't there is not an error.
func (db *DB) DeleteAccountKey(ctx context.Context, account string) error {
	if _, err := db.conn.ExecContext(ctx, `DELETE FROM accounts WHERE nuvio_user_id = ?`, account); err != nil {
		return fmt.Errorf("removing the TMDB key: %w", err)
	}
	return nil
}

// AccountKeyByToken returns the Nuvio account that owns the profile whose
// token it is, with that account's sealed TMDB key, nil when it has saved
// none. An unknown token is ErrProfileNotFound.
func (db *DB) AccountKeyByToken(ctx context.Context, token string) (string, []byte, error) {
	var account string
	var sealed []byte
	err := db.conn.QueryRowContext(ctx, `
		SELECT p.nuvio_user_id, a.tmdb_key_ciphertext
		FROM profiles p LEFT JOIN accounts a ON a.nuvio_user_id = p.nuvio_user_id
		WHERE p.token = ?
	`, token).Scan(&account, &sealed)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil, ErrProfileNotFound
	}
	if err != nil {
		return "", nil, fmt.Errorf("reading the owner's TMDB key: %w", err)
	}
	return account, sealed, nil
}
