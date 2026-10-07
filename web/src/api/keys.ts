import type { QueryClient } from '@tanstack/react-query'
import type { CatalogType, CommunityQuery } from './types'

/**
 * Query keys, scoped by profile wherever the endpoint is.
 *
 * Everything under `/api/p/{i}/...` keys on `['p', profileIndex, …]` so
 * opening the builder on a profile drops the whole subtree in one call
 * (`watchActiveProfile`) and no stale row from an earlier visit can survive —
 * this includes the community
 * routes, which are profile-scoped (they exclude the caller's own publications
 * and mark the ones it subscribes to). Genre lookups are account-wide, so they sit
 * outside that prefix and stay cached across a profile switch.
 */
export const queryKeys = {
  profile: (profileIndex: number) => ['p', profileIndex] as const,
  /** The prefix every profile's subtree sits under. */
  allProfiles: () => ['p'] as const,

  /** `GET /api/profiles` — the caller's Nuvio account, not any one selected
   *  profile, so this sits outside the `['p', i, …]` prefix like the genre
   *  keys below. */
  profiles: () => ['profiles'] as const,

  /** `GET /api/config` — the server's setup, the same for every account. */
  serverConfig: () => ['config'] as const,
  /** `GET /api/account/tmdb-key` — the signed-in account's own TMDB key, so
   *  outside the `['p', i, …]` prefix like `profiles`. */
  tmdbKey: () => ['account', 'tmdb-key'] as const,

  /** `GET /api/p/{i}/library`: the profile's own catalogs and collections and
   *  what a push would change in Nuvio, in one read. Any write to a row refreshes
   *  it, and so does a push. */
  library: (profileIndex: number) => ['p', profileIndex, 'library'] as const,

  /** Beside the library key rather than under it, so refreshing the library
   *  leaves Community alone; a write that changes Community refreshes this
   *  prefix itself. Every Community key sits under it. */
  community: (profileIndex: number) => ['p', profileIndex, 'community'] as const,

  /** The rows released and not yet acknowledged, each told about once
   *  (`ReleasedDialog`). */
  released: (profileIndex: number) => ['p', profileIndex, 'released'] as const,
  /** Community's pages for one query, read a page at a time. */
  communityList: (profileIndex: number, query: CommunityQuery) =>
    ['p', profileIndex, 'community', 'list', query.kind, query.sort, query.q] as const,
  publication: (profileIndex: number, publicationID: string) =>
    ['p', profileIndex, 'community', 'publication', publicationID] as const,

  /** What an Update would change in an added row, and what publishing an own
   *  row again would change. Under neither the library's nor Community's
   *  prefix, so a write never waits on them: they are read when shown and kept
   *  no longer than that (`useChanges`). */
  updateChanges: (profileIndex: number, publicationID: string) =>
    ['p', profileIndex, 'changes', 'update', publicationID] as const,
  changesSincePublish: (profileIndex: number, kind: 'catalog' | 'collection', id: string) =>
    ['p', profileIndex, 'changes', kind, id] as const,

  genres: (type: CatalogType) => ['genres', type] as const,
  certifications: (type: CatalogType) => ['certifications', type] as const,
  languages: () => ['languages'] as const,
  countries: () => ['countries'] as const,

  /** Keyed on region as well as type: the same service has a different id in
   *  each market, so two regions are two different lists. */
  watchProviders: (type: CatalogType, region: string) =>
    ['watch-providers', type, region] as const,
  watchRegions: () => ['watch-regions'] as const,

  /** Search results vary by query and over time, so these keep the default
   *  staleTime. The by-id keys name one id, which never changes, and are read
   *  with `staleTime: Infinity`. */
  companySearch: (type: CatalogType, query: string) => ['companies', 'search', type, query] as const,
  company: (id: number) => ['companies', 'id', id] as const,
  keywordSearch: (query: string) => ['keywords', 'search', query] as const,
  keyword: (id: number) => ['keywords', 'id', id] as const,
  /** TMDB movie collections, prefixed apart from Uno's own collections. */
  collectionSearch: (query: string) => ['tmdb-collections', 'search', query] as const,
  collection: (id: number) => ['tmdb-collections', 'id', id] as const,
  networkSearch: (query: string) => ['networks', 'search', query] as const,
  network: (id: number) => ['networks', 'id', id] as const,

  /**
   * Keyed on the **recipe**, not on a catalog id. Two catalogs with identical
   * filters share one cache entry, as does an unsaved catalog in the builder
   * that matches a saved one — which is also what makes the folder page's "All"
   * tab free: it fetches every source, and the per-catalog tabs clicked
   * afterwards hit the same, already-resolved keys.
   *
   * Account-wide, so it sits outside the `['p', i, …]` prefix — a recipe's
   * results don't depend on which profile asked.
   *
   * `genre` is part of the key: a folder reference narrowed to one genre is a
   * different result from the same recipe unfiltered. `''` is unfiltered.
   */
  catalogPreview: (type: CatalogType, params: string, genre = '') =>
    ['catalogs', 'preview', type, params, genre] as const,
  /** The prefix of every `catalogPreview` key. */
  catalogPreviews: () => ['catalogs', 'preview'] as const,
  /** The genres a pick can narrow this recipe by — keyed on the recipe for the
   *  same reason `catalogPreview` is. */
  catalogGenreOptions: (type: CatalogType, params: string) =>
    ['catalogs', 'genre-options', type, params] as const,
} as const

/**
 * Marks this profile's library stale, with every Community query beside it —
 * what a write that can add a copy, change a row's sharing or change what Nuvio
 * holds has to refresh. Settles once every active query has refetched.
 */
export async function invalidateProfileLists(queryClient: QueryClient, profileIndex: number): Promise<void> {
  await Promise.all(
    [queryKeys.library(profileIndex), queryKeys.community(profileIndex)].map((queryKey) =>
      queryClient.invalidateQueries({ queryKey }),
    ),
  )
}
