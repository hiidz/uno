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

//go:embed testdata/schema_v7.sql
var schemaV7 string

// v7Fixture is a v7 database holding a publisher's three publications and a
// subscriber's copy of each: a live catalog publication whose stored
// subscriber_count is wrong, an unpublished collection publication whose
// copy's folder and scoped catalog carry snapshot keys, and an unpublished
// catalog publication whose source was deleted.
type v7Fixture struct {
	path                                string
	publisher, subscriber               uuid.UUID
	liveSource                          uuid.UUID
	livePub, unpublishedPub, sourceGone uuid.UUID
	liveCopy, collectionCopy, goneCopy  uuid.UUID
	copyFolder, copyScoped              uuid.UUID
}

func newV7Fixture(t *testing.T) v7Fixture {
	t.Helper()
	f := v7Fixture{path: filepath.Join(t.TempDir(), "vault.db")}
	ids := []*uuid.UUID{&f.publisher, &f.subscriber, &f.liveSource, &f.livePub, &f.unpublishedPub, &f.sourceGone,
		&f.liveCopy, &f.collectionCopy, &f.goneCopy, &f.copyFolder, &f.copyScoped}
	for _, id := range ids {
		*id = uuid.New()
	}
	sourceCollection := uuid.New()
	raw := openRaw(t, f.path)
	const now = "2026-10-01T00:00:00Z"
	steps := []struct {
		query string
		args  []any
	}{
		{schemaV7 + "\nPRAGMA user_version = 7;", nil},
		{`INSERT INTO profiles VALUES (?, 'tp', 'user', 1, 'np1'), (?, 'ts', 'user', 2, 'np2')`, []any{f.publisher, f.subscriber}},
		{`INSERT INTO recipes VALUES ('r1', 'movie', 'tmdb', '{}', ?)`, []any{now}},
		{`INSERT INTO collections (id, title, owner_id, created_at, updated_at) VALUES (?, 'Source', ?, ?, ?), (?, 'Copy', ?, ?, ?)`,
			[]any{sourceCollection, f.publisher, now, now, f.collectionCopy, f.subscriber, now, now}},
		{`INSERT INTO catalogs (id, name, recipe_hash, owner_id, collection_id, sub_key, created_at, updated_at) VALUES
		  (?, 'Live', 'r1', ?, NULL, NULL, ?, ?),
		  (?, 'Live copy', 'r1', ?, NULL, NULL, ?, ?),
		  (?, 'Gone copy', 'r1', ?, NULL, NULL, ?, ?),
		  (?, 'Scoped', 'r1', ?, ?, 'ck', ?, ?)`,
			[]any{f.liveSource, f.publisher, now, now, f.liveCopy, f.subscriber, now, now, f.goneCopy, f.subscriber, now, now,
				f.copyScoped, f.subscriber, f.collectionCopy, now, now}},
		{`INSERT INTO folders (id, collection_id, title, sort_order, sub_key) VALUES (?, ?, 'F', 0, 'fk')`, []any{f.copyFolder, f.collectionCopy}},
		{`INSERT INTO folder_catalogs VALUES (?, ?, 0, '')`, []any{f.copyFolder, f.copyScoped}},
		{`INSERT INTO publications (id, publisher_id, kind, catalog_id, collection_id, title, snapshot, content_hash,
		                          catalog_count, folder_count, status, published_at, updated_at) VALUES
		  (?, ?, 'catalog', ?, NULL, 'Live', '{}', 'h1', 1, 0, 'live', ?, ?),
		  (?, ?, 'collection', NULL, ?, 'Source', '{}', 'h2', 1, 1, 'unpublished', ?, ?),
		  (?, ?, 'catalog', NULL, NULL, 'Gone', '{}', 'h3', 1, 0, 'unpublished', ?, ?)`,
			[]any{f.livePub, f.publisher, f.liveSource, now, now, f.unpublishedPub, f.publisher, sourceCollection, now, now,
				f.sourceGone, f.publisher, now, now}},
		{`INSERT INTO subscriptions VALUES
		  (?, ?, ?, ?, NULL, 'h1', ?), (?, ?, ?, NULL, ?, 'h2', ?), (?, ?, ?, ?, NULL, 'h3', ?)`,
			[]any{uuid.New(), f.subscriber, f.livePub, f.liveCopy, now, uuid.New(), f.subscriber, f.unpublishedPub, f.collectionCopy, now,
				uuid.New(), f.subscriber, f.sourceGone, f.goneCopy, now}},
		{`UPDATE publications SET subscriber_count = 5 WHERE id = ?`, []any{f.livePub}},
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

// The migration deletes the unpublished publications and releases their
// copies — marked unpublished, no snapshot keys left — keeps the live one
// with its subscriber counted afresh, and stamps version 8, which the vault
// then opens with one-way unpublish working.
func TestMigrateToV8(t *testing.T) {
	ctx := context.Background()
	f := newV7Fixture(t)

	var out bytes.Buffer
	if err := runCommand(ctx, []string{"migrate", "--db", f.path}, &out); err != nil {
		t.Fatalf("migrate: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "migrated to schema version 8: 2 unpublished publications deleted, their 2 subscribed copies released") {
		t.Errorf("output = %q, want the version and the counts", out.String())
	}
	if got := queryOne[string](t, f.path, `SELECT group_concat(id) FROM publications`); got != f.livePub.String() {
		t.Errorf("publications = %s, want only the live one", got)
	}
	if got := queryOne[int](t, f.path, `SELECT subscriber_count FROM publications`); got != 1 {
		t.Errorf("subscriber_count = %d, want 1, counted afresh", got)
	}
	if got := queryOne[int](t, f.path, `SELECT count(*) FROM subscriptions`); got != 1 {
		t.Errorf("subscriptions = %d, want the live one's only", got)
	}
	released := queryOne[int](t, f.path, `
		SELECT (SELECT count(*) FROM catalogs WHERE unpublished_at IS NOT NULL AND id = ?)
		     + (SELECT count(*) FROM collections WHERE unpublished_at IS NOT NULL AND id = ?)
		     + (SELECT count(*) FROM catalogs WHERE unpublished_at IS NULL AND id = ?)`, f.goneCopy, f.collectionCopy, f.liveCopy)
	if released != 3 {
		t.Errorf("released marks: %d of 3 rows as expected (both ended copies marked, the live copy not)", released)
	}
	if got := queryOne[int](t, f.path, `SELECT count(*) FROM folders WHERE sub_key IS NOT NULL`) +
		queryOne[int](t, f.path, `SELECT count(*) FROM catalogs WHERE sub_key IS NOT NULL`); got != 0 {
		t.Errorf("%d snapshot keys left on the released collection, want none", got)
	}

	db, err := vault.InitDB(f.path)
	if err != nil {
		t.Fatalf("the vault refuses the migrated database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.UnpublishCatalog(ctx, f.publisher, f.liveSource); err != nil {
		t.Fatal(err)
	}
	catalogs, err := db.GetUserCatalogs(ctx, f.subscriber)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range catalogs {
		if c.Subscription != nil || !c.PublisherUnpublished {
			t.Errorf("%s after unpublishing on v8 = subscription %+v, publisher unpublished %v; want released", c.Name, c.Subscription, c.PublisherUnpublished)
		}
	}
}

// A database at any version but 7 is refused, and so is a missing --db.
func TestMigrateRefusesOtherVersions(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "vault.db")
	db, err := vault.InitDB(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := runMigrate(ctx, []string{"--db", path}, &out); err == nil || !strings.Contains(err.Error(), "version 8") {
		t.Errorf("migrate of a v8 database = %v, want a version refusal", err)
	}
	if err := runMigrate(ctx, nil, &out); err == nil || !strings.Contains(err.Error(), "--db is required") {
		t.Errorf("migrate without --db = %v", err)
	}
	if err := runMigrate(ctx, []string{"--db", filepath.Join(t.TempDir(), "missing.db")}, &out); err == nil {
		t.Error("migrate of a missing file succeeded")
	}
}

// A database the migration can't carry through, here a live publication
// with no source, which version 8 can't hold, is refused and left exactly
// as it was.
func TestMigrateLeavesTheFileOnFailure(t *testing.T) {
	ctx := context.Background()
	f := newV7Fixture(t)
	// Version 7's trigger marks a publication unpublished once its source is
	// gone, so the status is set back on its own.
	raw := openRaw(t, f.path)
	for _, query := range []string{
		`UPDATE publications SET catalog_id = NULL WHERE id = ?`,
		`UPDATE publications SET status = 'live' WHERE id = ?`,
	} {
		if _, err := raw.Exec(query, f.livePub); err != nil {
			t.Fatal(err)
		}
	}
	var out bytes.Buffer
	if err := runMigrate(ctx, []string{"--db", f.path}, &out); err == nil {
		t.Fatal("migrate succeeded over a live publication with no source")
	}
	if got := queryOne[int](t, f.path, `PRAGMA user_version`); got != 7 {
		t.Errorf("user_version = %d, want 7", got)
	}
	if got := queryOne[int](t, f.path, `SELECT count(*) FROM publications WHERE status = 'unpublished'`); got != 2 {
		t.Errorf("unpublished publications = %d, want both still there", got)
	}
}
