import { use, useMemo } from 'react'
import { HomeEditsContext, HomeSelectionContext } from './HomeSelectionContext'
import type { HomeEdits, HomeSelection } from './HomeSelectionContext'
import { buildHomePreview, type HomeScreenPreview } from './preview'

export function useHomeSelection(): HomeSelection {
  const value = use(HomeSelectionContext)
  if (value === null) {
    throw new Error('useHomeSelection must be used inside <HomeSelectionProvider>')
  }
  return value
}

export function useHomeEdits(): HomeEdits {
  const value = use(HomeEditsContext)
  if (value === null) {
    throw new Error('useHomeEdits must be used inside <HomeSelectionProvider>')
  }
  return value
}

/** The pending home screen in Nuvio's own three bands — what both the List
 *  and the Preview view draw, so they can't disagree about the order. */
export function useHomePreview(): HomeScreenPreview {
  const { catalogs, collections, catalogById, collectionById } = useHomeSelection()
  return useMemo(
    () => buildHomePreview({ catalogs, collections, catalogById, collectionById }),
    [catalogs, collections, catalogById, collectionById],
  )
}

export type { HomeEdits, HomeSelection }
