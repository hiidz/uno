import { use } from 'react'
import { HomeEditsContext, HomeSelectionContext } from './HomeSelectionContext'
import type { HomeEdits, HomeSelection } from './HomeSelectionContext'

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

export type { HomeEdits, HomeSelection }
