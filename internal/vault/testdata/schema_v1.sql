-- A database as prod held it before versioned migrations, at user_version 0.
--
-- The tables are the schema from before is_default was removed (02a4173^).
-- taken_hash arrived later through hand-run ALTER TABLE, so it sits last in
-- both tables, and the one-link indexes were created by a later start.
--
-- The seed rows were written through live vault code and then given readable ids:
-- aaaaaaaa-… profiles, cccccccc-… collections, cacacaca-… catalogs,
-- ffffffff-… folders. Fingerprints are real provider.Fingerprint values and
-- taken_hash values are the real link hashes.
--
-- Alice (…01) publishes, Bob (…02) takes, Carol (…03) takes from Bob.
--
-- Collections:
--   …01 Weekend        Alice's, public, on home, pushed at its version. Its
--                      folders reference Popular Movies twice (unfiltered and
--                      under Comedy) and her private Private Picks (Drama).
--   …02 Horror Night   Alice's, public, on home, edited after Bob took it and
--                      after her push: version 2, pushed_version 1.
--   …03 Drafts         Alice's, private, off home, never pushed.
--   …04 Weekend        Bob's copy of …01: in step, re-shared (a public linked
--                      copy), on home, pushed.
--   …05 Horror Night   Bob's copy of …02: out of step, off home.
--   …06 Weekend        Carol's copy of …04: a copy of a copy, in step.
-- Catalogs:
--   …01 Popular Movies     Alice's, listed, public, on home.
--   …02 Top Rated Series   Alice's, listed, public, edited after Bob took it.
--   …03 Private Picks      Alice's, listed, private, on home with show_in_home 0.
--   …04 Was Public         Alice's, listed, made private after Bob took it.
--   …05 Car Chases         scoped to …01, under Action.
--   …06 Slashers           scoped to …02, under Horror; …07 Ghost Stories too.
--   …08 Idea Board         scoped to …03.
--   …09 Popular Movies     Bob's copy of …01: in step, re-shared, on home.
--   …10 Top Series         Bob's copy of …02: out of step, on home.
--   …11 Was Public         Bob's copy of …04: a link to a now-private source.
--   …12-…14                scoped to Bob's …04, each taken_from its source.
--   …15-…16                scoped to Bob's …05.
--   …17 Popular Movies     Carol's copy of …09: a copy of a copy, in step.
--   …18-…20                scoped to Carol's …06, taken_from Bob's …12-…14.

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
    is_default         INTEGER NOT NULL DEFAULT 0,
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
    is_default      INTEGER NOT NULL DEFAULT 0,
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
ALTER TABLE collections ADD COLUMN taken_hash TEXT;
ALTER TABLE catalogs ADD COLUMN taken_hash TEXT;
CREATE UNIQUE INDEX IF NOT EXISTS collections_one_link ON collections (owner_id, taken_from) WHERE taken_from IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS catalogs_one_link ON catalogs (owner_id, taken_from) WHERE taken_from IS NOT NULL AND collection_id IS NULL;

INSERT INTO profiles (id, token, nuvio_user_id, nuvio_profile_index, nuvio_profile_uuid) VALUES ('aaaaaaaa-0000-4000-8000-000000000001', 'token-alice', 'account-alice', 1, 'nuvio-profile-alice');
INSERT INTO profiles (id, token, nuvio_user_id, nuvio_profile_index, nuvio_profile_uuid) VALUES ('aaaaaaaa-0000-4000-8000-000000000002', 'token-bob', 'account-bob', 1, 'nuvio-profile-bob');
INSERT INTO profiles (id, token, nuvio_user_id, nuvio_profile_index, nuvio_profile_uuid) VALUES ('aaaaaaaa-0000-4000-8000-000000000003', 'token-carol', 'account-carol', 1, 'nuvio-profile-carol');

INSERT INTO collections (id, title, owner_id, is_public, pin_to_top, view_mode, show_all_tab, backdrop_image_url, focus_glow_enabled, home_sort_order, version, pushed_version, taken_from, taken_hash, created_at, updated_at) VALUES ('cccccccc-0000-4000-8000-000000000001', 'Weekend', 'aaaaaaaa-0000-4000-8000-000000000001', 1, 1, 'TABBED_GRID', 1, 'https://image.tmdb.org/t/p/original/weekend.jpg', 1, 0, 1, 1, NULL, NULL, '2026-09-27T16:24:21Z', '2026-09-27T16:24:21Z');
INSERT INTO collections (id, title, owner_id, is_public, pin_to_top, view_mode, show_all_tab, backdrop_image_url, focus_glow_enabled, home_sort_order, version, pushed_version, taken_from, taken_hash, created_at, updated_at) VALUES ('cccccccc-0000-4000-8000-000000000002', 'Horror Night', 'aaaaaaaa-0000-4000-8000-000000000001', 1, 0, 'ROWS', 0, '', 1, 1, 2, 1, NULL, NULL, '2026-09-27T16:24:21Z', '2026-09-27T16:24:21Z');
INSERT INTO collections (id, title, owner_id, is_public, pin_to_top, view_mode, show_all_tab, backdrop_image_url, focus_glow_enabled, home_sort_order, version, pushed_version, taken_from, taken_hash, created_at, updated_at) VALUES ('cccccccc-0000-4000-8000-000000000003', 'Drafts', 'aaaaaaaa-0000-4000-8000-000000000001', 0, 0, 'TABBED_GRID', 0, '', 0, NULL, 1, NULL, NULL, NULL, '2026-09-27T16:24:21Z', '2026-09-27T16:24:21Z');
INSERT INTO collections (id, title, owner_id, is_public, pin_to_top, view_mode, show_all_tab, backdrop_image_url, focus_glow_enabled, home_sort_order, version, pushed_version, taken_from, taken_hash, created_at, updated_at) VALUES ('cccccccc-0000-4000-8000-000000000004', 'Weekend', 'aaaaaaaa-0000-4000-8000-000000000002', 1, 0, 'TABBED_GRID', 1, 'https://image.tmdb.org/t/p/original/weekend.jpg', 1, 0, 2, 2, 'cccccccc-0000-4000-8000-000000000001', '07efc100546275e87a8dc084dc6b2eed6baf10aaa20d27977f5295feca767a3b', '2026-09-27T16:24:21Z', '2026-09-27T16:24:21Z');
INSERT INTO collections (id, title, owner_id, is_public, pin_to_top, view_mode, show_all_tab, backdrop_image_url, focus_glow_enabled, home_sort_order, version, pushed_version, taken_from, taken_hash, created_at, updated_at) VALUES ('cccccccc-0000-4000-8000-000000000005', 'Horror Night', 'aaaaaaaa-0000-4000-8000-000000000002', 0, 0, 'ROWS', 0, '', 1, NULL, 1, NULL, 'cccccccc-0000-4000-8000-000000000002', 'd9f320208e1d6e01b6e831aac861d1b03b6f74132b2fb2c3c2d5e68ec940106b', '2026-09-27T16:24:21Z', '2026-09-27T16:24:21Z');
INSERT INTO collections (id, title, owner_id, is_public, pin_to_top, view_mode, show_all_tab, backdrop_image_url, focus_glow_enabled, home_sort_order, version, pushed_version, taken_from, taken_hash, created_at, updated_at) VALUES ('cccccccc-0000-4000-8000-000000000006', 'Weekend', 'aaaaaaaa-0000-4000-8000-000000000003', 0, 0, 'TABBED_GRID', 1, 'https://image.tmdb.org/t/p/original/weekend.jpg', 1, NULL, 1, NULL, 'cccccccc-0000-4000-8000-000000000004', '07efc100546275e87a8dc084dc6b2eed6baf10aaa20d27977f5295feca767a3b', '2026-09-27T16:24:21Z', '2026-09-27T16:24:21Z');

INSERT INTO catalogs (id, type, name, provider, params, owner_id, is_public, collection_id, home_sort_order, show_in_home, taken_from, taken_hash, fingerprint, created_at, updated_at) VALUES ('cacacaca-0000-4000-8000-000000000001', 'movie', 'Popular Movies', 'tmdb', '{"sort_by":"popularity.desc"}', 'aaaaaaaa-0000-4000-8000-000000000001', 1, NULL, 0, 1, NULL, NULL, '7cfd3b841d8a78518650b7bbaef1dedf98a40546c2fe76242f9f81126e93c0b8', '2026-09-27T16:24:21Z', '2026-09-27T16:24:21Z');
INSERT INTO catalogs (id, type, name, provider, params, owner_id, is_public, collection_id, home_sort_order, show_in_home, taken_from, taken_hash, fingerprint, created_at, updated_at) VALUES ('cacacaca-0000-4000-8000-000000000002', 'series', 'Top Rated Series', 'tmdb', '{"sort_by":"vote_average.desc","vote_count_gte":500}', 'aaaaaaaa-0000-4000-8000-000000000001', 1, NULL, NULL, 1, NULL, NULL, '87c51e64c77342f2dd5aaaaa11a46738870b44e4a95be2384d4f559bd226311f', '2026-09-27T16:24:21Z', '2026-09-27T16:24:21Z');
INSERT INTO catalogs (id, type, name, provider, params, owner_id, is_public, collection_id, home_sort_order, show_in_home, taken_from, taken_hash, fingerprint, created_at, updated_at) VALUES ('cacacaca-0000-4000-8000-000000000003', 'movie', 'Private Picks', 'tmdb', '{"sort_by":"revenue.desc","with_original_language":"ko"}', 'aaaaaaaa-0000-4000-8000-000000000001', 0, NULL, 1, 0, NULL, NULL, 'f195de9fec8fbb7d7b5499373eb6282d3371b5df670eceade69e1315e5bb3ace', '2026-09-27T16:24:21Z', '2026-09-27T16:24:21Z');
INSERT INTO catalogs (id, type, name, provider, params, owner_id, is_public, collection_id, home_sort_order, show_in_home, taken_from, taken_hash, fingerprint, created_at, updated_at) VALUES ('cacacaca-0000-4000-8000-000000000004', 'movie', 'Was Public', 'tmdb', '{"sort_by":"primary_release_date.desc","with_genres":"99"}', 'aaaaaaaa-0000-4000-8000-000000000001', 0, NULL, NULL, 1, NULL, NULL, '07341fa15a92862550c6aa890d656567438d466bcde451f254360c4e2545e924', '2026-09-27T16:24:21Z', '2026-09-27T16:24:21Z');
INSERT INTO catalogs (id, type, name, provider, params, owner_id, is_public, collection_id, home_sort_order, show_in_home, taken_from, taken_hash, fingerprint, created_at, updated_at) VALUES ('cacacaca-0000-4000-8000-000000000005', 'movie', 'Car Chases', 'tmdb', '{"sort_by":"popularity.desc","with_keywords":"9748"}', 'aaaaaaaa-0000-4000-8000-000000000001', 0, 'cccccccc-0000-4000-8000-000000000001', NULL, 1, NULL, NULL, '53cb45abdd16576f4a9524cd520c7e3509855f44c2a37c85b7714c894d6a8af0', '2026-09-27T16:24:21Z', '2026-09-27T16:24:21Z');
INSERT INTO catalogs (id, type, name, provider, params, owner_id, is_public, collection_id, home_sort_order, show_in_home, taken_from, taken_hash, fingerprint, created_at, updated_at) VALUES ('cacacaca-0000-4000-8000-000000000006', 'movie', 'Slashers', 'tmdb', '{"sort_by":"popularity.desc","with_genres":"27"}', 'aaaaaaaa-0000-4000-8000-000000000001', 0, 'cccccccc-0000-4000-8000-000000000002', NULL, 1, NULL, NULL, 'f79f5f804daa40adf0e7c71c239cc7fbd6ca2c273ab55658bab14ecd695482f7', '2026-09-27T16:24:21Z', '2026-09-27T16:24:21Z');
INSERT INTO catalogs (id, type, name, provider, params, owner_id, is_public, collection_id, home_sort_order, show_in_home, taken_from, taken_hash, fingerprint, created_at, updated_at) VALUES ('cacacaca-0000-4000-8000-000000000007', 'series', 'Ghost Stories', 'tmdb', '{"sort_by":"popularity.desc","with_keywords":"162846"}', 'aaaaaaaa-0000-4000-8000-000000000001', 0, 'cccccccc-0000-4000-8000-000000000002', NULL, 1, NULL, NULL, '81735f9e7e2e7b73bbed32ca9190bf2aaa8a763562e943c36a7222b2d5a72969', '2026-09-27T16:24:21Z', '2026-09-27T16:24:21Z');
INSERT INTO catalogs (id, type, name, provider, params, owner_id, is_public, collection_id, home_sort_order, show_in_home, taken_from, taken_hash, fingerprint, created_at, updated_at) VALUES ('cacacaca-0000-4000-8000-000000000008', 'series', 'Idea Board', 'tmdb', '{"sort_by":"first_air_date.desc"}', 'aaaaaaaa-0000-4000-8000-000000000001', 0, 'cccccccc-0000-4000-8000-000000000003', NULL, 1, NULL, NULL, '98b8ebd0b0a8959b82bae1733b3d2836b7159cf2930fc20f7942784e75bfce51', '2026-09-27T16:24:21Z', '2026-09-27T16:24:21Z');
INSERT INTO catalogs (id, type, name, provider, params, owner_id, is_public, collection_id, home_sort_order, show_in_home, taken_from, taken_hash, fingerprint, created_at, updated_at) VALUES ('cacacaca-0000-4000-8000-000000000009', 'movie', 'Popular Movies', 'tmdb', '{"sort_by":"popularity.desc"}', 'aaaaaaaa-0000-4000-8000-000000000002', 1, NULL, 0, 1, 'cacacaca-0000-4000-8000-000000000001', '1da4fa7583d3b5d45d94e9cac5a150edda9c252639015c8bda79a4ffac92b12e', '7cfd3b841d8a78518650b7bbaef1dedf98a40546c2fe76242f9f81126e93c0b8', '2026-09-27T16:24:21Z', '2026-09-27T16:24:21Z');
INSERT INTO catalogs (id, type, name, provider, params, owner_id, is_public, collection_id, home_sort_order, show_in_home, taken_from, taken_hash, fingerprint, created_at, updated_at) VALUES ('cacacaca-0000-4000-8000-000000000010', 'series', 'Top Series', 'tmdb', '{"sort_by":"vote_average.desc","vote_count_gte":200}', 'aaaaaaaa-0000-4000-8000-000000000002', 0, NULL, 1, 1, 'cacacaca-0000-4000-8000-000000000002', '70b5bba43b20f277fb9cfedffff459ddceae47b2435b499b39aba15e22c53642', '57e085849a6c564dcf95ae44f1489dc5ee41bb8f541ef0ec9a304e874bc669db', '2026-09-27T16:24:21Z', '2026-09-27T16:24:21Z');
INSERT INTO catalogs (id, type, name, provider, params, owner_id, is_public, collection_id, home_sort_order, show_in_home, taken_from, taken_hash, fingerprint, created_at, updated_at) VALUES ('cacacaca-0000-4000-8000-000000000011', 'movie', 'Was Public', 'tmdb', '{"sort_by":"primary_release_date.desc","with_genres":"99"}', 'aaaaaaaa-0000-4000-8000-000000000002', 0, NULL, NULL, 1, 'cacacaca-0000-4000-8000-000000000004', '1af22d407832a1304aa037e3de39400303983de32ea398b1179ee5b8b533c252', '07341fa15a92862550c6aa890d656567438d466bcde451f254360c4e2545e924', '2026-09-27T16:24:21Z', '2026-09-27T16:24:21Z');
INSERT INTO catalogs (id, type, name, provider, params, owner_id, is_public, collection_id, home_sort_order, show_in_home, taken_from, taken_hash, fingerprint, created_at, updated_at) VALUES ('cacacaca-0000-4000-8000-000000000012', 'movie', 'Popular Movies', 'tmdb', '{"sort_by":"popularity.desc"}', 'aaaaaaaa-0000-4000-8000-000000000002', 0, 'cccccccc-0000-4000-8000-000000000004', NULL, 1, 'cacacaca-0000-4000-8000-000000000001', NULL, '7cfd3b841d8a78518650b7bbaef1dedf98a40546c2fe76242f9f81126e93c0b8', '2026-09-27T16:24:21Z', '2026-09-27T16:24:21Z');
INSERT INTO catalogs (id, type, name, provider, params, owner_id, is_public, collection_id, home_sort_order, show_in_home, taken_from, taken_hash, fingerprint, created_at, updated_at) VALUES ('cacacaca-0000-4000-8000-000000000013', 'movie', 'Car Chases', 'tmdb', '{"sort_by":"popularity.desc","with_keywords":"9748"}', 'aaaaaaaa-0000-4000-8000-000000000002', 0, 'cccccccc-0000-4000-8000-000000000004', NULL, 1, 'cacacaca-0000-4000-8000-000000000005', NULL, '53cb45abdd16576f4a9524cd520c7e3509855f44c2a37c85b7714c894d6a8af0', '2026-09-27T16:24:21Z', '2026-09-27T16:24:21Z');
INSERT INTO catalogs (id, type, name, provider, params, owner_id, is_public, collection_id, home_sort_order, show_in_home, taken_from, taken_hash, fingerprint, created_at, updated_at) VALUES ('cacacaca-0000-4000-8000-000000000014', 'movie', 'Private Picks', 'tmdb', '{"sort_by":"revenue.desc","with_original_language":"ko"}', 'aaaaaaaa-0000-4000-8000-000000000002', 0, 'cccccccc-0000-4000-8000-000000000004', NULL, 1, 'cacacaca-0000-4000-8000-000000000003', NULL, 'f195de9fec8fbb7d7b5499373eb6282d3371b5df670eceade69e1315e5bb3ace', '2026-09-27T16:24:21Z', '2026-09-27T16:24:21Z');
INSERT INTO catalogs (id, type, name, provider, params, owner_id, is_public, collection_id, home_sort_order, show_in_home, taken_from, taken_hash, fingerprint, created_at, updated_at) VALUES ('cacacaca-0000-4000-8000-000000000015', 'movie', 'Slashers', 'tmdb', '{"sort_by":"popularity.desc","with_genres":"27"}', 'aaaaaaaa-0000-4000-8000-000000000002', 0, 'cccccccc-0000-4000-8000-000000000005', NULL, 1, 'cacacaca-0000-4000-8000-000000000006', NULL, 'f79f5f804daa40adf0e7c71c239cc7fbd6ca2c273ab55658bab14ecd695482f7', '2026-09-27T16:24:21Z', '2026-09-27T16:24:21Z');
INSERT INTO catalogs (id, type, name, provider, params, owner_id, is_public, collection_id, home_sort_order, show_in_home, taken_from, taken_hash, fingerprint, created_at, updated_at) VALUES ('cacacaca-0000-4000-8000-000000000016', 'series', 'Ghost Stories', 'tmdb', '{"sort_by":"popularity.desc","with_keywords":"162846"}', 'aaaaaaaa-0000-4000-8000-000000000002', 0, 'cccccccc-0000-4000-8000-000000000005', NULL, 1, 'cacacaca-0000-4000-8000-000000000007', NULL, '81735f9e7e2e7b73bbed32ca9190bf2aaa8a763562e943c36a7222b2d5a72969', '2026-09-27T16:24:21Z', '2026-09-27T16:24:21Z');
INSERT INTO catalogs (id, type, name, provider, params, owner_id, is_public, collection_id, home_sort_order, show_in_home, taken_from, taken_hash, fingerprint, created_at, updated_at) VALUES ('cacacaca-0000-4000-8000-000000000017', 'movie', 'Popular Movies', 'tmdb', '{"sort_by":"popularity.desc"}', 'aaaaaaaa-0000-4000-8000-000000000003', 0, NULL, NULL, 1, 'cacacaca-0000-4000-8000-000000000009', '1da4fa7583d3b5d45d94e9cac5a150edda9c252639015c8bda79a4ffac92b12e', '7cfd3b841d8a78518650b7bbaef1dedf98a40546c2fe76242f9f81126e93c0b8', '2026-09-27T16:24:21Z', '2026-09-27T16:24:21Z');
INSERT INTO catalogs (id, type, name, provider, params, owner_id, is_public, collection_id, home_sort_order, show_in_home, taken_from, taken_hash, fingerprint, created_at, updated_at) VALUES ('cacacaca-0000-4000-8000-000000000018', 'movie', 'Popular Movies', 'tmdb', '{"sort_by":"popularity.desc"}', 'aaaaaaaa-0000-4000-8000-000000000003', 0, 'cccccccc-0000-4000-8000-000000000006', NULL, 1, 'cacacaca-0000-4000-8000-000000000012', NULL, '7cfd3b841d8a78518650b7bbaef1dedf98a40546c2fe76242f9f81126e93c0b8', '2026-09-27T16:24:21Z', '2026-09-27T16:24:21Z');
INSERT INTO catalogs (id, type, name, provider, params, owner_id, is_public, collection_id, home_sort_order, show_in_home, taken_from, taken_hash, fingerprint, created_at, updated_at) VALUES ('cacacaca-0000-4000-8000-000000000019', 'movie', 'Car Chases', 'tmdb', '{"sort_by":"popularity.desc","with_keywords":"9748"}', 'aaaaaaaa-0000-4000-8000-000000000003', 0, 'cccccccc-0000-4000-8000-000000000006', NULL, 1, 'cacacaca-0000-4000-8000-000000000013', NULL, '53cb45abdd16576f4a9524cd520c7e3509855f44c2a37c85b7714c894d6a8af0', '2026-09-27T16:24:21Z', '2026-09-27T16:24:21Z');
INSERT INTO catalogs (id, type, name, provider, params, owner_id, is_public, collection_id, home_sort_order, show_in_home, taken_from, taken_hash, fingerprint, created_at, updated_at) VALUES ('cacacaca-0000-4000-8000-000000000020', 'movie', 'Private Picks', 'tmdb', '{"sort_by":"revenue.desc","with_original_language":"ko"}', 'aaaaaaaa-0000-4000-8000-000000000003', 0, 'cccccccc-0000-4000-8000-000000000006', NULL, 1, 'cacacaca-0000-4000-8000-000000000014', NULL, 'f195de9fec8fbb7d7b5499373eb6282d3371b5df670eceade69e1315e5bb3ace', '2026-09-27T16:24:21Z', '2026-09-27T16:24:21Z');

INSERT INTO folders (id, collection_id, title, sort_order, tile_shape, hide_title, cover_emoji, cover_image_url, focus_gif_url, focus_gif_enabled, hero_video_url, hero_backdrop_url, title_logo_url) VALUES ('ffffffff-0000-4000-8000-000000000001', 'cccccccc-0000-4000-8000-000000000001', 'Action', 0, 'POSTER', 0, '💥', '', '', 1, '', '', '');
INSERT INTO folders (id, collection_id, title, sort_order, tile_shape, hide_title, cover_emoji, cover_image_url, focus_gif_url, focus_gif_enabled, hero_video_url, hero_backdrop_url, title_logo_url) VALUES ('ffffffff-0000-4000-8000-000000000002', 'cccccccc-0000-4000-8000-000000000001', 'Hidden Gems', 1, 'LANDSCAPE', 0, '', '', '', 1, '', '', '');
INSERT INTO folders (id, collection_id, title, sort_order, tile_shape, hide_title, cover_emoji, cover_image_url, focus_gif_url, focus_gif_enabled, hero_video_url, hero_backdrop_url, title_logo_url) VALUES ('ffffffff-0000-4000-8000-000000000003', 'cccccccc-0000-4000-8000-000000000002', 'Slashers', 0, 'POSTER', 0, '', '', '', 1, '', '', '');
INSERT INTO folders (id, collection_id, title, sort_order, tile_shape, hide_title, cover_emoji, cover_image_url, focus_gif_url, focus_gif_enabled, hero_video_url, hero_backdrop_url, title_logo_url) VALUES ('ffffffff-0000-4000-8000-000000000004', 'cccccccc-0000-4000-8000-000000000002', 'Hauntings', 1, 'SQUARE', 0, '', '', '', 0, '', '', '');
INSERT INTO folders (id, collection_id, title, sort_order, tile_shape, hide_title, cover_emoji, cover_image_url, focus_gif_url, focus_gif_enabled, hero_video_url, hero_backdrop_url, title_logo_url) VALUES ('ffffffff-0000-4000-8000-000000000005', 'cccccccc-0000-4000-8000-000000000003', 'Ideas', 0, 'LANDSCAPE', 0, '', '', '', 0, '', '', '');
INSERT INTO folders (id, collection_id, title, sort_order, tile_shape, hide_title, cover_emoji, cover_image_url, focus_gif_url, focus_gif_enabled, hero_video_url, hero_backdrop_url, title_logo_url) VALUES ('ffffffff-0000-4000-8000-000000000006', 'cccccccc-0000-4000-8000-000000000004', 'Action', 0, 'POSTER', 0, '💥', '', '', 1, '', '', '');
INSERT INTO folders (id, collection_id, title, sort_order, tile_shape, hide_title, cover_emoji, cover_image_url, focus_gif_url, focus_gif_enabled, hero_video_url, hero_backdrop_url, title_logo_url) VALUES ('ffffffff-0000-4000-8000-000000000007', 'cccccccc-0000-4000-8000-000000000004', 'Hidden Gems', 1, 'LANDSCAPE', 0, '', '', '', 1, '', '', '');
INSERT INTO folders (id, collection_id, title, sort_order, tile_shape, hide_title, cover_emoji, cover_image_url, focus_gif_url, focus_gif_enabled, hero_video_url, hero_backdrop_url, title_logo_url) VALUES ('ffffffff-0000-4000-8000-000000000008', 'cccccccc-0000-4000-8000-000000000005', 'Slashers', 0, 'POSTER', 0, '', '', '', 1, '', '', '');
INSERT INTO folders (id, collection_id, title, sort_order, tile_shape, hide_title, cover_emoji, cover_image_url, focus_gif_url, focus_gif_enabled, hero_video_url, hero_backdrop_url, title_logo_url) VALUES ('ffffffff-0000-4000-8000-000000000009', 'cccccccc-0000-4000-8000-000000000005', 'Ghosts', 1, 'SQUARE', 0, '', '', '', 0, '', '', '');
INSERT INTO folders (id, collection_id, title, sort_order, tile_shape, hide_title, cover_emoji, cover_image_url, focus_gif_url, focus_gif_enabled, hero_video_url, hero_backdrop_url, title_logo_url) VALUES ('ffffffff-0000-4000-8000-000000000010', 'cccccccc-0000-4000-8000-000000000006', 'Action', 0, 'POSTER', 0, '💥', '', '', 1, '', '', '');
INSERT INTO folders (id, collection_id, title, sort_order, tile_shape, hide_title, cover_emoji, cover_image_url, focus_gif_url, focus_gif_enabled, hero_video_url, hero_backdrop_url, title_logo_url) VALUES ('ffffffff-0000-4000-8000-000000000011', 'cccccccc-0000-4000-8000-000000000006', 'Hidden Gems', 1, 'LANDSCAPE', 0, '', '', '', 1, '', '', '');

INSERT INTO folder_catalogs (folder_id, catalog_id, sort_order, genre) VALUES ('ffffffff-0000-4000-8000-000000000001', 'cacacaca-0000-4000-8000-000000000001', 0, '');
INSERT INTO folder_catalogs (folder_id, catalog_id, sort_order, genre) VALUES ('ffffffff-0000-4000-8000-000000000001', 'cacacaca-0000-4000-8000-000000000005', 1, 'Action');
INSERT INTO folder_catalogs (folder_id, catalog_id, sort_order, genre) VALUES ('ffffffff-0000-4000-8000-000000000002', 'cacacaca-0000-4000-8000-000000000003', 0, 'Drama');
INSERT INTO folder_catalogs (folder_id, catalog_id, sort_order, genre) VALUES ('ffffffff-0000-4000-8000-000000000002', 'cacacaca-0000-4000-8000-000000000001', 1, 'Comedy');
INSERT INTO folder_catalogs (folder_id, catalog_id, sort_order, genre) VALUES ('ffffffff-0000-4000-8000-000000000005', 'cacacaca-0000-4000-8000-000000000008', 0, '');
INSERT INTO folder_catalogs (folder_id, catalog_id, sort_order, genre) VALUES ('ffffffff-0000-4000-8000-000000000005', 'cacacaca-0000-4000-8000-000000000002', 1, '');
INSERT INTO folder_catalogs (folder_id, catalog_id, sort_order, genre) VALUES ('ffffffff-0000-4000-8000-000000000008', 'cacacaca-0000-4000-8000-000000000015', 0, 'Horror');
INSERT INTO folder_catalogs (folder_id, catalog_id, sort_order, genre) VALUES ('ffffffff-0000-4000-8000-000000000009', 'cacacaca-0000-4000-8000-000000000016', 0, '');
INSERT INTO folder_catalogs (folder_id, catalog_id, sort_order, genre) VALUES ('ffffffff-0000-4000-8000-000000000006', 'cacacaca-0000-4000-8000-000000000012', 0, '');
INSERT INTO folder_catalogs (folder_id, catalog_id, sort_order, genre) VALUES ('ffffffff-0000-4000-8000-000000000006', 'cacacaca-0000-4000-8000-000000000013', 1, 'Action');
INSERT INTO folder_catalogs (folder_id, catalog_id, sort_order, genre) VALUES ('ffffffff-0000-4000-8000-000000000007', 'cacacaca-0000-4000-8000-000000000014', 0, 'Drama');
INSERT INTO folder_catalogs (folder_id, catalog_id, sort_order, genre) VALUES ('ffffffff-0000-4000-8000-000000000007', 'cacacaca-0000-4000-8000-000000000012', 1, 'Comedy');
INSERT INTO folder_catalogs (folder_id, catalog_id, sort_order, genre) VALUES ('ffffffff-0000-4000-8000-000000000010', 'cacacaca-0000-4000-8000-000000000018', 0, '');
INSERT INTO folder_catalogs (folder_id, catalog_id, sort_order, genre) VALUES ('ffffffff-0000-4000-8000-000000000010', 'cacacaca-0000-4000-8000-000000000019', 1, 'Action');
INSERT INTO folder_catalogs (folder_id, catalog_id, sort_order, genre) VALUES ('ffffffff-0000-4000-8000-000000000011', 'cacacaca-0000-4000-8000-000000000020', 0, 'Drama');
INSERT INTO folder_catalogs (folder_id, catalog_id, sort_order, genre) VALUES ('ffffffff-0000-4000-8000-000000000011', 'cacacaca-0000-4000-8000-000000000018', 1, 'Comedy');
INSERT INTO folder_catalogs (folder_id, catalog_id, sort_order, genre) VALUES ('ffffffff-0000-4000-8000-000000000003', 'cacacaca-0000-4000-8000-000000000006', 0, 'Horror');
INSERT INTO folder_catalogs (folder_id, catalog_id, sort_order, genre) VALUES ('ffffffff-0000-4000-8000-000000000004', 'cacacaca-0000-4000-8000-000000000007', 0, '');

