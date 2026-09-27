/**
 * The list of changes: named, per-row sentences for what a push would do,
 * shown under the top bar while the list is open and read aloud by the pending
 * count beside Push — the two must never disagree, so `pendingCount` in
 * `HomeSelectionContext` is this list's length, not a separate tally.
 *
 * Positions in the sentences ("3rd", "5th") are the row's place in the TV's
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

/** The TV's own order: pinned collections, then home-shown catalogs, then the
 *  remaining collections. Discover-only catalogs never appear here — they are
 *  precisely the rows with no place in this order. */
function tvRows(state: HomeState, isPinned: (id: string) => boolean): RowKey[] {
  const pinned = state.collections.filter(isPinned)
  const unpinned = state.collections.filter((id) => !isPinned(id))
  const shown = state.catalogs.filter((c) => c.showInHome)
  return [
    ...pinned.map((id): RowKey => ({ kind: 'collection', id })),
    ...shown.map((c): RowKey => ({ kind: 'catalog', id: c.id })),
    ...unpinned.map((id): RowKey => ({ kind: 'collection', id })),
  ]
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

/**
 * Every change a push would make, additions and removals, moves between home
 * and Discover, and the fewest moves that explain the new order within each
 * group — rows never cross from one group to another, so moves are found
 * group by group.
 */
export function computeHomeChanges({
  baseline,
  current,
  catalogById,
  collectionById,
  isPinned,
}: {
  baseline: HomeState
  current: HomeState
  catalogById: ReadonlyMap<string, Catalog>
  collectionById: ReadonlyMap<string, Collection>
  isPinned: (id: string) => boolean
}): HomeChange[] {
  const nameOf = (row: RowKey): string =>
    row.kind === 'catalog'
      ? (catalogById.get(row.id)?.name ?? 'Unavailable catalog')
      : (collectionById.get(row.id)?.title ?? 'Unavailable collection')
  const quoted = (row: RowKey) => `“${nameOf(row)}”`

  const cur = tvRows(current, isPinned)
  const was = tvRows(baseline, isPinned)
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

  for (const group of ['first', 'catalogs', 'after'] as const) {
    const common = cur.filter((row) => wasKeys.includes(keyOf(row)) && groupOf(row, isPinned) === group)
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

  // A collection on the TV whose content moved on since the push that put it
  // there — the folder sources Nuvio has are stale until the next push, even
  // though nothing about *this* selection changed. Restricted to collections
  // present in *both* baseline and current: a collection taken off the TV,
  // pushed, edited, then put back would otherwise show both "Added …" and
  // "changed since …" today, when only "Added …" should fire.
  const baseSet = new Set(baseline.collections)
  current.collections
    .filter((id) => baseSet.has(id))
    .forEach((id) => {
      const collection = collectionById.get(id)
      if (collection?.pushed_version == null) return
      if (collection.version !== collection.pushed_version) {
        list.push({
          key: `stale:${id}`,
          text: `${quoted({ kind: 'collection', id })} changed since it was last pushed`,
        })
      }
    })

  return list
}
