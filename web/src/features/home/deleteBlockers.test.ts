import { describe, expect, it } from 'vitest'
import { catalog, collection } from '@/test/fixtures'
import {
  catalogDeleteBlocker,
  collectionDeleteBlocker,
  deleteBlockersFor,
  deleteButton,
  pushedHome,
  PUSH_FIRST,
  TAKE_OFF_HOME,
  type PushedHome,
} from './deleteBlockers'

const doomed = catalog({ id: 'doomed' })

function home(overrides: Partial<PushedHome> = {}): PushedHome {
  return { catalogIDs: new Set(), collections: [], ...overrides }
}

describe('catalogDeleteBlocker', () => {
  it('holds a catalog with a Home row of its own', () => {
    expect(catalogDeleteBlocker('doomed', home({ catalogIDs: new Set(['doomed']) }))).toBe(TAKE_OFF_HOME)
  })

  it('names the first collection on Home that uses it', () => {
    const collections = [
      collection({ id: 'a', title: 'Other' }),
      collection({ id: 'b', title: 'First', catalogs: [doomed] }),
      collection({ id: 'c', title: 'Second', catalogs: [doomed] }),
    ]
    expect(catalogDeleteBlocker('doomed', home({ collections }))).toBe('Remove it from “First” and push first.')
  })

  it('holds every catalog while a collection on Home needs a push', () => {
    const collections = [collection({ id: 'a', needs_push: true })]
    expect(catalogDeleteBlocker('doomed', home({ collections }))).toBe(PUSH_FIRST)
  })

  it('lets a catalog go once nothing on Home holds it', () => {
    const collections = [collection({ id: 'a', catalogs: null })]
    expect(catalogDeleteBlocker('doomed', home({ collections, catalogIDs: new Set(['other']) }))).toBeNull()
  })
})

describe('collectionDeleteBlocker', () => {
  it('holds a collection on Home, and no other', () => {
    const pushed = home({ collections: [collection({ id: 'on' })] })
    expect(collectionDeleteBlocker('on', pushed)).toBe(TAKE_OFF_HOME)
    expect(collectionDeleteBlocker('off', pushed)).toBeNull()
  })
})

describe('pushedHome', () => {
  it('reads each collection through the library, which is fresher', () => {
    const stale = collection({ id: 'a', needs_push: false })
    const fresh = collection({ id: 'a', needs_push: true })
    const onlySelected = collection({ id: 'b' })
    const pushed = pushedHome([{ id: 'c1' }], [stale, onlySelected], new Map([['a', fresh]]))
    expect([...pushed.catalogIDs]).toEqual(['c1'])
    expect(pushed.collections).toEqual([fresh, onlySelected])
  })

  it('is empty before the selections load', () => {
    const pushed = pushedHome(undefined, undefined, new Map())
    expect(pushed.catalogIDs.size).toBe(0)
    expect(pushed.collections).toEqual([])
  })
})

describe('deleteBlockersFor', () => {
  it('answers by id for both kinds', () => {
    const blockers = deleteBlockersFor(home({ catalogIDs: new Set(['doomed']), collections: [collection({ id: 'on' })] }))
    expect(blockers.catalog('doomed')).toBe(TAKE_OFF_HOME)
    expect(blockers.collection('on')).toBe(TAKE_OFF_HOME)
    expect(blockers.collection('off')).toBeNull()
  })
})

describe('deleteButton', () => {
  it('is enabled and titled by its label while nothing holds the row', () => {
    expect(deleteButton('Night', null)).toEqual({ label: 'Delete Night', title: 'Delete Night', disabled: false })
  })

  it('is disabled and titled by the reason while something does', () => {
    expect(deleteButton('Night', TAKE_OFF_HOME)).toEqual({ label: 'Delete Night', title: TAKE_OFF_HOME, disabled: true })
  })
})
