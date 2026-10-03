import { describe, expect, it } from 'vitest'
import type { Collection } from '@/api'
import { collection, folder } from '@/test/fixtures'
import { pushBlock } from './pushBlock'

function byID(...collections: Collection[]): ReadonlyMap<string, Collection> {
  return new Map(collections.map((c) => [c.id, c]))
}

describe('pushBlock', () => {
  const full = collection({ id: 'full', title: 'Full', folders: [folder()] })
  const empty = collection({ id: 'empty', title: 'Empty', folders: [] })
  const unfolded = collection({ id: 'null', title: 'No folders at all', folders: null })

  it('blocks nothing for a Home whose collections all have folders', () => {
    expect(pushBlock(false, [{ id: 'full', pinToTop: false }], byID(full))).toBeNull()
  })

  it('blocks on the first collection on Home with no folders, by its title', () => {
    const home = [
      { id: 'full', pinToTop: false },
      { id: 'null', pinToTop: false },
      { id: 'empty', pinToTop: true },
    ]
    expect(pushBlock(false, home, byID(full, empty, unfolded))).toEqual({
      kind: 'empty-collection',
      title: 'No folders at all',
    })
  })

  it('ignores an empty collection that is not on Home, and one not loaded yet', () => {
    expect(pushBlock(false, [{ id: 'full', pinToTop: false }, { id: 'gone', pinToTop: false }], byID(full, empty))).toBeNull()
  })

  it('blocks a profile that uses profile 1’s addons, whatever Home holds', () => {
    expect(pushBlock(true, [{ id: 'empty', pinToTop: false }], byID(empty))).toEqual({ kind: 'shares-addons' })
  })
})
