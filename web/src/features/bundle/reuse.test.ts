import { describe, expect, it } from 'vitest'
import type { ImportCatalog, ImportCheck } from '@/api'
import { choicesForAll, importTally, liveCatalogs, liveMatches, reuseMap, withSkip } from './reuse'

function match(key: string, ...existing: string[]): ImportCatalog {
  return {
    key,
    name: `Catalog ${key}`,
    type: 'movie',
    params: '{}',
    existing: existing.map((id) => ({ id, name: `Mine ${id}` })),
  }
}

const matches = [match('c1', 'a'), match('c2', 'b', 'c')]

describe('choicesForAll', () => {
  it('starts every match as a copy, remembering its first existing catalog', () => {
    expect(choicesForAll(matches, false)).toEqual({
      c1: { useExisting: false, existingID: 'a' },
      c2: { useExisting: false, existingID: 'b' },
    })
  })

  it('uses the first existing catalog for every match when applied to all', () => {
    expect(choicesForAll(matches, true)).toEqual({
      c1: { useExisting: true, existingID: 'a' },
      c2: { useExisting: true, existingID: 'b' },
    })
  })
})

describe('reuseMap', () => {
  it('is empty while every match imports a copy', () => {
    expect(reuseMap(matches, choicesForAll(matches, false))).toEqual({})
  })

  it('maps each key set to use an existing catalog, and only those', () => {
    const choices = {
      c1: { useExisting: false, existingID: 'a' },
      c2: { useExisting: true, existingID: 'c' },
    }
    expect(reuseMap(matches, choices)).toEqual({ c2: 'c' })
  })

  it('maps every key after use-existing is applied to all', () => {
    expect(reuseMap(matches, choicesForAll(matches, true))).toEqual({ c1: 'a', c2: 'b' })
  })

  it('leaves out an id the match did not offer', () => {
    const choices = { c1: { useExisting: true, existingID: 'z' } }
    expect(reuseMap(matches, choices)).toEqual({})
  })
})

describe('withSkip', () => {
  it('adds a skipped collection once, and drops one taken back', () => {
    expect(withSkip([2], 0, true)).toEqual([2, 0])
    expect(withSkip([2, 0], 0, true)).toEqual([2, 0])
    expect(withSkip([2, 0], 2, false)).toEqual([0])
  })
})

describe('importTally and liveMatches', () => {
  /** Top-level c1 (matched) and c3, a collection holding c4, and Halloween
   *  holding c2 (matched) and c5. */
  const check: ImportCheck = {
    catalogs: [matches[0], match('c3')],
    collections: [
      { title: 'Cosy', folders: [], matched: false, catalogs: [match('c4')] },
      { title: 'Halloween', folders: [], matched: true, catalogs: [matches[1], match('c5')] },
    ],
  }

  it('counts every catalog and collection while nothing is reused or skipped', () => {
    expect(liveCatalogs(check, []).map((c) => c.key)).toEqual(['c1', 'c3', 'c4', 'c2', 'c5'])
    expect(liveMatches(check, []).map((c) => c.key)).toEqual(['c1', 'c2'])
    expect(importTally(check, choicesForAll(matches, false), [])).toEqual({ catalogs: 5, collections: 2 })
  })

  it('leaves out each catalog reused, and only a reuse that will be sent', () => {
    const choices = {
      c1: { useExisting: true, existingID: 'z' },
      c2: { useExisting: true, existingID: 'c' },
    }
    expect(importTally(check, choices, [])).toEqual({ catalogs: 4, collections: 2 })
  })

  it('leaves out a skipped collection with its own catalogs, and their matches with them', () => {
    expect(liveMatches(check, [1]).map((m) => m.key)).toEqual(['c1'])
    const reuseAll = choicesForAll(matches, true)
    expect(importTally(check, reuseAll, [1])).toEqual({ catalogs: 2, collections: 1 })
  })
})
