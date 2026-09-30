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
	want := "Dry run of " + path + ". Nothing was written to it.\n" + `Schema version 0 -> 5

Migration 1 (baseline)
  found 5 of 5 baseline tables already present; checked their columns
  collections: kept the legacy column is_default
  catalogs: kept the legacy column is_default

Migration 2 (recipes)
  stored 9 recipes for 20 catalogs
  rewrote the params of 0 catalogs in canonical form
  remapped catalog links: 3 in step, 1 behind their source, 0 edited since taken, 0 matching neither hash (left as they were)
  remapped collection links: 2 in step, 1 behind their source, 0 edited since taken, 0 matching neither hash (left as they were)

Migration 3 (publications)
  published 2 catalogs and 2 collections: every public row that was not a linked copy, as it stood
  left 2 public linked copies unpublished: each became a subscription or a detached copy
  left 0 public collections unpublished: each uses a catalog that became a subscription, which only its publisher can share
  catalog links: 2 became subscriptions (1 in step, 0 of those only because the copy already equals its source; 1 out of step); detached 2 (1 from a private source, 1 from a copy, 0 from a public source left unpublished)
  collection links: 2 became subscriptions (1 in step, 0 of those only because the copy already equals its source; 1 out of step); detached 1 (0 from a private source, 1 from a copy, 0 from a public source left unpublished)
  dropped is_public, taken_from and taken_hash from catalogs and collections, and the legacy is_default columns

Migration 4 (accounts)
  added accounts, which holds each account's own TMDB key, sealed

Migration 5 (pushed_hash)
  backfilled 2 push hashes; left 4 collections pending (changed since their last push, or never pushed)

Recipe check: 20 catalogs, 0 whose params before and after migrating would fetch different titles

Publication check: 4 publications, 0 that today's validators refuse (migrated as they stood)

Push hashes: 2 backfilled, 4 pending; live builder agrees on 2 of 2

Rows                       before    after
accounts                        -        0
catalogs                       20       20
collections                     6        6
folder_catalogs                18       18
folders                        11       11
profiles                        3        3
publications                    -        4
recipes                         -        9
subscriptions                   -        4
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

// The push-hash check lists every collection the live push payload hashes
// differently under its count.
func TestReportTextListsPushHashMismatches(t *testing.T) {
	got := reportText("vault.db", vault.MigrationReport{
		From: 4, To: 5, PushHashesBackfilled: 2, PushHashesPending: 1,
		PushHashMismatches: []string{"collection c1 (Weekend)"},
		RowsBefore:         map[string]int{}, RowsAfter: map[string]int{},
	})
	want := "Push hashes: 2 backfilled, 1 pending; live builder agrees on 1 of 2\n  collection c1 (Weekend)\n"
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

// The publication check lists every publication today's validators refuse
// under its count.
func TestReportTextListsPublicationProblems(t *testing.T) {
	got := reportText("vault.db", vault.MigrationReport{
		From: 2, To: 3, PublicationsChecked: 3,
		PublicationProblems: []string{"collection p1 (Weekend): invalid input: view mode must be one of the three"},
		RowsBefore:          map[string]int{}, RowsAfter: map[string]int{},
	})
	want := "Publication check: 3 publications, 1 that today's validators refuse (migrated as they stood)\n  collection p1 (Weekend): invalid input: view mode must be one of the three\n"
	if !strings.Contains(got, want) {
		t.Errorf("report:\n%s\nwant it to contain:\n%s", got, want)
	}
}
