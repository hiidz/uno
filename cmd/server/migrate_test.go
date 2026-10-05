package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/hiidz/uno/internal/vault"
)

// v6Fixture is a v6 database: one profile whose Home holds a pinned
// collection, two catalogs with a Discover-only one between them, and two
// unpinned collections, numbered as version 6 numbered them — catalogs and
// collections each from 0 — with a push record in version 6's shape, no
// positions on its Home.
type v6Fixture struct {
	path                    string
	profile                 uuid.UUID
	homeA, discover, homeB  vault.Catalog
	pinned, first, second   uuid.UUID
	collectionsBeforeRecord []json.RawMessage
}

func newV6Fixture(t *testing.T) v6Fixture {
	t.Helper()
	ctx := context.Background()
	f := v6Fixture{path: filepath.Join(t.TempDir(), "vault.db")}
	db, err := vault.InitDB(f.path)
	if err != nil {
		t.Fatal(err)
	}
	profile, err := db.ResolveOrCreateProfile(ctx, "user", 1, "nuvio-profile")
	if err != nil {
		t.Fatal(err)
	}
	f.profile = profile.ID
	for name, c := range map[string]*vault.Catalog{"A": &f.homeA, "Discover": &f.discover, "B": &f.homeB} {
		if *c, err = db.CreateUserCatalog(ctx, f.profile, vault.CatalogForm{Type: "movie", Name: name, Provider: "tmdb", Params: `{"sort_by":"popularity.desc"}`}); err != nil {
			t.Fatal(err)
		}
	}
	for title, id := range map[string]*uuid.UUID{"Pinned": &f.pinned, "First": &f.first, "Second": &f.second} {
		c, err := db.CreateUserCollection(ctx, f.profile, vault.CollectionForm{Title: title, Folders: []vault.FolderData{{Title: "F"}}})
		if err != nil {
			t.Fatal(err)
		}
		*id = c.ID
	}
	catalogs := vault.CatalogSelectionForm{Catalogs: []vault.SelectedCatalogInput{
		{CatalogID: f.homeA.ID, ShowInHome: true, Position: 0},
		{CatalogID: f.discover.ID, Position: 1},
		{CatalogID: f.homeB.ID, ShowInHome: true, Position: 2},
	}}
	collections := vault.CollectionSelectionForm{Collections: []vault.SelectedCollectionInput{
		{CollectionID: f.first, Position: 0},
		{CollectionID: f.pinned, PinToTop: true, Position: 1},
		{CollectionID: f.second, Position: 2},
	}}
	record, err := db.BuildPushRecord(ctx, f.profile, catalogs, collections)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SavePush(ctx, f.profile, record); err != nil {
		t.Fatal(err)
	}
	f.collectionsBeforeRecord = record.Collections
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	raw := openRaw(t, f.path)
	var stored string
	if err := raw.QueryRow(`SELECT record FROM push_records`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	v6Record := regexp.MustCompile(`,"position":\d+`).ReplaceAllString(stored, "")
	if _, err := raw.Exec(`UPDATE push_records SET record = ?`, v6Record); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`PRAGMA user_version = 6`); err != nil {
		t.Fatal(err)
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

// The migration renumbers Home as one list in the order version 6's bands
// showed it — the pinned collection, the catalogs with the Discover-only one
// where it was, then the other collections — gives the push record's Home
// those positions, keeps the bytes push sent, and stamps version 7, which the
// vault then opens.
func TestMigrateToV7(t *testing.T) {
	ctx := context.Background()
	f := newV6Fixture(t)

	var out bytes.Buffer
	if err := runMigrate(ctx, []string{"--db", f.path}, &out); err != nil {
		t.Fatalf("migrate: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "migrated to schema version 7") || !strings.Contains(out.String(), "1 push records") {
		t.Errorf("output = %q, want the version and the record count", out.String())
	}

	db, err := vault.InitDB(f.path)
	if err != nil {
		t.Fatalf("opening the migrated vault: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	want := map[uuid.UUID]int{f.pinned: 0, f.homeA.ID: 1, f.discover.ID: 2, f.homeB.ID: 3, f.first: 4, f.second: 5}
	got := map[uuid.UUID]int{}
	catalogs, err := db.GetCurrentCatalogSelection(ctx, f.profile)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range catalogs {
		got[c.ID] = *c.HomeSortOrder
	}
	collections, err := db.GetCurrentCollectionSelection(ctx, f.profile)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range collections {
		got[c.ID] = *c.HomeSortOrder
	}
	if !mapsEqual(got, want) {
		t.Errorf("positions = %v, want %v", got, want)
	}

	var stored string
	if err := openRaw(t, f.path).QueryRow(`SELECT record FROM push_records`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	var record vault.PushRecord
	if err := json.Unmarshal([]byte(stored), &record); err != nil {
		t.Fatal(err)
	}
	recordPositions := map[uuid.UUID]int{}
	for _, c := range record.Home.Catalogs {
		recordPositions[c.CatalogID] = c.Position
	}
	for _, c := range record.Home.Collections {
		recordPositions[c.CollectionID] = c.Position
	}
	if !mapsEqual(recordPositions, want) {
		t.Errorf("push record positions = %v, want %v", recordPositions, want)
	}
	if len(record.Collections) != len(f.collectionsBeforeRecord) {
		t.Fatalf("push record collections = %d, want %d", len(record.Collections), len(f.collectionsBeforeRecord))
	}
	for i := range record.Collections {
		if !bytes.Equal(record.Collections[i], f.collectionsBeforeRecord[i]) {
			t.Errorf("pushed collection %d = %s, want the bytes push sent: %s", i, record.Collections[i], f.collectionsBeforeRecord[i])
		}
	}
}

func mapsEqual(a, b map[uuid.UUID]int) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if w, ok := b[k]; !ok || w != v {
			return false
		}
	}
	return true
}

// The command needs --db naming an existing v6 database; a fresh v7 one is
// refused and left at version 7.
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
		"version 7":    {"--db", fresh},
	} {
		if err := runCommand(ctx, append([]string{"migrate"}, args...), &bytes.Buffer{}); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
	var version int
	if err := openRaw(t, fresh).QueryRow(`PRAGMA user_version`).Scan(&version); err != nil || version != 7 {
		t.Errorf("version after a refused migrate = %d (%v), want 7", version, err)
	}
}

// diffLines names the lines each side lacks.
func TestDiffLines(t *testing.T) {
	extra, missing := diffLines([]string{"a", "b"}, []string{"b", "c"})
	if !slices.Equal(extra, []string{"a"}) || !slices.Equal(missing, []string{"c"}) {
		t.Errorf("diffLines = %v, %v; want [a], [c]", extra, missing)
	}
}
