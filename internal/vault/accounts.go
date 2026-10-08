// Account keys: the key a Nuvio account saves for a provider on a server in
// per-account key mode, one per account and provider. The vault stores a key
// only sealed; sealing and opening it is internal/tmdbkey's.

package vault

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrNoAccountKey is an account that has saved no key for the provider.
var ErrNoAccountKey = errors.New("vault: the account has no key for the provider")

// AccountKey is an account's stored key for a provider: sealed, and its last
// four characters, which are all its owner is ever shown of it.
type AccountKey struct {
	Sealed []byte
	Last4  string
}

// SetAccountKey stores account's sealed key for provider and its last four
// characters, replacing any it had for that provider.
func (db *DB) SetAccountKey(ctx context.Context, account, provider string, key AccountKey) error {
	if _, err := db.conn.ExecContext(ctx, `
		INSERT INTO account_keys (nuvio_user_id, provider, key_ciphertext, key_last4, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (nuvio_user_id, provider) DO UPDATE SET
			key_ciphertext = excluded.key_ciphertext,
			key_last4      = excluded.key_last4,
			updated_at     = excluded.updated_at
	`, account, provider, key.Sealed, key.Last4, utcTimestamp(time.Now())); err != nil {
		return fmt.Errorf("storing the %s key: %w", provider, err)
	}
	return nil
}

// AccountKey returns account's stored key for provider, or ErrNoAccountKey.
func (db *DB) AccountKey(ctx context.Context, account, provider string) (AccountKey, error) {
	var key AccountKey
	err := db.conn.QueryRowContext(ctx, `
		SELECT key_ciphertext, key_last4 FROM account_keys WHERE nuvio_user_id = ? AND provider = ?
	`, account, provider).Scan(&key.Sealed, &key.Last4)
	if errors.Is(err, sql.ErrNoRows) {
		return AccountKey{}, ErrNoAccountKey
	}
	if err != nil {
		return AccountKey{}, fmt.Errorf("reading the %s key: %w", provider, err)
	}
	return key, nil
}

// DeleteAccountKey removes account's stored key for provider. Removing a key
// that isn't there is not an error.
func (db *DB) DeleteAccountKey(ctx context.Context, account, provider string) error {
	if _, err := db.conn.ExecContext(ctx, `
		DELETE FROM account_keys WHERE nuvio_user_id = ? AND provider = ?
	`, account, provider); err != nil {
		return fmt.Errorf("removing the %s key: %w", provider, err)
	}
	return nil
}

// AccountKeyByToken returns the Nuvio account that owns the profile whose
// token it is, with that account's sealed key for provider, nil when it has
// saved none. An unknown token is ErrProfileNotFound.
func (db *DB) AccountKeyByToken(ctx context.Context, token, provider string) (string, []byte, error) {
	var account string
	var sealed []byte
	err := db.conn.QueryRowContext(ctx, `
		SELECT p.nuvio_user_id, a.key_ciphertext
		FROM profiles p LEFT JOIN account_keys a ON a.nuvio_user_id = p.nuvio_user_id AND a.provider = ?
		WHERE p.token = ?
	`, provider, token).Scan(&account, &sealed)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil, ErrProfileNotFound
	}
	if err != nil {
		return "", nil, fmt.Errorf("reading the owner's %s key: %w", provider, err)
	}
	return account, sealed, nil
}
