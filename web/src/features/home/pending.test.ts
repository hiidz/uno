import { describe, expect, it } from 'vitest'
import {
  moveCollectionInBand,
  moveWithinBand,
  reorderCollectionBand,
  reorderWithinBand,
  toPushPayload,
  togglePinToTop,
  type HomeCatalogEntry,
  type HomeCollectionEntry,
} from './pending'

const shown = (id: string): HomeCatalogEntry => ({ id, showInHome: true })
const discover = (id: string): HomeCatalogEntry => ({ id, showInHome: false })
const first = (id: string): HomeCollectionEntry => ({ id, pinToTop: true })
const after = (id: string): HomeCollectionEntry => ({ id, pinToTop: false })
const isShown = (entry: HomeCatalogEntry) => entry.showInHome
const ids = (entries: { id: string }[]) => entries.map((entry) => entry.id)

describe('toPushPayload', () => {
  it('sends every row in selection order, a catalog with its home flag and a collection with its Show first', () => {
    expect(
      toPushPayload({ catalogs: [shown('b'), discover('a'), shown('c')], collections: [after('y'), first('x')] }),
    ).toEqual({
      catalogs: {
        catalogs: [
          { catalog_id: 'b', show_in_home: true },
          { catalog_id: 'a', show_in_home: false },
          { catalog_id: 'c', show_in_home: true },
        ],
      },
      collections: {
        collections: [
          { collection_id: 'y', pin_to_top: false },
          { collection_id: 'x', pin_to_top: true },
        ],
      },
    })
  })

  it('sends empty lists for an empty selection, never omitting them', () => {
    expect(toPushPayload({ catalogs: [], collections: [] })).toEqual({
      catalogs: { catalogs: [] },
      collections: { collections: [] },
    })
  })
})

describe('reorderWithinBand', () => {
  it('reorders the band and leaves the rest in their order after it', () => {
    const entries = [shown('a'), discover('d1'), shown('b'), discover('d2')]
    expect(ids(reorderWithinBand(entries, isShown, ['b', 'a']))).toEqual(['b', 'a', 'd1', 'd2'])
  })
})

describe('moveWithinBand', () => {
  const entries = [shown('a'), discover('d1'), shown('b'), shown('c')]

  it('swaps with the neighbour in the same band, stepping over the other band', () => {
    expect(ids(moveWithinBand(entries, isShown, 'b', -1))).toEqual(['b', 'a', 'c', 'd1'])
    expect(ids(moveWithinBand(entries, isShown, 'a', 1))).toEqual(['b', 'a', 'c', 'd1'])
  })

  it('returns the same list at either edge of the band', () => {
    expect(moveWithinBand(entries, isShown, 'a', -1)).toBe(entries)
    expect(moveWithinBand(entries, isShown, 'c', 1)).toBe(entries)
  })

  it('returns the same list for an id outside the band', () => {
    expect(moveWithinBand(entries, isShown, 'd1', 1)).toBe(entries)
    expect(moveWithinBand(entries, isShown, 'zz', 1)).toBe(entries)
  })
})

describe('the collection bands', () => {
  const entries = [first('p1'), after('u1'), first('p2'), after('u2')]

  it('reorders the pinned band or the other, leaving the other band as it was', () => {
    expect(ids(reorderCollectionBand(entries, 'pinned', ['p2', 'p1']))).toEqual(['p2', 'p1', 'u1', 'u2'])
    expect(ids(reorderCollectionBand(entries, 'unpinned', ['u2', 'u1']))).toEqual(['u2', 'u1', 'p1', 'p2'])
  })

  it('moves a collection within its own band only', () => {
    expect(ids(moveCollectionInBand(entries, 'p2', -1))).toEqual(['p2', 'p1', 'u1', 'u2'])
    expect(ids(moveCollectionInBand(entries, 'u1', 1))).toEqual(['u2', 'u1', 'p1', 'p2'])
    expect(moveCollectionInBand(entries, 'p1', -1)).toBe(entries)
    expect(moveCollectionInBand(entries, 'zz', 1)).toBe(entries)
  })

  it('flips one collection’s Show first and keeps its place in the selection', () => {
    expect(togglePinToTop(entries, 'u1')).toEqual([first('p1'), first('u1'), first('p2'), after('u2')])
    expect(togglePinToTop(entries, 'p2')).toEqual([first('p1'), after('u1'), after('p2'), after('u2')])
  })
})
