package main

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/hiidz/uno/internal/vault"
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
	want := "Dry run of " + path + ". Nothing was written to it.\n" + `Schema version 0 -> 2

Migration 1 (baseline)
  found 5 of 5 baseline tables already present; checked their columns
  collections: kept the legacy column is_default
  catalogs: kept the legacy column is_default

Migration 2 (recipes)
  stored 9 recipes for 20 catalogs
  rewrote the params of 0 catalogs in canonical form
  remapped catalog links: 3 in step, 1 behind their source, 0 edited since taken, 0 matching neither hash (left as they were)
  remapped collection links: 2 in step, 1 behind their source, 0 edited since taken, 0 matching neither hash (left as they were)

Recipe check: 20 catalogs, 0 whose params before and after migrating would fetch different titles

Rows                       before    after
catalogs                       20       20
collections                     6        6
folder_catalogs                18       18
folders                        11       11
profiles                        3        3
recipes                         -        9
`
	if out.String() != want {
		t.Errorf("output:\n%s\nwant:\n%s", out.String(), want)
	}
}

// The recipe check lists every mismatch it found under its count.
func TestReportTextListsRecipeMismatches(t *testing.T) {
	got := reportText("vault.db", vault.MigrationReport{
		From: 1, To: 2, RecipesChecked: 2,
		RecipeMismatches: []string{"catalog c1 (Popular): discover query differs"},
		RowsBefore:       map[string]int{}, RowsAfter: map[string]int{},
	})
	want := "Recipe check: 2 catalogs, 1 whose params before and after migrating would fetch different titles\n  catalog c1 (Popular): discover query differs\n"
	if !strings.Contains(got, want) {
		t.Errorf("report:\n%s\nwant it to contain:\n%s", got, want)
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
