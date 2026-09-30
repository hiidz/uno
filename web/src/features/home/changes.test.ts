import { describe, expect, it } from 'vitest'
import type { Catalog, Collection } from '@/api'
import { catalog, collection } from '@/test/fixtures'
import { computeHomeChanges, showFirstAction } from './changes'
import type { HomeCatalogEntry, HomeCollectionEntry, HomeState } from './pending'

const catalogById = new Map<string, Catalog>(
  [
    ['a', 'Alpha'],
    ['b', 'Bravo'],
    ['c', 'Charlie'],
    ['d', 'Delta'],
  ].map(([id, name]) => [id, catalog({ id, name })]),
)

const collectionById = new Map<string, Collection>([
  ['p', collection({ id: 'p', title: 'Pinned', pin_to_top: true })],
  ['x', collection({ id: 'x', title: 'X-ray' })],
  ['y', collection({ id: 'y', title: 'Yankee' })],
  ['stale', collection({ id: 'stale', title: 'Stale', needs_push: true })],
])

const shown = (...ids: string[]): HomeCatalogEntry[] => ids.map((id) => ({ id, showInHome: true }))
const discover = (...ids: string[]): HomeCatalogEntry[] => ids.map((id) => ({ id, showInHome: false }))

/** A selection; each collection by id takes the pin it was last pushed with,
 *  unless it comes as an entry with its own. */
function home(catalogs: HomeCatalogEntry[], collections: (string | HomeCollectionEntry)[] = []): HomeState {
  return {
    catalogs,
    collections: collections.map((c) =>
      typeof c === 'string' ? { id: c, pinToTop: collectionById.get(c)?.pin_to_top ?? false } : c,
    ),
  }
}

const first = (id: string): HomeCollectionEntry => ({ id, pinToTop: true })
const notFirst = (id: string): HomeCollectionEntry => ({ id, pinToTop: false })

function changes(baseline: HomeState, current: HomeState): string[] {
  return computeHomeChanges({
    baseline,
    current,
    catalogById,
    collectionById,
  }).map((change) => change.text)
}

describe('computeHomeChanges', () => {
  it('reports nothing for an unchanged selection', () => {
    const state = home([...shown('a', 'b'), ...discover('c')], ['p', 'x'])
    expect(changes(state, { ...state })).toEqual([])
  })

  it('reports an addition at its place in the running order, counting pinned collections first', () => {
    expect(changes(home([], ['p']), home(shown('a'), ['p']))).toEqual([
      'Added “Alpha”, 2nd on your home screen',
    ])
  })

  it('reports a removal', () => {
    expect(changes(home(shown('a', 'b')), home(shown('b')))).toEqual(['Removed “Alpha” from home screen'])
  })

  it('reports one drag as one move, not a move for every row it shifted', () => {
    const before = home(shown('a', 'b', 'c', 'd'))
    expect(changes(before, home(shown('b', 'c', 'd', 'a')))).toEqual(['Moved “Alpha” from 1st to 4th'])
    expect(changes(before, home(shown('d', 'a', 'b', 'c')))).toEqual(['Moved “Delta” from 4th to 1st'])
  })

  it('numbers collection moves by their place after the catalog rows', () => {
    expect(changes(home(shown('a', 'b'), ['x', 'y']), home(shown('a', 'b'), ['y', 'x']))).toEqual([
      'Moved “Yankee” from 4th to 3rd',
    ])
  })

  it('reports a row that moves up only because another left as no move', () => {
    expect(changes(home(shown('a', 'b', 'c')), home(shown('b', 'c')))).toEqual(['Removed “Alpha” from home screen'])
  })

  describe('Discover', () => {
    it('reports a home row sent to Discover as one move, not a removal', () => {
      expect(changes(home(shown('a', 'b')), home([...discover('a'), ...shown('b')]))).toEqual([
        'Moved “Alpha” to Discover',
      ])
    })

    it('reports a Discover row brought home at its new place', () => {
      expect(changes(home(discover('a')), home(shown('a')))).toEqual([
        'Moved “Alpha” out of Discover, 1st on your home screen',
      ])
    })

    it('reports a catalog added straight to Discover', () => {
      expect(changes(home([]), home(discover('a')))).toEqual(['Added “Alpha” to Discover'])
    })

    it('reports a Discover row taken off', () => {
      expect(changes(home(discover('a')), home([]))).toEqual(['Removed “Alpha” from home screen'])
    })
  })

  describe('Show first', () => {
    it('reports turning it on as its own line, not as a move of the row or of those it passed', () => {
      expect(changes(home(shown('a'), ['x', 'y']), home(shown('a'), [first('x'), 'y']))).toEqual([
        'Showing “X-ray” first',
      ])
    })

    it('reports turning it off', () => {
      expect(changes(home(shown('a'), ['p', 'x']), home(shown('a'), [notFirst('p'), 'x']))).toEqual([
        'No longer showing “Pinned” first',
      ])
    })

    it('still reports a real move inside a group beside a flip', () => {
      expect(
        changes(home(shown('a'), ['p', 'x', 'y']), home(shown('a'), [notFirst('p'), 'y', 'x'])),
      ).toEqual(['No longer showing “Pinned” first', 'Moved “Yankee” from 4th to 3rd'])
    })

    it('reports a collection added already shown first only as added, at its place among the first', () => {
      expect(changes(home(shown('a')), home(shown('a'), [first('x')]))).toEqual([
        'Added “X-ray”, 1st on your home screen',
      ])
    })

    it('names the row action by the edit it makes', () => {
      expect(showFirstAction(false)).toBe('Show first')
      expect(showFirstAction(true)).toBe('Don’t show first')
    })
  })

  describe('collections changed since their last push', () => {
    it('reports one still on the home screen', () => {
      const state = home([], ['stale'])
      expect(changes(state, { ...state })).toEqual(['“Stale” changed since it was last pushed'])
    })

    it('reports one put back on the home screen only as added', () => {
      expect(changes(home([]), home([], ['stale']))).toEqual(['Added “Stale”, 1st on your home screen'])
    })

    it('reports nothing for one whose pushed copy is current', () => {
      const state = home([], ['x'])
      expect(changes(state, { ...state })).toEqual([])
    })
  })

  it('names rows nothing describes as unavailable', () => {
    expect(changes(home([]), home(shown('gone'), ['missing']))).toEqual([
      'Added “Unavailable catalog”, 1st on your home screen',
      'Added “Unavailable collection”, 2nd on your home screen',
    ])
  })

  it('gives every change its own key', () => {
    const list = computeHomeChanges({
      baseline: home([...shown('a', 'b'), ...discover('c')], ['x', 'stale', 'y']),
      current: home([...shown('b', 'd'), ...discover('a')], ['stale', first('y'), notFirst('x')]),
      catalogById,
      collectionById,
    })
    expect(new Set(list.map((change) => change.key)).size).toBe(list.length)
  })
})
