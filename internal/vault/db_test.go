package vault

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
)

// userVersion reads PRAGMA user_version from the database at path.
func userVersion(t *testing.T, path string) int {
	t.Helper()
	d, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	var version int
	if err := d.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	return version
}

// An empty file gets the schema and schemaVersion, and opening it again
// leaves it as it is.
func TestInitDBCreatesTheSchemaOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vault.db")
	for range 2 {
		db, err := InitDB(path)
		if err != nil {
			t.Fatalf("InitDB: %v", err)
		}
		if _, err := db.conn.Exec(`SELECT count(*) FROM profiles`); err != nil {
			t.Errorf("profiles: %v", err)
		}
		db.Close()
		if got := userVersion(t, path); got != schemaVersion {
			t.Errorf("user_version = %d, want %d", got, schemaVersion)
		}
	}
}

// A database at another schema version is refused and left as it was.
func TestInitDBRefusesAnotherSchemaVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vault.db")
	d, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Exec(`PRAGMA user_version = 4`); err != nil {
		t.Fatal(err)
	}
	d.Close()

	if _, err := InitDB(path); err == nil || !strings.Contains(err.Error(), "schema version 4") {
		t.Fatalf("InitDB = %v, want a refusal naming version 4", err)
	}
	if got := userVersion(t, path); got != 4 {
		t.Errorf("user_version = %d, want 4", got)
	}
}
