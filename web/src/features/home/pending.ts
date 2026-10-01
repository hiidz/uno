/**
 * The Home pane's state, and the diff that drives the unpushed-changes count.
 *
 * Selection is edited entirely in browser memory. Nothing here writes to the
 * server — the whole selection travels in Push's request body, in one call.
 * So `baseline` is what the server last told us is live, and `current` is what
 * the user has pending; the gap between them is the only thing telling the
 * user their work isn't saved yet.
 */

import type { PushRequest } from '@/api'
import { moveByOne, orderByKeys } from '@/lib/order'

export interface HomeCatalogEntry {
  id: string
  /** Mirrors `catalogs.show_in_home`. */
  showInHome: boolean
}

export interface HomeCollectionEntry {
  id: string
  /** Mirrors `collections.pin_to_top`: Show first, which lifts the collection
   *  above every catalog row. Only push writes it, from here. */
  pinToTop: boolean
}

export interface HomeState {
  /** Ordered. Array position *is* `sort_order` at write time. */
  catalogs: HomeCatalogEntry[]
  /** Ordered, same rule. */
  collections: HomeCollectionEntry[]
}

export const EMPTY_HOME: HomeState = { catalogs: [], collections: [] }

/**
 * The wire shape Push sends. Array position *is* `sort_order` on both sides,
 * which is why this is a straight positional map and never sorts.
 */
export function toPushPayload(state: HomeState): PushRequest {
  return {
    catalogs: {
      catalogs: state.catalogs.map((c) => ({ catalog_id: c.id, show_in_home: c.showInHome })),
    },
    collections: {
      collections: state.collections.map((c) => ({ collection_id: c.id, pin_to_top: c.pinToTop })),
    },
  }
}

/**
 * Reorders the subset of `entries` for which `inBand` is true, to
 * `orderedBandIds`, leaving the rest in their existing relative order and
 * unaffected. The reordered band is always placed first in the result — the
 * one canonical order every band-aware edit here agrees on. Where the band
 * lands in this flat array is otherwise meaningless: `computeHomeChanges`'s
 * Nuvio's order is built by filtering on the same predicate, never by position
 * here.
 */
export function reorderWithinBand<T extends { id: string }>(
  entries: T[],
  inBand: (entry: T) => boolean,
  orderedBandIds: string[],
): T[] {
  const band = entries.filter(inBand)
  const rest = entries.filter((entry) => !inBand(entry))
  return [...orderByKeys(band, orderedBandIds, (entry) => entry.id), ...rest]
}

/** Moves the entry with `id` one step within its own band (the partition
 *  `inBand` defines), or returns `entries` unchanged at a band's edge or if
 *  `id` isn't found there. */
export function moveWithinBand<T extends { id: string }>(
  entries: T[],
  inBand: (entry: T) => boolean,
  id: string,
  direction: -1 | 1,
): T[] {
  const bandIds = entries.filter(inBand).map((entry) => entry.id)
  const moved = moveByOne(bandIds, id, direction)
  return moved === bandIds ? entries : reorderWithinBand(entries, inBand, moved)
}

/** Reorders one collection band — shown first (`pinned`) or not — to
 *  `orderedBandIds`, leaving the other band's rows as they were. */
export function reorderCollectionBand(
  collections: HomeCollectionEntry[],
  band: 'pinned' | 'unpinned',
  orderedBandIds: string[],
): HomeCollectionEntry[] {
  const pinned = band === 'pinned'
  return reorderWithinBand(collections, (c) => c.pinToTop === pinned, orderedBandIds)
}

/** Moves collection `id` one step within its own band, shown first or not. */
export function moveCollectionInBand(
  collections: HomeCollectionEntry[],
  id: string,
  direction: -1 | 1,
): HomeCollectionEntry[] {
  const pinned = collections.some((c) => c.id === id && c.pinToTop)
  return moveWithinBand(collections, (c) => c.pinToTop === pinned, id, direction)
}

/** Flips collection `id`'s Show first, which moves it to the other band at
 *  its place in the selection order. */
export function togglePinToTop(collections: HomeCollectionEntry[], id: string): HomeCollectionEntry[] {
  return collections.map((c) => (c.id === id ? { ...c, pinToTop: !c.pinToTop } : c))
}

/**
 * The ids of the rows that still exist — the library's and the two selection
 * responses' — or `null` until all of those have loaded, when nothing can be
 * said to be gone.
 */
export function existingRowIDs(
  selectionLoaded: boolean,
  libraryLoaded: boolean,
  libraryIDs: ReadonlySet<string>,
  ...selections: (readonly { id: string }[] | undefined)[]
): ReadonlySet<string> | null {
  if (!selectionLoaded || !libraryLoaded) return null
  return new Set([...libraryIDs, ...selections.flatMap((rows) => (rows ?? []).map((row) => row.id))])
}

/**
 * `state` without the rows `existing` lacks — deleted, here or in another tab,
 * which Push would otherwise be refused for naming — or `state` itself when
 * none are, when it is `null`, or when `existing` is `null`, which means
 * nothing is loaded to judge by yet.
 */
export function withoutDeleted<T extends HomeState | null>(state: T, existing: ReadonlySet<string> | null): T {
  if (state === null || existing === null) return state
  const catalogs = state.catalogs.filter((c) => existing.has(c.id))
  const collections = state.collections.filter((c) => existing.has(c.id))
  if (catalogs.length === state.catalogs.length && collections.length === state.collections.length) return state
  return { catalogs, collections } as T
}
