import { useCallback, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { fetchCatalogPreview, queryKeys } from '@/api'
import type { CatalogType } from '@/api'
import { noTiles, type CatalogTiles } from './tiles'

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
  /**
   * Forget what was requested, back to `idle`.
   *
   * A builder form outlives any one catalog it edits — it's seeded from a new
   * target rather than remounted — so without this, opening a second catalog
   * would inherit the first one's tiles and merely label them stale. Callers
   * reset wherever they seed the form.
   */
  reset: () => void
}

/** The recipe a request was made for, as opposed to the one in the form now. */
interface Requested {
  type: CatalogType
  params: string
}

export function useRecipeTiles(type: CatalogType, params: string): RecipePreview {
  const [requested, setRequested] = useState<Requested | null>(null)

  const query = useQuery({
    // Non-null asserted through `enabled`: the query never runs while
    // `requested` is null, and the key is only read when it does.
    queryKey: queryKeys.catalogPreview(requested?.type ?? type, requested?.params ?? params),
    queryFn: () => fetchCatalogPreview({ type: requested!.type, params: requested!.params }),
    enabled: requested !== null,
    staleTime: 5 * 60_000,
    // No retry: a 400 means the recipe is invalid and a 502 means TMDB won't
    // recover inside a retry window. Pressing the button again is the retry,
    // and unlike the Home pane there is always someone there to press it.
    retry: false,
  })

  const isSameRecipe =
    requested !== null && requested.type === type && requested.params === params

  function run() {
    if (isSameRecipe) {
      // Same key, so setting state would change nothing and the cached result
      // would stand. That's right for a result, and wrong for a failure — a
      // second press has to be a real retry.
      if (query.isError) void query.refetch()
      return
    }
    setRequested({ type, params })
  }

  // Stable, so a caller can reset from the same effect that seeds the form
  // without that effect re-running on every render.
  const reset = useCallback(() => setRequested(null), [])

  return {
    tiles:
      requested === null
        ? noTiles()
        : {
            items: query.data?.items ?? [],
            randomized: query.data?.randomized ?? false,
            isLoading: query.isPending,
            isError: query.isError,
          },
    idle: requested === null,
    isStale: requested !== null && !isSameRecipe,
    run,
    reset,
  }
}
