package migrations

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// fixtureAtRecipes opens the prod-shaped fixture with migrations 1 and 2
// applied, then changed by statements, which may be empty.
func fixtureAtRecipes(t *testing.T, statements string) *sql.DB {
	t.Helper()
	d := fixtureAtBaseline(t, "")
	if _, err := runRecipes(t, d); err != nil {
		t.Fatalf("recipes: %v", err)
	}
	if statements != "" {
		if _, err := d.Exec(statements); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}
	return d
}

// runPublications runs migration 3 in a transaction on d and commits it when
// it succeeds.
func runPublications(t *testing.T, d *sql.DB) ([]string, error) {
	t.Helper()
	tx, err := d.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	notes, err := publications(context.Background(), tx)
	if err != nil {
		return notes, err
	}
	return notes, tx.Commit()
}

// The fixture's rows, by the readable part of their ids.
const (
	alice = "aaaaaaaa-0000-4000-8000-000000000001"
	bob   = "aaaaaaaa-0000-4000-8000-000000000002"
	carol = "aaaaaaaa-0000-4000-8000-000000000003"
)

func catalogID(n string) string    { return "cacacaca-0000-4000-8000-0000000000" + n }
func collectionID(n string) string { return "cccccccc-0000-4000-8000-0000000000" + n }
func folderID(n string) string     { return "ffffffff-0000-4000-8000-0000000000" + n }

// On an empty database migration 3 rebuilds catalogs and collections without
// their sharing columns and adds the publication tables, indexes and
// triggers.
func TestPublicationsOnAnEmptyDatabase(t *testing.T) {
	d := openTestDB(t, "")
	if _, err := runBaseline(t, d); err != nil {
		t.Fatalf("baseline: %v", err)
	}
	if _, err := runRecipes(t, d); err != nil {
		t.Fatalf("recipes: %v", err)
	}
	notes, err := runPublications(t, d)
	if err != nil {
		t.Fatalf("publications: %v", err)
	}
	if want := "published 0 catalogs and 0 collections: every public row that was not a linked copy, as it stood"; notes[0] != want {
		t.Errorf("notes[0] = %q, want %q", notes[0], want)
	}
	if want := "dropped is_public, taken_from and taken_hash from catalogs and collections"; notes[len(notes)-1] != want {
		t.Errorf("last note = %q, want %q", notes[len(notes)-1], want)
	}
	for table, want := range map[string][]string{
		"catalogs":    {"id", "name", "recipe_hash", "owner_id", "collection_id", "home_sort_order", "show_in_home", "sub_key", "created_at", "updated_at"},
		"collections": {"id", "title", "owner_id", "pin_to_top", "view_mode", "show_all_tab", "backdrop_image_url", "focus_glow_enabled", "home_sort_order", "version", "pushed_version", "created_at", "updated_at"},
	} {
		if got := queryStrings(t, d, `SELECT name FROM pragma_table_info(?) ORDER BY cid`, table); !slices.Equal(got, want) {
			t.Errorf("%s' columns = %q, want %q", table, got, want)
		}
	}
	if got := queryStrings(t, d, `SELECT name FROM pragma_table_info('folders') WHERE name = 'sub_key'`); len(got) != 1 {
		t.Error("folders has no sub_key")
	}
	objects := queryStrings(t, d, `SELECT type || ' ' || name FROM sqlite_master
		WHERE name LIKE 'publications%' OR name LIKE 'subscriptions%' OR name LIKE 'catalogs%' OR name LIKE 'collections%' OR name LIKE 'recipes_%'
		ORDER BY type, name`)
	want := []string{
		"index catalogs_by_collection", "index catalogs_by_owner", "index catalogs_by_recipe", "index collections_by_owner",
		"index publications_by_catalog", "index publications_by_collection",
		"index subscriptions_by_catalog", "index subscriptions_by_collection",
		"table catalogs", "table collections", "table publications", "table subscriptions",
		"trigger publications_withdraw_on_scope", "trigger publications_withdraw_on_source_delete",
		"trigger recipes_drop_unused_on_delete", "trigger recipes_drop_unused_on_repoint",
		"trigger subscriptions_count_on_delete", "trigger subscriptions_count_on_insert",
	}
	if !slices.Equal(objects, want) {
		t.Errorf("objects =\n%q\nwant\n%q", objects, want)
	}
}

// Over prod's data, migration 3 publishes the public originals, turns the
// links to them into subscriptions, drops every other link, and keeps every
// id, version and pushed_version.
func TestPublicationsOverTheProdShapedFixture(t *testing.T) {
	d := fixtureAtRecipes(t, "")
	before := queryStrings(t, d, `
		SELECT 'catalog ' || id || ' ' || name || ' ' || coalesce(collection_id, '-') || ' ' || coalesce(home_sort_order, '-') || ' ' || show_in_home FROM catalogs
		UNION ALL SELECT 'collection ' || id || ' ' || version || ' ' || coalesce(pushed_version, '-') || ' ' || coalesce(home_sort_order, '-') || ' ' || pin_to_top FROM collections
		UNION ALL SELECT 'folder ' || id || ' ' || collection_id || ' ' || sort_order FROM folders
		ORDER BY 1`)
	notes, err := runPublications(t, d)
	if err != nil {
		t.Fatalf("publications: %v", err)
	}
	want := []string{
		"published 2 catalogs and 2 collections: every public row that was not a linked copy, as it stood",
		"left 2 public linked copies unpublished: each became a subscription or a detached copy",
		"left 0 public collections unpublished: each uses a catalog that became a subscription, which only its publisher can share",
		"catalog links: 2 became subscriptions (1 in step, 0 of those only because the copy already equals its source; 1 out of step); detached 2 (1 from a private source, 1 from a copy, 0 from a public source left unpublished)",
		"collection links: 2 became subscriptions (1 in step, 0 of those only because the copy already equals its source; 1 out of step); detached 1 (0 from a private source, 1 from a copy, 0 from a public source left unpublished)",
		"dropped is_public, taken_from and taken_hash from catalogs and collections, and the legacy is_default columns",
	}
	if !slices.Equal(notes, want) {
		t.Errorf("notes =\n%q\nwant\n%q", notes, want)
	}
	after := queryStrings(t, d, `
		SELECT 'catalog ' || id || ' ' || name || ' ' || coalesce(collection_id, '-') || ' ' || coalesce(home_sort_order, '-') || ' ' || show_in_home FROM catalogs
		UNION ALL SELECT 'collection ' || id || ' ' || version || ' ' || coalesce(pushed_version, '-') || ' ' || coalesce(home_sort_order, '-') || ' ' || pin_to_top FROM collections
		UNION ALL SELECT 'folder ' || id || ' ' || collection_id || ' ' || sort_order FROM folders
		ORDER BY 1`)
	if !slices.Equal(after, before) {
		t.Errorf("rows after =\n%q\nwant them as they were\n%q", after, before)
	}

	pubs := queryStrings(t, d, `
		SELECT kind || ' ' || coalesce(catalog_id, collection_id) || ' ' || owner_id || ' ' || title || ' ' ||
		       catalog_count || ' ' || folder_count || ' ' || subscriber_count || ' ' || status
		FROM publications ORDER BY kind, coalesce(catalog_id, collection_id)`)
	wantPubs := []string{
		"catalog " + catalogID("01") + " " + alice + " Popular Movies 1 0 1 live",
		"catalog " + catalogID("02") + " " + alice + " Top Rated Series 1 0 1 live",
		"collection " + collectionID("01") + " " + alice + " Weekend 3 2 1 live",
		"collection " + collectionID("02") + " " + alice + " Horror Night 2 2 1 live",
	}
	if !slices.Equal(pubs, wantPubs) {
		t.Errorf("publications =\n%q\nwant\n%q", pubs, wantPubs)
	}

	subs := queryStrings(t, d, `
		SELECT coalesce(s.catalog_id, s.collection_id) || ' ' || s.owner_id || ' → ' || coalesce(p.catalog_id, p.collection_id) || ' ' ||
		       CASE s.taken_hash WHEN p.content_hash THEN 'in step' ELSE s.taken_hash END
		FROM subscriptions s JOIN publications p ON p.id = s.publication_id ORDER BY 1`)
	wantSubs := []string{
		catalogID("09") + " " + bob + " → " + catalogID("01") + " in step",
		catalogID("10") + " " + bob + " → " + catalogID("02") + " " + outOfStep,
		collectionID("04") + " " + bob + " → " + collectionID("01") + " in step",
		collectionID("05") + " " + bob + " → " + collectionID("02") + " " + outOfStep,
	}
	if !slices.Equal(subs, wantSubs) {
		t.Errorf("subscriptions =\n%q\nwant\n%q", subs, wantSubs)
	}

	// Each sub_key is the key the publication's snapshot gives the source
	// row the copy was taken from, or paired with by position.
	keys := queryStrings(t, d, `
		SELECT c.id || ' ' || (c.sub_key = k.value ->> 'key') FROM catalogs c
		JOIN subscriptions s ON s.collection_id = c.collection_id JOIN publications p ON p.id = s.publication_id
		JOIN json_each(p.snapshot, '$.catalogs') k ON k.value ->> 'name' = c.name
		UNION ALL SELECT f.id || ' ' || (f.sub_key = k.value ->> 'key') FROM folders f
		JOIN subscriptions s ON s.collection_id = f.collection_id JOIN publications p ON p.id = s.publication_id
		JOIN json_each(p.snapshot, '$.collection.folders') k ON k.key = f.sort_order
		ORDER BY 1`)
	wantKeys := []string{
		catalogID("12") + " 1", catalogID("13") + " 1", catalogID("14") + " 1", catalogID("15") + " 1", catalogID("16") + " 1",
		folderID("06") + " 1", folderID("07") + " 1", folderID("08") + " 1", folderID("09") + " 1",
	}
	if !slices.Equal(keys, wantKeys) {
		t.Errorf("sub_keys matching the snapshot = %q, want %q", keys, wantKeys)
	}
	if got := queryStrings(t, d, `SELECT id FROM catalogs WHERE sub_key IS NOT NULL AND collection_id NOT IN (SELECT collection_id FROM subscriptions WHERE collection_id IS NOT NULL)`); len(got) != 0 {
		t.Errorf("catalogs outside a subscribed collection with a sub_key: %q", got)
	}
}

// A publication's snapshot holds the source as it stood, under stable keys.
// Weekend references Alice's listed Popular Movies twice and her private
// Private Picks, and each is in the snapshot once, in the order the folders
// first reference it.
func TestPublicationsSnapshotTheSource(t *testing.T) {
	d := fixtureAtRecipes(t, "")
	if _, err := runPublications(t, d); err != nil {
		t.Fatalf("publications: %v", err)
	}
	var id, raw string
	if err := d.QueryRow(`SELECT id, snapshot FROM publications WHERE collection_id = ?`, collectionID("01")).Scan(&id, &raw); err != nil {
		t.Fatal(err)
	}
	if id != derivedID("uno-publication/collection/"+collectionID("01")) {
		t.Errorf("publication id = %s, want the one derived from its source", id)
	}
	var s pubSnapshot
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, c := range s.Catalogs {
		names = append(names, c.Name+" "+c.Key+" "+string(c.Params))
	}
	wantNames := []string{
		"Popular Movies " + stableKey(id, catalogID("01")) + ` {"sort_by":"popularity.desc"}`,
		"Car Chases " + stableKey(id, catalogID("05")) + ` {"sort_by":"popularity.desc","with_keywords":"9748"}`,
		"Private Picks " + stableKey(id, catalogID("03")) + ` {"sort_by":"revenue.desc","with_original_language":"ko"}`,
	}
	if !slices.Equal(names, wantNames) {
		t.Errorf("snapshot catalogs =\n%q\nwant\n%q", names, wantNames)
	}
	f := s.Collection.Folders[1]
	if f.Key != stableKey(id, folderID("02")) || f.Title != "Hidden Gems" || len(f.Refs) != 2 ||
		f.Refs[0] != (pubRef{Catalog: stableKey(id, catalogID("03")), Genre: "Drama"}) {
		t.Errorf("second folder = %+v", f)
	}
	if s.Format != "uno-publication" || s.Version != 1 || s.Collection.BackdropImageURL != "https://image.tmdb.org/t/p/original/weekend.jpg" {
		t.Errorf("snapshot = %s", raw)
	}
}

// A copy whose taken_hash matches neither its source nor itself, as a hash
// computed under an older rule does, is still in step when it already
// equals its source, and is counted as such.
func TestPublicationsCountsIdenticalCopies(t *testing.T) {
	d := fixtureAtRecipes(t, `
		UPDATE collections SET taken_hash = 'stale' WHERE id = '`+collectionID("04")+`';
		UPDATE catalogs SET taken_hash = 'stale' WHERE id = '`+catalogID("09")+`';`)
	notes, err := runPublications(t, d)
	if err != nil {
		t.Fatalf("publications: %v", err)
	}
	for _, want := range []string{
		"catalog links: 2 became subscriptions (1 in step, 1 of those only because the copy already equals its source; 1 out of step); detached 2 (1 from a private source, 1 from a copy, 0 from a public source left unpublished)",
		"collection links: 2 became subscriptions (1 in step, 1 of those only because the copy already equals its source; 1 out of step); detached 1 (0 from a private source, 1 from a copy, 0 from a public source left unpublished)",
	} {
		if !slices.Contains(notes, want) {
			t.Errorf("notes = %q, want %q", notes, want)
		}
	}
	inStep := queryStrings(t, d, `SELECT coalesce(s.catalog_id, s.collection_id) FROM subscriptions s JOIN publications p ON p.id = s.publication_id
		WHERE s.taken_hash = p.content_hash ORDER BY 1`)
	if want := []string{catalogID("09"), collectionID("04")}; !slices.Equal(inStep, want) {
		t.Errorf("in-step copies = %q, want %q", inStep, want)
	}
}

// A public collection that uses a catalog becoming a subscription stays
// unpublished, and a link to it is dropped. Bob's Bob Picks uses his copy of
// Alice's Popular Movies, and Carol copied Bob Picks.
func TestPublicationsWithholdACollectionUsingASubscribedCatalog(t *testing.T) {
	d := fixtureAtRecipes(t, `
		INSERT INTO collections (id, title, owner_id, is_public, taken_from, taken_hash, created_at, updated_at) VALUES
			('`+collectionID("07")+`', 'Bob Picks', '`+bob+`', 1, NULL, NULL, '2026-09-27T16:24:21Z', '2026-09-27T16:24:21Z'),
			('`+collectionID("08")+`', 'Bob Picks', '`+carol+`', 0, '`+collectionID("07")+`', 'stale', '2026-09-27T16:24:21Z', '2026-09-27T16:24:21Z');
		INSERT INTO folders (id, collection_id, title, sort_order) VALUES ('`+folderID("12")+`', '`+collectionID("07")+`', 'Picks', 0);
		INSERT INTO folder_catalogs (folder_id, catalog_id, sort_order, genre) VALUES ('`+folderID("12")+`', '`+catalogID("09")+`', 0, '');`)
	notes, err := runPublications(t, d)
	if err != nil {
		t.Fatalf("publications: %v", err)
	}
	for _, want := range []string{
		"published 2 catalogs and 2 collections: every public row that was not a linked copy, as it stood",
		"left 1 public collections unpublished: each uses a catalog that became a subscription, which only its publisher can share",
		"collection links: 2 became subscriptions (1 in step, 0 of those only because the copy already equals its source; 1 out of step); detached 2 (0 from a private source, 1 from a copy, 1 from a public source left unpublished)",
	} {
		if !slices.Contains(notes, want) {
			t.Errorf("notes = %q, want %q", notes, want)
		}
	}
	if got := queryStrings(t, d, `SELECT collection_id FROM publications WHERE collection_id IN ('`+collectionID("07")+`')
		UNION ALL SELECT collection_id FROM subscriptions WHERE collection_id IN ('`+collectionID("08")+`')`); len(got) != 0 {
		t.Errorf("publications and subscriptions of Bob Picks = %q, want none", got)
	}
}

// A snapshot that doesn't encode fails the migration, naming its source.
func TestPublicationsRefusesASnapshotThatDoesNotEncode(t *testing.T) {
	for _, source := range []string{catalogID("01"), catalogID("05")} {
		d := fixtureAtRecipes(t, `UPDATE recipes SET params = 'not json' WHERE hash = (SELECT recipe_hash FROM catalogs WHERE id = '`+source+`')`)
		if _, err := runPublications(t, d); err == nil || !strings.Contains(err.Error(), "writing the snapshot of") {
			t.Errorf("publications with %s's params broken = %v, want the snapshot error", source, err)
		}
	}
}

// openMigrated is the fixture migrated to version 3, reopened with foreign
// keys on as the vault's pool opens it.
func openMigrated(t *testing.T) *sql.DB {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "vault.db")
	d := fixtureAtRecipes(t, "")
	if _, err := runPublications(t, d); err != nil {
		t.Fatalf("publications: %v", err)
	}
	if _, err := d.Exec(`VACUUM INTO ?`, path); err != nil {
		t.Fatal(err)
	}
	live, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(on)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { live.Close() })
	return live
}

// publicationState is each publication's source, status and subscriber
// count.
func publicationState(t *testing.T, d *sql.DB) []string {
	t.Helper()
	return queryStrings(t, d, `
		SELECT title || ' ' || coalesce(catalog_id, collection_id, '-') || ' ' || status || ' ' || subscriber_count
		FROM publications ORDER BY kind, title`)
}

// The triggers keep publications consistent through a cascade: deleting a
// source withdraws its publication, a scope change withdraws a listed
// catalog's, a catalog moved back to the library withdraws nothing, and
// deleting a copy removes its subscription from the count.
func TestPublicationTriggers(t *testing.T) {
	d := openMigrated(t)
	d.SetMaxOpenConns(1)
	mustExec(t, d, `DELETE FROM catalogs WHERE id = '`+catalogID("01")+`'`)
	mustExec(t, d, `UPDATE catalogs SET collection_id = '`+collectionID("03")+`' WHERE id = '`+catalogID("02")+`'`)
	mustExec(t, d, `UPDATE catalogs SET collection_id = NULL WHERE id = '`+catalogID("05")+`'`)
	mustExec(t, d, `DELETE FROM collections WHERE id = '`+collectionID("04")+`'`)
	got := publicationState(t, d)
	want := []string{
		"Popular Movies - withdrawn 1",
		"Top Rated Series " + catalogID("02") + " withdrawn 1",
		"Horror Night " + collectionID("02") + " live 1",
		"Weekend " + collectionID("01") + " live 0",
	}
	if !slices.Equal(got, want) {
		t.Errorf("after =\n%q\nwant\n%q", got, want)
	}
}

func mustExec(t *testing.T, d *sql.DB, statements string) {
	t.Helper()
	if _, err := d.Exec(statements); err != nil {
		t.Fatalf("%s: %v", statements, err)
	}
}

// A derived id is a version 8 UUID, the same for the same seed; a stable key
// is 16 hex digits.
func TestDerivedIDAndStableKey(t *testing.T) {
	id := derivedID("seed")
	if !regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-8[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`).MatchString(id) || id != derivedID("seed") || id == derivedID("other") {
		t.Errorf("derivedID = %s", id)
	}
	if key := stableKey("p", "s"); len(key) != 16 || key != shaHex([]byte("ps"))[:16] {
		t.Errorf("stableKey = %s", key)
	}
}
