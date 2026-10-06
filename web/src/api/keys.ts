import type { QueryClient } from '@tanstack/react-query'
import type { CatalogType } from './types'

/**
 * Query keys, scoped by profile wherever the endpoint is.
 *
 * Everything under `/api/p/{i}/...` keys on `['p', profileIndex, …]` so
 * switching profile slots invalidates the whole subtree in one call and no
 * stale row from another profile can survive — this includes the community
 * routes, which are profile-scoped (they exclude the caller's own publications
 * and mark the ones it subscribes to). Genre lookups are account-wide, so they sit
 * outside that prefix and stay cached across a profile switch.
 */
export const queryKeys = {
  profile: (profileIndex: number) => ['p', profileIndex] as const,

  /** `GET /api/profiles` — the caller's Nuvio account, not any one selected
   *  profile, so this sits outside the `['p', i, …]` prefix like the genre
   *  keys below. */
  profiles: () => ['profiles'] as const,

  /** `GET /api/config` — the server's setup, the same for every account. */
  serverConfig: () => ['config'] as const,
  /** `GET /api/account/tmdb-key` — the signed-in account's own TMDB key, so
   *  outside the `['p', i, …]` prefix like `profiles`. */
  tmdbKey: () => ['account', 'tmdb-key'] as const,

  ownedCatalogs: (profileIndex: number) => ['p', profileIndex, 'catalogs'] as const,
  ownedCollections: (profileIndex: number) => ['p', profileIndex, 'collections'] as const,

  /** What a push would change in Nuvio: edits and deletes since the last one.
   *  Any write to a row Nuvio may hold refreshes it, and so does a push. */
  pendingPush: (profileIndex: number) => ['p', profileIndex, 'push', 'pending'] as const,

  /** Beside the owned-list keys rather than under them, so refreshing a
   *  library list leaves Community alone; a write that changes Community
   *  refreshes this prefix itself. Every Community key sits under it. */
  community: (profileIndex: number) => ['p', profileIndex, 'community'] as const,
  /** Every live publication Community lists, in one call. */
  communityList: (profileIndex: number) => ['p', profileIndex, 'community', 'list'] as const,
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
 * Marks this profile's own catalog and collection lists stale, with what a push
 * would change and every Community query beside them — what a write that can
 * add a copy, change a row's sharing or change what Nuvio holds has to refresh.
 * Settles once every active query has refetched.
 */
export async function invalidateProfileLists(queryClient: QueryClient, profileIndex: number): Promise<void> {
  await Promise.all(
    [
      queryKeys.ownedCatalogs(profileIndex),
      queryKeys.ownedCollections(profileIndex),
      queryKeys.pendingPush(profileIndex),
      queryKeys.community(profileIndex),
    ].map((queryKey) => queryClient.invalidateQueries({ queryKey })),
  )
}
