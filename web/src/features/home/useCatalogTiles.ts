import { useMemo } from 'react'
import type { CatalogTiles } from '@/features/preview/tiles'
import { useRecipesTiles, type TileRecipe } from '@/features/preview/useRecipesTiles'
import { useHomeSelection } from './useHomeSelection'

/**
 * Tiles for the catalogs the Home pane's Preview view is about to draw.
 *
 * Resolution only: `ids` are catalog ids, and the recipe behind each is looked
 * up in `catalogById`, which merges the library with the selection response. An
 * id that resolves to nothing is skipped — a folder can reference a catalog
 * that has since been deleted, and there is no recipe to run for it.
 * `useRecipesTiles` does the fetching.
 *
 * A recipe with no saved catalog behind it — one still being typed in a builder
 * form — has no id to look up, so it uses `useRecipeTiles` instead.
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
