package vault

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/hiidz/uno/internal/provider"
	"github.com/hiidz/uno/internal/vault/migrations"
)

var updateSchema = flag.Bool("update", false, "rewrite testdata/schema_latest.sql from a freshly migrated database")

// execSQL runs statements against the database file at path, creating it if
// needed, on a connection of its own.
func execSQL(t *testing.T, path, statements string) {
	t.Helper()
	d, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if _, err := d.Exec(statements); err != nil {
		t.Fatalf("exec: %v", err)
	}
}

// fixtureDB writes testdata/schema_v1.sql to a fresh database file in WAL
// mode, as prod runs, and returns its path.
func fixtureDB(t *testing.T) string {
	t.Helper()
	fixture, err := os.ReadFile("testdata/schema_v1.sql")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "vault.db")
	execSQL(t, path, "PRAGMA journal_mode = WAL;\n"+string(fixture))
	return path
}

// queryColumn returns the first column of every row query returns from the
// database at path, formatted with %v.
func queryColumn(t *testing.T, path, query string) []string {
	t.Helper()
	d, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	rows, err := d.Query(query)
	if err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	defer rows.Close()
	var values []string
	for rows.Next() {
		var v any
		if err := rows.Scan(&v); err != nil {
			t.Fatal(err)
		}
		values = append(values, fmt.Sprint(v))
	}
	return values
}

func userVersionOf(t *testing.T, path string) string {
	t.Helper()
	return queryColumn(t, path, `PRAGMA user_version`)[0]
}

func backupsOf(t *testing.T, path string) []string {
	t.Helper()
	backups, err := filepath.Glob(path + ".pre-v*.bak")
	if err != nil {
		t.Fatal(err)
	}
	return backups
}

// dumpRows returns every row of every table in the database at path, each
// row's columns formatted with %q, in rowid order, keyed by table.
func dumpRows(t *testing.T, path string) map[string][]string {
	t.Helper()
	d, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	dump := map[string][]string{}
	for _, table := range queryColumn(t, path, `SELECT name FROM sqlite_master WHERE type = 'table'`) {
		rows, err := d.Query(`SELECT * FROM ` + table + ` ORDER BY rowid`)
		if err != nil {
			t.Fatal(err)
		}
		columns, _ := rows.Columns()
		for rows.Next() {
			values := make([]any, len(columns))
			pointers := make([]any, len(columns))
			for i := range values {
				pointers[i] = &values[i]
			}
			if err := rows.Scan(pointers...); err != nil {
				t.Fatal(err)
			}
			dump[table] = append(dump[table], fmt.Sprintf("%q", values))
		}
		rows.Close()
	}
	return dump
}

// step is a migration that runs statements and returns one note.
func step(version int, statements string) migrations.Migration {
	return migrations.Migration{Version: version, Name: fmt.Sprintf("step %d", version), Up: func(ctx context.Context, tx *sql.Tx) ([]string, error) {
		_, err := tx.ExecContext(ctx, statements)
		return []string{fmt.Sprintf("ran step %d", version)}, err
	}}
}

// A migration runs once: a later start runs only the ones added since, in
// version order, and a start with none pending writes no backup.
func TestMigrateRunsOnlyPendingMigrationsInOrder(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "vault.db")
	list := []migrations.Migration{
		step(1, `CREATE TABLE applied (version INTEGER)`),
		step(2, `INSERT INTO applied VALUES (2)`),
		step(3, `INSERT INTO applied VALUES (3)`),
	}
	if err := migrate(ctx, path, list[:2]); err != nil {
		t.Fatalf("migrate to 2: %v", err)
	}
	if got := userVersionOf(t, path); got != "2" {
		t.Fatalf("user_version = %s, want 2", got)
	}
	if err := migrate(ctx, path, list); err != nil {
		t.Fatalf("migrate to 3: %v", err)
	}
	if got := queryColumn(t, path, `SELECT version FROM applied ORDER BY rowid`); !slices.Equal(got, []string{"2", "3"}) {
		t.Errorf("applied = %q, want [2 3]: each migration once, in order", got)
	}
	if got := userVersionOf(t, path); got != "3" {
		t.Errorf("user_version = %s, want 3", got)
	}

	backups := len(backupsOf(t, path))
	if err := migrate(ctx, path, list); err != nil {
		t.Fatalf("migrate with nothing pending: %v", err)
	}
	if got := len(backupsOf(t, path)); got != backups {
		t.Errorf("backups = %d after a start with nothing pending, want %d", got, backups)
	}
}

// With a migration pending, the database is copied beside itself first, and
// the copy holds what was there before.
func TestMigrateBacksUpBeforeMigrating(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vault.db")
	execSQL(t, path, `CREATE TABLE kept (v TEXT); INSERT INTO kept VALUES ('before')`)
	if err := migrate(context.Background(), path, []migrations.Migration{step(1, `UPDATE kept SET v = 'after'`)}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	backups := backupsOf(t, path)
	if len(backups) != 1 {
		t.Fatalf("backups = %q, want one", backups)
	}
	if !regexp.MustCompile(`vault\.db\.pre-v1-\d{8}T\d{6}\.\d{3}Z\.bak$`).MatchString(backups[0]) {
		t.Errorf("backup %s, want vault.db.pre-v1-<UTC time>.bak", backups[0])
	}
	if got := queryColumn(t, backups[0], `SELECT v FROM kept`); !slices.Equal(got, []string{"before"}) {
		t.Errorf("backup holds %q, want [before]", got)
	}
	if got := userVersionOf(t, backups[0]); got != "0" {
		t.Errorf("backup user_version = %s, want 0", got)
	}
	if got := queryColumn(t, path, `SELECT v FROM kept`); !slices.Equal(got, []string{"after"}) {
		t.Errorf("database holds %q, want [after]", got)
	}
}

// A failing migration leaves nothing of itself behind, while the ones
// before it stay applied.
func TestMigrateRollsBackAFailingMigration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vault.db")
	list := []migrations.Migration{
		step(1, `CREATE TABLE t (v INTEGER); INSERT INTO t VALUES (1)`),
		{Version: 2, Name: "fails", Up: func(ctx context.Context, tx *sql.Tx) ([]string, error) {
			if _, err := tx.ExecContext(ctx, `INSERT INTO t VALUES (2)`); err != nil {
				return nil, err
			}
			return nil, errors.New("boom")
		}},
	}
	err := migrate(context.Background(), path, list)
	if err == nil || !strings.Contains(err.Error(), "migration 2 (fails): boom") {
		t.Fatalf("migrate = %v, want migration 2's failure", err)
	}
	if got := userVersionOf(t, path); got != "1" {
		t.Errorf("user_version = %s, want 1", got)
	}
	if got := queryColumn(t, path, `SELECT v FROM t`); !slices.Equal(got, []string{"1"}) {
		t.Errorf("t = %q, want [1]", got)
	}
}

// Foreign keys are off while a migration runs, so one that leaves a broken
// reference is caught by the check before commit and rolled back.
func TestMigrateRollsBackABrokenForeignKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vault.db")
	err := migrate(context.Background(), path, []migrations.Migration{step(1, `
		CREATE TABLE parent (id TEXT PRIMARY KEY);
		CREATE TABLE child (parent_id TEXT REFERENCES parent(id));
		INSERT INTO child VALUES ('missing');
	`)})
	if err == nil || !strings.Contains(err.Error(), "foreign_key_check found 1 broken references: child row 1 → parent") {
		t.Fatalf("migrate = %v, want the foreign key failure", err)
	}
	if got := userVersionOf(t, path); got != "0" {
		t.Errorf("user_version = %s, want 0", got)
	}
	if got := queryColumn(t, path, `SELECT name FROM sqlite_master`); len(got) != 0 {
		t.Errorf("schema after the rollback = %q, want empty", got)
	}
}

// A database at a version this build doesn't know — a newer build's, or a
// negative one set by hand — is refused before anything is written.
func TestInitDBRefusesAnUnknownVersion(t *testing.T) {
	for _, version := range []string{"3", "-1"} {
		t.Run(version, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "vault.db")
			execSQL(t, path, `PRAGMA user_version = `+version)
			_, err := InitDB(path)
			if err == nil || !strings.Contains(err.Error(), "schema version "+version+", which this build (versions 0 to 2) cannot read") {
				t.Fatalf("InitDB = %v, want the unknown-version refusal", err)
			}
			if got := backupsOf(t, path); len(got) != 0 {
				t.Errorf("backups = %q, want none", got)
			}
			if got := userVersionOf(t, path); got != version {
				t.Errorf("user_version = %s, want %s", got, version)
			}
		})
	}
}

// A start that fails the same way again leaves one backup, not one per
// restart; a change to the database in between gets a backup of its own.
func TestMigrateKeepsOneBackupPerFailingState(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "vault.db")
	execSQL(t, path, `CREATE TABLE kept (v TEXT); INSERT INTO kept VALUES ('one')`)
	failing := []migrations.Migration{step(1, `INSERT INTO missing VALUES (1)`)}

	for range 2 {
		if err := migrate(ctx, path, failing); err == nil {
			t.Fatal("migrate = nil, want the failure")
		}
	}
	if got := backupsOf(t, path); len(got) != 1 {
		t.Fatalf("backups after two identical failed starts = %q, want one", got)
	}

	execSQL(t, path, `INSERT INTO kept VALUES ('two')`)
	if err := migrate(ctx, path, failing); err == nil {
		t.Fatal("migrate = nil, want the failure")
	}
	if got := backupsOf(t, path); len(got) != 2 {
		t.Errorf("backups after a start on changed data = %q, want two", got)
	}
}

// A migration re-checks the version under the write lock: one that another
// process has already moved past is refused, not applied twice.
func TestApplyMigrationRefusesAVersionMovedMeanwhile(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "vault.db")
	execSQL(t, path, `PRAGMA user_version = 1`)
	d, err := openMigrationDB(path, "")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()

	_, err = applyMigration(ctx, d, step(1, `CREATE TABLE twice (v INTEGER)`))
	if err == nil || !strings.Contains(err.Error(), "schema version 1, not 0: another process migrated it meanwhile") {
		t.Fatalf("applyMigration = %v, want the moved-version refusal", err)
	}
	if got := queryColumn(t, path, `SELECT name FROM sqlite_master`); len(got) != 0 {
		t.Errorf("schema = %q, want the migration not run", got)
	}
}

// Prod's database adopts migration 1 with every row as it was — ids,
// versions and pushed versions included.
func TestMigrationOneAdoptsAProdDatabaseUnchanged(t *testing.T) {
	path := fixtureDB(t)
	before := dumpRows(t, path)

	if err := migrate(context.Background(), path, migrations.All()[:1]); err != nil {
		t.Fatalf("migrate to 1: %v", err)
	}

	if got := userVersionOf(t, path); got != "1" {
		t.Errorf("user_version = %s, want 1", got)
	}
	if after := dumpRows(t, path); !reflect.DeepEqual(after, before) {
		t.Errorf("rows changed by migration 1:\nbefore %q\nafter  %q", before, after)
	}
	if got := backupsOf(t, path); len(got) != 1 {
		t.Errorf("backups = %q, want one", got)
	}
}

// fixtureAtV1 is the fixture migrated to schema version 1, then changed by
// statements, which may be empty.
func fixtureAtV1(t *testing.T, statements string) string {
	t.Helper()
	path := fixtureDB(t)
	if err := migrate(context.Background(), path, migrations.All()[:1]); err != nil {
		t.Fatalf("migrate to 1: %v", err)
	}
	if statements != "" {
		execSQL(t, path, statements)
	}
	return path
}

// keptColumns lists the columns of each table that migration 2 must leave
// exactly as they were: every one but the catalog columns recipes replace
// and taken_hash, which it remaps.
var keptColumns = map[string]string{
	"profiles": `id, token, nuvio_user_id, nuvio_profile_index, nuvio_profile_uuid`,
	"folders": `id, collection_id, title, sort_order, tile_shape, hide_title, cover_emoji, cover_image_url,
		focus_gif_url, focus_gif_enabled, hero_video_url, hero_backdrop_url, title_logo_url`,
	"folder_catalogs": `folder_id, catalog_id, sort_order, genre`,
	"collections": `id, title, owner_id, is_public, pin_to_top, view_mode, show_all_tab, backdrop_image_url,
		focus_glow_enabled, home_sort_order, version, pushed_version, taken_from, created_at, updated_at`,
	"catalogs": `id, name, owner_id, is_public, collection_id, home_sort_order, show_in_home, taken_from,
		created_at, updated_at`,
}

// dumpKept returns keptColumns of every row of each table, in rowid order.
func dumpKept(t *testing.T, path string) map[string][]string {
	t.Helper()
	dump := map[string][]string{}
	for table, columns := range keptColumns {
		dump[table] = queryColumn(t, path, `SELECT json_array(`+columns+`) FROM `+table+` ORDER BY rowid`)
	}
	return dump
}

// Migration 2 over prod's database, odd params included, keeps every id,
// version and placement, gives each catalog the recipe the live
// canonicalizer and RecipeHash make of its params, one per distinct recipe,
// and the pool reads the result.
func TestInitDBMovesTheFixtureToRecipes(t *testing.T) {
	ctx := context.Background()
	path := fixtureAtV1(t, oddParams)
	before := dumpKept(t, path)
	oldParams := queryPairs(t, path, `SELECT id, type || char(9) || params FROM catalogs`)

	db, err := InitDB(path)
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	defer db.Close()

	if got := userVersionOf(t, path); got != "2" {
		t.Errorf("user_version = %s, want 2", got)
	}
	if after := dumpKept(t, path); !reflect.DeepEqual(after, before) {
		t.Errorf("rows changed by migration 2:\nbefore %q\nafter  %q", before, after)
	}
	if got := queryColumn(t, path, `SELECT name FROM pragma_table_info('catalogs') WHERE name IN ('type', 'provider', 'params', 'fingerprint')`); len(got) != 0 {
		t.Errorf("catalogs still has %q", got)
	}
	if got := queryColumn(t, path, `SELECT count(*) FROM recipes`); !slices.Equal(got, []string{"9"}) {
		t.Errorf("recipes = %s, want 9", got)
	}
	for id, recipe := range queryPairs(t, path, `SELECT c.id, r.type || char(9) || r.provider || char(9) || r.params || char(9) || c.recipe_hash FROM catalogs c JOIN recipes r ON r.hash = c.recipe_hash`) {
		catalogType, params, _ := strings.Cut(oldParams[id], "\t")
		canonical, err := provider.CanonicalParams(catalogType, "tmdb", params)
		if err != nil {
			t.Fatalf("catalog %s: %v", id, err)
		}
		want := strings.Join([]string{catalogType, "tmdb", canonical, RecipeHash(catalogType, "tmdb", canonical)}, "\t")
		if recipe != want {
			t.Errorf("catalog %s's recipe = %q, want %q", id, recipe, want)
		}
	}

	alice := uuid.MustParse("aaaaaaaa-0000-4000-8000-000000000001")
	bob := uuid.MustParse("aaaaaaaa-0000-4000-8000-000000000002")
	if catalogs, err := db.GetUserCatalogs(ctx, alice); err != nil || len(catalogs) != 4 || catalogs[0].Params == "" {
		t.Errorf("GetUserCatalogs(alice) = %+v, %v; want 4 with their recipes", catalogs, err)
	}
	if collections, err := db.GetUserCollections(ctx, bob); err != nil || len(collections) != 2 {
		t.Errorf("GetUserCollections(bob) = %d collections, %v; want 2", len(collections), err)
	}
}

// queryPairs runs a query of two TEXT columns against the database at path
// and returns the rows as a map from the first to the second.
func queryPairs(t *testing.T, path, query string) map[string]string {
	t.Helper()
	d, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	rows, err := d.Query(query)
	if err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	defer rows.Close()
	pairs := map[string]string{}
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			t.Fatal(err)
		}
		pairs[key] = value
	}
	return pairs
}

// linkState is whether a linked copy's taken_hash is its own link hash now,
// and its source's.
type linkState struct{ matchesCopy, matchesSource bool }

// catalogLinkStates reads each of ids' link state in db, by the link hashes
// the code computes over recipe hashes.
func catalogLinkStates(t *testing.T, db *DB, ids ...string) map[string]linkState {
	t.Helper()
	states := map[string]linkState{}
	for _, id := range ids {
		var takenHash, name, recipeHash, sourceName, sourceRecipe string
		err := db.conn.QueryRowContext(context.Background(), `
			SELECT c.taken_hash, c.name, c.recipe_hash, s.name, s.recipe_hash
			FROM catalogs c JOIN catalogs s ON s.id = c.taken_from WHERE c.id = ?
		`, id).Scan(&takenHash, &name, &recipeHash, &sourceName, &sourceRecipe)
		if err != nil {
			t.Fatalf("catalog %s: %v", id, err)
		}
		states[id] = linkState{catalogHash(name, recipeHash) == takenHash, catalogHash(sourceName, sourceRecipe) == takenHash}
	}
	return states
}

// collectionLinkStates is catalogLinkStates for linked collections.
func collectionLinkStates(t *testing.T, db *DB, ids ...string) map[string]linkState {
	t.Helper()
	ctx := context.Background()
	states := map[string]linkState{}
	for _, id := range ids {
		var takenFrom, takenHash string
		if err := db.conn.QueryRowContext(ctx, `SELECT taken_from, taken_hash FROM collections WHERE id = ?`, id).Scan(&takenFrom, &takenHash); err != nil {
			t.Fatalf("collection %s: %v", id, err)
		}
		copyHash, err := storedCollectionHash(ctx, db.conn, uuid.MustParse(id))
		if err != nil {
			t.Fatalf("collection %s: %v", id, err)
		}
		sourceHash, err := storedCollectionHash(ctx, db.conn, uuid.MustParse(takenFrom))
		if err != nil {
			t.Fatalf("collection %s's source: %v", id, err)
		}
		states[id] = linkState{copyHash == takenHash, sourceHash == takenHash}
	}
	return states
}

const (
	inStep = "in step"
	behind = "behind"
)

// Migration 2 remaps every taken_hash onto the link hashes over recipe
// hashes, so the fixture's linked copies are in or out of step exactly as
// its header says, and each copy, untouched since it was taken, still
// matches its taken_hash.
func TestFixtureLinksMatchTheirLabels(t *testing.T) {
	db, err := InitDB(fixtureDB(t))
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	defer db.Close()

	labels := map[string]string{
		"cacacaca-0000-4000-8000-000000000009": inStep,
		"cacacaca-0000-4000-8000-000000000010": behind,
		"cacacaca-0000-4000-8000-000000000011": inStep,
		"cacacaca-0000-4000-8000-000000000017": inStep,
		"cccccccc-0000-4000-8000-000000000004": inStep,
		"cccccccc-0000-4000-8000-000000000005": behind,
		"cccccccc-0000-4000-8000-000000000006": inStep,
	}
	states := catalogLinkStates(t, db, "cacacaca-0000-4000-8000-000000000009", "cacacaca-0000-4000-8000-000000000010",
		"cacacaca-0000-4000-8000-000000000011", "cacacaca-0000-4000-8000-000000000017")
	maps.Copy(states, collectionLinkStates(t, db, "cccccccc-0000-4000-8000-000000000004", "cccccccc-0000-4000-8000-000000000005",
		"cccccccc-0000-4000-8000-000000000006"))
	for id, label := range labels {
		if want := (linkState{matchesCopy: true, matchesSource: label == inStep}); states[id] != want {
			t.Errorf("%s (%s): %+v, want %+v", id, label, states[id], want)
		}
	}
}

// A copy edited without being unlinked keeps pointing at its source's hash
// by the live link hashes, so Update and Community judge it as they did
// before migration 2.
func TestMigrationTwoKeepsAnEditedCopyOnItsSource(t *testing.T) {
	const edited = "cacacaca-0000-4000-8000-000000000011"
	db, err := InitDB(fixtureAtV1(t, `UPDATE catalogs SET name = 'Renamed without unlinking' WHERE id = '`+edited+`'`))
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	defer db.Close()
	if got, want := catalogLinkStates(t, db, edited)[edited], (linkState{matchesSource: true}); got != want {
		t.Errorf("edited copy: %+v, want %+v", got, want)
	}
}

// oddParams gives two of the fixture's catalogs params in forms the builder
// never wrote: unknown keys, a zero value, unsorted keys, a trailing zero.
const oddParams = `
	UPDATE catalogs SET params = '{"vote_count_gte":0,"legacy_sort":"x","sort_by":"popularity.desc"}' WHERE id = 'cacacaca-0000-4000-8000-000000000001';
	UPDATE catalogs SET params = '{"vote_average_gte":7.50,"sort_by":"first_air_date.desc","endpoint":"/discover/tv"}' WHERE id = 'cacacaca-0000-4000-8000-000000000008';
`

// The dry run's recipe check finds that every catalog's params, odd forms
// included, fetch the same titles once canonical.
func TestDryRunFindsCanonicalParamsFetchTheSameTitles(t *testing.T) {
	report, err := DryRun(context.Background(), fixtureAtV1(t, oddParams), provider.SameRecipe)
	if err != nil {
		t.Fatalf("DryRun: %v", err)
	}
	if report.RecipesChecked != 20 || len(report.RecipeMismatches) != 0 {
		t.Errorf("recipe check: %d checked, mismatches %q; want 20 and none", report.RecipesChecked, report.RecipeMismatches)
	}
}

// schemaText is the schema of the database at path as SQLite stores it, in
// creation order, under a header naming its version.
func schemaText(t *testing.T, path string) string {
	t.Helper()
	var b strings.Builder
	fmt.Fprintf(&b, "-- The schema of a database migrated from empty, at user_version %s.\n", userVersionOf(t, path))
	b.WriteString("-- Generated by TestSchemaLatestMatchesAFreshMigration; rewrite it with\n")
	b.WriteString("-- go test ./internal/vault -run TestSchemaLatest -update\n")
	for _, statement := range queryColumn(t, path, `SELECT sql FROM sqlite_master WHERE sql IS NOT NULL ORDER BY rowid`) {
		b.WriteString("\n" + statement + ";\n")
	}
	return b.String()
}

// testdata/schema_latest.sql is the schema to read: it has to match what the
// migrations build from nothing.
func TestSchemaLatestMatchesAFreshMigration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vault.db")
	db, err := InitDB(path)
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	db.Close()

	got := schemaText(t, path)
	if *updateSchema {
		if err := os.WriteFile("testdata/schema_latest.sql", []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile("testdata/schema_latest.sql")
	if err != nil {
		t.Fatal(err)
	}
	if strings.ReplaceAll(string(want), "\r\n", "\n") != got {
		t.Errorf("testdata/schema_latest.sql is out of date; rewrite it with go test ./internal/vault -run TestSchemaLatest -update\ngot:\n%s", got)
	}
}

// listDir names the files in dir.
func listDir(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name()
	}
	return names
}

// A dry run reports both migrations over prod's database, with the recipe
// check over every catalog, and leaves the file, and the directory it sits
// in, exactly as they were.
func TestDryRunWritesNothing(t *testing.T) {
	path := fixtureDB(t)
	dir := filepath.Dir(path)
	bytesBefore, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	filesBefore := listDir(t, dir)

	report, err := DryRun(context.Background(), path, provider.SameRecipe)
	if err != nil {
		t.Fatalf("DryRun: %v", err)
	}

	want := MigrationReport{
		From: 0, To: 2,
		Applied: []AppliedMigration{
			{Version: 1, Name: "baseline", Notes: []string{
				"found 5 of 5 baseline tables already present; checked their columns",
				"collections: kept the legacy column is_default",
				"catalogs: kept the legacy column is_default",
			}},
			{Version: 2, Name: "recipes", Notes: []string{
				"stored 9 recipes for 20 catalogs",
				"rewrote the params of 0 catalogs in canonical form",
				"remapped catalog links: 3 in step, 1 behind their source, 0 edited since taken, 0 matching neither hash (left as they were)",
				"remapped collection links: 2 in step, 1 behind their source, 0 edited since taken, 0 matching neither hash (left as they were)",
			}},
		},
		RowsBefore:     map[string]int{"profiles": 3, "collections": 6, "catalogs": 20, "folders": 11, "folder_catalogs": 18},
		RowsAfter:      map[string]int{"profiles": 3, "collections": 6, "catalogs": 20, "folders": 11, "folder_catalogs": 18, "recipes": 9},
		RecipesChecked: 20,
	}
	if !reflect.DeepEqual(report, want) {
		t.Errorf("report = %+v\nwant %+v", report, want)
	}
	if bytesAfter, err := os.ReadFile(path); err != nil || !bytes.Equal(bytesAfter, bytesBefore) {
		t.Errorf("the database file changed (read error %v)", err)
	}
	if filesAfter := listDir(t, dir); !slices.Equal(filesAfter, filesBefore) {
		t.Errorf("directory = %q after the dry run, want %q", filesAfter, filesBefore)
	}
}

// The recipe check names each catalog whose params before and after differ
// by its lights, and checks nothing once the database is past migration 2.
func TestDryRunReportsRecipeMismatches(t *testing.T) {
	const odd = "cacacaca-0000-4000-8000-000000000008"
	path := fixtureDB(t)
	report, err := DryRun(context.Background(), path, func(catalogType, _, before, after string) error {
		if catalogType == "series" && strings.Contains(before, "first_air_date") {
			return errors.New("boom")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("DryRun: %v", err)
	}
	if want := []string{"catalog " + odd + " (Idea Board): boom"}; report.RecipesChecked != 20 || !slices.Equal(report.RecipeMismatches, want) {
		t.Errorf("recipe check: %d checked, mismatches %q; want 20 and %q", report.RecipesChecked, report.RecipeMismatches, want)
	}

	if err := migrate(context.Background(), path, migrations.All()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	report, err = DryRun(context.Background(), path, func(string, string, string, string) error {
		t.Error("the recipe check ran on a database past migration 2")
		return nil
	})
	if err != nil || report.RecipesChecked != 0 || report.From != 2 || report.To != 2 {
		t.Errorf("dry run past migration 2 = %+v, %v; want nothing applied or checked", report, err)
	}
}

// The -wal and -shm files of a database something else has open are theirs,
// and stay.
func TestDryRunKeepsSidecarsItDidNotCreate(t *testing.T) {
	path := fixtureDB(t)
	open, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer open.Close()
	if _, err := open.Exec(`SELECT count(*) FROM profiles`); err != nil {
		t.Fatal(err)
	}

	if _, err := DryRun(context.Background(), path, provider.SameRecipe); err != nil {
		t.Fatalf("DryRun: %v", err)
	}
	for _, sidecar := range []string{path + "-wal", path + "-shm"} {
		if _, err := os.Stat(sidecar); err != nil {
			t.Errorf("%s after the dry run: %v, want it kept", filepath.Base(sidecar), err)
		}
	}
}

// A dry run that can't read the file, finds a newer schema, or hits a failing
// migration says so, and still writes nothing. A failing migration's report
// still covers the ones applied before it.
func TestDryRunFailures(t *testing.T) {
	ctx := context.Background()
	failing := []migrations.Migration{step(1, `CREATE TABLE t (v INTEGER)`), step(2, `INSERT INTO missing VALUES (1)`)}
	for _, tc := range []struct {
		name  string
		setup string
		list  []migrations.Migration
		want  string
		// report is the partial report expected alongside the error.
		report MigrationReport
	}{
		{name: "missing file", want: "reading the database"},
		{
			name: "newer schema", setup: `PRAGMA user_version = 9`, list: failing,
			want:   "schema version 9, which this build (versions 0 to 2) cannot read",
			report: MigrationReport{From: 9, RowsBefore: map[string]int{}},
		},
		{
			name: "failing migration", setup: `CREATE TABLE kept (v INTEGER)`, list: failing, want: "no such table: missing",
			report: MigrationReport{
				From: 0, To: 1,
				Applied:    []AppliedMigration{{Version: 1, Name: "step 1", Notes: []string{"ran step 1"}}},
				RowsBefore: map[string]int{"kept": 0},
				RowsAfter:  map[string]int{"kept": 0, "t": 0},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "vault.db")
			if tc.setup != "" {
				execSQL(t, path, tc.setup)
			}
			filesBefore := listDir(t, filepath.Dir(path))
			report, err := dryRun(ctx, path, tc.list, provider.SameRecipe)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("dryRun = %v, want an error containing %q", err, tc.want)
			}
			if !reflect.DeepEqual(report, tc.report) {
				t.Errorf("report = %+v\nwant %+v", report, tc.report)
			}
			if filesAfter := listDir(t, filepath.Dir(path)); !slices.Equal(filesAfter, filesBefore) {
				t.Errorf("directory = %q after the dry run, want %q", filesAfter, filesBefore)
			}
		})
	}
}
