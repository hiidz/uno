package migrations

import (
	"context"
	"database/sql"
	"os"
	"slices"
	"strings"
	"testing"
)

// fixtureAtBaseline opens the prod-shaped fixture with the baseline applied,
// then changed by statements, which may be empty.
func fixtureAtBaseline(t *testing.T, statements string) *sql.DB {
	t.Helper()
	fixture, err := os.ReadFile("../testdata/schema_v1.sql")
	if err != nil {
		t.Fatal(err)
	}
	d := openTestDB(t, string(fixture))
	if _, err := runBaseline(t, d); err != nil {
		t.Fatalf("baseline: %v", err)
	}
	if statements != "" {
		if _, err := d.Exec(statements); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}
	return d
}

// runRecipes runs migration 2 in a transaction on d and commits it when it
// succeeds.
func runRecipes(t *testing.T, d *sql.DB) ([]string, error) {
	t.Helper()
	tx, err := d.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	notes, err := recipes(context.Background(), tx)
	if err != nil {
		return notes, err
	}
	return notes, tx.Commit()
}

// queryStrings returns the first column of every row query returns.
func queryStrings(t *testing.T, d *sql.DB, query string, args ...any) []string {
	t.Helper()
	rows, err := d.Query(query, args...)
	if err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	defer rows.Close()
	var values []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			t.Fatal(err)
		}
		values = append(values, v)
	}
	return values
}

// On an empty database migration 2 creates recipes, the new column, its
// index and the triggers, and drops the columns recipes replace.
func TestRecipesOnAnEmptyDatabase(t *testing.T) {
	d := openTestDB(t, "")
	if _, err := runBaseline(t, d); err != nil {
		t.Fatalf("baseline: %v", err)
	}
	notes, err := runRecipes(t, d)
	if err != nil {
		t.Fatalf("recipes: %v", err)
	}
	if !slices.Contains(notes, "stored 0 recipes for 0 catalogs") {
		t.Errorf("notes = %q", notes)
	}
	got := queryStrings(t, d, `SELECT name FROM pragma_table_info('catalogs') ORDER BY cid`)
	want := []string{"id", "name", "owner_id", "is_public", "collection_id", "home_sort_order", "show_in_home", "taken_from", "taken_hash", "created_at", "updated_at", "recipe_hash"}
	if !slices.Equal(got, want) {
		t.Errorf("catalogs' columns = %q, want %q", got, want)
	}
	objects := queryStrings(t, d, `SELECT name FROM sqlite_master WHERE name LIKE 'recipes%' OR name = 'catalogs_by_recipe' ORDER BY name`)
	if want := []string{"catalogs_by_recipe", "recipes", "recipes_drop_unused_on_delete", "recipes_drop_unused_on_repoint"}; !slices.Equal(objects, want) {
		t.Errorf("objects = %q, want %q", objects, want)
	}
}

// Over prod's data migration 2 shares one recipe between the catalogs that
// ask for the same one, leaves canonical params as they were, and keeps
// every linked copy in or out of step.
func TestRecipesOverTheProdShapedFixture(t *testing.T) {
	d := fixtureAtBaseline(t, "")
	notes, err := runRecipes(t, d)
	if err != nil {
		t.Fatalf("recipes: %v", err)
	}
	want := []string{
		"stored 9 recipes for 20 catalogs",
		"rewrote the params of 0 catalogs in canonical form",
		"remapped catalog links: 3 in step, 1 behind their source, 0 edited since taken, 0 matching neither hash (left as they were)",
		"remapped collection links: 2 in step, 1 behind their source, 0 edited since taken, 0 matching neither hash (left as they were)",
	}
	if !slices.Equal(notes, want) {
		t.Errorf("notes = %q\nwant %q", notes, want)
	}
	popular := queryStrings(t, d, `SELECT DISTINCT recipe_hash FROM catalogs WHERE name = 'Popular Movies'`)
	if len(popular) != 1 {
		t.Errorf("Popular Movies' recipes = %q, want the one they share", popular)
	}
	if got := queryStrings(t, d, `SELECT created_at FROM recipes WHERE hash = ?`, popular[0]); !slices.Equal(got, []string{"2026-09-27T16:24:21Z"}) {
		t.Errorf("recipe created_at = %q, want its oldest catalog's", got)
	}
}

// Params are rewritten in canonical form, and each unknown key dropped is
// named with the catalogs it was dropped from. A copy edited without being
// unlinked keeps pointing at its source, and a taken_hash matching neither
// the copy nor its source is left as it was.
func TestRecipesCanonicalizesAndRemaps(t *testing.T) {
	d := fixtureAtBaseline(t, `
		UPDATE catalogs SET params = '{"vote_count_gte":0,"legacy_sort":"x","sort_by":"popularity.desc"}' WHERE id = 'cacacaca-0000-4000-8000-000000000001';
		UPDATE catalogs SET params = '{"vote_average_gte":7.50,"sort_by":"first_air_date.desc","endpoint":"/discover/tv","Sort_By":"first_air_date.desc"}' WHERE id = 'cacacaca-0000-4000-8000-000000000008';
		UPDATE catalogs SET name = 'Renamed without unlinking' WHERE id = 'cacacaca-0000-4000-8000-000000000011';
		UPDATE catalogs SET taken_hash = 'unmatched' WHERE id = 'cacacaca-0000-4000-8000-000000000010';
		UPDATE collections SET title = 'Edited without unlinking' WHERE id = 'cccccccc-0000-4000-8000-000000000006';
		UPDATE collections SET taken_hash = 'unmatched' WHERE id = 'cccccccc-0000-4000-8000-000000000005';
	`)
	notes, err := runRecipes(t, d)
	if err != nil {
		t.Fatalf("recipes: %v", err)
	}
	for _, want := range []string{
		"stored 9 recipes for 20 catalogs",
		"rewrote the params of 2 catalogs in canonical form",
		`dropped the unknown params key "endpoint" from 1 catalogs: cacacaca-0000-4000-8000-000000000008`,
		`dropped the unknown params key "legacy_sort" from 1 catalogs: cacacaca-0000-4000-8000-000000000001`,
		"remapped catalog links: 2 in step, 0 behind their source, 1 edited since taken, 1 matching neither hash (left as they were)",
		"remapped collection links: 1 in step, 0 behind their source, 1 edited since taken, 1 matching neither hash (left as they were)",
	} {
		if !slices.Contains(notes, want) {
			t.Errorf("notes = %q, want %q", notes, want)
		}
	}
	params := queryStrings(t, d, `SELECT r.params FROM catalogs c JOIN recipes r ON r.hash = c.recipe_hash WHERE c.id = 'cacacaca-0000-4000-8000-000000000008'`)
	if want := `{"sort_by":"first_air_date.desc","vote_average_gte":7.5}`; !slices.Equal(params, []string{want}) {
		t.Errorf("params = %q, want %s", params, want)
	}
	for _, id := range []string{"cacacaca-0000-4000-8000-000000000010", "cccccccc-0000-4000-8000-000000000005"} {
		table := map[bool]string{true: "catalogs", false: "collections"}[strings.HasPrefix(id, "cacacaca")]
		if got := queryStrings(t, d, `SELECT taken_hash FROM `+table+` WHERE id = ?`, id); !slices.Equal(got, []string{"unmatched"}) {
			t.Errorf("%s taken_hash = %q, want it left as it was", id, got)
		}
	}
}

// Params that don't decode, a type no recipe has and a provider other than
// tmdb fail the migration, naming the catalog.
func TestRecipesRefusesAnUndecodableCatalog(t *testing.T) {
	for _, tc := range []struct{ name, statement, want string }{
		{"params", `UPDATE catalogs SET params = '{"vote_count_gte":"many"}' WHERE id = 'cacacaca-0000-4000-8000-000000000008'`,
			"catalog cacacaca-0000-4000-8000-000000000008 (Idea Board): params"},
		{"type", `UPDATE catalogs SET type = 'anime' WHERE id = 'cacacaca-0000-4000-8000-000000000008'`,
			`unknown catalog type "anime"`},
		{"provider", `UPDATE catalogs SET provider = 'letterboxd' WHERE id = 'cacacaca-0000-4000-8000-000000000008'`,
			`catalog cacacaca-0000-4000-8000-000000000008 (Idea Board): params {"sort_by":"first_air_date.desc"}: unknown provider "letterboxd"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := fixtureAtBaseline(t, tc.statement)
			if _, err := runRecipes(t, d); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("recipes = %v, want an error containing %q", err, tc.want)
			}
		})
	}
}

// Each case of remapTakenHash: a taken_hash that matched the copy or its
// source becomes that row's new hash, and one that matched neither stays.
func TestRemapTakenHash(t *testing.T) {
	copyHashes, source := linkHashes{old: "copy-old", new: "copy-new"}, linkHashes{old: "source-old", new: "source-new"}
	for _, tc := range []struct {
		taken     string
		copyOld   string
		wantHash  string
		wantCase  int
		sourceOld string
	}{
		{taken: "same", copyOld: "same", sourceOld: "same", wantHash: "copy-new", wantCase: linkInStep},
		{taken: "copy-old", copyOld: "copy-old", sourceOld: "source-old", wantHash: "copy-new", wantCase: linkBehind},
		{taken: "source-old", copyOld: "copy-old", sourceOld: "source-old", wantHash: "source-new", wantCase: linkEdited},
		{taken: "other", copyOld: "copy-old", sourceOld: "source-old", wantHash: "other", wantCase: linkNeither},
	} {
		c, s := copyHashes, source
		c.old, s.old = tc.copyOld, tc.sourceOld
		if hash, linkCase := remapTakenHash(tc.taken, c, s); hash != tc.wantHash || linkCase != tc.wantCase {
			t.Errorf("remapTakenHash(%s) = %s, %d; want %s, %d", tc.taken, hash, linkCase, tc.wantHash, tc.wantCase)
		}
	}
}

// Unknown keys are the ones matching no key of the type, whatever their
// case; params that aren't an object, or aren't a TMDB recipe, have none.
func TestUnknownParamsKeys(t *testing.T) {
	if got := unknownParamsKeys("series", "tmdb", `{"Sort_By":"a","with_networks":"1","with_collection":"10","zz":0}`); !slices.Equal(got, []string{"with_collection", "zz"}) {
		t.Errorf("unknown keys = %q, want [with_collection zz]", got)
	}
	for _, params := range []string{`[]`, `null`, `not json`} {
		if got := unknownParamsKeys("movie", "tmdb", params); got != nil {
			t.Errorf("unknown keys of %s = %q, want none", params, got)
		}
	}
	for _, recipe := range [][2]string{{"anime", "tmdb"}, {"movie", "letterboxd"}} {
		if got := unknownParamsKeys(recipe[0], recipe[1], `{"a":1}`); got != nil {
			t.Errorf("unknown keys of a %s recipe = %q, want none", recipe, got)
		}
	}
}
