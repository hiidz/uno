import { describe, expect, it } from 'vitest'
import { catalog, collection } from '@/test/fixtures'
import {
  bandOf,
  catalogEntries,
  collectionEntries,
  hydrateHome,
  moveInBand,
  reorderBand,
  toPushPayload,
  toggleShowInHome,
  togglePinToTop,
  withRow,
  withoutDeleted,
  withoutRow,
  type HomeCatalogEntry,
  type HomeCollectionEntry,
} from './pending'

const shown = (id: string): HomeCatalogEntry => ({ kind: 'catalog', id, showInHome: true })
const discover = (id: string): HomeCatalogEntry => ({ kind: 'catalog', id, showInHome: false })
const first = (id: string): HomeCollectionEntry => ({ kind: 'collection', id, pinToTop: true })
const after = (id: string): HomeCollectionEntry => ({ kind: 'collection', id, pinToTop: false })
const ids = (entries: { id: string }[]) => entries.map((entry) => entry.id)

describe('toPushPayload', () => {
  it('sends every row in Home order, a catalog with its home flag and a collection with its Show first, and the revision the Home was built from', () => {
    expect(toPushPayload({ rows: [shown('b'), after('y'), discover('a'), first('x'), shown('c')] }, 7)).toEqual({
      rows: [
        { catalog_id: 'b', show_in_home: true },
        { collection_id: 'y', pin_to_top: false },
        { catalog_id: 'a', show_in_home: false },
        { collection_id: 'x', pin_to_top: true },
        { catalog_id: 'c', show_in_home: true },
      ],
      home_revision: 7,
    })
  })

  it('sends an empty list for an empty Home, never omitting it', () => {
    expect(toPushPayload({ rows: [] }, 1)).toEqual({ rows: [], home_revision: 1 })
  })
})

describe('hydrateHome', () => {
  it('merges the owned rows on Home by home_position', () => {
    const home = hydrateHome(
      [
        catalog({ id: 'a', home_position: 2, show_in_home: true }),
        catalog({ id: 'd', home_position: 4, show_in_home: false }),
      ],
      [collection({ id: 'x', home_position: 0, pin_to_top: true }), collection({ id: 'y', home_position: 3 })],
    )
    expect(home.rows).toEqual([first('x'), shown('a'), after('y'), discover('d')])
  })

  it('leaves out every row off Home, whatever its flags', () => {
    const home = hydrateHome(
      [catalog({ id: 'off', show_in_home: true }), catalog({ id: 'a', home_position: 0, show_in_home: true })],
      [collection({ id: 'k', pin_to_top: true })],
    )
    expect(home.rows).toEqual([shown('a')])
  })
})

describe('bands and kinds', () => {
  it('puts a pinned collection first, a home row or unpinned collection in home, and a Discover catalog apart', () => {
    expect([first('p'), after('u'), shown('s'), discover('d')].map(bandOf)).toEqual(['pinned', 'home', 'home', 'discover'])
  })

  it('reads the catalogs and the collections out of one Home, each in order', () => {
    const state = { rows: [after('y'), shown('a'), first('x'), discover('b')] }
    expect(ids(catalogEntries(state))).toEqual(['a', 'b'])
    expect(ids(collectionEntries(state))).toEqual(['y', 'x'])
  })
})

describe('the bands', () => {
  const rows = [first('p1'), shown('a'), after('u1'), first('p2'), discover('d'), after('u2')]

  it('reorders the home rows, catalogs and collections mixed, leaving the pinned ones as they were', () => {
    const reordered = reorderBand(rows, 'home', ['u2', 'a', 'u1'])
    expect(ids(reordered.filter((r) => bandOf(r) === 'home'))).toEqual(['u2', 'a', 'u1'])
    expect(ids(reordered.filter((r) => bandOf(r) === 'pinned'))).toEqual(['p1', 'p2'])
  })

  it('reorders the pinned collections, leaving the home rows as they were', () => {
    const reordered = reorderBand(rows, 'pinned', ['p2', 'p1'])
    expect(ids(reordered.filter((r) => bandOf(r) === 'pinned'))).toEqual(['p2', 'p1'])
    expect(ids(reordered.filter((r) => bandOf(r) === 'home'))).toEqual(['a', 'u1', 'u2'])
  })

  it('moves a row within its own band only, across kinds', () => {
    const moved = moveInBand(rows, 'u1', -1)
    expect(ids(moved.filter((r) => bandOf(r) === 'home'))).toEqual(['u1', 'a', 'u2'])
    const overOthers = moveInBand(rows, 'u2', -1)
    expect(ids(overOthers.filter((r) => bandOf(r) === 'home'))).toEqual(['a', 'u2', 'u1'])
    expect(ids(overOthers.filter((r) => bandOf(r) !== 'home'))).toEqual(['p1', 'p2', 'd'])
    expect(moveInBand(rows, 'p1', -1)).toBe(rows)
    expect(moveInBand(rows, 'zz', 1)).toBe(rows)
  })

  it('flips one collection’s Show first and keeps its place in Home order', () => {
    expect(togglePinToTop(rows, 'u1')).toEqual([first('p1'), shown('a'), first('u1'), first('p2'), discover('d'), after('u2')])
    expect(togglePinToTop(rows, 'a')).toEqual(rows)
  })

  it('flips one catalog between a home row and Discover only', () => {
    expect(toggleShowInHome(rows, 'a')[1]).toEqual(discover('a'))
    expect(toggleShowInHome(rows, 'd')[4]).toEqual(shown('d'))
    expect(toggleShowInHome(rows, 'u1')[2]).toEqual(after('u1'))
  })
})

describe('withRow and withoutRow', () => {
  it('adds a row at the end of Home once', () => {
    const rows = [shown('a')]
    expect(withRow(rows, after('x'))).toEqual([shown('a'), after('x')])
    expect(withRow(rows, shown('a'))).toBe(rows)
  })

  it('takes a row off by its id', () => {
    expect(withoutRow([shown('a'), after('x')], 'a')).toEqual([after('x')])
  })
})

describe('withoutDeleted', () => {
  const state = { rows: [shown('a'), after('x'), discover('b'), first('y')] }

  it('is the state itself while every row exists, before anything can be judged, or for none', () => {
    expect(withoutDeleted(state, new Set(['a', 'b', 'x', 'y']))).toBe(state)
    expect(withoutDeleted(state, null)).toBe(state)
    expect(withoutDeleted(null, new Set())).toBeNull()
  })

  it('drops the rows that are gone, keeping the order of the rest', () => {
    expect(withoutDeleted(state, new Set(['b', 'y']))).toEqual({ rows: [discover('b'), first('y')] })
  })
})
