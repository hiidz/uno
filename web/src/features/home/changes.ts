/**
 * The list of changes: named, per-row sentences for what a push would do,
 * shown under the top bar while the list is open and read aloud by the pending
 * count beside Push — the two must never disagree, so `pendingCount` in
 * `HomeSelectionContext` is this list's length, not a separate tally.
 *
 * Positions in the sentences ("3rd", "5th") are the row's place in Nuvio's
 * own order — the same three-band order `preview.ts` draws — so a line here
 * reads exactly like the running-order row it describes.
 */

import type { Catalog, Collection } from '@/api'
import { ordinal } from '@/lib/ordinal'
import type { HomeState } from './pending'

interface RowKey {
  kind: 'catalog' | 'collection'
  id: string
}

function keyOf(row: RowKey): string {
  return `${row.kind}:${row.id}`
}

/** Nuvio's own order: pinned collections, then home-shown catalogs, then the
 *  remaining collections. Discover-only catalogs never appear here — they are
 *  precisely the rows with no place in this order. Each state carries its own
 *  pins, so the baseline's order is the one Nuvio has now. */
function orderedRows(state: HomeState): RowKey[] {
  const pinned = state.collections.filter((c) => c.pinToTop)
  const unpinned = state.collections.filter((c) => !c.pinToTop)
  const shown = state.catalogs.filter((c) => c.showInHome)
  return [
    ...pinned.map((c): RowKey => ({ kind: 'collection', id: c.id })),
    ...shown.map((c): RowKey => ({ kind: 'catalog', id: c.id })),
    ...unpinned.map((c): RowKey => ({ kind: 'collection', id: c.id })),
  ]
}

/** Whether a collection is shown first in `state`, by id. */
function pinsOf(state: HomeState): (id: string) => boolean {
  const pinned = new Set(state.collections.filter((c) => c.pinToTop).map((c) => c.id))
  return (id) => pinned.has(id)
}

function groupOf(row: RowKey, isPinned: (id: string) => boolean): 'first' | 'catalogs' | 'after' {
  if (row.kind === 'catalog') return 'catalogs'
  return isPinned(row.id) ? 'first' : 'after'
}

/**
 * The rows that kept their order: the longest run of old positions that's
 * already increasing. Ties keep the later row, so the one that actually moved
 * is the one this reports as moved.
 */
function lisKeep(oldPositions: number[]): Set<number> {
  const length = oldPositions.map(() => 1)
  const prev = oldPositions.map(() => -1)
  for (let i = 0; i < oldPositions.length; i++) {
    for (let j = 0; j < i; j++) {
      if (oldPositions[j] < oldPositions[i] && length[j] + 1 > length[i]) {
        length[i] = length[j] + 1
        prev[i] = j
      }
    }
  }
  let best = -1
  for (let i = 0; i < oldPositions.length; i++) {
    if (best < 0 || length[i] >= length[best]) best = i
  }
  const keep = new Set<number>()
  for (let i = best; i >= 0; i = prev[i]) keep.add(i)
  return keep
}

export interface HomeChange {
  key: string
  text: string
}

/** How a change names a row: its catalog's name or its collection's title, in
 *  quotes, or "Unavailable …" when nothing describes it. */
function quoter(
  catalogById: ReadonlyMap<string, Catalog>,
  collectionById: ReadonlyMap<string, Collection>,
): (row: RowKey) => string {
  return function inQuotes(row) {
    const name = row.kind === 'catalog' ? catalogName(catalogById, row.id) : collectionTitle(collectionById, row.id)
    return `“${name}”`
  }
}

function catalogName(catalogById: ReadonlyMap<string, Catalog>, id: string): string {
  return catalogById.get(id)?.name ?? 'Unavailable catalog'
}

function collectionTitle(collectionById: ReadonlyMap<string, Collection>, id: string): string {
  return collectionById.get(id)?.title ?? 'Unavailable collection'
}

/** A collection row's Show first action, by its pending pin: the edit it
 *  makes, which the list of changes then reports. */
export function showFirstAction(pinned: boolean): string {
  return pinned ? 'Don’t show first' : 'Show first'
}

/**
 * A line for each collection on the home screen both before and after whose
 * Show first changed — the one change that moves a row from one group to the
 * other — and those rows' keys. Moves are found within each group, so a row
 * that changed group is left out of them rather than read as moved.
 */
function pinFlips(
  baseline: HomeState,
  current: HomeState,
  quoted: (row: RowKey) => string,
): { list: HomeChange[]; keys: Set<string> } {
  const wasPinned = new Map(baseline.collections.map((c) => [c.id, c.pinToTop]))
  const flipped = current.collections
    .filter((c) => wasPinned.has(c.id) && wasPinned.get(c.id) !== c.pinToTop)
    .map((c) => ({ row: { kind: 'collection', id: c.id } as RowKey, pinToTop: c.pinToTop }))
  return {
    list: flipped.map(({ row, pinToTop }) => ({
      key: `pin:${keyOf(row)}`,
      text: pinToTop ? `Showing ${quoted(row)} first` : `No longer showing ${quoted(row)} first`,
    })),
    keys: new Set(flipped.map(({ row }) => keyOf(row))),
  }
}

/**
 * The fewest moves that explain the new order within each group, among the
 * rows in both orders — except `skip`, the rows that changed group. Positions
 * are each row's place in the whole order, before and after.
 */
function groupMoves(
  cur: RowKey[],
  was: RowKey[],
  isPinned: (id: string) => boolean,
  skip: Set<string>,
  quoted: (row: RowKey) => string,
): HomeChange[] {
  const curKeys = cur.map(keyOf)
  const wasKeys = was.map(keyOf)
  const kept = cur.filter((row) => wasKeys.includes(keyOf(row)) && !skip.has(keyOf(row)))
  const list: HomeChange[] = []
  for (const group of ['first', 'catalogs', 'after'] as const) {
    const common = kept.filter((row) => groupOf(row, isPinned) === group)
    const oldPositions = common.map((row) => wasKeys.indexOf(keyOf(row)))
    const keep = lisKeep(oldPositions)
    common.forEach((row, i) => {
      if (keep.has(i)) return
      const key = keyOf(row)
      const from = wasKeys.indexOf(key) + 1
      const to = curKeys.indexOf(key) + 1
      list.push({ key: `move:${key}`, text: `Moved ${quoted(row)} from ${ordinal(from)} to ${ordinal(to)}` })
    })
  }
  return list
}

/**
 * Every change a push would make, additions and removals, moves between home
 * and Discover, Show first turned on or off, and the fewest moves that explain
 * the new order within each group. Only Show first moves a row from one group
 * to another, and it is reported as that, so moves are found group by group.
 */
export function computeHomeChanges({
  baseline,
  current,
  catalogById,
  collectionById,
}: {
  baseline: HomeState
  current: HomeState
  catalogById: ReadonlyMap<string, Catalog>
  collectionById: ReadonlyMap<string, Collection>
}): HomeChange[] {
  const quoted = quoter(catalogById, collectionById)

  const cur = orderedRows(current)
  const was = orderedRows(baseline)
  const curKeys = cur.map(keyOf)
  const wasKeys = was.map(keyOf)

  const baseDiscover = new Set(baseline.catalogs.filter((c) => !c.showInHome).map((c) => c.id))
  const curDiscover = new Set(current.catalogs.filter((c) => !c.showInHome).map((c) => c.id))

  const list: HomeChange[] = []

  cur.forEach((row, i) => {
    const key = keyOf(row)
    if (wasKeys.includes(key)) return
    if (row.kind === 'catalog' && baseDiscover.has(row.id)) {
      list.push({
        key: `add:${key}`,
        text: `Moved ${quoted(row)} out of Discover, ${ordinal(i + 1)} on your home screen`,
      })
    } else {
      list.push({ key: `add:${key}`, text: `Added ${quoted(row)}, ${ordinal(i + 1)} on your home screen` })
    }
  })

  was.forEach((row) => {
    const key = keyOf(row)
    if (curKeys.includes(key)) return
    if (row.kind === 'catalog' && curDiscover.has(row.id)) return // reported below, as a move to Discover
    list.push({ key: `remove:${key}`, text: `Removed ${quoted(row)} from home screen` })
  })

  const flips = pinFlips(baseline, current, quoted)
  list.push(...flips.list, ...groupMoves(cur, was, pinsOf(current), flips.keys, quoted))

  current.catalogs
    .filter((c) => !c.showInHome && !baseDiscover.has(c.id))
    .forEach((c) => {
      const row: RowKey = { kind: 'catalog', id: c.id }
      const wasShown = baseline.catalogs.some((b) => b.id === c.id && b.showInHome)
      list.push({
        key: `discover:${c.id}`,
        text: wasShown ? `Moved ${quoted(row)} to Discover` : `Added ${quoted(row)} to Discover`,
      })
    })

  baseDiscover.forEach((id) => {
    const isShownNow = current.catalogs.some((c) => c.id === id && c.showInHome)
    if (!curDiscover.has(id) && !isShownNow) {
      list.push({ key: `discover-remove:${id}`, text: `Removed ${quoted({ kind: 'catalog', id })} from home screen` })
    }
  })

  // A collection in Nuvio whose content moved on since the push that put it
  // there — what Push would send for it differs from what it last sent
  // (`needs_push`), so Nuvio's copy is stale until the next push, even though
  // nothing about *this* selection changed. Restricted to collections present
  // in *both* baseline and current: a collection taken off the home screen,
  // pushed, edited, then put back would otherwise show both "Added …" and
  // "changed since …" today, when only "Added …" should fire.
  const baseSet = new Set(baseline.collections.map((c) => c.id))
  current.collections
    .filter((c) => baseSet.has(c.id))
    .forEach(({ id }) => {
      if (collectionById.get(id)?.needs_push) {
        list.push({
          key: `stale:${id}`,
          text: `${quoted({ kind: 'collection', id })} changed since it was last pushed`,
        })
      }
    })

  return list
}
