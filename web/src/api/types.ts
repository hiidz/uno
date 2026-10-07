/**
 * Hand-written to match the Go wire format — no codegen. Field names are the
 * snake_case JSON tags on `internal/vault/models.go`, not the Go field names.
 *
 * `params` is a JSON-encoded *string*, not a nested object.
 *
 * Nested arrays are typed `| null` as a guard — the Go side initialises every
 * list it returns, so `null` shouldn't reach here, but each read site coerces
 * anyway. `getList` does the same for top-level lists.
 */

export type CatalogType = 'movie' | 'series'

/** TMDB's own vocabulary for the same distinction. `series → tv` is a
 *  client-side translation only — never persisted. */
export type TMDBKind = 'movie' | 'tv'

export function tmdbKind(type: CatalogType): TMDBKind {
  return type === 'series' ? 'tv' : 'movie'
}

export interface Catalog {
  id: string
  type: CatalogType
  name: string
  provider: string
  /** JSON-encoded TMDBParams. Parse with `parseParams`, never `JSON.parse` at
   *  the call site — a malformed value must not take the list down. */
  params: string
  /** Scopes the catalog to one collection (hidden from the library, usable
   *  only in that collection's folders); `null` means listed. */
  collection_id: string | null
  /** Its place on Home, numbered with the collections there; absent when it
   *  isn't on Home. */
  home_position?: number
  /** On Home with a home row of its own; `false` for Discover only, and for
   *  every catalog off Home. Written only by Push. */
  show_in_home: boolean
  /** This catalog's own publication; `null` while it isn't published. Only
   *  the owner's own reads carry it. */
  publication: PublicationState | null
  /** The publication this listed catalog is a subscribed copy of; `null`
   *  for the owner's own catalog, and always `null` inside a collection. */
  subscription: SubscriptionState | null
  /** Its publisher unpublished what it was added from, which made it this
   *  profile's own; cleared once the owner acknowledges the release
   *  (`POST .../acknowledge-release`), never by a save. */
  publisher_unpublished: boolean
}

/** What a publisher's row shows of its publication. `changed_since_publish`
 *  is true once the saved row differs from what was published: people who
 *  added it keep getting the published version until the publisher publishes
 *  an update. */
export interface PublicationState {
  id: string
  changed_since_publish: boolean
}

/** What a subscribed copy shows of the publication it was subscribed from.
 *  Only an Update changes the copy; a save of it is refused. */
export interface SubscriptionState {
  publication_id: string
  update_available: boolean
}

export type TileShape = 'POSTER' | 'LANDSCAPE' | 'SQUARE'

/**
 * A collection's `view_mode`, as `validViewModes` in
 * `internal/vault/validation.go` holds it: required, and nothing else. It is a
 * collection-level setting that applies to every folder; it describes how a
 * folder's catalogs are laid out once you're inside it, not how the collection
 * itself sits on home.
 */
export type ViewMode = 'TABBED_GRID' | 'ROWS'

/** How a folder looks: everything it holds but its identity, title and refs.
 *  The wire shape of `vault.FolderArt`, shared by the saved folder, a
 *  snapshot's folder and the save payload. */
export interface FolderLook {
  tile_shape: TileShape
  hide_title: boolean
  cover_emoji: string
  cover_image_url: string
  /** An animated GIF Nuvio plays over the tile while it's focused, when
   *  `focus_gif_enabled` is set. */
  focus_gif_url: string
  focus_gif_enabled: boolean
  /** Hero media for Nuvio's Modern Home layout. */
  hero_backdrop_url: string
  hero_video_url: string
  title_logo_url: string
}

export type Folder = FolderLook & {
  id: string
  title: string
  /** Ordered. One catalog can appear more than once, under different genres. */
  refs: FolderRef[] | null
}

/** One entry in a folder's catalog list — `vault.FolderRef`. */
interface FolderRef {
  catalog_id: string
  /** The genre this reference is narrowed to, pushed as the folder source's
   *  `genre`; `''` for unfiltered. */
  genre: string
}

/** A collection's own settings, as a saved collection and a snapshot's
 *  collection both carry them. */
interface CollectionSettings {
  view_mode: ViewMode
  show_all_tab: boolean
  backdrop_image_url: string
  /** Nuvio's focus glow on this collection's home-screen folder cards. */
  focus_glow_enabled: boolean
}

export type Collection = CollectionSettings & {
  id: string
  title: string
  /** Show first, as last pushed: only Push writes it, from Home's pending
   *  selection (`HomeEntry.pinToTop`). */
  pin_to_top: boolean
  /** Its place on Home, numbered with the catalogs there; absent when it
   *  isn't on Home. */
  home_position?: number
  /** As on `Catalog`. A subscribed collection's catalogs are all scoped to
   *  it, so they carry no subscription of their own. */
  publication: PublicationState | null
  subscription: SubscriptionState | null
  /** As on `Catalog`. */
  publisher_unpublished: boolean
  folders: Folder[] | null
  /** Every catalog this collection's folders reference, listed or scoped —
   *  so the editor never needs the library to render a folder. */
  catalogs: Catalog[] | null
}

/**
 * One row `GET /api/p/{i}/push/pending` says a push would change in Nuvio:
 * `changed` (Nuvio holds it differently — a collection also when a catalog its
 * folders use changed), `added` (on Home, and Nuvio holds nothing for it) or
 * `removed` (deleted since the last push, which drops it). `name` is the row's
 * name now, or as the last push left it for a removed one.
 */
export interface PendingChange {
  kind: 'catalog' | 'collection'
  id: string
  name: string
  change: 'added' | 'changed' | 'removed'
}

/** One of the profile's rows released and not yet acknowledged — a copy
 *  whose publication ended, now the profile's own — as `GET /api/p/{i}/released`
 *  lists them, oldest release first. `name` is a collection's title. */
export interface ReleasedCopy {
  kind: 'catalog' | 'collection'
  id: string
  name: string
}

/** One catalog of a snapshot or a diff, in the bundle form: named by a `key`
 *  rather than an id, with `params` an object rather than the JSON-encoded
 *  string a `Catalog` carries. */
export interface SnapshotCatalog {
  key: string
  name: string
  type: CatalogType
  provider: string
  params: TMDBParams
}

/** One folder ref of a snapshot, naming its catalog by snapshot `key`. */
interface SnapshotRef {
  catalog: string
  genre: string
}

export type SnapshotFolder = FolderLook & {
  key: string
  title: string
  refs: SnapshotRef[] | null
}

type SnapshotCollection = CollectionSettings & {
  title: string
  folders: SnapshotFolder[] | null
}

/** What a publication froze when it was published: every catalog it publishes,
 *  at the top level, and for a collection its own fields and folders. */
interface Snapshot {
  format: string
  version: number
  catalogs: SnapshotCatalog[] | null
  collection?: SnapshotCollection
}

/** One row of `GET /api/p/{i}/community`: a publication someone else
 *  publishes, never naming its publisher. `catalog_names` names every catalog it
 *  holds, for search. `folders` lists a collection's folders in order as their
 *  tiles show them, empty for a catalog. `catalog` is a catalog publication's one catalog, so
 *  its row can be summarized without a detail call; `null` for a
 *  collection. */
export interface CommunityItem {
  id: string
  kind: 'catalog' | 'collection'
  title: string
  subscriber_count: number
  published_at: string
  updated_at: string
  subscribed: boolean
  update_available: boolean
  catalog_names: string[] | null
  folders: CommunityFolder[] | null
  catalog: SnapshotCatalog | null
}

/** One folder of a listed collection, as much as its tile shows. */
export interface CommunityFolder {
  title: string
  tile_shape: TileShape
  cover_emoji: string
  cover_image_url: string
}

/** `GET /api/p/{i}/community/{id}`: one publication with its snapshot. */
export interface PublicationDetail extends CommunityItem {
  snapshot: Snapshot
}

/**
 * One difference between two snapshots, from `GET .../changes` and
 * `GET .../changes-since-publish`: removals first, then additions, then
 * changes, each in folder order. A folder or a catalog is added or removed
 * (`folder` names the folder a catalog goes into or leaves, `genre` the genre
 * it is narrowed to there); a changed one names what changed in `aspect`, with
 * `was` its earlier name when renamed, and a catalog whose recipe changed
 * carries it as it is now in `catalog` and as it was in `was_catalog`. `key`
 * is the snapshot key of the folder or catalog an item is about, and
 * `folder_key` that of the folder a catalog goes into or leaves; a
 * collection's own items have neither.
 */
export interface SnapshotChange {
  op: 'removed' | 'added' | 'changed'
  kind: 'collection' | 'folder' | 'catalog'
  aspect?: 'name' | 'recipe' | 'settings' | 'art' | 'order' | 'catalog_order'
  key?: string
  name?: string
  was?: string
  folder_key?: string
  folder?: string
  genre?: string
  catalog?: SnapshotCatalog
  was_catalog?: SnapshotCatalog
}

/** What a subscribe, a duplicate or an Update answers: the caller's copy, a listed
 *  catalog or a collection by the publication's kind. */
export interface CommunityCopy {
  kind: 'catalog' | 'collection'
  catalog?: Catalog
  collection?: Collection
}

/** `POST /api/p/{i}/import/check` — what a bundle holds, in bundle order, for
 *  the import dialog to list: its top-level catalogs, and each collection with
 *  its folders' titles and its own catalogs, each marked with what it matches
 *  in this profile's library. A collection's position here is the one an
 *  import's `skip_collections` names. */
export interface ImportCheck {
  catalogs: ImportCatalog[]
  collections: ImportCollection[]
}

/** One bundle catalog as the dialog lists it, `params` in the canonical form
 *  a stored catalog's take. `existing` is every listed catalog of this
 *  profile's with the same recipe, sorted by name, which the import may point
 *  the catalog at instead of copying it; empty when none matches. */
export interface ImportCatalog {
  key: string
  name: string
  type: CatalogType
  params: string
  existing: Array<{ id: string; name: string }>
}

/** One bundle collection as the dialog lists it. `matched` is whether its
 *  title, trimmed and in any case, is one of this profile's collections'. */
interface ImportCollection {
  title: string
  folders: string[]
  matched: boolean
  catalogs: ImportCatalog[]
}

/** `POST /api/p/{i}/import` — the new listed catalogs and the new
 *  collections. A catalog the import reused is not among `catalogs`, and
 *  neither is a catalog imported inside a collection. */
export interface ImportResult {
  catalogs: Catalog[]
  collections: Collection[]
}

export interface Genre {
  id: number
  name: string
}

/** One entry in a country's age-rating scale, e.g. US movie "PG-13". `order`
 *  is the scale's own ranking (lowest to highest) — sort by it, not
 *  alphabetically, when building a min/max picker. */
export interface Certification {
  certification: string
  meaning: string
  order: number
}

/** `GET /api/certifications/{type}` — every country's rating scale, keyed by
 *  ISO 3166-1 country code. */
export type CertificationsByCountry = Record<string, Certification[]>

/** One entry in TMDB's ISO 639-1 language table — `iso_639_1` is what
 *  `with_original_language` expects; `english_name` is what a person reads. */
export interface Language {
  iso_639_1: string
  english_name: string
  name: string
}

/** One entry in TMDB's ISO 3166-1 country table — names the country codes
 *  `GET /api/certifications/{type}` returns, which come back as codes only. */
export interface Country {
  iso_3166_1: string
  english_name: string
  native_name: string
}

/** One entry from `GET /api/watch-providers/{type}` — a streaming service
 *  `with_watch_providers` accepts. `provider_id` is what goes on the wire;
 *  `provider_name` is what the picker shows. Already sorted server-side by
 *  TMDB's own prominence ranking. */
export interface WatchProvider {
  provider_id: number
  provider_name: string
  display_priority: number
  logo_path: string
}

/** One entry from `GET /api/watch-regions` — a country TMDB has streaming
 *  data for, which is what `watch_region` accepts. A strict subset of
 *  `Country`, so the two are not interchangeable. */
export interface WatchRegion {
  iso_3166_1: string
  english_name: string
  native_name: string
}

/** `GET /api/companies/{id}` — a production company `with_companies`
 *  accepts. */
export interface Company {
  id: number
  name: string
}

/** One result from `GET /api/companies/search?type=`. The server keeps only
 *  companies with at least 5 titles of that type, sorted by `title_count`
 *  descending, top 10. `origin_country` is an ISO 3166-1 code, or `""` when
 *  TMDB has none. */
export interface CompanySearchResult extends Company {
  origin_country: string
  /** How many titles of the searched type TMDB credits to the company. */
  title_count: number
}

/** One result from `GET /api/keywords/search` or `GET /api/keywords/{id}` —
 *  a TMDB keyword `with_keywords` accepts. */
export interface Keyword {
  id: number
  name: string
}

/** `GET /api/networks/{id}` — a TV network `with_networks` accepts.
 *  `origin_country` is an ISO 3166-1 code, or `""` when TMDB has none. */
export interface Network {
  id: number
  name: string
  origin_country: string
}

/** One result from `GET /api/networks/search`: the same shape and rules as
 *  `CompanySearchResult`, with `title_count` counting series. */
export interface NetworkSearchResult extends Network {
  title_count: number
}

/** One result from `GET /api/collections/search` or `GET /api/collections/{id}`
 *  — a TMDB movie collection `with_collection` accepts, e.g. "Star Wars Collection".
 *  Not Uno's own `Collection`. */
export interface TMDBCollection {
  id: number
  name: string
}

/**
 * One tile from `POST /api/catalogs/preview`.
 *
 * `tmdb_id` is a render key and nothing else — it is **not** an addon meta id.
 * Stremio needs IMDB ids, so the addon path spends ~20 extra calls per page
 * resolving them; preview skips that and stays a single TMDB call.
 */
export interface PreviewItem {
  tmdb_id: number
  title: string
  year: string
  /** Absolute `image.tmdb.org` URL, or `''` when TMDB has no poster. */
  poster?: string
}

export interface CatalogPreview {
  /** The recipe shuffles: the addon path picks a random TMDB page per call
   *  while preview always fetches page 1, so these titles won't match Nuvio.
   *  Surfaced so the UI can say so rather than implying a prediction. */
  randomized: boolean
  items: PreviewItem[]
  /** TMDB's count of matches across every page these filters return, not just
   *  `items` — which is one page. */
  total_results: number
}

/** The body `POST /api/catalogs/preview` takes: a recipe, not a catalog.
 *  There is no `provider` or `endpoint` — the first is hardcoded server-side,
 *  the second derived from `type` there. Accepting a path from the client would
 *  let it point the server's own TMDB requests off-host. */
export interface PreviewRequest {
  type: CatalogType
  /** The JSON-encoded params string, exactly as `Catalog.params` carries it. */
  params: string
  /** A genre name from the recipe's genre options, narrowing the result the way
   *  a folder reference's genre narrows that row in Nuvio. */
  genre?: string
}

/** The body `POST /api/catalogs/genre-options` takes — a recipe, like
 *  `PreviewRequest`, so a draft catalog with no id has options too. */
export interface GenreOptionsRequest {
  type: CatalogType
  params: string
}

/**
 * One profile in the caller's Nuvio account — `GET /api/profiles`, matching
 * `nuvio.NuvioProfile`. Not a Uno vault row: this exists before Uno has ever
 * seen the profile, which is exactly what the picker is choosing between.
 */
export interface NuvioProfile {
  id: string
  user_id: string
  profile_index: number
  name: string
  /** The profile uses profile 1's addons in Nuvio: Nuvio's apps read profile
   *  1's addon list for it, so Uno's addon pushed here would show nowhere,
   *  and push refuses it. */
  uses_primary_addons: boolean
  /** The circle Nuvio draws for a profile without a picture. */
  avatar_color_hex: string
  /** The picture Nuvio's apps show for the profile: its own upload, else its
   *  built-in avatar's image in Nuvio's storage; `''` when they show its
   *  colour. Added by Uno (`pickerProfile`), not one of Nuvio's fields. */
  avatar_image_url: string
  /** The profile has a PIN in Nuvio. Uno doesn't ask for it. */
  pin_enabled: boolean
}

/**
 * `POST /api/profiles/select`'s response: the profile's `manifest_url`, which
 * the handler computes at selection time from `SITE_BASE_URL` (never sent to
 * the client directly) so the builder can show the addon URL before any push.
 */
export interface SelectedProfile {
  manifest_url: string
}

/**
 * The union of `TMDBCommonParams`, `TMDBMovieParams` and `TMDBTVParams` from
 * `internal/provider/models.go`. Every field is optional because Go marshals
 * with `omitempty` — an absent field means "not filtered on", never zero.
 *
 * Movie-only and series-only fields are marked; the catalog's `type` decides
 * which set is meaningful, and the backend validates accordingly.
 */
export interface TMDBParams {
  randomized?: boolean

  sort_by?: string
  with_genres?: string
  without_genres?: string
  with_original_language?: string

  vote_average_gte?: number
  vote_average_lte?: number
  vote_count_gte?: number
  vote_count_lte?: number
  with_runtime_gte?: number
  with_runtime_lte?: number

  with_watch_providers?: string
  watch_region?: string

  /** TMDB id lists: comma-joined means all of them, pipe-joined any of them. */
  with_companies?: string
  with_keywords?: string
  /** Comma-joined TMDB id lists; a title carrying any of them is left out. */
  without_companies?: string
  without_keywords?: string

  certification?: string
  certification_gte?: string
  certification_lte?: string
  certification_country?: string

  // movie only
  primary_release_date_gte?: string
  primary_release_date_lte?: string
  released_within_days?: number
  /** One TMDB collection id; TMDB takes no list here. */
  with_collection?: string

  // series only
  first_air_date_gte?: string
  first_air_date_lte?: string
  aired_within_days?: number
  /** TMDB network ids: comma-joined means all of them, pipe-joined any of
   *  them. TMDB has no network exclusion, so there is no `without_networks`. */
  with_networks?: string
}

/** `GET /api/config`: how this server reaches TMDB. `per-account` means each
 *  Nuvio account brings its own TMDB key, which the picker asks for. */
export interface ServerConfig {
  tmdb_key_mode: 'shared' | 'per-account'
}

/** `GET /api/account/tmdb-key`: all the SPA is told of the signed-in
 *  account's TMDB key — whether it has saved one, and its last four
 *  characters, which come only with a saved key. The key itself never comes
 *  back. */
export type TMDBKeyStatus = { set: false } | { set: true; last4: string }
