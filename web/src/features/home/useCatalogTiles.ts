import { useMemo } from 'react'
import type { CatalogTiles } from '@/features/preview/tiles'
import { useRecipesTiles, type TileRecipe } from '@/features/preview/useRecipesTiles'
import { useHomeSelection } from './useHomeSelection'

/**
 * Tiles for the Home pane's catalog rows — the List view's strips and the
 * Preview's rows.
 *
 * Resolution only: `ids` are catalog ids, and the recipe behind each is looked
 * up in `catalogById`, which merges the library with the selection response. An
 * id that resolves to nothing is skipped — a selected row nothing describes
 * any more has no recipe to run. `useRecipesTiles` does the fetching.
 */
export function useCatalogTiles(ids: readonly string[]): ReadonlyMap<string, CatalogTiles> {
  const home = useHomeSelection()

  // Memoised on the *joined* ids, not the array, so a caller passing an inline
  // `.map(...)` doesn't rebuild the query list on every render. The ids are
  // read back out of the joined string rather than closed over, so the
  // dependency list stays complete without a lint escape.
  const idKey = ids.join(' ')
  const recipes = useMemo<TileRecipe[]>(
    () =>
      (idKey === '' ? [] : idKey.split(' '))
        .map((id) => {
          const catalog = home.catalogById.get(id)
          return catalog ? { id, type: catalog.type, params: catalog.params } : null
        })
        .filter((recipe) => recipe !== null),
    [idKey, home.catalogById],
  )

  return useRecipesTiles(recipes)
}
