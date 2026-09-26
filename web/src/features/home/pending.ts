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

export interface HomeState {
  /** Ordered. Array position *is* `sort_order` at write time. */
  catalogs: HomeCatalogEntry[]
  /** Ordered, same rule. */
  collections: string[]
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
    collections: { collection_ids: state.collections },
  }
}

/**
 * Reorders the subset of `entries` for which `inBand` is true, to
 * `orderedBandIds`, leaving the rest in their existing relative order and
 * unaffected. The reordered band is always placed first in the result — the
 * one canonical order every band-aware edit here agrees on. Where the band
 * lands in this flat array is otherwise meaningless: `computeHomeChanges`'s
 * TV order is built by filtering on the same predicate, never by position
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
