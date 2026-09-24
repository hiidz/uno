import { describe, expect, it } from 'vitest'
import type { ImportMatch } from '@/api'
import { choicesForAll, reuseMap } from './reuse'

function match(key: string, ...existing: string[]): ImportMatch {
  return {
    key,
    name: `Catalog ${key}`,
    type: 'movie',
    scope: 'listed',
    collection: '',
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
