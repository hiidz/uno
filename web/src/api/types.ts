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
  owner_id: string
  is_public: boolean
  /** Scopes the catalog to one collection (hidden from the library, usable
   *  only in that collection's folders); `null` means listed. */
  collection_id: string | null
  created_at: string
  updated_at: string
  /** True for a listed catalog still linked to the community catalog it was
   *  taken from: Community offers it Update, and a save that changes its name
   *  or recipe unlinks it. Always false on a catalog inside a collection. */
  linked: boolean
}

export type TileShape = 'POSTER' | 'LANDSCAPE' | 'SQUARE'

export interface Folder {
  id: string
  collection_id: string
  title: string
  sort_order: number
  tile_shape: TileShape | ''
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
  /** Ordered. One catalog can appear more than once, under different genres. */
  refs: FolderRef[] | null
}

/** One entry in a folder's catalog list — `vault.FolderRef`. */
export interface FolderRef {
  catalog_id: string
  /** The genre this reference is narrowed to, pushed as the folder source's
   *  `genre`; `''` for unfiltered. */
  genre: string
}

export interface Collection {
  id: string
  title: string
  owner_id: string
  is_public: boolean
  pin_to_top: boolean
  view_mode: string
  show_all_tab: boolean
  backdrop_image_url: string
  /** Nuvio's focus glow on this collection's home-screen folder cards. */
  focus_glow_enabled: boolean
  created_at: string
  updated_at: string
  /** Bumped by every content write (never touched by push); starts at 1. */
  version: number
  /** The `version` push last read and sent for this collection; `null` means
   *  never pushed. A collection is pending re-push when `version !==
   *  pushed_version`. */
  pushed_version: number | null
  /** True while this collection is linked to the community collection it was
   *  taken from: Community offers it Update, and a save that changes anything
   *  besides `is_public` unlinks it. */
  linked: boolean
  folders: Folder[] | null
  /** Every catalog this collection's folders reference, listed or scoped —
   *  so the editor never needs the library to render a folder. */
  catalogs: Catalog[] | null
}

/**
 * `GET /api/p/{i}/catalogs/selection` — the full catalog row plus its own
 * `show_in_home` flag, ordered by home position.
 *
 * The closed graph means a catalog has exactly one owner and can only ever
 * be selected by that owner, so `show_in_home` is a property of the catalog
 * row itself (`catalogs.show_in_home`) — this shape exists for the ordering
 * and the response's wire stability, not to disambiguate per-profile values.
 */
export interface SelectedCatalog extends Catalog {
  show_in_home: boolean
}

/** `GET /api/p/{i}/community/catalogs` — a public catalog owned by someone
 *  else, one row per distinct recipe. `taken` is true while this profile
 *  holds a linked copy of it, and `update_available` while that copy is
 *  behind it. */
export interface CommunityCatalog extends Catalog {
  taken: boolean
  update_available: boolean
}

/** `GET /api/p/{i}/community/collections` — a public collection owned by
 *  someone else. `taken` is true while this profile holds a linked copy of
 *  it, and `update_available` while that copy is behind it. */
export interface CommunityCollection extends Collection {
  taken: boolean
  update_available: boolean
}

/** `POST /api/p/{i}/import/check` — what a bundle holds, and every catalog in
 *  it whose recipe matches one of this profile's listed catalogs. `catalogs`
 *  counts top-level and collection catalogs together. */
export interface ImportCheck {
  catalogs: number
  collections: number
  folders: number
  matches: ImportMatch[]
}

/** One bundle catalog the import may point at an existing catalog instead of
 *  copying. `scope` is `listed` for a top-level catalog, with `collection`
 *  empty, and `scoped` for one of a collection's own, with `collection` that
 *  collection's title. `existing` is sorted by name and never empty. */
export interface ImportMatch {
  key: string
  name: string
  type: CatalogType
  scope: 'listed' | 'scoped'
  collection: string
  existing: Array<{ id: string; name: string }>
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
}

/**
 * `POST /api/profiles/select`'s response — `vault.Profile` embedded (Uno's
 * own resolved-or-created row for this profile) plus `manifest_url`, which
 * the handler computes at selection time from `SITE_BASE_URL` (never sent to
 * the client directly) so the builder can show the addon URL before any push.
 */
export interface SelectedProfile {
  id: string
  token: string
  nuvio_user_id: string
  nuvio_profile_index: number
  nuvio_profile_uuid: string
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
