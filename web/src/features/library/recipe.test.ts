import { describe, expect, it } from 'vitest'
import type { Catalog } from '@/api'
import { catalog as row } from '@/test/fixtures'
import { describeRecipe } from './recipe'

function catalog(type: Catalog['type'], params: object): Catalog {
  return row({ type, params: JSON.stringify(params) })
}

describe('describeRecipe', () => {
  it('mentions a collection on movie catalogs only', () => {
    const lookup = new Map<number, string>()
    expect(describeRecipe(catalog('movie', { with_collection: '10' }), lookup)).toEqual([
      'from a movie collection',
    ])
    expect(describeRecipe(catalog('series', { with_collection: '10' }), lookup)).toEqual([])
  })

  it('counts included and left-out companies and keywords', () => {
    const params = {
      with_companies: '420|2',
      without_companies: '9993',
      with_keywords: '9715',
      without_keywords: '849,12',
    }
    expect(describeRecipe(catalog('series', params), new Map())).toEqual([
      'from 2 studios',
      'not from 1 studio',
      'tagged with 1 keyword',
      'not tagged with 2 keywords',
    ])
  })

  it('counts networks on series catalogs only', () => {
    const params = { with_networks: '213|49' }
    expect(describeRecipe(catalog('series', params), new Map())).toEqual(['on 2 networks'])
    expect(describeRecipe(catalog('movie', params), new Map())).toEqual([])
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
