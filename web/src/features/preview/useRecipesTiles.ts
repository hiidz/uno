import { useQueries } from '@tanstack/react-query'
import { fetchCatalogPreview, queryKeys } from '@/api'
import type { CatalogType } from '@/api'
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
 * Takes recipes rather than catalog ids so it serves both callers: the Home
 * pane, which resolves ids through its own `catalogById` (see
 * `useCatalogTiles`), and the collection builder, which already holds the
 * catalogs a folder references and has no Home state to resolve through.
 */
export interface TileRecipe {
  /** The key the resulting tiles are filed under — a catalog id at both call
   *  sites, but this hook never looks it up, only hands it back. */
  id: string
  type: CatalogType
  params: string
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
      queryKey: queryKeys.catalogPreview(recipe.type, recipe.params),
      queryFn: () => fetchCatalogPreview({ type: recipe.type, params: recipe.params }),
      staleTime: 5 * 60_000,
      // No retry: a 400 means the recipe is invalid and a 502 means TMDB won't
      // recover inside a retry window. Either way placeholders stay, and
      // reopening the view is the manual retry.
      retry: false,
    })),
  })

  const tiles = new Map<string, CatalogTiles>()
  recipes.forEach((recipe, i) => {
    const result = results[i]
    if (!result) return
    tiles.set(recipe.id, {
      items: result.data?.items ?? [],
      randomized: result.data?.randomized ?? false,
      isLoading: result.isPending,
      isError: result.isError,
    })
  })
  return tiles
}
