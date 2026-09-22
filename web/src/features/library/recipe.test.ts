import { describe, expect, it } from 'vitest'
import type { Catalog } from '@/api'
import { describeRecipe } from './recipe'

function catalog(type: Catalog['type'], params: object): Catalog {
  return { type, params: JSON.stringify(params) } as Catalog
}

describe('describeRecipe', () => {
  it('mentions a collection on movie catalogs only', () => {
    const lookup = new Map<number, string>()
    expect(describeRecipe(catalog('movie', { with_collection: '10' }), lookup)).toEqual([
      'from a movie collection',
    ])
    expect(describeRecipe(catalog('series', { with_collection: '10' }), lookup)).toEqual([])
  })

  it('describes a collection row by the collection and shuffle alone', () => {
    const lookup = new Map<number, string>([[28, 'Action']])
    const params = { with_collection: '10', sort_by: 'popularity.desc', with_genres: '28' }
    expect(describeRecipe(catalog('movie', params), lookup)).toEqual(['from a movie collection'])
    expect(describeRecipe(catalog('movie', { ...params, randomized: true }), lookup)).toEqual([
      'from a movie collection',
      'shuffled',
    ])
  })
})
