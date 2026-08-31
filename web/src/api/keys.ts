import type { CatalogType } from './types'

/**
 * Query keys, scoped by profile wherever the endpoint is.
 *
 * Everything under `/api/p/{i}/...` keys on `['p', profileIndex, …]` so
 * switching profile slots invalidates the whole subtree in one call and no
 * stale row from another profile can survive. Community reads and genre
 * lookups are account-wide, so they sit outside that prefix and stay cached
 * across a profile switch.
 */
export const queryKeys = {
  profile: (profileIndex: number) => ['p', profileIndex] as const,

  /** `GET /api/profiles` — the caller's Nuvio account, not any one selected
   *  profile, so this sits outside the `['p', i, …]` prefix like the
   *  community and genre keys below. */
  profiles: () => ['profiles'] as const,

  ownedCatalogs: (profileIndex: number) => ['p', profileIndex, 'catalogs'] as const,
  ownedCollections: (profileIndex: number) => ['p', profileIndex, 'collections'] as const,

  catalogSelection: (profileIndex: number) =>
    ['p', profileIndex, 'catalogs', 'selection'] as const,
  collectionSelection: (profileIndex: number) =>
    ['p', profileIndex, 'collections', 'selection'] as const,

  communityCatalogs: () => ['catalogs', 'community'] as const,
  communityCollections: () => ['collections', 'community'] as const,

  genres: (type: CatalogType) => ['genres', type] as const,
  certifications: (type: CatalogType) => ['certifications', type] as const,
  languages: () => ['languages'] as const,
  countries: () => ['countries'] as const,

  /**
   * Keyed on the **recipe**, not on a catalog id. Two catalogs with identical
   * filters share one cache entry, as does an unsaved catalog in the builder
   * that matches a saved one — which is also what makes the folder page's "All"
   * tab free: it fetches every source, and the per-catalog tabs clicked
   * afterwards hit the same, already-resolved keys.
   *
   * Account-wide, so it sits outside the `['p', i, …]` prefix — a recipe's
   * results don't depend on which profile asked.
   */
  catalogPreview: (type: CatalogType, params: string) =>
    ['catalogs', 'preview', type, params] as const,
} as const
