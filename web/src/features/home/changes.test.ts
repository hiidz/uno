import { describe, expect, it } from 'vitest'
import type { Catalog, Collection, PendingChange } from '@/api'
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

function changes(baseline: HomeState, current: HomeState, waiting: PendingChange[] = []): string[] {
  return computeHomeChanges({
    baseline,
    current,
    catalogById,
    collectionById,
    waiting,
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

  describe('Pin', () => {
    it('reports pinning as its own line, not as a move of the row or of those it passed', () => {
      expect(changes(home(shown('a'), ['x', 'y']), home(shown('a'), [first('x'), 'y']))).toEqual([
        'Pinned “X-ray”',
      ])
    })

    it('reports unpinning', () => {
      expect(changes(home(shown('a'), ['p', 'x']), home(shown('a'), [notFirst('p'), 'x']))).toEqual([
        'Unpinned “Pinned”',
      ])
    })

    it('still reports a real move inside a group beside a flip', () => {
      expect(
        changes(home(shown('a'), ['p', 'x', 'y']), home(shown('a'), [notFirst('p'), 'y', 'x'])),
      ).toEqual(['Unpinned “Pinned”', 'Moved “Yankee” from 4th to 3rd'])
    })

    it('reports a collection added already pinned only as added, at its place among the pinned', () => {
      expect(changes(home(shown('a')), home(shown('a'), [first('x')]))).toEqual([
        'Added “X-ray”, 1st on your home screen',
      ])
    })

    it('names the row action by the edit it makes', () => {
      expect(showFirstAction(false)).toBe('Pin')
      expect(showFirstAction(true)).toBe('Unpin')
    })
  })

  describe('what the server says is waiting for a push', () => {
    const waiting = (kind: PendingChange['kind'], id: string, name: string, change: PendingChange['change']): PendingChange[] => [
      { kind, id, name, change },
    ]

    it('reports a collection still on the home screen that changed since its push', () => {
      const state = home([], ['x'])
      expect(changes(state, { ...state }, waiting('collection', 'x', 'X-ray', 'changed'))).toEqual([
        '“X-ray” changed since it was last pushed',
      ])
    })

    it('reports an edited catalog on the home screen the same way, as already saved', () => {
      const state = home(shown('a'))
      const list = computeHomeChanges({ baseline: state, current: { ...state }, catalogById, collectionById, waiting: waiting('catalog', 'a', 'Alpha', 'changed') })
      expect(list).toEqual([{ key: 'waiting:catalog:a', text: '“Alpha” changed since it was last pushed', saved: true }])
    })

    it('reports a row on the home screen that Nuvio holds nothing for', () => {
      const state = home(shown('a'))
      expect(changes(state, { ...state }, waiting('catalog', 'a', 'Alpha', 'added'))).toEqual(['“Alpha” isn’t in Nuvio yet'])
    })

    it('reports a deleted row by the name Nuvio still holds it under', () => {
      const state = home(shown('a'))
      const list = computeHomeChanges({ baseline: state, current: { ...state }, catalogById, collectionById, waiting: waiting('collection', 'gone', 'Deleted one', 'removed') })
      expect(list).toEqual([{ key: 'remove:collection:gone', text: 'Removed “Deleted one” from home screen', saved: true }])
    })

    it('says a removal once when this tab already takes the row off the home screen', () => {
      expect(changes(home(shown('a')), home([]), waiting('catalog', 'a', 'Alpha', 'removed'))).toEqual([
        'Removed “Alpha” from home screen',
      ])
    })

    it('reports a changed row this tab puts back on the home screen only as added', () => {
      expect(changes(home([]), home([], ['x']), waiting('collection', 'x', 'X-ray', 'changed'))).toEqual([
        'Added “X-ray”, 1st on your home screen',
      ])
    })

    it('reports nothing when nothing waits', () => {
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
      waiting: [
        { kind: 'collection', id: 'stale', name: 'Stale', change: 'changed' },
        { kind: 'catalog', id: 'gone', name: 'Gone', change: 'removed' },
      ],
      catalogById,
      collectionById,
    })
    expect(new Set(list.map((change) => change.key)).size).toBe(list.length)
  })
})
