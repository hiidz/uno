import { describe, expect, it } from 'vitest'
import { moveByOne, orderByKeys, reorder } from './order'

describe('reorder', () => {
  it('moves an item to another item’s place', () => {
    expect(reorder(['a', 'b', 'c'], 'c', 'a')).toEqual(['c', 'a', 'b'])
    expect(reorder(['a', 'b', 'c'], 'a', 'c')).toEqual(['b', 'c', 'a'])
  })

  it('leaves the list alone when either id is missing', () => {
    const ids = ['a', 'b']
    expect(reorder(ids, 'z', 'a')).toBe(ids)
  })
})

describe('moveByOne', () => {
  it('swaps an item with its neighbour', () => {
    expect(moveByOne(['a', 'b', 'c'], 'b', -1)).toEqual(['b', 'a', 'c'])
    expect(moveByOne(['a', 'b', 'c'], 'b', 1)).toEqual(['a', 'c', 'b'])
  })

  it('returns the same list at either edge or for a missing id', () => {
    const ids = ['a', 'b']
    expect(moveByOne(ids, 'a', -1)).toBe(ids)
    expect(moveByOne(ids, 'b', 1)).toBe(ids)
    expect(moveByOne(ids, 'z', 1)).toBe(ids)
  })
})

describe('orderByKeys', () => {
  const items = [{ key: 'a' }, { key: 'b' }, { key: 'c' }, { key: 'd' }]
  const keys = (list: { key: string }[]) => list.map((item) => item.key)

  it('follows the given order', () => {
    expect(keys(orderByKeys(items, ['c', 'a', 'd', 'b'], (item) => item.key))).toEqual(['c', 'a', 'd', 'b'])
  })

  it('keeps unlisted items after the listed ones, and skips unknown keys', () => {
    expect(keys(orderByKeys(items, ['c', 'z'], (item) => item.key))).toEqual(['c', 'a', 'b', 'd'])
  })
})
