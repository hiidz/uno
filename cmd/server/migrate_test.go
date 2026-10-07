package main

import (
	"bytes"
	"context"
	"database/sql"
	_ "embed"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/hiidz/uno/internal/vault"
)

//go:embed testdata/schema_v8.sql
var schemaV8 string

// v8Fixture is a v8 database holding a publisher's collection publication and
// a subscriber's copy of it, whose folder and scoped catalog carry snapshot
// keys; a listed catalog copy released when its publication ended; a
// FOLLOW_LAYOUT collection with a folder of no tile shape; and two catalogs
// sharing one recipe.
type v8Fixture struct {
	path                                     string
	publisher, subscriber                    uuid.UUID
	source, copyCollection, followLayout     uuid.UUID
	sourceScoped, copyScoped, released, twin uuid.UUID
	copyFolder, emptyFolder                  uuid.UUID
	publication, twinPublication, twinCopy   uuid.UUID
}

func newV8Fixture(t *testing.T, snapshotViewMode string) v8Fixture {
	t.Helper()
	f := v8Fixture{path: filepath.Join(t.TempDir(), "vault.db")}
	for _, id := range []*uuid.UUID{&f.publisher, &f.subscriber, &f.source, &f.copyCollection, &f.followLayout,
		&f.sourceScoped, &f.copyScoped, &f.released, &f.twin, &f.copyFolder, &f.emptyFolder, &f.publication, &f.twinPublication, &f.twinCopy} {
		*id = uuid.New()
	}
	movie := vault.RecipeHash("movie", "tmdb", `{"sort_by":"popularity.desc"}`)
	series := vault.RecipeHash("series", "tmdb", `{}`)
	snapshot := `{"format":"uno-publication","version":1,"catalogs":[],"collection":{"title":"Source","view_mode":"` +
		snapshotViewMode + `","folders":[{"key":"fk","title":"F","tile_shape":"POSTER","refs":[]}]}}`
	raw := openRaw(t, f.path)
	const now = "2026-10-01T00:00:00Z"
	steps := []struct {
		query string
		args  []any
	}{
		{schemaV8 + "\nPRAGMA user_version = 8;", nil},
		{`INSERT INTO profiles VALUES (?, 'tp', 'user', 1, 'np1'), (?, 'ts', 'user', 2, 'np2')`, []any{f.publisher, f.subscriber}},
		{`INSERT INTO recipes VALUES (?, 'movie', 'tmdb', '{"sort_by":"popularity.desc"}', ?), (?, 'series', 'tmdb', '{}', ?)`,
			[]any{movie, now, series, now}},
		{`INSERT INTO collections (id, title, owner_id, view_mode, home_sort_order, created_at, updated_at, unpublished_at) VALUES
		  (?, 'Source', ?, 'ROWS', NULL, ?, ?, NULL), (?, 'Copy', ?, 'ROWS', 0, ?, ?, NULL), (?, 'Follow', ?, 'FOLLOW_LAYOUT', NULL, ?, ?, ?)`,
			[]any{f.source, f.publisher, now, now, f.copyCollection, f.subscriber, now, now, f.followLayout, f.subscriber, now, now, now}},
		{`INSERT INTO catalogs (id, name, recipe_hash, owner_id, collection_id, home_sort_order, sub_key, created_at, updated_at, unpublished_at) VALUES
		  (?, 'Scoped', ?, ?, ?, NULL, NULL, ?, ?, NULL),
		  (?, 'Scoped copy', ?, ?, ?, NULL, 'ck', ?, ?, NULL),
		  (?, 'Released', ?, ?, NULL, 1, NULL, ?, ?, ?),
		  (?, 'Twin', ?, ?, NULL, NULL, NULL, ?, ?, NULL),
		  (?, 'Twin copy', ?, ?, NULL, NULL, NULL, ?, ?, NULL)`,
			[]any{f.sourceScoped, movie, f.publisher, f.source, now, now, f.copyScoped, movie, f.subscriber, f.copyCollection, now, now,
				f.released, series, f.subscriber, now, now, now, f.twin, movie, f.publisher, now, now,
				f.twinCopy, movie, f.subscriber, now, now}},
		{`INSERT INTO folders (id, collection_id, title, sort_order, tile_shape, sub_key) VALUES (?, ?, 'F', 0, 'SQUARE', 'fk'), (?, ?, 'E', 0, '', NULL)`,
			[]any{f.copyFolder, f.copyCollection, f.emptyFolder, f.followLayout}},
		{`INSERT INTO folder_catalogs VALUES (?, ?, 0, ''), (?, ?, 1, 'Horror')`, []any{f.copyFolder, f.copyScoped, f.copyFolder, f.copyScoped}},
		{`INSERT INTO publications (id, publisher_id, kind, catalog_id, collection_id, title, snapshot, content_hash,
		                          catalog_count, folder_count, published_at, updated_at) VALUES
		  (?, ?, 'collection', NULL, ?, 'Source', ?, 'h1', 1, 1, ?, ?),
		  (?, ?, 'catalog', ?, NULL, 'Twin', '{}', 'h2', 1, 0, ?, ?)`,
			[]any{f.publication, f.publisher, f.source, snapshot, now, now, f.twinPublication, f.publisher, f.twin, now, now}},
		{`INSERT INTO subscriptions VALUES (?, ?, ?, NULL, ?, 'h1', ?), (?, ?, ?, ?, NULL, 'h2', ?)`,
			[]any{uuid.New(), f.subscriber, f.publication, f.copyCollection, now, uuid.New(), f.subscriber, f.twinPublication, f.twinCopy, now}},
	}
	for _, step := range steps {
		if _, err := raw.Exec(step.query, step.args...); err != nil {
			t.Fatalf("seeding: %v\n%s", err, step.query)
		}
	}
	return f
}

// openRaw opens the database at path without the vault, closed with the test.
func openRaw(t *testing.T, path string) *sql.DB {
	t.Helper()
	raw, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = raw.Close() })
	return raw
}

// queryOne is the single value query reads from the database at path.
func queryOne[T any](t *testing.T, path, query string, args ...any) T {
	t.Helper()
	var v T
	if err := openRaw(t, path).QueryRow(query, args...).Scan(&v); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return v
}

// The migration moves each catalog's recipe onto its row, gives the
// FOLLOW_LAYOUT collection TABBED_GRID and the empty tile shape POSTER, keeps
// every row's release mark, snapshot keys, subscription and Home place, and
// stamps version 9, which the vault then opens and reads.
func TestMigrateToV9(t *testing.T) {
	ctx := context.Background()
	f := newV8Fixture(t, "ROWS")

	var out bytes.Buffer
	if err := runCommand(ctx, []string{"migrate", "--db", f.path}, &out); err != nil {
		t.Fatalf("migrate: %v\n%s", err, out.String())
	}
	for _, want := range []string{
		"collections whose view mode becomes TABBED_GRID: 1",
		"folders whose empty tile shape becomes POSTER: 1",
		"catalogs                5        5",
		"folder_catalogs         2        2",
		"publications            2        2",
		"subscriptions           2        2",
		"recipes                 2        -",
		"migrated to schema version 9",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output lacks %q:\n%s", want, out.String())
		}
	}
	if got := queryOne[int](t, f.path, `PRAGMA user_version`); got != 9 {
		t.Errorf("user_version = %d, want 9", got)
	}
	if got := queryOne[string](t, f.path, `SELECT type || ' ' || params FROM catalogs WHERE id = ?`, f.released); got != "series {}" {
		t.Errorf("released catalog's recipe = %q, want series {}", got)
	}
	if got := queryOne[string](t, f.path, `SELECT view_mode FROM collections WHERE id = ?`, f.followLayout); got != "TABBED_GRID" {
		t.Errorf("FOLLOW_LAYOUT collection's view mode = %q, want TABBED_GRID", got)
	}
	if got := queryOne[string](t, f.path, `SELECT group_concat(tile_shape, ',') FROM (SELECT tile_shape FROM folders ORDER BY title)`); got != "POSTER,SQUARE" {
		t.Errorf("tile shapes = %q, want the empty one POSTER and the other kept", got)
	}

	db, err := vault.InitDB(f.path)
	if err != nil {
		t.Fatalf("opening the migrated vault: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	released, err := db.ReleasedCopies(ctx, f.subscriber)
	if err != nil {
		t.Fatal(err)
	}
	if len(released) != 2 || released[0].ID != f.released || released[1].ID != f.followLayout {
		t.Errorf("released copies = %+v, want the two marked before, and only those", released)
	}
	catalogs, err := db.GetUserCatalogs(ctx, f.subscriber)
	if err != nil || len(catalogs) != 2 {
		t.Fatalf("subscriber's catalogs = %+v, %v; want the released one and the live copy", catalogs, err)
	}
	for _, c := range catalogs {
		switch c.ID {
		case f.released:
			if c.RecipeHash != vault.RecipeHash("series", "tmdb", "{}") || c.HomeSortOrder == nil || !c.PublisherUnpublished {
				t.Errorf("released catalog = %+v; want its recipe, Home place and mark kept", c)
			}
		case f.twinCopy:
			if c.Subscription == nil || c.Subscription.PublicationID != f.twinPublication || c.PublisherUnpublished {
				t.Errorf("live copy = %+v; want still subscribed and not released", c)
			}
		}
	}
	if got := queryOne[string](t, f.path, `SELECT f.sub_key || ' ' || c.sub_key FROM folders f, catalogs c WHERE f.id = ? AND c.id = ?`,
		f.copyFolder, f.copyScoped); got != "fk ck" {
		t.Errorf("copy's snapshot keys = %q, want fk ck", got)
	}
}

// --report prints the findings and changes nothing.
func TestMigrateReportChangesNothing(t *testing.T) {
	f := newV8Fixture(t, "ROWS")
	var out bytes.Buffer
	if err := runCommand(context.Background(), []string{"migrate", "--db", f.path, "--report"}, &out); err != nil {
		t.Fatalf("migrate --report: %v", err)
	}
	if !strings.Contains(out.String(), "collections whose view mode becomes TABBED_GRID: 1\n  "+f.followLayout.String()) {
		t.Errorf("output = %q, want the FOLLOW_LAYOUT collection listed", out.String())
	}
	if got := queryOne[int](t, f.path, `PRAGMA user_version`); got != 8 {
		t.Errorf("user_version = %d, want 8 still", got)
	}
}

// A publication whose snapshot holds a view mode version 9 refuses stops the
// migration before it changes anything, and so does a database at any version
// but 8.
func TestMigrateRefuses(t *testing.T) {
	f := newV8Fixture(t, "FOLLOW_LAYOUT")
	var out bytes.Buffer
	err := runCommand(context.Background(), []string{"migrate", "--db", f.path}, &out)
	if err == nil || !strings.Contains(err.Error(), "refusing to migrate: 1 rows") {
		t.Fatalf("migrate = %v, want refused", err)
	}
	if !strings.Contains(out.String(), "publications whose snapshot holds a view mode or tile shape version 9 refuses: 1") {
		t.Errorf("output = %q, want the publication listed", out.String())
	}
	if got := queryOne[int](t, f.path, `PRAGMA user_version`); got != 8 {
		t.Errorf("user_version = %d, want 8 still", got)
	}
	if err := runCommand(context.Background(), []string{"migrate", "--db", f.path}, &out); err == nil {
		t.Error("a second refused run succeeded")
	}

	fresh := filepath.Join(t.TempDir(), "fresh.db")
	db, err := vault.InitDB(fresh)
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	if err := runCommand(context.Background(), []string{"migrate", "--db", fresh}, &out); err == nil || !strings.Contains(err.Error(), "schema version 9") {
		t.Errorf("migrate of a v9 database = %v, want refused", err)
	}
	for _, args := range [][]string{{"migrate"}, {"migrate", "--db", filepath.Join(t.TempDir(), "none.db")}} {
		if err := runCommand(context.Background(), args, &out); err == nil {
			t.Errorf("runCommand(%v) succeeded", args)
		}
	}
}
