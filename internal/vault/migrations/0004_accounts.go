package migrations

import (
	"context"
	"database/sql"
	"fmt"
)

// accounts adds the accounts table, empty: one row per Nuvio account that has
// saved its own TMDB key, which a server in per-account key mode uses for
// everything that account's profiles ask of TMDB. The key is stored sealed
// (AES-GCM under the server's secret, bound to the account id) beside its last
// four characters, which is all the builder ever shows of it.
func accounts(ctx context.Context, tx *sql.Tx) ([]string, error) {
	if _, err := tx.ExecContext(ctx, accountsDDL); err != nil {
		return nil, fmt.Errorf("creating accounts: %w", err)
	}
	return []string{"added accounts, which holds each account's own TMDB key, sealed"}, nil
}

const accountsDDL = `
CREATE TABLE accounts (
    nuvio_user_id       TEXT PRIMARY KEY,  -- Nuvio auth.users.id, as profiles.nuvio_user_id
    tmdb_key_ciphertext BLOB NOT NULL,     -- nonce || AES-GCM sealed key
    tmdb_key_last4      TEXT NOT NULL,     -- the key's last four characters, shown to its owner
    updated_at          TEXT NOT NULL
);
`
