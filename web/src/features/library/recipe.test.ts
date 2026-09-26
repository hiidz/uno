import { describe, expect, it } from 'vitest'
import type { Catalog } from '@/api'
import { catalog as row } from '@/test/fixtures'
import { describeRecipe, recipeSentence } from './recipe'

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

describe('recipeSentence', () => {
  const lookup = new Map<number, string>([[27, 'Horror']])

  it('makes the genres the subject and the sort the first clause', () => {
    const params = { sort_by: 'vote_average.desc', with_genres: '27' }
    expect(recipeSentence(catalog('movie', params), lookup)).toBe(
      'Shows horror movies, highest rated.',
    )
  })

  it('names the kind alone when no genre is picked, keeping A-Z in capitals', () => {
    expect(recipeSentence(catalog('series', { sort_by: 'title.asc' }), lookup)).toBe(
      'Shows series, A-Z.',
    )
    expect(recipeSentence(catalog('movie', { sort_by: 'original_title.desc' }), lookup)).toBe(
      'Shows movies, Z-A by original title.',
    )
  })

  it('leaves out a genre the lookup cannot name rather than reading its id as a count', () => {
    const params = { sort_by: 'vote_average.desc', with_genres: '27' }
    expect(recipeSentence(catalog('movie', params), new Map())).toBe(
      'Shows movies, highest rated.',
    )
    const mixed = { with_genres: '27,99999', randomized: true }
    expect(recipeSentence(catalog('movie', mixed), lookup)).toBe('Shows horror movies, shuffled.')
  })

  it('carries the remaining segments as clauses', () => {
    const params = { with_genres: '27', randomized: true }
    expect(recipeSentence(catalog('movie', params), lookup)).toBe('Shows horror movies, shuffled.')
  })

  it('describes a collection row by its collection', () => {
    const params = { with_collection: '10', randomized: true }
    expect(recipeSentence(catalog('movie', params), lookup)).toBe(
      'Shows the films in one movie collection, shuffled.',
    )
  })

  it('is empty for a catalog with no filters', () => {
    expect(recipeSentence(catalog('movie', {}), lookup)).toBe('')
  })
})
