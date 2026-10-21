import { useSyncExternalStore } from 'react'
import type { QueryClient } from '@tanstack/react-query'
import { queryKeys } from '@/api/keys'

const STORAGE_PREFIX = 'uno:profile:'

/** The profile this tab's builder is open on, for one account. Seeded once
 *  from storage, then held here: another tab's pick never moves it. */
let active: { account: string; index: number | null } | null = null
const listeners = new Set<() => void>()

/** The profile the builder opens on for `account`: the last one picked in
 *  this tab, or else the last one picked in this browser, or null. */
export function activeProfile(account: string): number | null {
  if (active?.account !== account) active = { account, index: readStored(account) }
  return active.index
}

/** Opens the builder on `index` for `account` from now on, in this tab and in
 *  any tab or reload that starts after. Every pick is announced, the same
 *  profile's again included (`watchActiveProfile`). */
export function pickProfile(account: string, index: number) {
  active = { account, index }
  try {
    localStorage.setItem(STORAGE_PREFIX + account, String(index))
  } catch {
    // Storage is blocked: the pick lasts as long as this tab, and a reload
    // asks again.
  }
  for (const listener of listeners) listener()
}

function readStored(account: string): number | null {
  let stored: string | null = null
  try {
    stored = localStorage.getItem(STORAGE_PREFIX + account)
  } catch {
    return null
  }
  return stored !== null && /^[1-6]$/.test(stored) ? Number(stored) : null
}

function subscribe(listener: () => void): () => void {
  listeners.add(listener)
  return () => listeners.delete(listener)
}

/** `account`'s active profile, or null when none is picked. */
export function useActiveProfile(account: string): number | null {
  return useSyncExternalStore(subscribe, () => activeProfile(account))
}

/**
 * Drops every profile's cached queries each time a profile is picked, the
 * same one again included, so what the builder shows is fetched, not whatever
 * a visit to some other profile left in the cache. The pick happens ahead of
 * the navigation that mounts the builder, so nothing is observing what goes.
 * A change of account clears the whole cache (`lib/query-client.ts`).
 */
export function watchActiveProfile(queryClient: QueryClient): () => void {
  return subscribe(() => queryClient.removeQueries({ queryKey: queryKeys.allProfiles() }))
}
