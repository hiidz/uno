# Data model

SQLite. `internal/vault/schema.sql` is the schema, embedded in the binary. Row structs and wire
DTOs are in `internal/vault/models.go`, all with explicit `snake_case` JSON tags.
`internal/vault/db.go` opens the database in WAL mode with a 5s `busy_timeout` and foreign keys
on, and creates the schema in it when it is empty.
Every write transaction begins `IMMEDIATE` (`_txlock=immediate`), taking the write lock up front
and waiting out `busy_timeout` for it: a deferred one that reads before it writes, as most writes
do, would fail at once with `SQLITE_BUSY` once another write committed. A read-only
transaction (`sql.TxOptions{ReadOnly: true}`, `ValidateSelectionAccess`) still begins deferred.

**Schema version.** `PRAGMA user_version` is the schema version, `schemaVersion` in `db.go`.
`InitDB` reads it in one transaction. At `0`, an empty file, it creates the schema and sets the
version in that same transaction; at `schemaVersion` it does nothing; any other version fails the
start, naming both. A schema change edits `schema.sql` and bumps `schemaVersion`.

```mermaid
erDiagram
  PROFILES ||--o{ CATALOGS : owns
  PROFILES ||--o{ COLLECTIONS : owns
  COLLECTIONS ||--o{ FOLDERS : contains
  COLLECTIONS ||--o{ CATALOGS : scopes
  FOLDERS ||--o{ FOLDER_CATALOGS : contains
  CATALOGS ||--o{ FOLDER_CATALOGS : "referenced via"
  RECIPES ||--o{ CATALOGS : "asked for by"
  CATALOGS |o--o| PUBLICATIONS : "published as"
  COLLECTIONS |o--o| PUBLICATIONS : "published as"
  PUBLICATIONS ||--o{ SUBSCRIPTIONS : "followed by"
  SUBSCRIPTIONS |o--|| CATALOGS : "copy"
  SUBSCRIPTIONS |o--|| COLLECTIONS : "copy"
  ACCOUNTS ||..o{ PROFILES : "keys every profile of"
  PROFILES ||--o| PUSH_RECORDS : "last pushed"

  PUBLICATIONS {
    uuid id PK "kept across republishes"
    uuid publisher_id FK
    string kind "catalog | collection"
    uuid catalog_id FK "nullable — the source; NULL once deleted"
    uuid collection_id FK "nullable — the source; NULL once deleted"
    string title
    json snapshot "format uno-publication, version 1"
    string content_hash "sha256 hex of snapshot"
    int catalog_count
    int folder_count
    int subscriber_count
    string status "live | unpublished"
    string published_at
    string updated_at
  }
  SUBSCRIPTIONS {
    uuid id PK
    uuid subscriber_id FK
    uuid publication_id FK
    uuid catalog_id FK "nullable — the copy, for a catalog"
    uuid collection_id FK "nullable — the copy, for a collection"
    string subscribed_hash "the content hash the copy was last written from"
    string created_at
  }

  RECIPES {
    string hash PK "sha256 hex of uno-recipe/1, type, provider and params"
    string type "movie | series"
    string provider "tmdb"
    json params "canonical form"
    string created_at
  }
  PROFILES {
    uuid id PK
    string token UK
    string nuvio_user_id
    int nuvio_profile_index
    string nuvio_profile_uuid
  }
  CATALOGS {
    uuid id PK
    string name
    string recipe_hash FK "its recipe: type, provider and params"
    uuid owner_id FK
    uuid collection_id FK "nullable — NULL means listed"
    int home_sort_order "nullable — place on Home, one numbering with the other table; NULL means not on Home"
    bool show_in_home
    string sub_key "nullable — in a subscribed collection, its snapshot key"
    string created_at
    string updated_at
  }
  COLLECTIONS {
    uuid id PK
    string title
    uuid owner_id FK
    bool pin_to_top "Pin, as last pushed; only push writes it"
    string view_mode
    bool show_all_tab
    string backdrop_image_url
    bool focus_glow_enabled "defaults to 1, matching Nuvio"
    int home_sort_order "nullable — place on Home, one numbering with the other table; NULL means not on Home"
    string created_at
    string updated_at
  }
  PUSH_RECORDS {
    uuid profile_id PK_FK
    string nuvio_profile_uuid "the Nuvio profile it was pushed to"
    json record "vault.PushRecord: what the last push sent"
    string pushed_at
  }
  FOLDERS {
    uuid id PK
    uuid collection_id FK
    string title
    int sort_order
    string tile_shape
    bool hide_title
    string cover_emoji
    string cover_image_url
    string focus_gif_url
    bool focus_gif_enabled "defaults to 1, matching Nuvio"
    string hero_backdrop_url
    string hero_video_url
    string title_logo_url
    string sub_key "nullable — in a subscribed collection, its snapshot key"
  }
  FOLDER_CATALOGS {
    uuid folder_id PK_FK
    uuid catalog_id PK_FK
    int sort_order
    string genre "'' means unfiltered"
  }
```

## `profiles`

```sql
CREATE TABLE IF NOT EXISTS profiles (
    id                  TEXT    PRIMARY KEY,     -- UUID
    token               TEXT    NOT NULL UNIQUE, -- URL slug, e.g. /u/{token}/...
    nuvio_user_id       TEXT    NOT NULL,        -- Nuvio auth.users.id (the account)
    nuvio_profile_index INTEGER NOT NULL,        -- Nuvio profile slot, 1..6
    nuvio_profile_uuid  TEXT    NOT NULL,        -- Nuvio profile row's own id

    UNIQUE (nuvio_user_id, nuvio_profile_index),
    CHECK  (nuvio_profile_index BETWEEN 1 AND 6)
);
```

**The composite key is `(nuvio_user_id, nuvio_profile_index)`, not `nuvio_profile_uuid`,** and
the reason propagates through the whole API. The *index* is what every downstream Nuvio call is
scoped by (`p_profile_id` is the integer slot, never the UUID), and the index is the only
identifier available at lookup time — the resolve-or-create input is `(sub from the JWT,
profile_index from the picker)`. The UUID isn't known until *after* a `sync_pull_profiles` round
trip, so it cannot be the thing you query by. This is also why the bearer-auth CRUD routes
resolve profiles by `(sub, profileIndex)`, not by UUID.

`nuvio_profile_uuid` is stored as a tripwire rather than a key. `sync_push_profiles` is
full-replace over slots 1..6, so a user can delete slot 2 and create a *different* profile in
that slot. Uno's link is to a slot *number*, and storing Nuvio's own profile UUID alongside it is
the only way to detect that a slot's occupant changed. `ResolveOrCreateProfile` **silently
overwrites** the stored UUID on drift rather than prompting, and keeps the library and the Home
layout. Each push record is stamped with it (`push_records`, below), and a record whose stamp no
longer matches describes a Nuvio profile that is gone: the addon serves it no more, and Home
waits for a push.

`profiles.token` has exactly one job: identifying a profile in the public addon URLs. It is not a
write credential.

## `accounts`

```sql
CREATE TABLE accounts (
    nuvio_user_id       TEXT PRIMARY KEY,  -- Nuvio auth.users.id, as profiles.nuvio_user_id
    tmdb_key_ciphertext BLOB NOT NULL,     -- nonce || AES-GCM sealed key
    tmdb_key_last4      TEXT NOT NULL,     -- the key's last four characters, shown to its owner
    updated_at          TEXT NOT NULL
);
```

One row per Nuvio account that has saved its own TMDB key, on a server in per-account key mode
(`docs/configuration.md` → *TMDB key modes*). It belongs to the account, not a profile: every
profile of the account uses it, in the builder and on the addon routes. There is no foreign key to
`profiles`, whose `nuvio_user_id` is not unique; a row outlives the account's profiles until its
owner removes the key.

- **Sealed, never in the clear.** `internal/tmdbkey` seals the key with AES-256-GCM under
  `UNO_SECRET`, with the account id as additional data, so a ciphertext copied to another row
  doesn't open. The vault stores and returns the sealed bytes only (`SetAccountKey`,
  `AccountKey`, `AccountKeyByToken`, `DeleteAccountKey`, and `ServedCatalog`'s join).
- **`tmdb_key_last4`** is all its owner is ever shown of the key.
- **Replace and remove.** Saving again replaces the row; removing deletes it. A server that loses
  `UNO_SECRET` can't open any row, and each owner enters the key again.
- **Shared mode** neither reads nor writes the table; rows stay, unused.

## `push_records`

```sql
CREATE TABLE push_records (
    profile_id         TEXT PRIMARY KEY REFERENCES profiles(id),
    nuvio_profile_uuid TEXT NOT NULL, -- the Nuvio profile it was pushed to: profiles.nuvio_profile_uuid then
    record             TEXT NOT NULL, -- JSON, vault.PushRecord
    pushed_at          TEXT NOT NULL  -- RFC3339 UTC
);
```

One row per profile: what its last push put in Nuvio, as one JSON document
(`vault.PushRecord`, `internal/vault/pushrecord.go`).

- **What it holds.** `collections`: each pushed collection as the exact bytes push sent
  (`PushJSON`), in Home order. `home`: the Home selection the push carried,
  `{catalogs: [{catalog_id, show_in_home, position}], collections: [{collection_id, pin_to_top,
  position}]}`, `position` each row's place on Home in one numbering across both lists.
  `catalogs`: every catalog Nuvio can reach, `{id, name, type, provider, params}` with params
  inline: those with their own Home row in Home order, then those only the collections' folders
  use, in collection, folder and ref order, once each, the order `GetPublishedCatalogs` lists.
- **One builder.** `BuildPushRecord` builds it from a pending selection, reading only the
  caller's own rows; `StoredPushRecord` builds it from the Home columns, which the list of what
  waits for a push compares the held record with.
- **Written only by push**, in `SavePush`'s transaction with the Home columns, after Nuvio
  accepted the push, replaced whole and stamped with `profiles.nuvio_profile_uuid` as it is then.
  Nothing cascades into it from `catalogs` or `collections`, and it never points into `recipes`,
  so deleting a row never loses what Nuvio holds.
- **Read by** the addon and by the builder's reads. `GetPublishedCatalogs` and `ServedCatalog`
  serve the manifest and each catalog from its `catalogs` and `home`, so Nuvio is served only
  what the last push put there; every collection read compares `collections` for `needs_push`
  (*Key rules*); `PendingPush` compares all of it with what a push would send now; and push's
  merge drops the collections it lists. All of them read only a **current** record: one whose
  stamp equals `profiles.nuvio_profile_uuid` now. A record stamped with another id was pushed to
  a Nuvio profile the slot no longer has, so it counts as none: Nuvio holds nothing, the addon
  serves an empty manifest and no catalog, everything on Home waits for a push, and the next push
  replaces the record. A profile that never pushed has no record either.

## Key rules

- **A catalog has a scope: listed or scoped to one collection.** `catalogs.collection_id` is
  `NULL` for a listed catalog (in the library, usable on home and in any of the owner's folders)
  or a collection id for one scoped to exactly that collection (hidden from the library, usable
  only in that collection's folders, deleted with it). A scoped catalog is made only by
  `CreateUserCatalog` with `collection_id` set, which enforces that the target collection is owned
  by the same profile (`requireOwnedCollection`; a subscribed copy is refused, see
  *Publications and subscriptions*), or inline in a collection's save. A scoped catalog is never
  on the home screen — the
  schema's own `CHECK (collection_id IS NULL OR home_sort_order IS NULL)` exists as a backstop and
  would surface as a 500, so the Go layer rejects it before that CHECK is ever hit. A scoped
  catalog is never published on its own: it is published with its collection. A catalog's scope is
  not part of `UpdateUserCatalog`: a `collection_id` sent with one is not read, so a listed
  catalog stays listed. Nothing changes a catalog's scope once it exists: a scoped catalog
  reaches the library only as a copy, a new listed row created with its name and recipe.
  `GetUserCatalogs` (the library) returns listed catalogs
  only — a scoped one is reached through its owning collection's own response instead.
- **A scoped catalog is written only through its collection's save.** `UpdateUserCatalog` and
  `DeleteUserCatalog` refuse a row whose `collection_id` is set (`ErrInvalidInput`, a 400).
  `CollectionForm.CatalogEdits` (`vault.ScopedCatalogEdit`) carries the new name and recipe for
  catalogs already scoped to the collection being saved, and `applyCatalogEdits` writes them
  inside `UpdateUserCollection`'s transaction, after the collection row and before the folder
  rewrite and its orphan cleanup. Each edit's catalog must be owned by the caller and scoped to
  this same collection, with the stored type and provider. An edit whose name and recipe
  (`recipe_hash`) both match the row is skipped, so `updated_at` stays put. An edit never
  changes `collection_id`. `CreateUserCollection` refuses any edit,
  since a new collection has no scoped catalogs. So an edit made in the collection editor lands
  with the rest of the collection, and a discarded one never wrote anything.
- **A profile's data graph is closed: references never cross an owner boundary.** A folder may
  reference a catalog only if `internal/vault/access.go`'s `validateFolderRefs` accepts it: the
  catalog's `owner_id` must equal the collection's `owner_id`, and the catalog's `collection_id`
  must be `NULL` (listed) or equal to that same collection (scoped to it already). There is no
  "or public" branch anywhere in a write path — a published catalog can only enter another
  profile's graph as a copy with fresh ids (a subscribe or a duplicate), never through a live
  reference.
  `CreateUserCollection` has no collection id yet, so its folders may reference listed catalogs
  only. `CollectionWithFolders.Catalogs` carries every catalog a collection's folders reference,
  listed or scoped, so the editor never needs the library to render a folder.
- **A scoped catalog with no remaining folder reference in its collection is deleted on that
  collection's next whole-tree Save.** `UpdateUserCollection` runs this cleanup in the same
  transaction as the folder rewrite, right after `rewriteFolderCatalogRefs` for every folder:
  `DELETE FROM catalogs WHERE collection_id = ? AND id NOT IN (` the catalog ids still referenced
  by that collection's folders `)`. This is also what catches a scoped catalog created via
  `POST .../catalogs` and abandoned before Save — it has no folder ref yet, so the next Save (or
  the collection's own deletion, by cascade) removes it.
- **`catalogs.recipe_hash` names the catalog's recipe.** Its type, provider and params live in
  `recipes`, one row per distinct recipe (see *Recipes* below). Every catalog read joins its
  recipe in, so each catalog still carries `type` and `params` on the wire, and `recipe_hash`
  never reaches it. The import check offers the caller's listed catalogs with the same one. A
  copy shares its snapshot's recipe.
- **Sharing is by publication.** A row is put in Community by publishing it as a frozen snapshot, and
  another profile follows it through a subscribed copy; see *Publications and subscriptions*
  below.
- **`catalogs.id` is permanent once created** — never rename or recycle it. It is baked into
  `vault.ManifestID` and therefore into Nuvio's `catalogSources[].catalogId`.
- **`recipes.params` is opaque `TEXT` at the schema level.** For `provider = 'tmdb'` there is an
  app-level shape in `internal/provider` (`TMDBMovieParams` / `TMDBTVParams` on
  `TMDBCommonParams` + `BaseParams`), with `Validate()` covering cross-field rules, dispatched by
  `validateCatalogParams` from the create/update handlers. It crosses the wire as a JSON-encoded
  **string**, not a nested object, in its canonical form.
- **A catalog's `type` is immutable once the row exists.** It is the type of the catalog's
  recipe, and every write that repoints a catalog at another recipe keeps it. `UpdateUserCatalog`
  reads the stored type in its own opening `SELECT` and rejects the write with
  `ErrInvalidInput` if the incoming form's `type` differs; a collection's catalog edit and a
  subscribed catalog's Update refuse a different type the same way, and a subscribed
  collection's Update writes a snapshot catalog of another type as a new catalog rather than an
  edit. A catalog's type is baked into the
  pushed collections blob (each folder source names its catalog's type), so changing it would
  alter what Nuvio should have with no save of any collection. The catalog editor
  already locks the field once a row exists; this is the write path enforcing the same rule
  server-side. Duplicate is the supported way to get a different-typed copy.
- **Two distinct removal mechanisms — don't conflate them.**
  - **Hard delete** (`DeleteUserCatalog` / `DeleteUserCollection`): owner-scoped, one
    transaction, all downstream cleanup via `ON DELETE CASCADE`, **allowed any time**, Home or
    not. Nuvio keeps what the last push put there, served from the push record (`push_records`,
    above), until the next push drops it, so a delete never leaves a Nuvio client with an empty
    row or tile before it has synced. The record never points into `recipes` and nothing cascades
    into it, so a deleted row's name, type and params are still there for the addon and for the
    list of what waits for a push, which names it as removed. A collection whose folders used a
    deleted listed catalog loses it from those folders by cascade, so one on Home shows as
    changed. Deleting a published row unpublishes it, and every copy another profile holds
    survives, marked unpublished (*Publications and subscriptions*, below). Deleting a subscribed
    copy removes its subscription. The next push drops the deleted collection from Nuvio's
    blob by the ids the last push sent (`PushedCollectionIDs`).
  - **Unselect**: reachable only through push, which folds the whole pending selection straight
    into `catalogs.home_sort_order`/`show_in_home` and `collections.home_sort_order`
    (`saveCatalogSelectionTx`/`saveCollectionSelectionTx`, `internal/vault`). Every owned row's
    `home_sort_order` is cleared first, then each incoming entry's is set to its `position`, one
    numbering across catalogs and collections (a row's place in push's ordered body);
    an id that isn't owned (or, for a catalog, isn't listed — `AND collection_id IS NULL`) affects
    0 rows and is `ErrInvalidInput` naming the id. There is no separate join table and no separate
    access-check query — the `UPDATE`'s own `WHERE` clause is the validation.
- **`catalogs.show_in_home` drives the manifest's per-catalog genre extra, but the manifest
  reads it through `vault.GetPublishedCatalogs`, not the raw column.** `GetPublishedCatalogs` is
  the union of every owned catalog with `home_sort_order`
  non-`NULL` (its own `show_in_home`), plus every catalog referenced by a folder of a collection
  that is itself on the home screen (`collections.home_sort_order` non-`NULL`), with a forced
  `ShowInHome = false` — a catalog reachable only through a folder never gets an automatic home
  row. Deduped by id: a catalog on both the home screen and in an on-TV folder appears once,
  keeping its own `show_in_home`. `buildManifest` (`internal/addon/addon.go`) emits the result as
  an explicit per-catalog `showInHome`, and a `ShowInHome = false` result also gets a required
  `genre` extra whose first option is `"All"`. Nuvio TV reads the field, while Nuvio mobile,
  Nuvio desktop and Stremio read the required extra. `docs/architecture.md`'s addon-server
  section covers both.
  `GetCurrentCatalogSelection` (the narrower `home_sort_order IS NOT NULL` query)
  remains the pre-push validation/selection-editor view; only the manifest needs the wider
  published set. The addon's catalog route checks that same set for the one catalog it is asked
  for (`ServedCatalog`), so it serves exactly what the manifest lists.
- **`collections.pin_to_top` (Pin) is written only by push**, from its selection's entry
  for each collection it puts on Home (`saveCollectionSelectionTx`), in the same statement as
  `home_sort_order`. A collection save never writes it (the form has no pin), a new collection
  (a create, subscribe, duplicate, Duplicate or import) starts unpinned, and one push leaves off Home
  keeps its last pin, which putting it back on Home starts from. Like Home or Discover for a
  catalog, it is a pending edit on the Home pane until push.
- **What a collection last sent to Nuvio is in its owner's push record** (`push_records`, below):
  the exact push JSON (`PushJSON`, *Push wire shape*) that push built and sent *before* the local
  write. A new collection (a create, subscribe, duplicate, Duplicate or import) is off Home, and no
  record holds it.
  - **`needs_push`** is on every collection read: `true` when the collection is on Home and what
    push would send for it now, with its stored `pin_to_top`, differs from the bytes the record
    holds for it, or the record holds none. Off Home it is always `false`. Only push writes the
    Home columns, so a collection on Home was in the last push and the record holds it. The Home
    pane lists it as "changed since it was last pushed" (`web/src/features/home/changes.ts`).
  - So `needs_push` flags exactly the edits that change a pushed collection — a folder's
    catalogs, genre or images, a title, a setting — and nothing else: a rename and back, or a
    save that changes nothing, leaves it unflagged. A catalog's name and recipe are not in the
    pushed collection (a source names its catalog by id and type), so editing one doesn't set it.
    The addon serves a catalog from the record, so such an edit, like a delete, reaches Nuvio
    at the next push, and `GET /api/p/{i}/push/pending` (`PendingPush`) lists it: a catalog on
    Home as changed, and a collection whose folders use it as changed too.
  - A Save landing between push's build and its local write leaves the row sending something
    other than what the record holds, so it still reads as needing a push.
  - If the push JSON ever gains a field, every collection reads as needing a push once, which is
    right: Nuvio lacks the field.
- **No cascade on `owner_id`** (`catalogs`/`collections`). Irrelevant until
  profile deletion exists; revisit then.
- **`folder_catalogs.genre` narrows one folder reference, not the catalog.** It is a genre
  *name* from the catalog's manifest `genre` extra (`provider.GenreExtraOptions`), `''` for
  unfiltered. Push sends it as the folder source's `genre`, and Nuvio sends that back as the
  `genre` extra when it loads the folder's row, so the same catalog can show Westerns in one
  folder and war films in another, or both in one folder, with no second catalog. It is stored as a name rather than a
  TMDB id because the name is what goes over the wire both ways. It is not validated against the
  recipe on save: a later recipe edit can leave a stored genre outside the options (now required
  or excluded), and the addon path then serves that row unfiltered, the same as any unknown
  extra. The collection editor flags that case rather than clearing it. A publication's snapshot,
  and so every subscribe, duplicate and Update, and a Duplicate carry it onto the copy
  (`extractBundle` keeps each ref's genre). On the wire each folder carries an ordered
  `refs: [{catalog_id, genre}]` (`vault.FolderRef`), not a list of catalog ids, because one
  catalog can be two refs. A genre picked in Nuvio's own editor doesn't survive a push, because
  push rebuilds every collection the profile owns from Uno's data.
- **`folder_catalogs` is `PRIMARY KEY (folder_id, catalog_id, genre)`.** One catalog can appear
  in a folder more than once under different genres, and push sends each as its own source with
  the same `catalogId`. Confirmed on Nuvio desktop and mobile with a hand-edited collection: both
  sources show, each filtered. The same catalog under the *same* genre twice is pointless, so
  `CollectionForm.Validate` rejects it as a 400 (comparing trimmed genres, as they're stored)
  before it can reach the primary key as a 500. An inline `new` entry in a save payload carries
  a client `key` (the editor's draft id), and every entry sharing a key resolves to the one
  catalog that save creates, so Validate treats a repeated `(key, genre)` the same way and
  rejects entries that share a key but not a spec. The same catalog in two *different* folders is
  allowed and supported. `GetPublishedCatalogs` still lists a catalog once however many refs
  point at it.
- **A builder write or an import stores its input normalized, then validates it**
  (`CollectionForm.normalized` and `CatalogForm.normalized`, `internal/vault/validation.go`). Every
  text field the editors trim is trimmed: titles, catalog names (the names of new and edited
  scoped catalogs included), the cover emoji and the media URLs. An empty `view_mode` or
  `tile_shape` is stored as `TABBED_GRID` or `POSTER`, which is what every Nuvio client shows
  for one (see "Push wire shape" below). So an untouched editor save writes back exactly what is
  stored, and a published row reads as unchanged after one. A publish snapshots the values it
  reads, and a subscribe, duplicate, Update or Duplicate writes the values it reads, without
  normalizing them, so a subscribed copy snapshots exactly like its publication. Rows written before
  normalization can still hold `''` or padding; Preview and the editor read an empty value the
  way Nuvio does.
- **`folders.tile_shape` defaults to `'LANDSCAPE'` at the schema level**, but every write sends
  the column, so the default is unreachable in practice. Nuvio's apps read an absent key as a
  poster tile (see "Push wire shape" below), which a Uno push never produces.
- **`catalogs.created_at`/`updated_at` and `collections.created_at`/`updated_at` are `TEXT`
  RFC3339 UTC**, generated in Go with `time.Now().UTC().Format(time.RFC3339)` and parsed back to
  `time.Time` in `internal/vault/scan.go`; `encoding/json` serialises the Go field as RFC3339 on
  the wire. Every insert sets both to the same instant; every update rewrites only `updated_at`.
- **Nuvio appearance fields.** `collections.focus_glow_enabled` (the TV's focus glow on the
  collection's home-screen folder cards), `folders.focus_gif_url`/`focus_gif_enabled` (an
  animated GIF played over a folder tile while it's focused), and
  `folders.hero_backdrop_url`/`hero_video_url`/`title_logo_url` (hero media that Nuvio's own
  editor labels "(Modern Home)") are stored, edited in the collection editor, carried by a
  publication's snapshot and every copy, and pushed. Uno's Preview renders none of them. The two
  flags default to `1` because Nuvio reads an absent flag as on. Every URL among them, plus
  `collections.backdrop_image_url`, must be an absolute `http`/`https` URL under 2048 characters
  — `CollectionForm.Validate` enforces it on save, on a publish, and on a subscribe, duplicate, Update
  or Duplicate against the form built from the snapshot or source. Uno never renders these, but
  Nuvio's clients do, and a subscribe carries them into a profile that didn't author them, so a
  `javascript:` or `data:` value must not reach the push.
- **`collections.backdrop_image_url` is stored, editable in the collection editor, pushed to
  Nuvio, and never rendered by Uno.** It is a collection-level field, and a collection renders on
  home as a row of folder tiles rather than as its own page, so no current surface wants it.

## Vault invariants

- Every CRUD/selection method takes a pre-resolved `profileID uuid.UUID`, never a token — profile
  resolution happens exactly once per request, in the API layer's `requireProfile` middleware.
  `ResolveProfileID` (token → profile ID) exists exclusively for the addon server.
- `ResolveOrCreateProfile` is the **only** path that creates a profile row. It handles the
  SELECT-then-INSERT race by catching `SQLITE_CONSTRAINT_UNIQUE` (`isUniqueConstraintErr`) and
  falling back to a fresh SELECT rather than surfacing a 500.
- **Folders and their catalog refs are not independently addressable.** All access goes through
  the parent collection's whole-tree upsert. There are no standalone
  `listFolders`/`addFolder`/`reorder` operations, and adding them would be a fresh decision
  rather than resumption of a deferred plan. This shape is what forces the collection editor's
  one dirty state and one Save button.
- **Every stored string and list has a size ceiling, and all of them live in
  `internal/vault/validation.go`.** `maxNameLen` (200) bounds a catalog's `name` and a
  collection's and a folder's `title`; `maxParamsLen` (8192) a catalog's `params` JSON;
  `maxCoverEmojiLen` (32) a folder's `cover_emoji`; `maxGenreLen` (64) a folder ref's genre;
  `maxMediaURLLen` (2048) every media URL; `maxNewKeyLen` (128) an inline-`new` entry's client
  key; and `maxFoldersPerCollection`/`maxRefsPerFolder` (100 each) how many folders a collection
  holds and how many catalog refs a folder holds. These are bounds against absurdity, not product
  limits — each one sits well past anything the builder can produce — and they exist because
  `api.maxRequestBodyBytes` (1 MiB) is no substitute: 1 MiB is thousands of folders, and every one
  of these strings is stored, served from the public addon route, and pushed into Nuvio's
  collections blob as a full replace. `CatalogForm.Validate` and `CollectionForm.Validate` enforce
  them on save (an import, whose body limit is 4 MiB, is held to the same rules), a folder's inline `new` entries and the save's `catalog_edits` included, since
  either becomes a catalog row write in the same transaction.
- **Empty lists serialize as `[]`, never `null`.** The row parsers in `internal/vault/scan.go`
  initialize their slices, and `jsonwire.OrEmpty[T]` (`internal/jsonwire`) covers the
  map-lookup, decoded-response and client-input spots that produce nested
  `Folders`/`CatalogIDs` slices. The push-internal batch
  loaders still `return nil, nil` on empty input, deliberately — nothing serializes their output.
  The frontend's `?? []` coercion (`getList`, and the `| null` on nested array types in
  `web/src/api/types.ts`) is retained as defensive handling. Any new list endpoint must
  initialize or `jsonwire.OrEmpty` its slice: four client-side comments (`web/src/api/http.ts`,
  `web/src/features/library/useLibrary.ts`, `web/src/features/home/preview.ts`,
  `web/src/features/collections/collectionForm.ts`) assert the no-`null` rule unconditionally,
  and one nil slice on the wire makes all four wrong.
- **An id list binds as one parameter.** Every query over a list of ids reads it with
  `IN (SELECT value FROM json_each(?))`, bound to the JSON array `idsJSON` makes, so a list of
  any length is one parameter and never nears SQLite's parameter limit.

## Recipes

**A recipe is what a catalog asks its provider for: its type, provider and params.** `recipes`
holds each distinct recipe once, addressed by a hash of its content, and every catalog that
asks for it points there through `catalogs.recipe_hash` (`internal/vault/recipes.go`).

- **The canonical form** is per provider and type. `provider.CanonicalParams` decodes params as
  that recipe's params struct and encodes it again, then sorts the keys:
  - keys the type doesn't know are dropped;
  - zero values are dropped too, since zero means unset for every field;
  - the result is compact, with numbers written the way Go writes them.

  Two encodings of one recipe give the same bytes, so the SPA's key order or a `0` it sends for
  an empty field never makes a second recipe. `recipes.params` is always canonical, and the wire
  carries it back as each catalog's `params`. The editor's dirty check compares re-serialized
  form state, not the params string, so that needs nothing on the SPA side.
- **Checked first, then made canonical.** `api.checkRecipe`, which every client-supplied recipe
  goes through (a catalog save, a collection save's new entries and catalog edits, and each
  catalog of an import), validates the params as sent and only then canonicalizes them. The
  canonical form drops unknown keys, so this order is what keeps a key the check rejects, such as
  `with_networks` on a movie recipe, a 400 rather than silently dropped. The vault stores the
  params it is given.
- **The hash.** `vault.RecipeHash` is sha256 hex over `uno-recipe/1`, the type, the provider and
  the canonical params, separated by newlines. The vault computes it from the bytes it stores and
  never takes one from a caller, so a hash never names content other than its own.
  - `TestRecipeHashIsPinned` holds it to a literal.
  - Changing it or the canonical form changes every stored `recipe_hash`, the params inside
    every snapshot and the subscriptions' `subscribed_hash` with them, so it is a schema change.
- **Stored with the write that uses it.** `ensureRecipe` inserts a recipe unless it is stored
  already, in the same transaction as the catalog write that points at it:
  - `insertCatalog`, which a catalog save, a subscribe or duplicate and a collection save's new
    entries all go through;
  - `UpdateUserCatalog`;
  - a collection save's catalog edit;
  - a subscribed catalog's Update.

  A snapshot carries every recipe's params inline, so a copy never needs the publisher's recipe
  row, which the publisher's later edits may have deleted.
- **Deleted once unused.** Two triggers delete a recipe as soon as no catalog references it, in
  the transaction of the change that left it unused. `recipes_drop_unused_on_delete` covers a
  deleted catalog, cascades included: a collection's delete, and the orphan cleanup of a
  collection save. `recipes_drop_unused_on_repoint` covers a catalog repointed at another
  recipe. The `catalogs_by_recipe` index keeps their check cheap.
- **Copies share.** A subscribe, a duplicate, an Update and a Duplicate write the recipe they copy,
  which is the same recipe row whenever it is still stored. `changesNothing` and the import
  check's matches compare `recipe_hash`.

## Publications and subscriptions

**A row is put in Community by publishing it, and followed by subscribing to it.** Publishing
freezes the row's content as a snapshot; the publisher's later edits stay private until they
publish again. Another profile subscribes to a publication and gets a copy of the snapshot as its own
rows, which Update brings up to a newer snapshot. `internal/vault/publications.go`,
`subscriptions.go`, `community.go` and `snapshot.go`.

- **The snapshot** (`Snapshot`, format `uno-publication`, version 1) is the bundle form with
  every catalog's params inline, for a collection its own fields and folders, and every catalog
  and folder under a **stable key**: the first 16 hex digits of the sha256 of the publication's
  id and the source row's id (`stableKey`). The same row has the same key in every snapshot of
  one publication, which is how Update pairs a copy with a newer snapshot. A collection's
  snapshot holds every catalog its folders reference, listed or scoped, in the order they are
  first referenced, once each.
  - **The content hash** is the sha256 of the snapshot's stored bytes. It is the publication's
    `content_hash`. `TestSnapshotIsPinned` holds the bytes
    to literal hashes. Changing what a snapshot holds or how it encodes changes every content
    hash, and every subscriber would see an update that changes nothing, so it is a schema
    change.
- **Publish** (`PublishCatalog`, `PublishCollection`) snapshots an owner's listed catalog or
  collection. It runs the form validators the snapshot's copies are written through and the TMDB
  recipe check over every catalog it publishes, which is the consent to publish a private library
  catalog a collection references. It reads and checks through the pool, then reads the source
  again inside the write transaction and writes only if its snapshot is unchanged, so a source
  edited while TMDB was checking it is `ErrConflict` rather than published unchecked. A catalog
  inside a collection and a subscribed copy are refused (`ErrInvalidInput`): only its publisher
  publishes a publication. A copy that is duplicated is the caller's own, and publishes
  like any other row. A collection that references a catalog its owner subscribes to
  publishes: its snapshot freezes that catalog as it stands, under the new publication's own
  keys, so a profile that subscribes to the collection gets a scoped copy with no subscription
  of its own and nothing linking it to the original publication. When the owner Updates that
  catalog, the collection reads as changed since publishing, and the owner publishes it again
  when they choose.
  - **Republishing** rewrites the same publication row: its id and `published_at` are kept, and
    an unpublished publication is live again.
  - **The owner's row** carries `publication {id, status, changed_since_publish}`. The flag is
    set when the row as it stands no longer snapshots to the stored content hash; it is a hint
    to the owner only. Only the owner's own reads carry sharing state (`selectCatalogs`,
    `selectCollections`), the catalogs of their collection trees included: the addon's
    `GetPublishedCatalogs`, push's `BuildPushRecord`, `GetCatalogsByIDs` and
    `GetCollectionsByIDs`, the trees' catalogs too, read without the joins or the hash
    (`selectLeanCatalogs`, `selectLeanCollections`), and carry `null`.
- **Unpublish.** `UnpublishCatalog`/`UnpublishCollection` unpublish a live publication. So does
  deleting its source (the source column is `ON DELETE SET NULL`, and the
  `publications_unpublish_on_source_delete` trigger sets `status`). An unpublished publication
  leaves Community; its subscribers keep their copies, marked unpublished, and can still read its
  last snapshot, but Update answers not found. Nothing else unpublishes: no write scopes a
  listed catalog.
- **No collapse.** Two publications of the same content, a recipe two profiles both publish or
  an identical collection, are both listed.
- **Subscribe** (`Subscribe`) writes a live publication of someone else's as the caller's own
  rows: a catalog as a listed catalog, a collection as a collection with every catalog scoped to
  it, each catalog and folder carrying its snapshot key in `sub_key`. The copy is unpublished,
  off Home and never pushed. Only the form validators run: the recipes were checked
  against TMDB at publish, so a subscribe makes no TMDB call. `subscriptions` is unique on
  `(subscriber_id, publication_id)`, so a second subscribe is `ErrConflict`, and deleting the copy
  deletes its subscription by cascade. `subscriber_count` is kept by the
  `subscriptions_count_*` triggers.
- **Only Update writes a subscribed copy.** A catalog save, a collection save, and a catalog
  created in it are `ErrInvalidInput` (`refuseSubscribedCopy`, run in the write's transaction
  ahead of the write, over the caller's own subscriptions): the copy keeps its subscription, its
  `sub_key`s and its ids. Update shares the collection update core without the refusal. A
  publish of a subscribed copy is `ErrInvalidInput` too: only its publisher changes or
  publishes it. Its home order, show-in-home and pin change through push, like any
  row's, and it can be deleted, which removes its subscription by cascade.
  - **The copy's row** carries `subscription {publication_id, update_available, unpublished}`.
    `update_available` is true while the publication is live and the subscription's
    `subscribed_hash` differs from its content hash.
- **Update** (`UpdateSubscription`) brings a copy up to the current snapshot, in one
  transaction. A copy whose content already equals the snapshot is only marked in step, with no
  check, since nothing is written. Otherwise the snapshot is written through the validators a
  save runs, and refused with `ErrInvalidInput` when they refuse it: a catalog copy takes the
  snapshot's name and recipe, keeping its id, and a collection copy is overwritten by key
  through the update core a collection save uses:
  - a snapshot folder whose key names one of the copy's folders keeps that folder's id; the
    others are new folders under their keys, and the copy's folders the snapshot no longer has
    are removed;
  - a snapshot catalog whose key names one of the copy's scoped catalogs, of the same type and
    provider, is a catalog edit of it; any other is a new scoped catalog under its key, and a
    catalog no folder references any more is removed;
  - the copy's pin and Home placement stay; on Home, an update that changes what push sends for
    it leaves it `needs_push`, so Home shows the update as a change to push.

  A copy Update reaches is still as it was written, since nothing else writes one: there is
  nothing to conflict with.
- **What changed** (`snapshot_diff.go`, `snapshot_changes.go`) is one comparison, `diffSnapshots`,
  of two snapshots by snapshot key, returning `SnapshotChange` items (`op` removed, added or
  changed, a `kind` of collection, folder or catalog, and what changed in `aspect`). It serves
  two reads, fetched only when shown, and compares nothing with the push record:
  - `UpdateChanges`: what Update would change in a subscribed copy. The copy as a snapshot under
    its own `sub_key`s (`copyTreeSnapshot`, `copySnapshot`) against the publication's current
    one. `ErrPublicationNotFound` unless the caller subscribes and the publication is live.
  - `CatalogChangesSincePublish`/`CollectionChangesSincePublish`: what publishing an own row
    again would change. The row as it stands, snapshotted under its publication's keys, against the
    stored snapshot: a library catalog a collection uses is in it, so editing one alone shows. A row
    never published is an empty list, a row that can't be published a 400 (`publishableCatalog`,
    `publishableCollection`), and an unpublished row is compared like a live one.

  The items come in the order removals, additions, changes, each in folder order:
  - a folder gone from the other side is one item, then each catalog it holds that the other side
    has nowhere (`folder` names the folder, `genre` the genre it is narrowed to there); in a
    folder both sides hold, each ref the other side lacks, matched by catalog key and genre, so
    narrowing a ref reads as one removed and one added; a catalog moved between folders is removed
    from one and added to the other. Additions are the same read the other way;
  - a catalog whose name or recipe changed is one item however many folders use it, carrying the
    catalog as it is now (`catalog`) and as it was (`was_catalog`) when its recipe changed, and its
    earlier name (`was`) when its name did;
  - a collection's name and settings (view mode, show-all tab, backdrop, focus glow), a folder's
    name, its art (everything it holds but its title and refs) and the order of its catalogs, and the
    order of the folders, each as an item.

  The list is empty exactly when the two snapshots have the same content hash
  (`TestDiffIsEmptyExactlyWhenTheContentHashMatches`), so a row flagged To publish or Update
  available always has something to show, and an edit and its undo has nothing. The SPA puts the
  items into words (`docs/frontend.md`, *Sharing*).
- **Duplicate** (`DuplicatePublication`) is a subscribe without the subscription: an editable copy with
  no `sub_key`s, and any number of them beside a subscription.
- **Community** (`ListCommunity`) is every live publication not the caller's own, newest first,
  in one call; the SPA searches, filters and sorts it. A row is light: counts, dates,
  `subscribed` and `update_available` from a join with the caller's subscriptions, the names of
  the catalogs it holds (`catalog_names`, read from the snapshot, for search), a collection's
  folder titles in order (`folder_titles`, from the same snapshot; `[]` for a catalog), and for a
  catalog its recipe. It never carries a publisher. `GetPublication` returns one publication with its
  snapshot: a live one, or an unpublished one the caller subscribes to.

## Recipe params (TMDB)

The provider is read-only and sessionless: a live TMDB HTTP client plus typed "recipe" structs
describing what a TMDB-backed catalog may ask for.

- **Type hierarchy.** `BaseParams` (provider-agnostic behavior — `randomized`) →
  `TMDBCommonParams` (every discover filter both types share: genres, language, vote and runtime
  ranges, watch providers, production companies and keywords, **and certification**) → `TMDBMovieParams` / `TMDBTVParams`
  (type-specific only: the date window, plus movie's `with_collection` and series'
  `with_networks`). Movie has
  `primary_release_date_*` and `released_within_days`; series has `first_air_date_*` and
  `aired_within_days` — a different axis, since a 2015 show still matches "aired in the last 30
  days". `with_collection` is one TMDB collection id (e.g. `10`, Star Wars), movie only.
  `with_networks` is a comma (AND) or pipe (OR) separated list of TMDB network ids (e.g.
  `213|49`, Netflix or HBO), series only: `/discover/movie` has no network filter. It has no
  `without_networks` partner, because `/discover/tv` accepts that param and ignores it (the same
  total with or without it).
- **A collection catalog is a TMDB collection source, not a discover filter.** A collection
  recipe is a movie recipe with `with_collection` set: its titles are one predefined TMDB
  collection's films. `/discover/movie` accepts `with_collection` and ignores it — the same total
  with or without it — so such a recipe never reaches discover. `collectionItems`
  (`internal/provider/collections.go`), shared by `FetchCatalogPage` and `PreviewCatalog`, reads the films from `/collection/{id}`'s
  `parts` (memoized for 24 hours, since a collection gains films), sorts them by `release_date`
  ascending (undated last, ties by id), filters them locally by `genre_ids` for a client genre
  pick, and shuffles them instead when `randomized`. The whole collection is page 1; a later page
  is empty, and a preview's `total_results` is the filtered count. The addon path still resolves
  each film's IMDB id through `resolveMetas`. `Validate()` makes the recipe exclusive:
  `randomized` is the only field allowed beside `with_collection`, so no saved filter sits
  unapplied. The check walks every field of the params struct by reflection (`firstSetField`)
  and names the first one set, so a field added later is covered without editing it.
- **Validation split, part one: the rules that need no network.** `Validate()` on each leaf type
  checks the `sort_by` enum (per type — movie and tv have different sort vocabularies),
  fixed-vs-rolling date exclusivity, that the rolling window isn't negative, (movie) that
  `with_collection` is a single id rather than a `,`/`|` list with no other filter beside it, and
  (series) that `with_networks` holds at most 20 ids, then delegates
  to `TMDBCommonParams`'s shared check for the two required-together pairs (certification needs
  a country, watch providers need a region), the numeric bounds (`vote_average_*` within
  0–10, and no negative `vote_count_*` or `with_runtime_*`), and the id-list cap:
  `with_companies`, `with_keywords`, `without_companies` and `without_keywords` each hold at
  most 20 ids (`maxEntityIDs`, counted with
  `parseIDList`), so an over-cap recipe is rejected before any TMDB lookup. Zero means "unset" for every numeric
  field (`setIntIf`/`setFloatIf` in `query.go`), so these bound what is sent rather than
  requiring a value. A negative rolling window is rejected rather than ignored — `DiscoverQuery`'s
  `> 0` guard would otherwise drop it silently and save a filter that never applies.
- **Validation split, part two: the vocabulary TMDB owns.** Genre ids, `with_original_language`,
  `watch_region`, `with_watch_providers` and the certification country and scale can only be
  checked against TMDB's own published lists, so they can't live on the params structs —
  `TMDBClient.ValidateParams` (`internal/provider/validate.go`) does it, and
  `api.validateCatalogParams` runs it after `Validate()`. Each list is fetched only when the
  field needing it is set, and every one of them is memoized on the client, so a warm process
  validates a recipe without touching the network at all. A rejected value wraps
  `provider.ErrInvalidParams` (a 400); TMDB being unreachable wraps `errUpstreamValidation`,
  which `writeVaultError` answers with a 502 — a recipe that could not be checked has not been
  found at fault, so it is never reported as the caller's.
- **Companies and keywords are checked per id.** `with_companies` and `with_keywords` are
  comma (AND) or pipe (OR) separated TMDB id lists, like `with_genres`, but TMDB publishes no
  whole list of either — only `/company/{id}`, `/keyword/{id}` and `/search/*`. So
  `checkEntityIDs` shares `checkIDList`'s id-list parse (`parseIDList`) and then looks each id up
  through `TMDBClient.Company`/`Keyword` instead of testing set membership. Each looked-up id is
  memoized in a per-id `memo` with no expiry, bounded at `maxEntityCacheEntries` (see
  `docs/architecture.md`), so a warm process still validates without the network; a TMDB 404
  (`provider.ErrNotFound`) is a rejected value and becomes `ErrInvalidParams`, and is never
  cached, while any other lookup failure stays a 502. A cold cache costs one TMDB call per
  distinct id, which the 20-id cap in `Validate()` bounds at 20 per field.
- **`without_companies` and `without_keywords` are the same id lists, left out.** They are
  checked the same way, through the same lookups, and passed to both discover endpoints under
  the same names. TMDB drops a title carrying any listed id whether the list is comma- or
  pipe-joined, so the builder always stores them comma-joined, as it does `without_genres`.
  Nothing rejects an id that sits in both the `with_` and `without_` list, just as nothing does
  for genres. The builder's pickers keep the two apart instead.
- **`with_collection` goes through the same per-id check, movie only.** `ValidateParams` decodes
  it alongside `TMDBCommonParams` for either catalog type and looks the one id up through
  `TMDBClient.Collection` (`/collection/{id}`). A series recipe carrying it is rejected with
  `ErrInvalidParams`: `DecodeParams` is a plain `json.Unmarshal`, which ignores keys the type
  doesn't declare, so without this the field would be saved on a series catalog and never
  applied.
- **`with_networks` goes through the same per-id check, series only.** `ValidateParams` decodes
  it alongside `with_collection` and looks each id up through `TMDBClient.Network`
  (`/network/{id}`, memoized like the other per-id lookups). A movie recipe carrying it is
  rejected with `ErrInvalidParams`, for the same `json.Unmarshal` reason. Validation never reads
  the network export that network search uses (`docs/architecture.md`), so saving a recipe does
  not depend on TMDB's file host.
- **A publish checks what it publishes; a copy re-checks the form rules.** A publish runs the TMDB
  recipe check over every catalog its snapshot publishes, via a validator passed in by `api` —
  `internal/vault` is the leaf package and cannot reach `internal/provider` — and requires it:
  nil is a programming error, not "skip the check". One rejected recipe fails the whole publish.
  It first runs the form validators the snapshot's copies are written through, because being
  stored is not evidence a row was ever checked: rows written before a given check existed reach
  here too. A catalog snapshot is checked by `CatalogForm.Validate`; a collection snapshot by
  `CollectionForm.Validate` over the form that writes it as a new collection: the collection's
  and every folder's enum values and media URLs, their titles, a folder's cover emoji, each
  folder's ref count and ref genres, and every catalog the copy writes as a new row. A stored row
  that never passed one of those checks — an unrecognized `view_mode`, a title past `maxNameLen`,
  a blank catalog name, a `type` or `provider` Uno doesn't accept, params that aren't JSON — is
  not publishable, and fails with `ErrInvalidInput` → 400. A subscribe, a duplicate and an Update run
  the same form validators over what they write from the snapshot, and nothing else: the
  snapshot's recipes were checked against TMDB when it was published, so none of them makes a
  TMDB call. An Update that writes nothing checks nothing.
  `DuplicateCollection` runs `CollectionForm.Validate` over the form built from the caller's own
  source, with no params check: a recipe TMDB has since outgrown must not block you from
  duplicating your own collection. A stale enum, an overlong title, or a scoped catalog with a
  blank name or an unknown type or provider is stale whoever owns it, and that includes
  `maxNameLen` on the `" (copy)"`-suffixed title it writes, so a collection whose title already
  fills the bound cannot be duplicated rather than being copied into a row the collection
  editor's own save would then refuse. The listed catalogs a Duplicate references are not
  checked: they stay `catalog_id` refs to rows the caller already owns, and nothing is written
  from them.
- **A publish reads and checks before it opens a transaction.** The TMDB check can reach the
  network, and holding SQLite's write lock across a cold-cache call would stall every other
  writer. So `publish` reads the source through the pool and checks it, then opens the write
  transaction, reads the source again and writes the publication only if the source still
  snapshots to what was checked. A Duplicate reads its source through the pool too, then writes
  it through `createCollectionTx`, the create core `CreateUserCollection` runs, each scoped
  catalog copy one of the form's `new` entries.
- **Certification applies to both types.** `certification`, `certification.gte`,
  `certification.lte`, and `certification_country` sit on `TMDBCommonParams` and map in
  `commonQuery` (`internal/provider/query.go`), so `/discover/tv` gets them too. The **value
  vocabulary differs per type** (US movie is `G`/`PG`/`PG-13`/`R`/`NC-17`; US TV is
  `TV-Y`…`TV-MA`), which is why the picker's options come from `GET /api/certifications/{type}`
  rather than a shared hardcoded list.
- **Underscore-to-dot translation lives in one place.** Storage tags are underscore-only
  (`vote_average_gte`) while TMDB's real range params use a dot (`vote_average.gte`);
  `commonQuery` and each recipe type's `DiscoverQuery` translate field-by-field, and both
  `FetchCatalogPage` and `PreviewCatalog` go through them.
- **Vocabulary.** Uno and Stremio say `movie`/`series`; TMDB says `movie`/`tv`. Every route the
  UI calls is on Uno's vocabulary, `GET /api/genres/{type}` included, and the translation is
  entirely server-side via `tmdbMediaType` — the same map `resolveMetas` uses. The one
  place `movie`/`tv` legitimately survives on the client is the genre-lookup keying in
  `web/src/api/types.ts`, because **the two genre id spaces are genuinely separate** (`878`
  Science Fiction is movie-only; tv has `10765` Sci-Fi & Fantasy). That's a real TMDB fact, not a
  wire leak, and nothing persists it.
- **The manifest's genre extra is derived, never stored.** Nothing in `params` configures it.
  Every catalog offers TMDB's genre list for its type, minus the genres that can't narrow its
  recipe (`provider.GenreExtraOptions`; the rule is in `docs/architecture.md`'s addon-server
  section). A client's pick is ANDed onto an AND (comma) `with_genres`, and replaces an OR
  (pipe) one.
- **`randomized` is "shuffle by page":** `FetchCatalogPage` picks a random TMDB page in
  `[1, 20]` (`maxRandomPage`) instead of the requested page. Deep discover pages thin out fast,
  so the range is capped rather than sampled from `total_pages`. A recipe with fewer pages than
  that answers an overshooting pick with an empty page, so an empty randomized page is refetched
  as page 1 — one discover call for a pick that lands, two for one that overshoots. Label it
  honestly in UI ("shuffle"), not "true random". Preview always asks page 1 and returns the flag
  instead. A collection recipe (`with_collection`) has no pages to pick from: `randomized` shuffles
  its film list instead of sorting it by release date.
- **A second provider touches only the code that runs a recipe.**
  - **Storage is provider-neutral.** `recipes` keys a recipe by its type, provider and params
    together, so another provider's recipes never collide with TMDB's. Catalogs, links,
    bundles, Community and push carry the provider string without reading it.
  - **What runs a recipe is TMDB's alone.** There is no `Provider` interface, deliberately —
    deferred until a second provider is real enough to show what it should abstract over.
  - **A new provider needs:**
    - its name in `validProviders` (`vault.CatalogForm.Validate()`,
      `internal/vault/validation.go`) *and* past the provider check at the top of
      `validateCatalogParams` (`internal/api/provider.go`). Miss either and its rows are
      silently rejected everywhere. The `api` check is the one that actually parses `params`,
      so it must reject early rather than rely on the vault check alone;
    - its own params type and canonical form, reached by provider in `provider.decodeRecipe`,
      which `CanonicalParams` goes through. `DecodeParams` maps a catalog type to TMDB's params
      struct only;
    - its own way to fetch a page, preview a recipe and offer genre options. The addon's
      `CatalogHandler` and `buildManifest` and the preview routes call the TMDB client
      directly;
    - its own editor in the SPA, whose catalog editor and `TMDBParams` type are TMDB's.
  - `recipes.provider` is free text at the schema level (`TEXT`, no `CHECK`); the constraint is
    app-level only.

## Bundle format

**A bundle is catalogs and collections with every row id replaced by a key.** It is the file
export writes and import reads, and the in-memory shape every collection copy and a
publication's snapshot are built on. The Go types in `internal/vault/bundle.go` (`Bundle`, `BundleCatalog`,
`BundleCollection`, `BundleFolder`, `BundleRef`) are its only definition. Keys are snake_case.
Version 1:

```json
{ "format": "uno", "version": 1,
  "catalogs": [ { "key": "c1", "name": "80s Horror", "type": "movie", "provider": "tmdb", "params": { } } ],
  "collections": [ {
    "title": "Halloween", "view_mode": "TABBED_GRID", "show_all_tab": true,
    "backdrop_image_url": "", "focus_glow_enabled": true,
    "catalogs": [ { "key": "c2", "name": "Slashers", "type": "movie", "provider": "tmdb", "params": { } } ],
    "folders": [ { "title": "Classics", "tile_shape": "POSTER", "hide_title": false, "cover_emoji": "",
                   "cover_image_url": "", "focus_gif_url": "", "focus_gif_enabled": true,
                   "hero_backdrop_url": "", "hero_video_url": "", "title_logo_url": "",
                   "refs": [ { "catalog": "c1", "genre": "" }, { "catalog": "c2", "genre": "Horror" } ] } ] } ] }
```

- **Two kinds of catalog list.** The top-level `catalogs` are listed catalogs. A collection's own
  `catalogs` are scoped to it; one that no ref uses is ignored on import.
- **Never in the bundle:** row ids, `owner_id`, `pin_to_top`, `collection_id`, timestamps,
  `home_sort_order`, `show_in_home`, `sub_key`, a publication or subscription, `recipe_hash` and
  anything from a push record. A publication's snapshot is built on the same form, with its
  own format name and stable keys (*Publications and subscriptions*, above).
- **Export writes every field.** Booleans are plain bools, and an empty list is `[]`. `params` is
  the stored recipe as a JSON object.
- **Export placement** (`ExportBundle`): the selected listed catalogs come first, in library
  order, then each listed catalog a selected collection references, once across all of them.
  Keys are `c1`, `c2`, … in the order catalogs are emitted, so a collection's scoped catalogs take
  keys between the top-level ones. A scoped catalog can't be selected by itself; it travels
  inside its collection.
- **File-level rules** (`Bundle.Validate`): format `"uno"` and version 1; each key non-empty, at
  most 64 bytes, and unique across the whole bundle; `params` a JSON object; a ref names a
  top-level key or one of its own collection's keys, never another collection's; at most 200
  catalogs (top-level and in collections together) and 50 collections. Every problem is listed in
  one 400. Decoding refuses a key a type has no field for, naming it (the import routes decode
  strictly), and compacts `params`, and `checkRecipe` puts them in canonical form, so an
  imported recipe is stored the way a saved one is.
  Everything else is bounded by the form validators the rows are written through:
  `CatalogForm`'s rules for each new listed catalog, `CollectionForm.Validate` for each
  collection, and `checkRecipe` for every recipe (see `docs/architecture.md`).

**Import creates copies** (`ImportBundle`):

- Every row id is minted by Uno; the file only ever supplies keys. Importing one file twice gives
  two independent sets, unless the second leaves out what the first wrote: the import check
  matches collections by title and catalogs by recipe, and the import route can skip a bundle
  collection with its own catalogs (`skip_collections`, `docs/architecture.md`) before the vault
  sees the bundle.
- Imported rows are unpublished and off Home, subscribed to nothing, and in no push record. Titles are kept as they are, with no "(copy)" suffix.
- **Optional reuse.** `reuse` maps a bundle catalog key, top-level or a collection's own, to one
  of the importer's own *listed* catalogs; its refs then point at that row and no new row is
  written for it. The import check offers the listed catalogs whose recipe matches. A reuse
  target owned by someone else, or scoped to one of the importer's collections, fails the import.
  Reuse can map two bundle catalogs onto one row, so within a folder a repeated (catalog, trimmed
  genre) ref is dropped, keeping the first.
- **All or nothing.** Every check that needs no database runs before the transaction. Inside it,
  the new listed catalogs are inserted first, in bundle order, then each collection through
  `createCollectionTx`. Any failure rolls everything back.

## Push wire shape (Nuvio collections)

**The wire shape is camelCase, and it is not Uno's own.** Real `collections_json` uses
`backdropImageUrl`, `pinToTop`, `focusGlowEnabled`, `viewMode`, `showAllTab`, `coverImageUrl`,
`coverEmoji`, `focusGifUrl`, `focusGifEnabled`, `heroBackdropUrl`, `heroVideoUrl`,
`titleLogoUrl`, `tileShape`, `hideTitle`, plus `addonId`/`type`/`catalogId` inside each source — a different
convention from every other Nuvio surface (RPC params and REST table rows are snake_case) *and*
from Uno's own Builder API. Push therefore has dedicated types in `internal/vault/pushpayload.go`
(`PushCollection`, `PushFolder`, `CatalogSource`) built by `CollectionWithFolders.PushPayload`;
**never `json.Marshal` a `vault.CollectionWithFolders` into this payload.** `wire_test.go` beside
it checks every key they write against the samples. A dangling catalog ref (an id the tree's
catalogs lack) is skipped rather than failing the whole push. `pinToTop` comes from the push's own
selection entry for the collection (`applySelection`, `internal/vault/pushrecord.go`), not from
the stored row, which push then brings up to it.

**Push keeps what it sends.** `PushJSON` is the payload's exact bytes, which push both sends and
stores in the push record, and which a collection read builds again to decide `needs_push` (*Key
rules*). The two can only agree if they marshal the same way, so there is one builder, in the
vault.

**Nuvio's apps fill absent keys with defaults of their own, and disagree.** NuvioTV parses the
blob with Gson (`CollectionsDataStore`), which builds its Kotlin classes without running their
declared defaults: an absent boolean reads as `false` (`showAllTab`, `hideTitle`, `pinToTop`), an
absent `tileShape` as `POSTER` and an absent `viewMode` as `TABBED_GRID`; only `focusGlowEnabled`
and `focusGifEnabled`, read as `?: true`, default to `true`. NuvioMobile and NuvioDesktop parse
with kotlinx, which does run them: `showAllTab` and `focusGifEnabled` default to `true`, and
`tileShape` to a poster. So every boolean and `tileShape` goes out on every push, never
`omitempty`: an omitted value means different things on different apps. The appearance URLs,
`coverImageUrl`, `coverEmoji` and `backdropImageUrl` are `omitempty`, since an absent URL and an
empty one mean the same thing there. A present but empty or unrecognised `tileShape` is `POSTER`
(`PosterShape.fromString`), and a present but empty or unrecognised `viewMode` is `TABBED_GRID`
(`FolderViewMode.fromString`). Read at NuvioTV `e374881`, NuvioMobile `7be1b56` and NuvioDesktop
`ed77003`.

**Arrays go out as arrays, never `null`.** `folders` and each folder's `catalogSources` are `[]`
when empty (`PushPayload` and `pushSources` build them with `make`;
`TestPushPayloadSendsArraysNeverNull` pins it). NuvioTV's parser throws on a null one, and a
throw empties the whole blob it was reading, which the TV then ignores, keeping its local copy
(see *Nuvio integration* in `docs/architecture.md`).

**Field name:** the code sends `catalogSources`, as the public doc documents, and never
`sources`. NuvioTV reads `sources` whenever the key is present, even as `[]`, while NuvioMobile,
NuvioDesktop and nuvio-web fall back to `catalogSources` when `sources` is empty, so a folder
carrying both would be empty on the TV alone. **Confirmed against a real Nuvio profile:** a
collection Uno has pushed round-trips its folder sources under `catalogSources`. A collection
built or re-saved in a Nuvio app writes `sources` — the two sample files below use it. Push's
merge (`pushCollections`, `internal/api/push.go`) reads only a pulled collection's `id`, so the
key doesn't matter there.

**The real `sources[]` entry is wider than what Uno emits.** The entries in
`docs/api/samples/collections-basic.json` carry five keys — `addonId`, `catalogId`, `type`,
`genre`, `provider`. The entries in `docs/api/samples/collections-extended.json` carry thirteen: those five
plus `filters`, `mediaType`, `sortBy`, `sortHow`, `title`, `tmdbId`, `tmdbSourceType`,
`traktListId`. So a Nuvio folder can source content from TMDB directly and from Trakt lists, not
only from an installed addon's catalog, and can sort and filter per reference. Uno emits only the
addon-catalog form (`PushPayload` → `vault.CatalogSource`: `addonId`, `type`,
`catalogId`, plus `genre` when the reference has one), which is correct for what Uno owns; collections Uno doesn't own pass through push as raw `json.RawMessage`, which is what keeps
their wider entries intact, and that protection holds only while Uno never imports one into its
own database.
