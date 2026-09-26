import { describe, expect, it } from 'vitest'
import { moveWithinBand, reorderWithinBand, toPushPayload, type HomeCatalogEntry } from './pending'

const shown = (id: string): HomeCatalogEntry => ({ id, showInHome: true })
const discover = (id: string): HomeCatalogEntry => ({ id, showInHome: false })
const isShown = (entry: HomeCatalogEntry) => entry.showInHome
const ids = (entries: { id: string }[]) => entries.map((entry) => entry.id)

describe('toPushPayload', () => {
  it('sends every row in selection order, with its home flag', () => {
    expect(
      toPushPayload({ catalogs: [shown('b'), discover('a'), shown('c')], collections: ['y', 'x'] }),
    ).toEqual({
      catalogs: {
        catalogs: [
          { catalog_id: 'b', show_in_home: true },
          { catalog_id: 'a', show_in_home: false },
          { catalog_id: 'c', show_in_home: true },
        ],
      },
      collections: { collection_ids: ['y', 'x'] },
    })
  })

  it('sends empty lists for an empty selection, never omitting them', () => {
    expect(toPushPayload({ catalogs: [], collections: [] })).toEqual({
      catalogs: { catalogs: [] },
      collections: { collection_ids: [] },
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
