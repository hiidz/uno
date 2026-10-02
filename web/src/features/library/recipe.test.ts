import { describe, expect, it } from 'vitest'
import type { Catalog } from '@/api'
import { catalog as row } from '@/test/fixtures'
import { describeRecipe, recipeFacts, recipeSentence } from './recipe'

function catalog(type: Catalog['type'], params: object): Catalog {
  return row({ type, params: JSON.stringify(params) })
}

describe('describeRecipe', () => {
  it('mentions a collection on movie catalogs only', () => {
    const lookup = new Map<number, string>()
    expect(describeRecipe(catalog('movie', { with_collection: '10' }), lookup)).toEqual([
      'from a film series',
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
    expect(describeRecipe(catalog('movie', params), lookup)).toEqual(['from a film series'])
    expect(describeRecipe(catalog('movie', { ...params, randomized: true }), lookup)).toEqual([
      'from a film series',
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
      'Shows the films in one film series, shuffled.',
    )
  })

  it('is empty for a catalog with no filters', () => {
    expect(recipeSentence(catalog('movie', {}), lookup)).toBe('')
  })
})

describe('recipeFacts', () => {
  const lookup = new Map<number, string>([
    [80, 'Crime'],
    [53, 'Thriller'],
  ])

  it('has the type alone for a recipe that sets nothing else', () => {
    expect(recipeFacts(catalog('movie', {}), lookup)).toEqual([{ label: 'Type', value: 'Movies' }])
    expect(recipeFacts(catalog('series', {}), lookup)).toEqual([{ label: 'Type', value: 'Series' }])
  })

  it('lists each filter a movie recipe sets, in reading order', () => {
    const params = {
      with_genres: '80,53',
      primary_release_date_gte: '1940-01-01',
      primary_release_date_lte: '1959-12-31',
      vote_average_gte: 7,
      vote_count_gte: 200,
      sort_by: 'popularity.desc',
    }
    expect(recipeFacts(catalog('movie', params), lookup)).toEqual([
      { label: 'Type', value: 'Movies' },
      { label: 'Genres', value: 'Crime and Thriller' },
      { label: 'Released', value: '1940–1959' },
      { label: 'Rating', value: '7.0 or more' },
      { label: 'Votes', value: '200 or more' },
      { label: 'Order', value: 'Most popular' },
    ])
  })

  it('joins genres with "or" when the list is pipe-joined, and names the ones left out', () => {
    const facts = recipeFacts(catalog('movie', { with_genres: '80|53', without_genres: '53' }), lookup)
    expect(facts).toContainEqual({ label: 'Genres', value: 'Crime or Thriller' })
    expect(facts).toContainEqual({ label: 'Without genres', value: 'Thriller' })
  })

  it('words a series window as Aired, and a rolling or upcoming window as it reads', () => {
    expect(recipeFacts(catalog('series', { first_air_date_gte: '2000-01-01' }), lookup)).toContainEqual({
      label: 'Aired',
      value: '2000 or later',
    })
    expect(recipeFacts(catalog('movie', { released_within_days: 730 }), lookup)).toContainEqual({
      label: 'Released',
      value: 'In the last 2 years',
    })
    expect(recipeFacts(catalog('movie', { released_within_days: 1 }), lookup)).toContainEqual({
      label: 'Released',
      value: 'Not out yet',
    })
  })

  it('gives runtime, language and certification their own tiles', () => {
    const params = {
      with_runtime_lte: 110,
      with_original_language: 'ja',
      certification: 'PG-13',
      certification_country: 'US',
    }
    const facts = recipeFacts(catalog('movie', params), lookup)
    expect(facts).toContainEqual({ label: 'Runtime', value: '110 min or less' })
    expect(facts).toContainEqual({ label: 'Language', value: expect.stringMatching(/japanese/i) })
    expect(facts).toContainEqual({ label: 'Rated', value: expect.stringMatching(/^PG-13 in /) })
    expect(recipeFacts(catalog('movie', { certification_gte: 'R' }), lookup)).toContainEqual({ label: 'Rated', value: 'R' })
  })

  it('counts streaming services, studios, keywords and networks until their names are known', () => {
    const params = {
      with_watch_providers: '8|337',
      watch_region: 'US',
      with_companies: '420|2',
      without_companies: '9993',
      with_keywords: '9715',
      without_keywords: '849,12',
      with_networks: '213',
    }
    const series = recipeFacts(catalog('series', params), lookup)
    expect(series).toContainEqual({ label: 'Streaming services', value: expect.stringMatching(/^2 in / )})
    expect(series).toContainEqual({ label: 'Studios', value: '2' })
    expect(series).toContainEqual({ label: 'Left-out studio', value: '1' })
    expect(series).toContainEqual({ label: 'Keyword', value: '1' })
    expect(series).toContainEqual({ label: 'Left-out keywords', value: '2' })
    expect(series).toContainEqual({ label: 'Network', value: '1' })
    expect(recipeFacts(catalog('movie', params), lookup).map((f) => f.label)).not.toContain('Network')
    expect(recipeFacts(catalog('movie', { with_watch_providers: '8' }), lookup)).toContainEqual({
      label: 'Streaming service',
      value: '1',
    })
  })

  it('names studios, keywords, networks and streaming services once every name is known', () => {
    const params = {
      with_watch_providers: '8|337',
      watch_region: 'US',
      with_companies: '420|2',
      without_companies: '9993',
      with_keywords: '9715,12',
      without_keywords: '849',
      with_networks: '213',
    }
    const names = {
      company: new Map([[420, 'Studio Ghibli'], [2, 'Pixar'], [9993, 'Troma']]),
      keyword: new Map([[9715, 'slasher'], [12, 'zombie'], [849, 'gore']]),
      network: new Map([[213, 'Netflix']]),
      provider: new Map([[8, 'Netflix'], [337, 'Disney Plus']]),
    }
    const series = recipeFacts(catalog('series', params), lookup, names)
    expect(series).toContainEqual({ label: 'Studios', value: 'Studio Ghibli or Pixar' })
    expect(series).toContainEqual({ label: 'Left-out studio', value: 'Troma' })
    expect(series).toContainEqual({ label: 'Keywords', value: 'slasher and zombie' })
    expect(series).toContainEqual({ label: 'Left-out keyword', value: 'gore' })
    expect(series).toContainEqual({ label: 'Network', value: 'Netflix' })
    expect(series).toContainEqual({ label: 'Streaming services', value: expect.stringMatching(/^Netflix or Disney Plus in / ) })
  })

  it('counts a list until all its names are known, and leaves a movie recipe’s networks out', () => {
    const params = { with_companies: '420|2', with_networks: '213' }
    const partial = { company: new Map([[420, 'Studio Ghibli']]), network: new Map([[213, 'Netflix']]) }
    expect(recipeFacts(catalog('series', params), lookup, partial)).toContainEqual({ label: 'Studios', value: '2' })
    expect(recipeFacts(catalog('movie', params), lookup, partial).map((f) => f.label)).not.toContain('Network')
  })

  it('says an unknown sort as it was stored, and a shuffled recipe as shuffled', () => {
    const facts = recipeFacts(catalog('movie', { sort_by: 'odd.desc', randomized: true }), lookup)
    expect(facts).toContainEqual({ label: 'Order', value: 'odd.desc' })
    expect(facts.at(-1)).toEqual({ label: 'Shuffled', value: 'Yes' })
  })

  it('reads a film series row as the film series alone', () => {
    const params = { with_collection: '10', sort_by: 'popularity.desc', with_genres: '80', randomized: true }
    expect(recipeFacts(catalog('movie', params), lookup)).toEqual([
      { label: 'Type', value: 'Movies' },
      { label: 'From', value: 'A film series' },
      { label: 'Shuffled', value: 'Yes' },
    ])
    expect(recipeFacts(catalog('series', { with_collection: '10' }), lookup)).toEqual([{ label: 'Type', value: 'Series' }])
  })
})
