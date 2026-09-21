import { use } from 'react'
import { HomeSelectionContext } from './HomeSelectionContext'
import type { HomeSelection } from './HomeSelectionContext'

export function useHomeSelection(): HomeSelection {
  const value = use(HomeSelectionContext)
  if (value === null) {
    throw new Error('useHomeSelection must be used inside <HomeSelectionProvider>')
  }
  return value
}

export type { HomeSelection }
