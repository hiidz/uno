-- The vault's schema. InitDB creates it in an empty database.

CREATE TABLE profiles (
    id                  TEXT    PRIMARY KEY,     -- UUID
    token               TEXT    NOT NULL UNIQUE, -- URL slug, e.g. /u/{token}/...
    nuvio_user_id       TEXT    NOT NULL,         -- Nuvio auth.users.id (the account)
    nuvio_profile_index INTEGER NOT NULL,         -- Nuvio profile slot, 1..6
    nuvio_profile_uuid  TEXT    NOT NULL,         -- Nuvio profile row's own id; detects slot reuse

    UNIQUE (nuvio_user_id, nuvio_profile_index),
    CHECK  (nuvio_profile_index BETWEEN 1 AND 6)
);

CREATE TABLE folders (
    id                TEXT    PRIMARY KEY,
    collection_id     TEXT    NOT NULL REFERENCES collections(id) ON DELETE CASCADE,
    title             TEXT    NOT NULL,
    sort_order        INTEGER NOT NULL,
    tile_shape        TEXT    NOT NULL,           -- POSTER | LANDSCAPE | SQUARE
    hide_title        INTEGER NOT NULL DEFAULT 0,
    cover_emoji       TEXT    NOT NULL DEFAULT '',
    cover_image_url   TEXT    NOT NULL DEFAULT '',
    focus_gif_url     TEXT    NOT NULL DEFAULT '',
    focus_gif_enabled INTEGER NOT NULL DEFAULT 1,
    hero_video_url    TEXT    NOT NULL DEFAULT '',
    hero_backdrop_url TEXT    NOT NULL DEFAULT '',
    title_logo_url    TEXT    NOT NULL DEFAULT '',
    sub_key           TEXT                        -- in a subscribed collection: its key in the snapshot
);

CREATE INDEX folders_by_collection ON folders (collection_id, sort_order);

CREATE TABLE folder_catalogs (
    folder_id  TEXT    NOT NULL REFERENCES folders(id) ON DELETE CASCADE,
    catalog_id TEXT    NOT NULL REFERENCES catalogs(id) ON DELETE CASCADE,
    sort_order INTEGER NOT NULL,
    genre      TEXT    NOT NULL DEFAULT '',
    PRIMARY KEY (folder_id, catalog_id, genre)
);

CREATE INDEX folder_catalogs_by_order ON folder_catalogs (folder_id, sort_order);

CREATE TABLE "collections" (
    id                 TEXT    PRIMARY KEY,
    title              TEXT    NOT NULL,
    owner_id           TEXT    NOT NULL REFERENCES profiles(id),
    pin_to_top         INTEGER NOT NULL DEFAULT 0,
    view_mode          TEXT    NOT NULL DEFAULT 'TABBED_GRID',
    show_all_tab       INTEGER NOT NULL DEFAULT 0,
    backdrop_image_url TEXT    NOT NULL DEFAULT '',
    focus_glow_enabled INTEGER NOT NULL DEFAULT 1,
    home_sort_order    INTEGER,                    -- place on Home, numbered with catalogs.home_sort_order; NULL = not on Home
    created_at         TEXT    NOT NULL,           -- RFC3339 UTC
    updated_at         TEXT    NOT NULL,           -- RFC3339 UTC
    unpublished_at     TEXT                        -- RFC3339 UTC: when the publication it was added from was unpublished; NULL once acknowledged
);

CREATE INDEX collections_by_owner ON collections (owner_id);

CREATE TABLE catalogs (
    id              TEXT    PRIMARY KEY,         -- UUID, permanent once selected
    name            TEXT    NOT NULL,
    type            TEXT    NOT NULL,            -- Stremio's word: movie | series
    provider        TEXT    NOT NULL,            -- tmdb, for now
    params          TEXT    NOT NULL,            -- canonical JSON: known keys, no zero values, keys sorted
    owner_id        TEXT    NOT NULL REFERENCES profiles(id),
    collection_id   TEXT    REFERENCES collections(id) ON DELETE CASCADE, -- NULL = listed
    home_sort_order INTEGER,                     -- place on Home, numbered with collections.home_sort_order; NULL = not on Home
    show_in_home    INTEGER NOT NULL DEFAULT 1,  -- whether the home row appears when on the TV
    sub_key         TEXT,                        -- in a subscribed collection: its key in the snapshot
    created_at      TEXT    NOT NULL,            -- RFC3339 UTC
    updated_at      TEXT    NOT NULL,            -- RFC3339 UTC
    unpublished_at  TEXT,                        -- RFC3339 UTC: when the publication it was added from was unpublished; NULL once acknowledged
    CHECK (collection_id IS NULL OR home_sort_order IS NULL)
);

CREATE INDEX catalogs_by_owner ON catalogs (owner_id);

CREATE INDEX catalogs_by_collection ON catalogs (collection_id);

CREATE TABLE publications (
    id               TEXT    PRIMARY KEY,         -- UUID, kept when an update is published
    publisher_id     TEXT    NOT NULL REFERENCES profiles(id),
    kind             TEXT    NOT NULL CHECK (kind IN ('catalog', 'collection')),
    catalog_id       TEXT    REFERENCES catalogs(id) ON DELETE CASCADE,    -- the source, for a catalog
    collection_id    TEXT    REFERENCES collections(id) ON DELETE CASCADE, -- the source, for a collection
    title            TEXT    NOT NULL,
    snapshot         TEXT    NOT NULL,            -- JSON, format uno-publication
    content_hash     TEXT    NOT NULL,            -- sha256 hex of snapshot
    catalog_count    INTEGER NOT NULL,
    folder_count     INTEGER NOT NULL,
    subscriber_count INTEGER NOT NULL DEFAULT 0,
    published_at     TEXT    NOT NULL,            -- RFC3339 UTC, when first published
    updated_at       TEXT    NOT NULL,            -- RFC3339 UTC
    CHECK ((catalog_id IS NULL) <> (collection_id IS NULL)),
    CHECK (kind = 'catalog' OR catalog_id IS NULL),
    CHECK (kind = 'collection' OR collection_id IS NULL)
);

CREATE UNIQUE INDEX publications_by_catalog ON publications (catalog_id) WHERE catalog_id IS NOT NULL;

CREATE UNIQUE INDEX publications_by_collection ON publications (collection_id) WHERE collection_id IS NOT NULL;

CREATE TABLE subscriptions (
    id             TEXT PRIMARY KEY,
    subscriber_id  TEXT NOT NULL REFERENCES profiles(id),
    publication_id TEXT NOT NULL REFERENCES publications(id) ON DELETE CASCADE,
    catalog_id     TEXT REFERENCES catalogs(id) ON DELETE CASCADE,    -- the copy, for a catalog
    collection_id  TEXT REFERENCES collections(id) ON DELETE CASCADE, -- the copy, for a collection
    subscribed_hash TEXT NOT NULL,               -- the content hash the copy was last written from
    created_at     TEXT NOT NULL,                -- RFC3339 UTC
    UNIQUE (subscriber_id, publication_id),
    CHECK ((catalog_id IS NULL) <> (collection_id IS NULL))
);

CREATE UNIQUE INDEX subscriptions_by_catalog ON subscriptions (catalog_id) WHERE catalog_id IS NOT NULL;

CREATE UNIQUE INDEX subscriptions_by_collection ON subscriptions (collection_id) WHERE collection_id IS NOT NULL;

-- Ending a publication, by unpublishing it or by deleting its source, releases
-- its subscribers ahead of the cascade that deletes their subscriptions: each
-- subscribed row becomes its subscriber's own, marked unpublished until they
-- acknowledge it, with no snapshot keys left in it.
CREATE TRIGGER publications_release_subscribers BEFORE DELETE ON publications
BEGIN
    UPDATE catalogs SET unpublished_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
    WHERE id IN (SELECT catalog_id FROM subscriptions WHERE publication_id = OLD.id);
    UPDATE collections SET unpublished_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
    WHERE id IN (SELECT collection_id FROM subscriptions WHERE publication_id = OLD.id);
    UPDATE catalogs SET sub_key = NULL
    WHERE collection_id IN (SELECT collection_id FROM subscriptions WHERE publication_id = OLD.id);
    UPDATE folders SET sub_key = NULL
    WHERE collection_id IN (SELECT collection_id FROM subscriptions WHERE publication_id = OLD.id);
END;

CREATE TRIGGER subscriptions_count_on_insert AFTER INSERT ON subscriptions
BEGIN
    UPDATE publications SET subscriber_count = subscriber_count + 1 WHERE id = NEW.publication_id;
END;

CREATE TRIGGER subscriptions_count_on_delete AFTER DELETE ON subscriptions
BEGIN
    UPDATE publications SET subscriber_count = subscriber_count - 1 WHERE id = OLD.publication_id;
END;

CREATE TABLE accounts (
    nuvio_user_id       TEXT PRIMARY KEY,  -- Nuvio auth.users.id, as profiles.nuvio_user_id
    tmdb_key_ciphertext BLOB NOT NULL,     -- nonce || AES-GCM sealed key
    tmdb_key_last4      TEXT NOT NULL,     -- the key's last four characters, shown to its owner
    updated_at          TEXT NOT NULL
);

CREATE INDEX folder_catalogs_by_catalog ON folder_catalogs (catalog_id);

-- One per profile: what its last push put in Nuvio, as one JSON document
-- (vault.PushRecord), replaced whole by each push. Nothing cascades into it
-- from catalogs or collections, so deleting a row never loses what Nuvio holds.
CREATE TABLE push_records (
    profile_id         TEXT PRIMARY KEY REFERENCES profiles(id),
    nuvio_profile_uuid TEXT NOT NULL, -- the Nuvio profile it was pushed to: profiles.nuvio_profile_uuid then
    record             TEXT NOT NULL, -- JSON, vault.PushRecord
    pushed_at          TEXT NOT NULL  -- RFC3339 UTC
);
