package main

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

// prodDB writes the vault's prod-shaped fixture to a fresh database file and
// returns its path.
func prodDB(t *testing.T) string {
	t.Helper()
	fixture, err := os.ReadFile("../../internal/vault/testdata/schema_v1.sql")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "vault.db")
	d, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if _, err := d.Exec(string(fixture)); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestMigrateCommandPrintsTheDryRun(t *testing.T) {
	path := prodDB(t)
	var out strings.Builder
	if err := command([]string{"migrate", "--dry-run", "--db", path}, &out); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	want := "Dry run of " + path + ". Nothing was written to it.\n" + `Schema version 0 -> 1

Migration 1 (baseline)
  found 5 of 5 baseline tables already present; checked their columns
  collections: kept the legacy column is_default
  catalogs: kept the legacy column is_default

Rows                       before    after
catalogs                       20       20
collections                     6        6
folder_catalogs                18       18
folders                        11       11
profiles                        3        3
`
	if out.String() != want {
		t.Errorf("output:\n%s\nwant:\n%s", out.String(), want)
	}
}

// A failing migration still prints what the dry run read, ahead of the
// error.
func TestMigrateCommandPrintsThePartialReportOfAFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vault.db")
	d, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Exec(`CREATE TABLE catalogs (id TEXT PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	d.Close()

	var out strings.Builder
	err = runMigrate([]string{"--dry-run", "--db", path}, &out)
	if err == nil || !strings.Contains(err.Error(), "migration 1 (baseline): ") {
		t.Fatalf("migrate = %v, want migration 1's failure", err)
	}
	want := "Dry run of " + path + ". Nothing was written to it.\n" + `Schema version 0 -> 0

Rows                       before    after
catalogs                        0        0
`
	if out.String() != want {
		t.Errorf("output:\n%s\nwant:\n%s", out.String(), want)
	}
}

func TestMigrateCommandRefusals(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{name: "no --dry-run", args: []string{"--db", "vault.db"}, want: "usage: uno migrate --dry-run --db <path>"},
		{name: "no --db", args: []string{"--dry-run"}, want: "usage: uno migrate --dry-run --db <path>"},
		{name: "unknown flag", args: []string{"--apply"}, want: "flag provided but not defined: -apply"},
		{name: "missing database", args: []string{"--dry-run", "--db", filepath.Join(t.TempDir(), "absent.db")}, want: "reading the database"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out strings.Builder
			err := runMigrate(tc.args, &out)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("runMigrate(%q) = %v, want an error containing %q", tc.args, err, tc.want)
			}
			if out.Len() != 0 {
				t.Errorf("printed %q, want nothing", out.String())
			}
		})
	}
}

func TestCountCellMarksAnAbsentTable(t *testing.T) {
	if got := countCell(map[string]int{"catalogs": 2}, "recipes"); got != "-" {
		t.Errorf("countCell of an absent table = %q, want -", got)
	}
}
