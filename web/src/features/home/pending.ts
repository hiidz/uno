/**
 * The Home pane's state, and the diff that drives the unpushed-changes count.
 *
 * Selection is edited entirely in browser memory. Nothing here writes to the
 * server — the whole selection travels in Push's request body, in one call.
 * So `baseline` is what the server last told us is live, and `current` is what
 * the user has pending; the gap between them is the only thing telling the
 * user their work isn't saved yet.
 */

import type { Catalog, Collection, PushRequest } from '@/api'
import { moveByOne, orderByKeys } from '@/lib/order'

/** One row on Home: a catalog, or a collection. A catalog and a collection
 *  never share an id. */
export type HomeEntry =
  | {
      kind: 'catalog'
      id: string
      /** Mirrors `catalogs.show_in_home`: a home row, or Discover only. */
      showInHome: boolean
    }
  | {
      kind: 'collection'
      id: string
      /** Mirrors `collections.pin_to_top`: Show first, which lifts the
       *  collection above every other row. Only push writes it, from here. */
      pinToTop: boolean
    }

export type HomeCatalogEntry = Extract<HomeEntry, { kind: 'catalog' }>
export type HomeCollectionEntry = Extract<HomeEntry, { kind: 'collection' }>

/** An edit to Home's rows: the rows as they are to the rows as they become. */
export type HomeRowsEdit = (rows: HomeEntry[]) => HomeEntry[]

export interface HomeState {
  /** Ordered: a row's place here is its place on Home, which Push sends. */
  rows: HomeEntry[]
}

export const EMPTY_HOME: HomeState = { rows: [] }

/**
 * Where a row sits on Nuvio's home screen: `pinned`, the collections shown
 * first; `home`, the catalogs with a home row and the other collections,
 * mixed in one order; or `discover`, a catalog with no home row.
 */
export type HomeBand = 'pinned' | 'home' | 'discover'

export function bandOf(entry: HomeEntry): HomeBand {
  if (entry.kind === 'collection' && entry.pinToTop) return 'pinned'
  if (entry.kind === 'collection' || entry.showInHome) return 'home'
  return 'discover'
}

/** The catalogs in `state`, in Home order. */
export function catalogEntries(state: HomeState): HomeCatalogEntry[] {
  return state.rows.filter((entry): entry is HomeCatalogEntry => entry.kind === 'catalog')
}

/** The collections in `state`, in Home order. */
export function collectionEntries(state: HomeState): HomeCollectionEntry[] {
  return state.rows.filter((entry): entry is HomeCollectionEntry => entry.kind === 'collection')
}

/** The owned rows on Home as one Home, by each row's `home_position`; a row
 *  without one is off Home. */
export function hydrateHome(catalogs: readonly Catalog[], collections: readonly Collection[]): HomeState {
  const placed = [...catalogs.flatMap(placedCatalog), ...collections.flatMap(placedCollection)]
  placed.sort((a, b) => a.position - b.position)
  return { rows: placed.map((p) => p.entry) }
}

/** A Home row with its place, which `hydrateHome` sorts by. */
interface PlacedEntry {
  position: number
  entry: HomeEntry
}

function placedCatalog(c: Catalog): PlacedEntry[] {
  if (c.home_position === undefined) return []
  return [{ position: c.home_position, entry: { kind: 'catalog', id: c.id, showInHome: c.show_in_home } }]
}

function placedCollection(c: Collection): PlacedEntry[] {
  if (c.home_position === undefined) return []
  return [{ position: c.home_position, entry: { kind: 'collection', id: c.id, pinToTop: c.pin_to_top } }]
}

/**
 * The wire shape Push sends. A row's place in `rows` *is* its position on
 * Home, which is why this is a straight positional map and never sorts.
 */
export function toPushPayload(state: HomeState): PushRequest {
  return { rows: state.rows.map(toPushRow) }
}

function toPushRow(entry: HomeEntry): PushRequest['rows'][number] {
  if (entry.kind === 'catalog') return { catalog_id: entry.id, show_in_home: entry.showInHome }
  return { collection_id: entry.id, pin_to_top: entry.pinToTop }
}

/**
 * Reorders the subset of `entries` for which `inBand` is true, to
 * `orderedBandIds`, leaving the rest in their existing relative order and
 * unaffected. The reordered band is always placed first in the result — the
 * one canonical order every band-aware edit here agrees on. Where the band
 * lands in this flat array is otherwise meaningless: Nuvio's order is built by
 * filtering on the same predicate, never by position here.
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

/** `rows` with `entry` added at the end of Home, or `rows` itself when the row
 *  is on Home already. */
export function withRow(rows: HomeEntry[], entry: HomeEntry): HomeEntry[] {
  if (rows.some((row) => row.id === entry.id)) return rows
  return [...rows, entry]
}

/** `rows` without row `id`. */
export function withoutRow(rows: HomeEntry[], id: string): HomeEntry[] {
  return rows.filter((row) => row.id !== id)
}

/** Reorders one band — pinned or home — to `orderedBandIds`, leaving every
 *  other row as it was. */
export function reorderBand(rows: HomeEntry[], band: HomeBand, orderedBandIds: string[]): HomeEntry[] {
  return reorderWithinBand(rows, (entry) => bandOf(entry) === band, orderedBandIds)
}

/** Moves row `id` one step within its own band. */
export function moveInBand(rows: HomeEntry[], id: string, direction: -1 | 1): HomeEntry[] {
  const row = rows.find((entry) => entry.id === id)
  if (!row) return rows
  const band = bandOf(row)
  return moveWithinBand(rows, (entry) => bandOf(entry) === band, id, direction)
}

/** Flips collection `id`'s Show first, which moves it to the other band at
 *  its place in Home order. */
export function togglePinToTop(rows: HomeEntry[], id: string): HomeEntry[] {
  return rows.map((entry) => flipped(entry, id, 'collection'))
}

/** Flips catalog `id` between a home row and Discover only. */
export function toggleShowInHome(rows: HomeEntry[], id: string): HomeEntry[] {
  return rows.map((entry) => flipped(entry, id, 'catalog'))
}

/** `entry` with its flag flipped when it is the `kind` row `id`: a
 *  collection's Show first, a catalog's home row. */
function flipped(entry: HomeEntry, id: string, kind: HomeEntry['kind']): HomeEntry {
  if (entry.id !== id || entry.kind !== kind) return entry
  if (entry.kind === 'collection') return { ...entry, pinToTop: !entry.pinToTop }
  return { ...entry, showInHome: !entry.showInHome }
}

/**
 * `state` without the rows `existing` lacks — deleted, here or in another tab,
 * which Push would otherwise be refused for naming — or `state` itself when
 * none are, when it is `null`, or when `existing` is `null`, which means
 * nothing is loaded to judge by yet.
 */
export function withoutDeleted<T extends HomeState | null>(state: T, existing: ReadonlySet<string> | null): T {
  if (state === null || existing === null) return state
  const rows = state.rows.filter((entry) => existing.has(entry.id))
  if (rows.length === state.rows.length) return state
  return { rows } as T
}
