package main

import (
	"bytes"
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hiidz/uno/internal/vault"
)

// schemaV11 is prod's version 11: schema.sql as it was when prod took
// version 11, still holding the release mark (unpublished_at) and the release
// trigger that sets it.
//
//go:embed testdata/schema_v11.sql
var schemaV11 string

// schemaV11Fresh is the version 11 schema.sql made once the release mark was
// gone, which the migration refuses.
//
//go:embed testdata/schema_v11_fresh.sql
var schemaV11Fresh string

// releaseMark sets the release mark on the subscriber's copy, as prod's
// release trigger did when a publication ended.
const releaseMark = `UPDATE catalogs SET unpublished_at = '2026-01-02T00:00:00Z' WHERE id = 'c2';`

// v11Rows is a row in every table: two profiles, a collection with a folder
// holding a catalog scoped to it, a listed catalog published and its
// subscriber's copy, an account's key and a push record.
const v11Rows = `
INSERT INTO profiles VALUES ('p1', 'tok1', 'u1', 1, 'n1'), ('p2', 'tok2', 'u2', 1, 'n2');
INSERT INTO collections (id, title, owner_id, created_at, updated_at)
  VALUES ('col1', 'Studios', 'p1', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z');
INSERT INTO catalogs (id, name, type, provider, params, owner_id, collection_id, created_at, updated_at) VALUES
  ('c1', 'Popular', 'movie', 'tmdb', '{}', 'p1', NULL, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z'),
  ('c2', 'Popular (copy)', 'movie', 'tmdb', '{}', 'p2', NULL, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z'),
  ('c3', 'Pixar', 'movie', 'tmdb', '{}', 'p1', 'col1', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z');
INSERT INTO folders (id, collection_id, title, sort_order, tile_shape) VALUES ('f1', 'col1', 'Pixar', 0, 'POSTER');
INSERT INTO folder_catalogs (folder_id, catalog_id, sort_order) VALUES ('f1', 'c3', 0);
INSERT INTO publications (id, publisher_id, kind, catalog_id, title, snapshot, content_hash, published_at, updated_at)
  VALUES ('pub1', 'p1', 'catalog', 'c1', 'Popular', '{}', 'h', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z');
INSERT INTO subscriptions (id, subscriber_id, publication_id, catalog_id, subscribed_hash, created_at)
  VALUES ('s1', 'p2', 'pub1', 'c2', 'h', '2026-01-01T00:00:00Z');
INSERT INTO accounts VALUES ('u1', x'0102', 'abcd', '2026-01-01T00:00:00Z');
INSERT INTO push_records VALUES ('p1', 'n1', '{}', '2026-01-01T00:00:00Z');
`

// v11Tables are the tables a v11 database and a v12 one share, with how many
// rows v11Rows puts in each.
var v11Tables = map[string]int{
	"profiles": 2, "collections": 1, "catalogs": 3, "folders": 1, "folder_catalogs": 1,
	"publications": 1, "subscriptions": 1, "push_records": 1,
}

// newV11Database is a database at a new path, made from schema plus extra,
// holding v11Rows and stamped version.
func newV11Database(t *testing.T, schema, extra string, version int) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "vault.db")
	raw, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = raw.Close() }()
	if _, err := raw.Exec(schema + "\n" + v11Rows + extra + fmt.Sprintf("\nPRAGMA user_version = %d;", version)); err != nil {
		t.Fatal(err)
	}
	return path
}

// count is the one number query returns, read from the database at path.
func count(t *testing.T, path, query string) int {
	t.Helper()
	raw, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = raw.Close() }()
	var n int
	if err := raw.QueryRow(query).Scan(&n); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return n
}

// structureAt is the structure of the database at path, as the migration
// compares it.
func structureAt(t *testing.T, path string) []string {
	t.Helper()
	raw, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = raw.Close() }()
	lines, err := structureOf(context.Background(), raw)
	if err != nil {
		t.Fatal(err)
	}
	return lines
}

// Prod's v11 database migrates to one this build opens, with a fresh v12
// database's structure, the release mark and its trigger gone, every row of
// every other table kept, every revision 1 and no account key; ending a
// publication then runs the new release trigger, and a second run is refused.
func TestMigrateToV12(t *testing.T) {
	ctx := context.Background()
	path := newV11Database(t, schemaV11, releaseMark, 11)
	var out bytes.Buffer
	if err := runCommand(ctx, []string{"migrate", "--db", path}, &out); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if !strings.Contains(out.String(), "schema version 12") {
		t.Errorf("output = %q, want it to name version 12", out.String())
	}
	fresh, err := freshStructure(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got := structureAt(t, path); strings.Join(got, "\n") != strings.Join(fresh, "\n") {
		t.Errorf("structure after migrating:\n%s\nwant a fresh v12 database's:\n%s", strings.Join(got, "\n"), strings.Join(fresh, "\n"))
	}
	for table, want := range v11Tables {
		if got := count(t, path, `SELECT count(*) FROM `+table); got != want {
			t.Errorf("%s holds %d rows, want %d", table, got, want)
		}
	}
	for query, want := range map[string]int{
		`SELECT user_version FROM pragma_user_version`:                                        12,
		`SELECT count(*) FROM catalogs WHERE revision = 1`:                                    3,
		`SELECT count(*) FROM collections WHERE revision = 1`:                                 1,
		`SELECT count(*) FROM profiles WHERE home_revision = 1`:                               2,
		`SELECT count(*) FROM account_keys`:                                                   0,
		`SELECT count(*) FROM sqlite_master WHERE name = 'accounts'`:                          0,
		`SELECT subscriber_count FROM publications`:                                           1,
		`SELECT count(*) FROM pragma_foreign_key_check`:                                       0,
		`SELECT count(*) FROM sqlite_master WHERE name = 'subscriptions_count_on_insert'`:     1,
		`SELECT count(*) FROM pragma_table_info('catalogs') WHERE name = 'unpublished_at'`:    0,
		`SELECT count(*) FROM pragma_table_info('collections') WHERE name = 'unpublished_at'`: 0,
		`SELECT count(*) FROM sqlite_master WHERE sql LIKE '%unpublished_at%'`:                0,
	} {
		if got := count(t, path, query); got != want {
			t.Errorf("%s = %d, want %d", query, got, want)
		}
	}
	db, err := vault.InitDB(path)
	if err != nil {
		t.Fatalf("InitDB after migrating: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	endPublication(t, path)
	if got := count(t, path, `SELECT count(*) FROM subscriptions`); got != 0 {
		t.Errorf("subscriptions after the publication ended = %d, want 0", got)
	}
	if err := runMigrate(ctx, []string{"--db", path}, &out); err == nil || !strings.Contains(err.Error(), "version 12") {
		t.Errorf("a second migrate = %v, want it refused at version 12", err)
	}
}

// endPublication deletes the publication v11Rows holds, as unpublishing does,
// with foreign keys on as the vault runs: the release trigger and the cascade
// to its subscription run.
func endPublication(t *testing.T, path string) {
	t.Helper()
	raw, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(on)")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = raw.Close() }()
	if _, err := raw.Exec(`DELETE FROM publications WHERE id = 'pub1'`); err != nil {
		t.Fatalf("ending the publication: %v", err)
	}
}

// A database at any version but 11, or whose structure isn't prod's v11, is
// refused and left byte for byte as it was: a v11 without the release mark
// fails as the migration drops it.
func TestMigrateRefusesAndLeavesTheFile(t *testing.T) {
	ctx := context.Background()
	fresh := filepath.Join(t.TempDir(), "fresh.db")
	db, err := vault.InitDB(fresh)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, path, refusal string
	}{
		{"version 10", newV11Database(t, schemaV11, "", 10), "schema version 10"},
		{"version 12", fresh, "schema version 12"},
		{"a v11 without the release mark", newV11Database(t, schemaV11Fresh, "", 11), "applying the changes"},
		{"another structure", newV11Database(t, schemaV11, "CREATE INDEX stray ON catalogs (name);", 11), "differs from a fresh v12"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before, err := os.ReadFile(tc.path)
			if err != nil {
				t.Fatal(err)
			}
			var out bytes.Buffer
			if err := runMigrate(ctx, []string{"--db", tc.path}, &out); err == nil || !strings.Contains(err.Error(), tc.refusal) {
				t.Fatalf("migrate = %v, want it refused with %q", err, tc.refusal)
			}
			after, err := os.ReadFile(tc.path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, after) {
				t.Error("the refused database's file changed")
			}
		})
	}
}

// A path that is no database file, a directory, is refused when the
// transaction starts.
func TestMigrateRefusesADirectory(t *testing.T) {
	var out bytes.Buffer
	if err := runMigrate(context.Background(), []string{"--db", t.TempDir()}, &out); err == nil {
		t.Error("migrate of a directory succeeded")
	}
}

// migrate needs --db naming a file that is there, and no flag it doesn't
// know.
func TestMigrateRefusesItsArguments(t *testing.T) {
	ctx := context.Background()
	var out bytes.Buffer
	if err := runMigrate(ctx, nil, &out); err == nil {
		t.Error("migrate with no --db succeeded")
	}
	if err := runMigrate(ctx, []string{"--db", filepath.Join(t.TempDir(), "none.db")}, &out); err == nil {
		t.Error("migrate of a missing file succeeded")
	}
	if err := runMigrate(ctx, []string{"--nope"}, &out); err == nil {
		t.Error("migrate with an unknown flag succeeded")
	}
}
