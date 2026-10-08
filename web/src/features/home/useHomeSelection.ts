import { use, useMemo } from 'react'
import { HomeEditsContext, HomeSelectionContext } from './HomeSelectionContext'
import type { HomeEdits, HomeSelection } from './HomeSelectionContext'
import { buildHomePreview, type HomeScreenPreview } from './preview'

/** The pending Home: its rows, what a push would change, and the
 *  `home_revision` the next push sends. Throws outside a
 *  `HomeSelectionProvider`. */
export function useHomeSelection(): HomeSelection {
  const value = use(HomeSelectionContext)
  if (value === null) {
    throw new Error('useHomeSelection must be used inside <HomeSelectionProvider>')
  }
  return value
}

/** The actions that edit the pending Home (add, remove, reorder, toggle).
 *  Throws outside a `HomeSelectionProvider`. */
export function useHomeEdits(): HomeEdits {
  const value = use(HomeEditsContext)
  if (value === null) {
    throw new Error('useHomeEdits must be used inside <HomeSelectionProvider>')
  }
  return value
}

/** The pending home screen in Nuvio's own two bands — what both the List and
 *  the Preview view draw, so they can't disagree about the order. */
export function useHomePreview(): HomeScreenPreview {
  const { rows, catalogById, collectionById } = useHomeSelection()
  return useMemo(() => buildHomePreview({ rows, catalogById, collectionById }), [rows, catalogById, collectionById])
}

export type { HomeEdits, HomeSelection }
