package migrations

import (
	"context"
	"database/sql"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

// A migration is frozen: what it does can't depend on live Uno code, so the
// package's own files import the standard library and nothing else. Standard
// library import paths are the ones whose first element has no dot.
func TestMigrationsImportOnlyTheStandardLibrary(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, spec := range parsed.Imports {
			path, _ := strconv.Unquote(spec.Path.Value)
			first, _, _ := strings.Cut(path, "/")
			if strings.Contains(first, ".") {
				t.Errorf("%s imports %s; a migration imports only the standard library", file, path)
			}
		}
	}
}

func TestAllIsNumberedFromOne(t *testing.T) {
	for i, m := range All() {
		if m.Version != i+1 || m.Name == "" || m.Up == nil {
			t.Errorf("All()[%d] = version %d, name %q, Up set %t; want version %d with a name and an Up", i, m.Version, m.Name, m.Up != nil, i+1)
		}
	}
}

// openTestDB opens a fresh database in a temporary directory, after running
// setup on it when setup isn't empty.
func openTestDB(t *testing.T, setup string) *sql.DB {
	t.Helper()
	d, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "vault.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	if setup != "" {
		if _, err := d.Exec(setup); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}
	return d
}

// runBaseline runs baseline in a transaction on d and commits it when it
// succeeds.
func runBaseline(t *testing.T, d *sql.DB) ([]string, error) {
	t.Helper()
	tx, err := d.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	notes, err := baseline(context.Background(), tx)
	if err != nil {
		return notes, err
	}
	return notes, tx.Commit()
}

func tables(t *testing.T, d *sql.DB) []string {
	t.Helper()
	rows, err := d.Query(`SELECT name FROM sqlite_master WHERE type = 'table' ORDER BY name`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		names = append(names, name)
	}
	return names
}

func TestBaselineCreatesTheSchemaOnAnEmptyDatabase(t *testing.T) {
	d := openTestDB(t, "")
	notes, err := runBaseline(t, d)
	if err != nil {
		t.Fatalf("baseline: %v", err)
	}
	if len(notes) != 0 {
		t.Errorf("notes = %q, want none on an empty database", notes)
	}
	want := []string{"catalogs", "collections", "folder_catalogs", "folders", "profiles"}
	if got := tables(t, d); !slices.Equal(got, want) {
		t.Errorf("tables = %q, want %q", got, want)
	}
}

// Prod's tables predate migrations: is_default still sits in catalogs and
// collections, and taken_hash was appended by ALTER TABLE. The baseline adopts
// them as they are.
func TestBaselineAdoptsTheProdShapedFixture(t *testing.T) {
	fixture, err := os.ReadFile("../testdata/schema_v1.sql")
	if err != nil {
		t.Fatal(err)
	}
	d := openTestDB(t, string(fixture))
	notes, err := runBaseline(t, d)
	if err != nil {
		t.Fatalf("baseline: %v", err)
	}
	want := []string{
		"found 5 of 5 baseline tables already present; checked their columns",
		"collections: kept the legacy column is_default",
		"catalogs: kept the legacy column is_default",
	}
	if !slices.Equal(notes, want) {
		t.Errorf("notes = %q, want %q", notes, want)
	}
}

// A table that differs from the baseline fails the migration with every
// difference named: a missing column, a changed one and an unknown one. A
// type differing only in case is the same type, as it is to SQLite.
func TestBaselineRefusesATableOfAnotherShape(t *testing.T) {
	d := openTestDB(t, `CREATE TABLE catalogs (
		id TEXT PRIMARY KEY, type text NOT NULL, name TEXT NOT NULL, provider TEXT NOT NULL,
		params TEXT NOT NULL DEFAULT '', owner_id TEXT NOT NULL, is_public INTEGER NOT NULL DEFAULT 0,
		collection_id TEXT, home_sort_order INTEGER, show_in_home INTEGER NOT NULL DEFAULT 1,
		taken_from TEXT, fingerprint TEXT, created_at TEXT NOT NULL, updated_at TEXT NOT NULL,
		is_default TEXT, extra TEXT
	)`)
	_, err := runBaseline(t, d)
	if err == nil {
		t.Fatal("baseline = nil, want the shape mismatch")
	}
	for _, want := range []string{
		"catalogs: column taken_hash is missing",
		`catalogs: column is "fingerprint TEXT", want "fingerprint TEXT NOT NULL"`,
		`catalogs: unexpected column "extra TEXT"`,
		`catalogs: unexpected column "is_default TEXT"`,
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q", err, want)
		}
	}
	if strings.Contains(err.Error(), "column type") || strings.Count(err.Error(), "\n") != 4 {
		t.Errorf("error %q, want exactly the four problems above", err)
	}
	if got := tables(t, d); !slices.Equal(got, []string{"catalogs"}) {
		t.Errorf("tables after the failed baseline = %q, want only the original catalogs", got)
	}
}
