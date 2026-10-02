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

import type { Catalog, Collection, PendingChange } from '@/api'
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
  /** Already saved in the vault and waiting only on a push, so closing the
   *  page doesn't lose it. Every other change exists in this tab alone. */
  saved?: boolean
}

/** How many of `changes` exist in this tab alone — what leaving the page loses. */
export function countUnsaved(changes: HomeChange[]): number {
  let count = 0
  for (const change of changes) if (!change.saved) count++
  return count
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

/** A collection row's Pin or Unpin action, by its pending pin: the edit it
 *  makes, which the list of changes then reports. */
export function showFirstAction(pinned: boolean): string {
  return pinned ? 'Unpin' : 'Pin'
}

/**
 * A line for each collection on the home screen both before and after whose
 * pin changed — the one change that moves a row from one group to the
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
      text: pinToTop ? `Pinned ${quoted(row)}` : `Unpinned ${quoted(row)}`,
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

function holds(state: HomeState, row: RowKey): boolean {
  return (row.kind === 'catalog' ? state.catalogs : state.collections).some((entry) => entry.id === row.id)
}

/**
 * The lines for what the server says a push would change in Nuvio, already
 * saved, which this tab's own edits don't say: a row edited since its last push
 * (a collection also when a catalog its folders use was), one Nuvio holds
 * nothing for, and one deleted, which the next push drops. The first two are
 * left out for a row this tab takes off the home screen, where the removal is
 * the change. A deleted row's line is the removal's own, so it isn't said twice
 * when `taken`, the keys already listed, has it.
 */
function waitingChanges(
  waiting: readonly PendingChange[],
  baseline: HomeState,
  current: HomeState,
  taken: ReadonlySet<string>,
): HomeChange[] {
  return waiting
    .map((item) => waitingLine(item, baseline, current, taken))
    .filter((line): line is HomeChange => line !== null)
}

/** `waitingChanges`' line for one row, or `null` when this tab says it already. */
function waitingLine(
  { kind, id, name, change }: PendingChange,
  baseline: HomeState,
  current: HomeState,
  taken: ReadonlySet<string>,
): HomeChange | null {
  const row: RowKey = { kind, id }
  if (change === 'removed') return removalLine(row, name, taken)
  if (!holds(baseline, row) || !holds(current, row)) return null
  return { key: `waiting:${keyOf(row)}`, text: waitingText(change, name), saved: true }
}

/** The line for a deleted row, unless `taken` says this tab has it already. */
function removalLine(row: RowKey, name: string, taken: ReadonlySet<string>): HomeChange | null {
  const key = `remove:${keyOf(row)}`
  return taken.has(key) ? null : { key, text: `Removed “${name}” from home screen`, saved: true }
}

function waitingText(change: 'added' | 'changed', name: string): string {
  return change === 'added' ? `“${name}” isn’t in Nuvio yet` : `“${name}” changed since it was last pushed`
}

/**
 * Every change a push would make, additions and removals, moves between home
 * and Discover, a pin or unpin, and the fewest moves that explain
 * the new order within each group. Only a pin moves a row from one group
 * to another, and it is reported as that, so moves are found group by group.
 * `waiting` is what the server says is waiting for a push beyond those edits.
 */
export function computeHomeChanges({
  baseline,
  current,
  catalogById,
  collectionById,
  waiting = [],
}: {
  baseline: HomeState
  current: HomeState
  catalogById: ReadonlyMap<string, Catalog>
  collectionById: ReadonlyMap<string, Collection>
  waiting?: readonly PendingChange[]
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

  list.push(...waitingChanges(waiting, baseline, current, new Set(list.map((change) => change.key))))

  return list
}
