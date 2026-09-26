import { useQueries } from '@tanstack/react-query'
import { fetchCatalogPreview, queryKeys } from '@/api'
import type { CatalogPreview, CatalogType } from '@/api'
import type { CatalogTiles } from './tiles'

/**
 * Real TMDB tiles for a set of recipes a preview view is about to draw.
 *
 * One query per recipe, never a batch, so rows fail, retry and cache
 * independently and one slow TMDB call can't hold up every row.
 *
 * All rows are fetched at once, with no lazy loading: preview rows don't scroll
 * and there's no "load more", so one call is the complete answer for a row.
 *
 * There is no server-side cache behind this, so the query cache here is the
 * only one. That's load-bearing for the folder page's "All" tab: All fetches
 * every source, and because the keys are per-recipe, clicking through the
 * per-catalog tabs afterwards costs nothing.
 *
 * Takes recipes rather than catalog ids so it serves every caller: the Home
 * pane, which resolves ids through its own `catalogById` (see
 * `useCatalogTiles`), and a folder page, whose sources carry their own recipe
 * and genre — the collection editor's draft included.
 */
export interface TileRecipe {
  /** The key the resulting tiles are filed under — a catalog id for a Home
   *  row, a source's `key` on a folder page. This hook never looks it up,
   *  only hands it back. */
  id: string
  type: CatalogType
  params: string
  /** A folder reference's genre, narrowing the recipe; absent or `''` is
   *  unfiltered. */
  genre?: string
}

/** How long a recipe's tiles stay fresh, and no retry: a 400 means the recipe
 *  is invalid and a 502 means TMDB won't recover inside a retry window.
 *  Asking again is the retry — the Run button, or reopening the view. */
export const PREVIEW_QUERY_OPTIONS = { staleTime: 5 * 60_000, retry: false } as const

/** A preview query's state as the tiles a view draws. */
export function tilesFrom(result: {
  data?: CatalogPreview
  isPending: boolean
  isError: boolean
}): CatalogTiles {
  return {
    items: result.data?.items ?? [],
    randomized: result.data?.randomized ?? false,
    isLoading: result.isPending,
    isError: result.isError,
  }
}

export function useRecipesTiles(
  recipes: readonly TileRecipe[],
): ReadonlyMap<string, CatalogTiles> {
  const results = useQueries({
    queries: recipes.map((recipe) => ({
      // Per-recipe, not per-catalog-id: two catalogs with identical filters
      // resolve to one entry and one fetch. TanStack also dedupes duplicate
      // keys within a single useQueries call, which is what makes a folder
      // holding the same catalog twice free rather than doubled.
      queryKey: queryKeys.catalogPreview(recipe.type, recipe.params, recipe.genre),
      queryFn: () =>
        fetchCatalogPreview({ type: recipe.type, params: recipe.params, genre: recipe.genre || undefined }),
      ...PREVIEW_QUERY_OPTIONS,
    })),
  })

  const tiles = new Map<string, CatalogTiles>()
  recipes.forEach((recipe, i) => {
    const result = results[i]
    if (result) tiles.set(recipe.id, tilesFrom(result))
  })
  return tiles
}
