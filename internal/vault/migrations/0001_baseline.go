package migrations

import (
	"context"
	"database/sql"
	"fmt"
	"maps"
	"slices"
	"strings"
)

// baselineDDL is the schema every database had before versioned
// migrations, with IF NOT EXISTS throughout, so a database created before
// them (user_version 0, tables present) and an empty one take the same path.
const baselineDDL = `
CREATE TABLE IF NOT EXISTS profiles (
    id                  TEXT    PRIMARY KEY,     -- UUID
    token               TEXT    NOT NULL UNIQUE, -- URL slug, e.g. /u/{token}/...
    nuvio_user_id       TEXT    NOT NULL,         -- Nuvio auth.users.id (the account)
    nuvio_profile_index INTEGER NOT NULL,         -- Nuvio profile slot, 1..6
    nuvio_profile_uuid  TEXT    NOT NULL,         -- Nuvio profile row's own id; detects slot reuse

    UNIQUE (nuvio_user_id, nuvio_profile_index),
    CHECK  (nuvio_profile_index BETWEEN 1 AND 6)
);
CREATE TABLE IF NOT EXISTS collections (
    id                 TEXT    PRIMARY KEY,
    title              TEXT    NOT NULL,
    owner_id           TEXT    NOT NULL REFERENCES profiles(id),
    is_public          INTEGER NOT NULL DEFAULT 0,
    pin_to_top         INTEGER NOT NULL DEFAULT 0,
    view_mode          TEXT    NOT NULL DEFAULT 'TABBED_GRID',
    show_all_tab       INTEGER NOT NULL DEFAULT 0,
    backdrop_image_url TEXT    NOT NULL DEFAULT '',
    focus_glow_enabled INTEGER NOT NULL DEFAULT 1,
    home_sort_order    INTEGER,                 -- NULL = not on the TV
    version            INTEGER NOT NULL DEFAULT 1, -- +1 on every content write
    pushed_version     INTEGER,                 -- version push read and sent; NULL = never pushed
    taken_from         TEXT    REFERENCES collections(id) ON DELETE SET NULL,
    taken_hash         TEXT,                    -- the original's content hash when this link was last in step
    created_at         TEXT    NOT NULL,        -- RFC3339 UTC
    updated_at         TEXT    NOT NULL         -- RFC3339 UTC
);
CREATE INDEX IF NOT EXISTS collections_by_owner ON collections (owner_id);
CREATE UNIQUE INDEX IF NOT EXISTS collections_one_link ON collections (owner_id, taken_from) WHERE taken_from IS NOT NULL;
CREATE TABLE IF NOT EXISTS catalogs (
    id              TEXT    PRIMARY KEY,          -- UUID, permanent once selected
    type            TEXT    NOT NULL,             -- Stremio's word: movie | series
    name            TEXT    NOT NULL,
    provider        TEXT    NOT NULL,             -- tmdb, for now
    params          TEXT    NOT NULL DEFAULT '',
    owner_id        TEXT    NOT NULL REFERENCES profiles(id),
    is_public       INTEGER NOT NULL DEFAULT 0,
    collection_id   TEXT    REFERENCES collections(id) ON DELETE CASCADE, -- NULL = listed
    home_sort_order INTEGER,                    -- NULL = not on the TV
    show_in_home    INTEGER NOT NULL DEFAULT 1, -- whether the home row appears when on the TV
    taken_from      TEXT    REFERENCES catalogs(id) ON DELETE SET NULL,
    taken_hash      TEXT,                         -- the original's content hash when this link was last in step
    fingerprint     TEXT    NOT NULL,             -- sha256 hex, see internal/provider.Fingerprint
    created_at      TEXT    NOT NULL,             -- RFC3339 UTC
    updated_at      TEXT    NOT NULL,             -- RFC3339 UTC
    CHECK (collection_id IS NULL OR (is_public = 0 AND home_sort_order IS NULL))
);
CREATE INDEX IF NOT EXISTS catalogs_by_owner      ON catalogs (owner_id);
CREATE INDEX IF NOT EXISTS catalogs_by_collection ON catalogs (collection_id);
CREATE UNIQUE INDEX IF NOT EXISTS catalogs_one_link ON catalogs (owner_id, taken_from) WHERE taken_from IS NOT NULL AND collection_id IS NULL;
CREATE TABLE IF NOT EXISTS folders (
    id                TEXT    PRIMARY KEY,
    collection_id     TEXT    NOT NULL REFERENCES collections(id) ON DELETE CASCADE,
    title             TEXT    NOT NULL,
    sort_order        INTEGER NOT NULL,
    tile_shape        TEXT    NOT NULL DEFAULT 'LANDSCAPE',
    hide_title        INTEGER NOT NULL DEFAULT 0,
    cover_emoji       TEXT    NOT NULL DEFAULT '',
    cover_image_url   TEXT    NOT NULL DEFAULT '',
    focus_gif_url     TEXT    NOT NULL DEFAULT '',
    focus_gif_enabled INTEGER NOT NULL DEFAULT 1,
    hero_video_url    TEXT    NOT NULL DEFAULT '',
    hero_backdrop_url TEXT    NOT NULL DEFAULT '',
    title_logo_url    TEXT    NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS folders_by_collection ON folders (collection_id, sort_order);
CREATE TABLE IF NOT EXISTS folder_catalogs (
    folder_id  TEXT    NOT NULL REFERENCES folders(id) ON DELETE CASCADE,
    catalog_id TEXT    NOT NULL REFERENCES catalogs(id) ON DELETE CASCADE,
    sort_order INTEGER NOT NULL,
    genre      TEXT    NOT NULL DEFAULT '',
    PRIMARY KEY (folder_id, catalog_id, genre)
);
CREATE INDEX IF NOT EXISTS folder_catalogs_by_order ON folder_catalogs (folder_id, sort_order);
`

// tableShape is one table's columns, each written the way describeColumn
// prints a pragma_table_info row. Column order is not part of the shape: a
// column added by ALTER TABLE sits last whatever its place in baselineDDL.
type tableShape struct {
	table   string
	columns []string
}

// baselineShape is every column of baselineDDL's tables.
var baselineShape = []tableShape{
	{"profiles", []string{
		"id TEXT PK 1",
		"token TEXT NOT NULL",
		"nuvio_user_id TEXT NOT NULL",
		"nuvio_profile_index INTEGER NOT NULL",
		"nuvio_profile_uuid TEXT NOT NULL",
	}},
	{"collections", []string{
		"id TEXT PK 1",
		"title TEXT NOT NULL",
		"owner_id TEXT NOT NULL",
		"is_public INTEGER NOT NULL DEFAULT 0",
		"pin_to_top INTEGER NOT NULL DEFAULT 0",
		"view_mode TEXT NOT NULL DEFAULT 'TABBED_GRID'",
		"show_all_tab INTEGER NOT NULL DEFAULT 0",
		"backdrop_image_url TEXT NOT NULL DEFAULT ''",
		"focus_glow_enabled INTEGER NOT NULL DEFAULT 1",
		"home_sort_order INTEGER",
		"version INTEGER NOT NULL DEFAULT 1",
		"pushed_version INTEGER",
		"taken_from TEXT",
		"taken_hash TEXT",
		"created_at TEXT NOT NULL",
		"updated_at TEXT NOT NULL",
	}},
	{"catalogs", []string{
		"id TEXT PK 1",
		"type TEXT NOT NULL",
		"name TEXT NOT NULL",
		"provider TEXT NOT NULL",
		"params TEXT NOT NULL DEFAULT ''",
		"owner_id TEXT NOT NULL",
		"is_public INTEGER NOT NULL DEFAULT 0",
		"collection_id TEXT",
		"home_sort_order INTEGER",
		"show_in_home INTEGER NOT NULL DEFAULT 1",
		"taken_from TEXT",
		"taken_hash TEXT",
		"fingerprint TEXT NOT NULL",
		"created_at TEXT NOT NULL",
		"updated_at TEXT NOT NULL",
	}},
	{"folders", []string{
		"id TEXT PK 1",
		"collection_id TEXT NOT NULL",
		"title TEXT NOT NULL",
		"sort_order INTEGER NOT NULL",
		"tile_shape TEXT NOT NULL DEFAULT 'LANDSCAPE'",
		"hide_title INTEGER NOT NULL DEFAULT 0",
		"cover_emoji TEXT NOT NULL DEFAULT ''",
		"cover_image_url TEXT NOT NULL DEFAULT ''",
		"focus_gif_url TEXT NOT NULL DEFAULT ''",
		"focus_gif_enabled INTEGER NOT NULL DEFAULT 1",
		"hero_video_url TEXT NOT NULL DEFAULT ''",
		"hero_backdrop_url TEXT NOT NULL DEFAULT ''",
		"title_logo_url TEXT NOT NULL DEFAULT ''",
	}},
	{"folder_catalogs", []string{
		"folder_id TEXT NOT NULL PK 1",
		"catalog_id TEXT NOT NULL PK 2",
		"sort_order INTEGER NOT NULL",
		"genre TEXT NOT NULL DEFAULT '' PK 3",
	}},
}

// legacyColumns are the columns a database created before is_default was
// removed still carries. Nothing reads or writes them, and the default fills
// them on insert.
var legacyColumns = map[string]string{
	"catalogs":    "is_default INTEGER NOT NULL DEFAULT 0",
	"collections": "is_default INTEGER NOT NULL DEFAULT 0",
}

// baseline creates the schema on an empty database and adopts it on one
// created before migrations, then checks that every table has exactly the
// baseline's columns, legacy is_default aside.
func baseline(ctx context.Context, tx *sql.Tx) ([]string, error) {
	notes, err := existingTablesNote(ctx, tx)
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, baselineDDL); err != nil {
		return nil, fmt.Errorf("creating the baseline schema: %w", err)
	}
	shapeNotes, err := checkBaselineShape(ctx, tx)
	return append(notes, shapeNotes...), err
}

// existingTablesNote says how many of the baseline's tables were already
// there before baselineDDL ran, or nothing on an empty database.
func existingTablesNote(ctx context.Context, tx *sql.Tx) ([]string, error) {
	var found int
	err := tx.QueryRowContext(ctx, `
		SELECT count(*) FROM sqlite_master
		WHERE type = 'table' AND name IN ('profiles', 'collections', 'catalogs', 'folders', 'folder_catalogs')
	`).Scan(&found)
	if err != nil {
		return nil, fmt.Errorf("listing existing tables: %w", err)
	}
	if found == 0 {
		return nil, nil
	}
	return []string{fmt.Sprintf("found %d of %d baseline tables already present; checked their columns", found, len(baselineShape))}, nil
}

// checkBaselineShape compares every baseline table's columns with
// baselineShape, and fails naming every difference at once.
func checkBaselineShape(ctx context.Context, tx *sql.Tx) ([]string, error) {
	var notes, problems []string
	for _, want := range baselineShape {
		got, err := tableColumns(ctx, tx, want.table)
		if err != nil {
			return nil, err
		}
		tableNotes, tableProblems := compareShape(want, got)
		notes = append(notes, tableNotes...)
		problems = append(problems, tableProblems...)
	}
	if len(problems) > 0 {
		return nil, fmt.Errorf("the existing tables do not match the baseline schema:\n  %s", strings.Join(problems, "\n  "))
	}
	return notes, nil
}

// tableColumns reads table's columns, name to describeColumn's form.
func tableColumns(ctx context.Context, tx *sql.Tx, table string) (map[string]string, error) {
	rows, err := tx.QueryContext(ctx, `SELECT name, type, "notnull", dflt_value, pk FROM pragma_table_info(?)`, table)
	if err != nil {
		return nil, fmt.Errorf("reading %s's columns: %w", table, err)
	}
	defer func() { _ = rows.Close() }()
	columns := map[string]string{}
	for rows.Next() {
		var name, typ string
		var notNull bool
		var dflt sql.NullString
		var pk int
		if err := rows.Scan(&name, &typ, &notNull, &dflt, &pk); err != nil {
			return nil, fmt.Errorf("reading %s's columns: %w", table, err)
		}
		columns[name] = describeColumn(name, typ, notNull, dflt.String, pk)
	}
	return columns, rows.Err()
}

// describeColumn writes one column the way baselineShape lists it. SQLite
// compares type names without regard to case, so the type is upper-cased.
func describeColumn(name, typ string, notNull bool, dflt string, pk int) string {
	parts := []string{name, strings.ToUpper(typ)}
	if notNull {
		parts = append(parts, "NOT NULL")
	}
	if dflt != "" {
		parts = append(parts, "DEFAULT "+dflt)
	}
	if pk > 0 {
		parts = append(parts, fmt.Sprintf("PK %d", pk))
	}
	return strings.Join(parts, " ")
}

// compareShape returns a problem for each column of want that got lacks or
// describes differently, then judges the columns got has left over.
func compareShape(want tableShape, got map[string]string) (notes, problems []string) {
	left := maps.Clone(got)
	for _, column := range want.columns {
		name, _, _ := strings.Cut(column, " ")
		actual, ok := left[name]
		delete(left, name)
		if !ok {
			problems = append(problems, fmt.Sprintf("%s: column %s is missing", want.table, name))
		} else if actual != column {
			problems = append(problems, fmt.Sprintf("%s: column is %q, want %q", want.table, actual, column))
		}
	}
	notes, extra := judgeExtraColumns(want.table, left)
	return notes, append(problems, extra...)
}

// judgeExtraColumns returns a note for a legacy column and a problem for any
// other column the baseline doesn't have, in name order.
func judgeExtraColumns(table string, extra map[string]string) (notes, problems []string) {
	for _, name := range slices.Sorted(maps.Keys(extra)) {
		if extra[name] == legacyColumns[table] {
			notes = append(notes, fmt.Sprintf("%s: kept the legacy column %s", table, name))
			continue
		}
		problems = append(problems, fmt.Sprintf("%s: unexpected column %q", table, extra[name]))
	}
	return notes, problems
}
