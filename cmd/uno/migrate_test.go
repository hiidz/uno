package main

import (
	"bytes"
	"context"
	"database/sql"
	_ "embed"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hiidz/uno/internal/vault"
)

//go:embed testdata/schema_v10.sql
var schemaV10 string

// v10Rows is one row in every table a publication touches: a profile, a
// catalog it publishes, the publication with its counts, and a subscriber's
// copy and subscription.
const v10Rows = `
INSERT INTO profiles VALUES ('p1', 'tok1', 'u1', 1, 'n1'), ('p2', 'tok2', 'u2', 1, 'n2');
INSERT INTO catalogs (id, name, type, provider, params, owner_id, created_at, updated_at) VALUES
  ('c1', 'Popular', 'movie', 'tmdb', '{}', 'p1', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z'),
  ('c2', 'Popular (copy)', 'movie', 'tmdb', '{}', 'p2', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z');
INSERT INTO publications (id, publisher_id, kind, catalog_id, title, snapshot, content_hash, catalog_count, folder_count, published_at, updated_at)
  VALUES ('pub1', 'p1', 'catalog', 'c1', 'Popular', '{}', 'h', 1, 0, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z');
INSERT INTO subscriptions (id, subscriber_id, publication_id, catalog_id, subscribed_hash, created_at)
  VALUES ('s1', 'p2', 'pub1', 'c2', 'h', '2026-01-01T00:00:00Z');
`

// newV10Database is a v10 database at a new path, made from the v10 schema and
// holding v10Rows.
func newV10Database(t *testing.T, extra string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "vault.db")
	raw, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = raw.Close() }()
	if _, err := raw.Exec(schemaV10 + "\n" + v10Rows + extra + "\nPRAGMA user_version = 10;"); err != nil {
		t.Fatal(err)
	}
	return path
}

// count is how many rows query returns, read from the database at path.
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

// A v10 database migrates to one this build opens, with its rows, the new index
// and none of the dropped columns; a second run is refused.
func TestMigrateToV11(t *testing.T) {
	ctx := context.Background()
	path := newV10Database(t, "")
	var out bytes.Buffer
	if err := runCommand(ctx, []string{"migrate", "--db", path}, &out); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if !strings.Contains(out.String(), "schema version 11") {
		t.Errorf("output = %q, want it to name version 11", out.String())
	}
	for query, want := range map[string]int{
		`SELECT count(*) FROM publications`:         1,
		`SELECT count(*) FROM subscriptions`:        1,
		`SELECT count(*) FROM catalogs`:             2,
		`SELECT subscriber_count FROM publications`: 1,
		`SELECT count(*) FROM pragma_table_info('publications') WHERE name LIKE '%\_count' ESCAPE '\'`: 1,
		`SELECT count(*) FROM sqlite_master WHERE name = 'subscriptions_by_publication'`:               1,
		`SELECT count(*) FROM pragma_table_info('catalogs') WHERE name = 'unpublished_at'`:             0,
		`SELECT count(*) FROM pragma_table_info('collections') WHERE name = 'unpublished_at'`:          0,
		`SELECT count(*) FROM pragma_foreign_key_check`:                                                0,
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
	if err := runMigrate(ctx, []string{"--db", path}, &out); err == nil || !strings.Contains(err.Error(), "version 11") {
		t.Errorf("a second migrate = %v, want it refused at version 11", err)
	}
}

// A database whose structure isn't a v10 one's is refused and left at version
// 10, its new index not made.
func TestMigrateRefusesAnotherStructure(t *testing.T) {
	ctx := context.Background()
	path := newV10Database(t, "CREATE INDEX stray ON catalogs (name);")
	var out bytes.Buffer
	err := runMigrate(ctx, []string{"--db", path}, &out)
	if err == nil || !strings.Contains(err.Error(), "differs from a fresh v11") {
		t.Fatalf("migrate = %v, want it refused for its structure", err)
	}
	for query, want := range map[string]int{
		`SELECT user_version FROM pragma_user_version`:                                        10,
		`SELECT count(*) FROM sqlite_master WHERE name = 'subscriptions_by_publication'`:      0,
		`SELECT count(*) FROM pragma_table_info('publications') WHERE name = 'catalog_count'`: 1,
	} {
		if got := count(t, path, query); got != want {
			t.Errorf("%s = %d, want %d", query, got, want)
		}
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

// migrate needs --db naming a file that is there.
func TestMigrateRefusesItsArguments(t *testing.T) {
	ctx := context.Background()
	var out bytes.Buffer
	if err := runMigrate(ctx, nil, &out); err == nil {
		t.Error("migrate with no --db succeeded")
	}
	if err := runMigrate(ctx, []string{"--db", filepath.Join(t.TempDir(), "none.db")}, &out); err == nil {
		t.Error("migrate of a missing file succeeded")
	}
}
