package vault

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// baselineVersion is the oldest schema version an upgrade starts from: the
// version of the first public release.
const baselineVersion = 12

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

// newDBFile writes a database at path holding ddl and stamped version.
func newDBFile(t *testing.T, path, ddl string, version int) {
	t.Helper()
	d, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if _, err := d.Exec(ddl + fmt.Sprintf("\nPRAGMA user_version = %d;", version)); err != nil {
		t.Fatal(err)
	}
}

// rowsText renders every row query returns, one per line.
func rowsText(t *testing.T, d dbtx, query string, args ...any) string {
	t.Helper()
	rows, err := d.QueryContext(t.Context(), query, args...)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for rows.Next() {
		values := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			t.Fatal(err)
		}
		fmt.Fprintln(&b, values...)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

// schemaShape describes the database's schema: each table's columns, foreign
// keys and UNIQUE or PRIMARY KEY constraints, and each index, trigger and
// view's SQL with its whitespace collapsed. It leaves out column order,
// collations and CHECK constraints, which a migration's ALTER TABLE writes
// differently from schema.sql.
func schemaShape(t *testing.T, d dbtx) string {
	t.Helper()
	var b strings.Builder
	objects := rowsText(t, d, `SELECT type, name FROM sqlite_schema WHERE name NOT LIKE 'sqlite_%' ORDER BY type, name`)
	for line := range strings.Lines(objects) {
		kind, name, _ := strings.Cut(strings.TrimSpace(line), " ")
		fmt.Fprintln(&b, kind, name)
		if kind != "table" {
			var ddl string
			if err := d.QueryRowContext(t.Context(), `SELECT sql FROM sqlite_schema WHERE name = ?`, name).Scan(&ddl); err != nil {
				t.Fatal(err)
			}
			fmt.Fprintln(&b, strings.Join(strings.Fields(ddl), " "))
			continue
		}
		b.WriteString(rowsText(t, d, `SELECT name, type, "notnull", dflt_value, pk, hidden FROM pragma_table_xinfo(?) ORDER BY name`, name))
		b.WriteString(rowsText(t, d, `SELECT id, seq, "table", "from", "to", on_update, on_delete, match FROM pragma_foreign_key_list(?) ORDER BY id, seq`, name))
		b.WriteString(rowsText(t, d, `SELECT l.origin, l."unique", group_concat(i.name, ',') AS cols
			FROM pragma_index_list(?) AS l, pragma_index_info(l.name) AS i
			WHERE l.origin <> 'c' GROUP BY l.name ORDER BY l.origin, cols`, name))
	}
	return b.String()
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

// A database at a version with no upgrade path, or newer than this build, is
// refused and left as it was.
func TestInitDBRefusesAVersionItCannotUpgrade(t *testing.T) {
	for _, version := range []int{4, schemaVersion + 1} {
		path := filepath.Join(t.TempDir(), "vault.db")
		newDBFile(t, path, "", version)

		want := fmt.Sprintf("schema version %d", version)
		if _, err := InitDB(path); err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("InitDB = %v, want a refusal naming version %d", err, version)
		}
		if got := userVersion(t, path); got != version {
			t.Errorf("user_version = %d, want %d", got, version)
		}
	}
}

// Every version from the baseline up has a step to the next, and a frozen
// copy of its schema.sql in testdata for TestUpgradeMatchesAFreshSchema.
func TestEveryVersionFromTheBaselineUpgrades(t *testing.T) {
	for v := baselineVersion; v < schemaVersion; v++ {
		if _, ok := migrations[v]; !ok {
			t.Errorf("no migration from schema version %d", v)
		}
	}
	for v := baselineVersion; v <= schemaVersion; v++ {
		if _, err := os.Stat(filepath.Join("testdata", fmt.Sprintf("schema_v%d.sql", v))); err != nil {
			t.Errorf("no frozen schema for version %d: %v", v, err)
		}
	}
}

// A database holding any earlier release's schema ends up, through InitDB,
// with the same schema as a fresh one. The frozen copy for schemaVersion
// matches schema.sql itself, so a schema change without a version bump fails
// here too.
func TestUpgradeMatchesAFreshSchema(t *testing.T) {
	fresh, err := InitDB(filepath.Join(t.TempDir(), "fresh.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Close()
	want := schemaShape(t, fresh.conn)

	for v := baselineVersion; v <= schemaVersion; v++ {
		t.Run(fmt.Sprintf("v%d", v), func(t *testing.T) {
			ddl, err := os.ReadFile(filepath.Join("testdata", fmt.Sprintf("schema_v%d.sql", v)))
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "vault.db")
			newDBFile(t, path, string(ddl), v)

			db, err := InitDB(path)
			if err != nil {
				t.Fatalf("InitDB: %v", err)
			}
			defer db.Close()
			if got := schemaShape(t, db.conn); got != want {
				t.Errorf("schema after upgrading from version %d:\n%s\nwant:\n%s", v, got, want)
			}
		})
	}
}

// upgradeFromBaseline opens a fresh database and runs steps on it, from
// schemaVersion to schemaVersion+len(steps), on a pool of one connection.
func upgradeFromBaseline(t *testing.T, steps ...string) (*DB, string, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "vault.db")
	db, err := InitDB(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	db.conn.SetMaxOpenConns(1)
	byVersion := map[int]string{}
	for i, step := range steps {
		byVersion[schemaVersion+i] = step
	}
	return db, path, db.upgrade(t.Context(), path, schemaVersion, schemaVersion+len(steps), byVersion)
}

// An upgrade runs every step in order, stamps the last version, leaves a
// backup at the starting version, and turns foreign keys back on.
func TestUpgradeRunsEveryStep(t *testing.T) {
	db, path, err := upgradeFromBaseline(t,
		`CREATE TABLE t (x INTEGER)`,
		`ALTER TABLE t ADD COLUMN y TEXT`,
	)
	if err != nil {
		t.Fatalf("upgrade: %v", err)
	}
	if _, err := db.conn.Exec(`INSERT INTO t (x, y) VALUES (1, 'a')`); err != nil {
		t.Errorf("t after the upgrade: %v", err)
	}
	if got := userVersion(t, path); got != schemaVersion+2 {
		t.Errorf("user_version = %d, want %d", got, schemaVersion+2)
	}
	if got := userVersion(t, fmt.Sprintf("%s.v%d.bak", path, schemaVersion)); got != schemaVersion {
		t.Errorf("backup user_version = %d, want %d", got, schemaVersion)
	}
	var foreignKeys int
	if err := db.conn.QueryRow(`PRAGMA foreign_keys`).Scan(&foreignKeys); err != nil || foreignKeys != 1 {
		t.Errorf("foreign_keys = %d (%v), want 1", foreignKeys, err)
	}
}

// Rebuilding a parent table, as a step that changes a CHECK must, keeps the
// rows that refer to it: the old table's drop runs no ON DELETE action. A
// trigger naming the table is dropped before the rebuild and created after
// it, or the rename fails.
func TestUpgradeRebuildsATableWithoutCascading(t *testing.T) {
	db, _, err := upgradeFromBaseline(t,
		`CREATE TABLE p (id INTEGER PRIMARY KEY, n INTEGER NOT NULL DEFAULT 0);
		 CREATE TABLE c (p INTEGER REFERENCES p (id) ON DELETE CASCADE);
		 CREATE TRIGGER c_count AFTER INSERT ON c BEGIN UPDATE p SET n = n + 1 WHERE id = NEW.p; END;
		 INSERT INTO p (id) VALUES (1); INSERT INTO c VALUES (1);`,
		`DROP TRIGGER c_count;
		 CREATE TABLE p_new (id INTEGER PRIMARY KEY CHECK (id > 0), n INTEGER NOT NULL DEFAULT 0);
		 INSERT INTO p_new SELECT id, n FROM p;
		 DROP TABLE p;
		 ALTER TABLE p_new RENAME TO p;
		 CREATE TRIGGER c_count AFTER INSERT ON c BEGIN UPDATE p SET n = n + 1 WHERE id = NEW.p; END;`,
	)
	if err != nil {
		t.Fatalf("upgrade: %v", err)
	}
	var children int
	if err := db.conn.QueryRow(`SELECT count(*) FROM c`).Scan(&children); err != nil || children != 1 {
		t.Errorf("rows in c = %d (%v), want 1", children, err)
	}
}

// A failing step, or one that leaves a dangling reference, rolls the whole
// upgrade back.
func TestUpgradeRollsBackOnFailure(t *testing.T) {
	cases := map[string][]string{
		"failing step": {`CREATE TABLE t (x INTEGER)`, `INSERT INTO missing VALUES (1)`},
		"dangling reference": {`CREATE TABLE t (x INTEGER);
			CREATE TABLE c (p INTEGER REFERENCES profiles (id));
			INSERT INTO c VALUES (999);`},
	}
	for name, steps := range cases {
		t.Run(name, func(t *testing.T) {
			db, path, err := upgradeFromBaseline(t, steps...)
			if err == nil {
				t.Fatal("upgrade succeeded, want an error")
			}
			if got := userVersion(t, path); got != schemaVersion {
				t.Errorf("user_version = %d, want %d", got, schemaVersion)
			}
			if _, err := db.conn.Exec(`SELECT * FROM t`); err == nil {
				t.Error("table t survived the rollback")
			}
		})
	}
}

// A missing step stops the upgrade before the backup or any change.
func TestUpgradeRefusesAGap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vault.db")
	db, err := InitDB(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	steps := map[int]string{schemaVersion: `CREATE TABLE t (x INTEGER)`}
	err = db.upgrade(t.Context(), path, schemaVersion, schemaVersion+2, steps)
	if err == nil || !strings.Contains(err.Error(), fmt.Sprintf("no upgrade from version %d", schemaVersion+1)) {
		t.Fatalf("upgrade = %v, want a refusal naming version %d", err, schemaVersion+1)
	}
	if _, err := os.Stat(fmt.Sprintf("%s.v%d.bak", path, schemaVersion)); err == nil {
		t.Error("backup written despite the gap")
	}
}

// A backup replaces one left by an earlier attempt.
func TestBackupReplacesAnOldOne(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vault.db")
	db, err := InitDB(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	backup := path + ".bak"
	if err := os.WriteFile(backup, []byte("stale"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := db.backup(t.Context(), backup); err != nil {
		t.Fatalf("backup: %v", err)
	}
	if got := userVersion(t, backup); got != schemaVersion {
		t.Errorf("backup user_version = %d, want %d", got, schemaVersion)
	}
}

// A backup that can't be written fails the upgrade before any step runs.
func TestUpgradeStopsWhenTheBackupFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vault.db")
	db, err := InitDB(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	steps := map[int]string{schemaVersion: `CREATE TABLE t (x INTEGER)`}
	missingDir := filepath.Join(t.TempDir(), "missing", "vault.db")
	if err := db.upgrade(t.Context(), missingDir, schemaVersion, schemaVersion+1, steps); err == nil {
		t.Fatal("upgrade succeeded, want a backup error")
	}
	if got := userVersion(t, path); got != schemaVersion {
		t.Errorf("user_version = %d, want %d", got, schemaVersion)
	}
}
