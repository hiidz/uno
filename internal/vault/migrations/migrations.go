// Package migrations holds the vault's schema migrations, one file each,
// applied in version order by the runner in internal/vault. A migration is
// frozen once it ships: it is never edited, and it imports only the standard
// library, never live Uno code, so what it does to a database cannot change
// when that code does.
package migrations

import (
	"context"
	"database/sql"
)

// Migration moves the schema from Version-1 to Version.
//
// Up runs inside the transaction the runner opens for it, on a connection
// with foreign keys off; the runner checks foreign keys and sets
// user_version to Version before committing. Up makes no network calls. The
// notes it returns describe what it found or changed, for the startup log and
// the dry-run report.
type Migration struct {
	Version int
	Name    string
	Up      func(ctx context.Context, tx *sql.Tx) ([]string, error)
}

// All returns every migration, in version order: the one at index i has
// Version i+1.
func All() []Migration {
	return []Migration{
		{Version: 1, Name: "baseline", Up: baseline},
	}
}
