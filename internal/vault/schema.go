package vault

const schema = `
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
    created_at         TEXT    NOT NULL,        -- RFC3339 UTC
    updated_at         TEXT    NOT NULL         -- RFC3339 UTC
);
CREATE INDEX IF NOT EXISTS collections_by_owner ON collections (owner_id);
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
    fingerprint     TEXT    NOT NULL,             -- sha256 hex, see internal/provider.Fingerprint
    created_at      TEXT    NOT NULL,             -- RFC3339 UTC
    updated_at      TEXT    NOT NULL,             -- RFC3339 UTC
    CHECK (collection_id IS NULL OR (is_public = 0 AND home_sort_order IS NULL))
);
CREATE INDEX IF NOT EXISTS catalogs_by_owner      ON catalogs (owner_id);
CREATE INDEX IF NOT EXISTS catalogs_by_collection ON catalogs (collection_id);
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
