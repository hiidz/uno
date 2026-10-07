import type { QueryClient } from '@tanstack/react-query'
import type { Location } from 'react-router-dom'
import { queryKeys } from '@/api/keys'

/** The slice of the data router `watchActiveProfile` reads. */
interface WatchedRouter {
  state: { location: Pick<Location, 'pathname' | 'state'> }
  subscribe: (listener: (state: { location: Pick<Location, 'pathname' | 'state'> }) => void) => () => void
}

/**
 * The profile the builder is showing at this location, or null off `/configure`.
 * The router's location state is the one place the active profile is set: the
 * picker navigates with it, and a history entry carries it back.
 */
function activeProfileIndex(location: Pick<Location, 'pathname' | 'state'>): number | null {
  if (location.pathname !== '/configure') return null
  const state = location.state as { profileIndex?: unknown } | null
  return typeof state?.profileIndex === 'number' ? state.profileIndex : null
}

/**
 * Drops every profile's cached queries each time the builder opens on a
 * profile — from the picker, from a history entry, or from one profile's entry
 * to another's — so what it shows is fetched, not whatever a visit to some other
 * profile left in the cache. The drop happens as the location changes, ahead of
 * the render that mounts the builder, so nothing is observing what goes.
 */
export function watchActiveProfile(router: WatchedRouter, queryClient: QueryClient): () => void {
  let active = activeProfileIndex(router.state.location)
  return router.subscribe((state) => {
    const next = activeProfileIndex(state.location)
    if (next === active) return
    active = next
    if (next !== null) queryClient.removeQueries({ queryKey: queryKeys.allProfiles() })
  })
}
