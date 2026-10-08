import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { fetchCatalogPreview, queryKeys } from '@/api'
import type { CatalogType } from '@/api'
import { noTiles, type CatalogTiles } from './tiles'
import { PREVIEW_QUERY_OPTIONS, tilesFrom } from './useRecipesTiles'

/**
 * TMDB tiles for a recipe that has no saved catalog behind it — one still being
 * typed in the builder.
 *
 * **On demand, never on change.** `useCatalogTiles` fetches for the user, as
 * soon as a preview view opens; this fetches only when someone presses the
 * button. A recipe under edit changes on every keystroke, and a request per
 * keystroke is both wasteful and unreadable — the tiles would never settle long
 * enough to judge.
 *
 * The key is `queryKeys.catalogPreview`, the same one the Home pane uses, so a
 * recipe that matches a catalog already on the home screen resolves from cache
 * with no request at all.
 */
export interface RecipePreview {
  tiles: CatalogTiles
  /**
   * TMDB's count of matches across every page these filters return, not just
   * `tiles.items` — which is one page, capped at `TILES_PER_PAGE`. `null`
   * before a result has landed (idle, loading, or errored), matching `tiles`.
   */
  totalResults: number | null
  /** Nothing has been requested yet — draw the button and nothing else. */
  idle: boolean
  /**
   * The filters have moved on since the tiles on screen were fetched.
   *
   * The one way this feature can mislead: tiles from an older recipe sitting
   * under filters that no longer produced them. The caller must say so rather
   * than let them pass as current. Not cleared automatically — going blank on
   * every keystroke destroys the comparison the preview exists for.
   */
  isStale: boolean
  /** Fetch the recipe as it stands now. */
  run: () => void
}

/** The recipe a request was made for, as opposed to the one in the form now. */
interface Requested {
  type: CatalogType
  params: string
}

/** The catalog editor's on-demand preview of one recipe. Nothing is fetched
 *  until `run`; after that, `isStale` says when the form has moved on from the
 *  recipe the tiles on screen came from. */
export function useRecipeTiles(type: CatalogType, params: string): RecipePreview {
  const [requested, setRequested] = useState<Requested | null>(null)

  const query = useQuery({
    // Non-null asserted through `enabled`: the query never runs while
    // `requested` is null, and the key is only read when it does.
    queryKey: queryKeys.catalogPreview(requested?.type ?? type, requested?.params ?? params),
    queryFn: () => fetchCatalogPreview({ type: requested!.type, params: requested!.params }),
    enabled: requested !== null,
    ...PREVIEW_QUERY_OPTIONS,
  })

  const isSameRecipe =
    requested !== null && requested.type === type && requested.params === params

  function run() {
    if (isSameRecipe) {
      // Same key, so setting state would change nothing and the cached result
      // would stand. That's right for a fixed result, and wrong for a failure —
      // a second press has to be a real retry — or for a shuffled recipe, whose
      // next run is a different page.
      if (query.isError || query.data?.randomized) void query.refetch()
      return
    }
    setRequested({ type, params })
  }

  return {
    tiles: requested === null ? noTiles() : tilesFrom(query),
    totalResults: query.data?.total_results ?? null,
    idle: requested === null,
    isStale: requested !== null && !isSameRecipe,
    run,
  }
}
