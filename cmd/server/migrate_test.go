package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/hiidz/uno/internal/vault"
)

// schemaV5 is the vault's schema at version 5, kept as test data until the
// migration is deleted.
//
//go:embed testdata/schema_v5.sql
var schemaV5 string

// v5Fixture is a v5 database: a publisher with a catalog row and a
// collection on Home, a subscriber with copies in step, out of step and of a
// withdrawn publication, and a profile with nothing.
type v5Fixture struct {
	path                                    string
	publisher, subscriber, idle             uuid.UUID
	homeRow, inFolder, withdrawnSource      uuid.UUID
	inStep, outOfStep, ofWithdrawn          uuid.UUID
	night, folder                           uuid.UUID
	live, liveToo, withdrawn, collectionPub uuid.UUID
}

// nightPushHash is the hash of what push sends for the fixture's Night
// collection, as push stored it at v5.
func (f v5Fixture) nightPushHash(t *testing.T) string {
	t.Helper()
	tree := vault.CollectionWithFolders{
		Collection: vault.Collection{ID: f.night, Title: "Night", PinToTop: true, ViewMode: "TABBED_GRID", FocusGlowEnabled: true},
		Folders: []vault.FolderWithCatalogs{{
			Folder: vault.Folder{ID: f.folder, Title: "F", TileShape: "LANDSCAPE", FocusGIFEnabled: true},
			Refs:   []vault.FolderRef{{CatalogID: f.inFolder, Genre: "Horror"}, {CatalogID: f.homeRow}},
		}},
		Catalogs: []vault.Catalog{{ID: f.inFolder, Type: "movie", Provider: "tmdb"}, {ID: f.homeRow, Type: "movie", Provider: "tmdb"}},
	}
	raw, err := tree.PushJSON()
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// newV5Fixture writes the fixture's v5 database, with Night's pushed hash
// set to pushedHash(f).
func newV5Fixture(t *testing.T, pushedHash func(v5Fixture) string) v5Fixture {
	t.Helper()
	f := v5Fixture{path: filepath.Join(t.TempDir(), "vault.db")}
	for _, id := range []*uuid.UUID{&f.publisher, &f.subscriber, &f.idle, &f.homeRow, &f.inFolder, &f.withdrawnSource,
		&f.inStep, &f.outOfStep, &f.ofWithdrawn, &f.night, &f.folder, &f.live, &f.liveToo, &f.withdrawn, &f.collectionPub} {
		*id = uuid.New()
	}
	db := openRaw(t, f.path)
	exec(t, db, schemaV5+"\nPRAGMA user_version = 5;")
	const now = "2026-10-01T00:00:00Z"
	for _, p := range []uuid.UUID{f.publisher, f.subscriber, f.idle} {
		exec(t, db, `INSERT INTO profiles (id, token, nuvio_user_id, nuvio_profile_index, nuvio_profile_uuid) VALUES (?, ?, ?, 1, ?)`,
			p.String(), "token-"+p.String(), "user-"+p.String(), "nuvio-"+p.String())
	}
	exec(t, db, `INSERT INTO recipes (hash, type, provider, params, created_at) VALUES ('r1', 'movie', 'tmdb', '{}', ?)`, now)
	catalog := func(id, owner uuid.UUID, name string, home any) {
		exec(t, db, `INSERT INTO catalogs (id, name, recipe_hash, owner_id, home_sort_order, created_at, updated_at) VALUES (?, ?, 'r1', ?, ?, ?, ?)`,
			id.String(), name, owner.String(), home, now, now)
	}
	catalog(f.homeRow, f.publisher, "Home row", 0)
	catalog(f.inFolder, f.publisher, "In a folder", nil)
	catalog(f.withdrawnSource, f.publisher, "Withdrawn source", nil)
	catalog(f.inStep, f.subscriber, "In step", nil)
	catalog(f.outOfStep, f.subscriber, "Out of step", nil)
	catalog(f.ofWithdrawn, f.subscriber, "Of a withdrawn one", nil)

	exec(t, db, `INSERT INTO collections (id, title, owner_id, pin_to_top, home_sort_order, created_at, updated_at) VALUES (?, 'Night', ?, 1, 0, ?, ?)`,
		f.night.String(), f.publisher.String(), now, now)
	exec(t, db, `INSERT INTO collections (id, title, owner_id, created_at, updated_at) VALUES (?, 'Off home', ?, ?, ?)`,
		uuid.NewString(), f.publisher.String(), now, now)
	exec(t, db, `INSERT INTO folders (id, collection_id, title, sort_order) VALUES (?, ?, 'F', 0)`, f.folder.String(), f.night.String())
	exec(t, db, `INSERT INTO folder_catalogs (folder_id, catalog_id, sort_order, genre) VALUES (?, ?, 0, 'Horror'), (?, ?, 1, '')`,
		f.folder.String(), f.inFolder.String(), f.folder.String(), f.homeRow.String())
	exec(t, db, `UPDATE collections SET pushed_hash = ? WHERE id = ?`, pushedHash(f), f.night.String())

	publication := func(id uuid.UUID, kind string, source uuid.UUID, hash, status string) {
		column := kind + "_id"
		exec(t, db, `INSERT INTO publications (id, owner_id, kind, `+column+`, title, snapshot, content_hash, catalog_count, folder_count, status, published_at, updated_at)
			VALUES (?, ?, ?, ?, 'T', '{}', ?, 1, 0, ?, ?, ?)`, id.String(), f.publisher.String(), kind, source.String(), hash, status, now, now)
	}
	publication(f.live, "catalog", f.homeRow, "h1", "live")
	publication(f.liveToo, "catalog", f.inFolder, "h2", "live")
	publication(f.withdrawn, "catalog", f.withdrawnSource, "h3", "withdrawn")
	publication(f.collectionPub, "collection", f.night, "h4", "live")
	subscription := func(pub, copyID uuid.UUID, hash string) {
		exec(t, db, `INSERT INTO subscriptions (id, owner_id, publication_id, catalog_id, taken_hash, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
			uuid.NewString(), f.subscriber.String(), pub.String(), copyID.String(), hash, now)
	}
	subscription(f.live, f.inStep, "h1")
	subscription(f.liveToo, f.outOfStep, "old")
	subscription(f.withdrawn, f.ofWithdrawn, "h3")
	return f
}

// openRaw opens path with no pragmas, closed with the test.
func openRaw(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func exec(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
}

func queryInt(t *testing.T, db *sql.DB, query string, args ...any) int {
	t.Helper()
	var n int
	if err := db.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return n
}

// The migration keeps every row, reads every subscription and publication
// as before, backfills a record for every profile that leaves Night needing
// no push, leaves a structure matching a fresh v6 database, and its triggers
// still fire.
func TestMigrateToV6(t *testing.T) {
	ctx := context.Background()
	f := newV5Fixture(t, func(f v5Fixture) string { return f.nightPushHash(t) })
	before, err := rowCounts(ctx, openRaw(t, f.path))
	if err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := runMigrate(ctx, []string{"--db", f.path}, &out); err != nil {
		t.Fatalf("migrate: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "push_records") || !strings.Contains(out.String(), "3 push records backfilled") {
		t.Errorf("output = %s, want the counts and the backfill", out.String())
	}

	raw := openRaw(t, f.path)
	after, err := rowCounts(ctx, raw)
	if err != nil {
		t.Fatal(err)
	}
	for table, n := range before {
		if after[table] != n {
			t.Errorf("%s: %d rows after, want %d", table, after[table], n)
		}
	}
	if after["push_records"] != 3 {
		t.Errorf("push records = %d, want one per profile", after["push_records"])
	}
	got, err := structureOf(ctx, raw)
	if err != nil {
		t.Fatal(err)
	}
	want, err := freshStructure(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, want) {
		extra, missing := diffLines(got, want)
		t.Errorf("structure differs from a fresh v6 database: has %v, lacks %v", extra, missing)
	}

	checkSharingReadsTheSame(t, f)
	checkTriggersFire(t, f, raw)
}

// checkSharingReadsTheSame opens the migrated database through the vault and
// reads each copy's update and unpublished state, each publication's status,
// Night's needs_push, and the publisher's backfilled record.
func checkSharingReadsTheSame(t *testing.T, f v5Fixture) {
	t.Helper()
	ctx := context.Background()
	db, err := vault.InitDB(f.path)
	if err != nil {
		t.Fatalf("InitDB on the migrated database: %v", err)
	}
	defer func() { _ = db.Close() }()

	copies, err := db.GetUserCatalogs(ctx, f.subscriber)
	if err != nil {
		t.Fatal(err)
	}
	wantCopies := map[uuid.UUID]vault.SubscriptionState{
		f.inStep:      {PublicationID: f.live},
		f.outOfStep:   {PublicationID: f.liveToo, UpdateAvailable: true},
		f.ofWithdrawn: {PublicationID: f.withdrawn, Unpublished: true},
	}
	for _, c := range copies {
		if c.Subscription == nil || *c.Subscription != wantCopies[c.ID] {
			t.Errorf("%s: subscription %+v, want %+v", c.Name, c.Subscription, wantCopies[c.ID])
		}
	}

	sources, err := db.GetUserCatalogs(ctx, f.publisher)
	if err != nil {
		t.Fatal(err)
	}
	wantStatus := map[uuid.UUID]string{f.homeRow: "live", f.inFolder: "live", f.withdrawnSource: "unpublished"}
	for _, c := range sources {
		if c.Publication == nil || c.Publication.Status != wantStatus[c.ID] {
			t.Errorf("%s: publication %+v, want status %q", c.Name, c.Publication, wantStatus[c.ID])
		}
	}

	home, err := db.GetCurrentCollectionSelection(ctx, f.publisher)
	if err != nil || len(home) != 1 || home[0].NeedsPush || !home[0].PinToTop {
		t.Errorf("Home = %+v, %v; want Night, pinned, needing no push", home, err)
	}
	record, err := db.BuildPushRecord(ctx, f.publisher,
		vault.CatalogSelectionForm{Catalogs: []vault.SelectedCatalogInput{{CatalogID: f.homeRow, ShowInHome: true}}},
		vault.CollectionSelectionForm{Collections: []vault.SelectedCollectionInput{{CollectionID: f.night, PinToTop: true}}})
	if err != nil {
		t.Fatal(err)
	}
	var stored string
	if err := openRaw(t, f.path).QueryRow(`SELECT record FROM push_records WHERE profile_id = ?`, f.publisher.String()).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if built, _ := json.Marshal(record); string(built) != stored {
		t.Errorf("backfilled record = %s\nwant what a push of the stored Home builds: %s", stored, built)
	}
}

// checkTriggersFire checks the count triggers and the unpublish trigger on
// the migrated database, and that the old status is refused.
func checkTriggersFire(t *testing.T, f v5Fixture, db *sql.DB) {
	t.Helper()
	count := func() int {
		return queryInt(t, db, `SELECT subscriber_count FROM publications WHERE id = ?`, f.live.String())
	}
	exec(t, db, `INSERT INTO catalogs (id, name, recipe_hash, owner_id, created_at, updated_at) VALUES ('idle-copy', 'C', 'r1', ?, 'now', 'now')`, f.idle.String())
	exec(t, db, `INSERT INTO subscriptions (id, subscriber_id, publication_id, catalog_id, subscribed_hash, created_at) VALUES ('s', ?, ?, 'idle-copy', 'h1', 'now')`,
		f.idle.String(), f.live.String())
	if n := count(); n != 2 {
		t.Errorf("subscriber count after a subscribe = %d, want 2", n)
	}
	exec(t, db, `DELETE FROM subscriptions WHERE id = 's'`)
	if n := count(); n != 1 {
		t.Errorf("subscriber count after an unsubscribe = %d, want 1", n)
	}
	exec(t, db, `UPDATE publications SET catalog_id = NULL WHERE id = ?`, f.liveToo.String())
	if status := queryString(t, db, `SELECT status FROM publications WHERE id = ?`, f.liveToo.String()); status != "unpublished" {
		t.Errorf("status after its source went = %q, want unpublished", status)
	}
	if _, err := db.Exec(`UPDATE publications SET status = 'withdrawn' WHERE id = ?`, f.live.String()); err == nil {
		t.Error("status 'withdrawn' was accepted; want the CHECK to refuse it")
	}
}

func queryString(t *testing.T, db *sql.DB, query string, args ...any) string {
	t.Helper()
	var s string
	if err := db.QueryRow(query, args...).Scan(&s); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return s
}

// While a collection on Home needs a push by v5's rule, the migration
// refuses, names it, and leaves the database at v5 as it was.
func TestMigrateRefusesWhileACollectionNeedsAPush(t *testing.T) {
	ctx := context.Background()
	f := newV5Fixture(t, func(v5Fixture) string { return "stale" })
	var out bytes.Buffer
	err := migrateToV6(ctx, f.path, &out)
	if err == nil || !strings.Contains(err.Error(), "need a push") || !strings.Contains(err.Error(), `"Night"`) {
		t.Fatalf("migrate = %v, want a refusal naming Night", err)
	}
	db := openRaw(t, f.path)
	if v := queryInt(t, db, `PRAGMA user_version`); v != 5 {
		t.Errorf("user_version = %d, want 5", v)
	}
	if n := queryInt(t, db, `SELECT count(*) FROM sqlite_master WHERE name IN ('push_records', 'publications_v6')`); n != 0 {
		t.Errorf("v6 tables left behind: %d", n)
	}
	if n := queryInt(t, db, `SELECT count(*) FROM publications WHERE status = 'withdrawn' AND owner_id = ?`, f.publisher.String()); n != 1 {
		t.Errorf("v5 publications after the refusal: %d withdrawn, want 1", n)
	}
}

// A broken reference stops the migration before anything commits.
func TestMigrateRefusesABrokenReference(t *testing.T) {
	f := newV5Fixture(t, func(f v5Fixture) string { return f.nightPushHash(t) })
	exec(t, openRaw(t, f.path), `INSERT INTO folder_catalogs (folder_id, catalog_id, sort_order) VALUES ('no-folder', 'no-catalog', 0)`)
	err := migrateToV6(context.Background(), f.path, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "foreign_key_check") {
		t.Fatalf("migrate = %v, want the foreign key refusal", err)
	}
	if v := queryInt(t, openRaw(t, f.path), `PRAGMA user_version`); v != 5 {
		t.Errorf("user_version = %d, want 5", v)
	}
}

// The command needs --db naming an existing v5 database.
func TestMigrateRefusals(t *testing.T) {
	ctx := context.Background()
	fresh := filepath.Join(t.TempDir(), "fresh.db")
	db, err := vault.InitDB(fresh)
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	for name, args := range map[string][]string{
		"no --db":      {},
		"missing file": {"--db", filepath.Join(t.TempDir(), "none.db")},
		"unknown flag": {"--nope"},
		"version 6":    {"--db", fresh},
	} {
		if err := runCommand(ctx, append([]string{"migrate"}, args...), &bytes.Buffer{}); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

// diffLines names the lines each side lacks.
func TestDiffLines(t *testing.T) {
	extra, missing := diffLines([]string{"a", "b"}, []string{"b", "c"})
	if !slices.Equal(extra, []string{"a"}) || !slices.Equal(missing, []string{"c"}) {
		t.Errorf("diffLines = %v, %v; want [a], [c]", extra, missing)
	}
}
