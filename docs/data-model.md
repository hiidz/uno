# Data model

SQLite, defined in `internal/vault/schema.go`. Row structs and wire DTOs are in
`internal/vault/models.go`, all with explicit `snake_case` JSON tags. `internal/vault/db.go`
opens the database in WAL mode with a 5s `busy_timeout` and foreign keys on.

There are no migrations: `InitDB` runs `CREATE TABLE IF NOT EXISTS`, which never alters an
existing table. Any schema change means deleting and recreating both the local `vault.db` and the
`uno-data` compose volume (`docker compose down -v`).

```mermaid
erDiagram
  PROFILES ||--o{ CATALOGS : owns
  PROFILES ||--o{ COLLECTIONS : owns
  COLLECTIONS ||--o{ FOLDERS : contains
  COLLECTIONS ||--o{ CATALOGS : scopes
  FOLDERS ||--o{ FOLDER_CATALOGS : contains
  CATALOGS ||--o{ FOLDER_CATALOGS : "referenced via"

  PROFILES {
    uuid id PK
    string token UK
    string nuvio_user_id
    int nuvio_profile_index
    string nuvio_profile_uuid
  }
  CATALOGS {
    uuid id PK
    string type
    string name
    string provider
    json params
    uuid owner_id FK
    bool is_public
    bool is_default
    uuid collection_id FK "nullable — NULL means listed"
    int home_sort_order "nullable — NULL means not on the TV"
    bool show_in_home
    uuid taken_from FK "nullable — bookkeeping only, never rendered"
    string fingerprint "sha256 hex of type+provider+canonical params"
    string created_at
    string updated_at
  }
  COLLECTIONS {
    uuid id PK
    string title
    uuid owner_id FK
    bool is_public
    bool is_default
    bool pin_to_top
    string view_mode
    bool show_all_tab
    string backdrop_image_url
    bool focus_glow_enabled "defaults to 1, matching Nuvio"
    int home_sort_order "nullable — NULL means not on the TV"
    int version "starts at 1, +1 on every content write; never touched by push"
    int pushed_version "nullable — NULL means never pushed; the version push last read and sent"
    uuid taken_from FK "nullable — bookkeeping only, never rendered"
    string created_at
    string updated_at
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
overwrites** the stored UUID on drift rather than prompting; nothing acts on the tripwire.

`profiles.token` has exactly one job: identifying a profile in the public addon URLs. It is not a
write credential.

## Key rules

- **A catalog has a scope: listed or scoped to one collection.** `catalogs.collection_id` is
  `NULL` for a listed catalog (in the library, usable on home and in any of the owner's folders)
  or a collection id for one scoped to exactly that collection (hidden from the library, usable
  only in that collection's folders, deleted with it). `CreateUserCatalog`/`UpdateUserCatalog`
  enforce that the target collection is owned by the same profile, that a scoped catalog is never
  public, and that a scoped catalog is never on the home screen — the schema's own
  `CHECK (collection_id IS NULL OR (is_public = 0 AND home_sort_order IS NULL))` exists as a
  backstop and would surface as a 500, so the Go layer rejects all three before that CHECK is ever
  hit. Demoting a listed catalog into a collection (`UpdateUserCatalog` with `collection_id` set)
  additionally requires it to be off the home screen (`requireNotOnHome`, checking
  `home_sort_order IS NULL` directly on the row) and every existing folder ref to it to already be
  inside the target collection; promoting a scoped catalog back to listed (clearing
  `collection_id`) is always allowed. `GetUserCatalogs` (the library) returns listed catalogs
  only — a scoped one is reached through its owning collection's own response instead.
- **A profile's data graph is closed: references never cross an owner boundary.** A folder may
  reference a catalog only if `internal/vault/access.go`'s `validateFolderRefs` accepts it: the
  catalog's `owner_id` must equal the collection's `owner_id`, and the catalog's `collection_id`
  must be `NULL` (listed) or equal to that same collection (scoped to it already). There is no
  "or public" branch anywhere in a write path — a community catalog can only enter another
  profile's graph through Take (a copy with a fresh id), never through a live reference.
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
- **`catalogs.fingerprint` and `catalogs.taken_from`/`collections.taken_from` back the
  cross-owner sharing model.** `fingerprint` is a sha256 hex of the catalog's type, provider, and
  canonically re-marshaled params (`provider.Fingerprint`), computed by the create/update
  handlers before every insert/update — `GetCommunityCatalogs` collapses rows sharing a
  fingerprint to the oldest `created_at`, and it never reaches the wire. `taken_from` records the
  source row `TakeCatalog`/`TakeCollection` copied from, purely to answer "you already took this"
  (`taken: bool` on community rows, an `EXISTS` against the caller's own `taken_from` values);
  both are `json:"-"` and never rendered as attribution.
- **`catalogs.id` is permanent once created** — never rename or recycle it. It is baked into
  `addon.ManifestID` and therefore into Nuvio's `catalogSources[].catalogId`.
- **`catalogs.params` is opaque `TEXT` at the schema level.** For `provider = 'tmdb'` there is an
  app-level shape in `internal/provider` (`TMDBMovieParams` / `TMDBTVParams` on
  `TMDBCommonParams` + `BaseParams`), with `Validate()` covering cross-field rules, dispatched by
  `validateCatalogParams` from the create/update handlers. It crosses the wire as a JSON-encoded
  **string**, not a nested object.
- **`catalogs.type` is immutable once the row exists.** `UpdateUserCatalog` reads the stored
  `type` in its own opening `SELECT` and rejects the write with `ErrInvalidInput` if the incoming
  form's `type` differs — a catalog's type is baked into the pushed collections blob (each folder
  source names its catalog's type), so changing it would alter what Nuvio should have without
  bumping any collection's `version`. The catalog editor already locks the field once a row
  exists; this is the write path enforcing the same rule server-side. Duplicate is the supported
  way to get a different-typed copy.
- **Two distinct removal mechanisms — don't conflate them.**
  - **Hard delete** (`DeleteUserCatalog` / `DeleteUserCollection`): owner-scoped single `DELETE`,
    all downstream cleanup via `ON DELETE CASCADE`. A public row leaves Community, but every copy
    another profile already took survives: those copies are independent rows whose `taken_from`
    is `ON DELETE SET NULL`, so the delete only clears that bookkeeping link.
  - **Unselect**: reachable only through push, which folds the whole pending selection straight
    into `catalogs.home_sort_order`/`show_in_home` and `collections.home_sort_order`
    (`saveCatalogSelectionTx`/`saveCollectionSelectionTx`, `internal/vault`). Every owned row's
    `home_sort_order` is cleared first, then each incoming id is set in turn with its array index;
    an id that isn't owned (or, for a catalog, isn't listed — `AND collection_id IS NULL`) affects
    0 rows and is `ErrInvalidInput` naming the id. There is no separate join table and no separate
    access-check query — the `UPDATE`'s own `WHERE` clause is the validation.
- **`catalogs.show_in_home` drives the manifest's per-catalog genre extra, but the addon server
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
  remains the pre-push validation/selection-editor view; only the addon server needs the wider
  published set.
- **`collections.version` bumps on every content write (`UpdateUserCollection`), starting at 1 on
  insert (`CreateUserCollection`, `TakeCollection`, `DuplicateCollection`) — push never touches it.**
  `collections.pushed_version` is stamped by `SaveSelectionsForPush` with the version
  `pushCollections` read for that collection *before* calling Nuvio, for every collection in the
  pushed selection, and only there. `NULL` means never pushed. The frontend flags a pending change
  when `version !== pushed_version` (`web/src/features/home/changes.ts`) — an integer compare, not
  a clock, so a Save landing between push's read and its local write (even inside the same second)
  is never mistaken for pushed.
- **No cascade on `owner_id`** (`catalogs`/`collections`). Irrelevant until profile deletion
  exists; revisit then.
- **`folder_catalogs.genre` narrows one folder reference, not the catalog.** It is a genre
  *name* from the catalog's manifest `genre` extra (`provider.GenreExtraOptions`), `''` for
  unfiltered. Push sends it as the folder source's `genre`, and Nuvio sends that back as the
  `genre` extra when it loads the folder's row, so the same catalog can show Westerns in one
  folder and war films in another, or both in one folder, with no second catalog. It is stored as a name rather than a
  TMDB id because the name is what goes over the wire both ways. It is not validated against the
  recipe on save: a later recipe edit can leave a stored genre outside the options (now required
  or excluded), and the addon path then serves that row unfiltered, the same as any unknown
  extra. The collection editor flags that case rather than clearing it. Take and Duplicate carry
  it onto the copy (`copyCollectionTree`). On the wire each folder carries an ordered
  `refs: [{catalog_id, genre}]` (`vault.FolderRef`), not a list of catalog ids, because one
  catalog can be two refs. A genre picked in Nuvio's own editor doesn't survive a push, because
  push rebuilds every Uno-managed collection from Uno's data.
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
- **`folders.tile_shape` defaults to `'LANDSCAPE'` at the schema level, and Preview reads `''`
  as `POSTER`.** The two point different directions and neither is wrong: the schema
  default only applies to a row inserted without the column, and the collection editor always
  sends a value — including `''`, which it keeps as its own "Default" option rather than
  normalising. Nothing inserts a folder without `tile_shape`, so the schema default is
  unreachable in practice. Preview's `POSTER` is what the TV does: push always sends `tileShape`,
  and Nuvio maps `''` or any unrecognised value to `POSTER`. Nuvio's `SQUARE` default applies
  only when the key is absent, which a Uno push never produces (see "Push wire shape" below).
- **`catalogs.created_at`/`updated_at` and `collections.created_at`/`updated_at` are `TEXT`
  RFC3339 UTC**, generated in Go with `time.Now().UTC().Format(time.RFC3339)` and parsed back to
  `time.Time` in `internal/vault/scan.go`; `encoding/json` serialises the Go field as RFC3339 on
  the wire. Every insert sets both to the same instant; every update rewrites only `updated_at`.
- **Dead-by-design columns.** `is_default` on `catalogs`/`collections` is never read and never
  set true by any code path, and is tagged `json:"-"` on both structs so it doesn't reach the
  wire; the column stays because a real "default catalog" feature is buildable later and
  dropping it would force a delete-and-recreate.
- **Nuvio appearance fields.** `collections.focus_glow_enabled` (the TV's focus glow on the
  collection's home-screen folder cards), `folders.focus_gif_url`/`focus_gif_enabled` (an
  animated GIF played over a folder tile while it's focused), and
  `folders.hero_backdrop_url`/`hero_video_url`/`title_logo_url` (hero media that Nuvio's own
  editor labels "(Modern Home)") are stored, edited in the collection editor, copied by
  Take/Duplicate, and pushed. Uno's Preview renders none of them. The two flags default to `1`
  because Nuvio reads an absent flag as on.
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

## Recipe params (TMDB)

The provider is read-only and sessionless: a live TMDB HTTP client plus typed "recipe" structs
describing what a TMDB-backed catalog may ask for.

- **Type hierarchy.** `BaseParams` (provider-agnostic behavior — `randomized`) →
  `TMDBCommonParams` (every discover filter both types share: genres, language, vote and runtime
  ranges, watch providers, **and certification**) → `TMDBMovieParams` / `TMDBTVParams`
  (type-specific only: the date window). Movie has `primary_release_date_*` and
  `released_within_days`; series has `first_air_date_*` and `aired_within_days` — a different
  axis, since a 2015 show still matches "aired in the last 30 days".
- **Validation split.** `Validate()` on each leaf type checks the `sort_by` enum (per type —
  movie and tv have different sort vocabularies) and fixed-vs-rolling date exclusivity, then
  delegates to `TMDBCommonParams`'s shared check for the two required-together pairs:
  certification needs a country, watch providers need a region.
- **Certification applies to both types.** `certification`, `certification.gte`,
  `certification.lte`, and `certification_country` sit on `TMDBCommonParams` and map in
  `commonQuery` (`internal/provider/query.go`), so `/discover/tv` gets them too. The **value
  vocabulary differs per type** (US movie is `G`/`PG`/`PG-13`/`R`/`NC-17`; US TV is
  `TV-Y`…`TV-MA`), which is why the picker's options come from `GET /api/certifications/{type}`
  rather than a shared hardcoded list.
- **Underscore-to-dot translation lives in one place.** Storage tags are underscore-only
  (`vote_average_gte`) while TMDB's real range params use a dot (`vote_average.gte`);
  `buildDiscoverQuery`/`commonQuery` translate field-by-field, and both `FetchCatalogPage` and
  `PreviewCatalog` go through them.
- **Vocabulary.** Uno and Stremio say `movie`/`series`; TMDB says `movie`/`tv`. Every route the
  UI calls is on Uno's vocabulary, `GET /api/genres/{type}` included, and the translation is
  entirely server-side via `externalIDsMediaType` — the same map `resolveMetas` uses. The one
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
  so the range is capped rather than sampled from `total_pages`. Label it honestly in UI
  ("shuffle"), not "true random". Preview always asks page 1 and returns the flag instead.
- **A second provider needs two places updated, not one.** `validProviders` in
  `vault.CatalogForm.Validate()` (`internal/vault/validation.go`) *and* the provider check at the
  top of `validateCatalogParams` (`internal/api/provider.go`), plus its own recipe type and
  branch in the latter. Miss either and its rows are silently rejected everywhere. The `api`
  check is the one that actually parses `params`, so it must reject early rather than rely on
  the vault check alone. `catalogs.provider` is free text at the schema level (`TEXT`, no
  `CHECK`); the constraint is app-level only. There is no `Provider` interface, deliberately —
  deferred until a second provider is real enough to show what the interface should abstract
  over.

## Push wire shape (Nuvio collections)

**The wire shape is camelCase, and it is not Uno's own.** Real `collections_json` uses
`backdropImageUrl`, `pinToTop`, `focusGlowEnabled`, `viewMode`, `showAllTab`, `coverImageUrl`,
`coverEmoji`, `focusGifUrl`, `focusGifEnabled`, `heroBackdropUrl`, `heroVideoUrl`,
`titleLogoUrl`, `tileShape`, `hideTitle`, plus `addonId`/`type`/`catalogId` inside each source — a different
convention from every other Nuvio surface (RPC params and REST table rows are snake_case) *and*
from Uno's own Builder API. Push therefore has dedicated types in `internal/nuvio/types.go`
(`PushCollection`, `PushFolder`, `CatalogSource`) built by `buildPushCollection`; **never
`json.Marshal` a `vault.CollectionWithFolders` into this payload.** A dangling catalog ref (an id
missing from the resolved map) is skipped rather than failing the whole push.

**Nuvio fills absent keys with its own defaults**, and they don't all match Uno's (from
NuvioTV's `CollectionsDataStore` and `domain/model/Collection.kt`): `focusGlowEnabled`,
`focusGifEnabled` and `showAllTab` default to `true`, and `tileShape` defaults to `SQUARE`.
So every boolean and `tileShape` goes out on every push, never `omitempty`: an omitted `false`
would read as `true` on the TV. The appearance URLs, `coverImageUrl`, `coverEmoji` and
`backdropImageUrl` are `omitempty`, since an absent URL and an empty one mean the same thing
there. A present but empty or unrecognised `tileShape` is `POSTER` (`PosterShape.fromString`).

**Field name:** the code sends `catalogSources`, as the public doc documents. **Confirmed against
a real Nuvio profile:** a collection Uno has pushed round-trips its folder sources
under that same key, `catalogSources` — pulling it back after a push shows `catalogSources`
populated and `sources` empty. A Nuvio-native collection (built in Nuvio's own UI, never pushed
by Uno) may still use `sources` — the two sample files below, which predate any Uno push, use it
— so push's own merge (`pushCollections`, `internal/api/push.go`) still parses **both** keys when
deciding whether a pulled collection is Uno-managed (`isUnoManaged`); the Uno-pushed case is
confirmed to use `catalogSources` only.

**The real `sources[]` entry is wider than what Uno emits.** The entries in
`docs/api/samples/collections-basic.json` carry five keys — `addonId`, `catalogId`, `type`,
`genre`, `provider`. The entries in `docs/api/samples/collections-extended.json` carry thirteen: those five
plus `filters`, `mediaType`, `sortBy`, `sortHow`, `title`, `tmdbId`, `tmdbSourceType`,
`traktListId`. So a Nuvio folder can source content from TMDB directly and from Trakt lists, not
only from an installed addon's catalog, and can sort and filter per reference. Uno emits only the
addon-catalog form (`buildPushCollection` → `nuvio.CatalogSource`: `addonId`, `type`,
`catalogId`, plus `genre` when the reference has one), which is correct for what Uno owns; collections Uno doesn't own pass through push as raw `json.RawMessage`, which is what keeps
their wider entries intact, and that protection holds only while Uno never imports one into its
own database.
