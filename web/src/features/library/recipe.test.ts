import { describe, expect, it } from 'vitest'
import type { Catalog } from '@/api'
import { catalog as row } from '@/test/fixtures'
import { describeRecipe, foldedLine, openFacts, recipeFacts } from './recipe'

function catalog(type: Catalog['type'], params: object): Catalog {
  return row({ type, params: JSON.stringify(params) })
}

describe('describeRecipe', () => {
  it('mentions a collection on movie catalogs only', () => {
    const lookup = new Map<number, string>()
    expect(describeRecipe(catalog('movie', { with_collection: '10' }), lookup)).toEqual([
      'from a TMDB collection',
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
      'from 2 production companies',
      'not from 1 production company',
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
    expect(describeRecipe(catalog('movie', params), lookup)).toEqual(['from a TMDB collection'])
    expect(describeRecipe(catalog('movie', { ...params, randomized: true }), lookup)).toEqual([
      'from a TMDB collection',
      'shuffled',
    ])
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
    expect(facts).toContainEqual({ label: 'Age rating', value: expect.stringMatching(/^PG-13 in /) })
    expect(recipeFacts(catalog('movie', { certification_gte: 'R' }), lookup)).toContainEqual({ label: 'Age rating', value: 'R' })
  })

  it('counts streaming services, production companies, keywords and networks until their names are known', () => {
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
    expect(series).toContainEqual({ label: 'Production companies', value: '2' })
    expect(series).toContainEqual({ label: 'Left-out production company', value: '1' })
    expect(series).toContainEqual({ label: 'Keyword', value: '1' })
    expect(series).toContainEqual({ label: 'Left-out keywords', value: '2' })
    expect(series).toContainEqual({ label: 'Network', value: '1' })
    expect(recipeFacts(catalog('movie', params), lookup).map((f) => f.label)).not.toContain('Network')
    expect(recipeFacts(catalog('movie', { with_watch_providers: '8' }), lookup)).toContainEqual({
      label: 'Streaming service',
      value: '1',
    })
  })

  it('names production companies, keywords, networks and streaming services once every name is known', () => {
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
    expect(series).toContainEqual({ label: 'Production companies', value: 'Studio Ghibli or Pixar' })
    expect(series).toContainEqual({ label: 'Left-out production company', value: 'Troma' })
    expect(series).toContainEqual({ label: 'Keywords', value: 'slasher and zombie' })
    expect(series).toContainEqual({ label: 'Left-out keyword', value: 'gore' })
    expect(series).toContainEqual({ label: 'Network', value: 'Netflix' })
    expect(series).toContainEqual({ label: 'Streaming services', value: expect.stringMatching(/^Netflix or Disney Plus in / ) })
  })

  it('counts a list until all its names are known, and leaves a movie recipe’s networks out', () => {
    const params = { with_companies: '420|2', with_networks: '213' }
    const partial = { company: new Map([[420, 'Studio Ghibli']]), network: new Map([[213, 'Netflix']]) }
    expect(recipeFacts(catalog('series', params), lookup, partial)).toContainEqual({ label: 'Production companies', value: '2' })
    expect(recipeFacts(catalog('movie', params), lookup, partial).map((f) => f.label)).not.toContain('Network')
  })

  it('says an unknown sort as it was stored, and a shuffled recipe as shuffled', () => {
    const facts = recipeFacts(catalog('movie', { sort_by: 'odd.desc', randomized: true }), lookup)
    expect(facts).toContainEqual({ label: 'Order', value: 'odd.desc' })
    expect(facts.at(-1)).toEqual({ label: 'Shuffled', value: 'Yes' })
  })

  it('reads a TMDB collection row as the TMDB collection alone', () => {
    const params = { with_collection: '10', sort_by: 'popularity.desc', with_genres: '80', randomized: true }
    expect(recipeFacts(catalog('movie', params), lookup)).toEqual([
      { label: 'Type', value: 'Movies' },
      { label: 'From', value: 'A TMDB collection' },
      { label: 'Shuffled', value: 'Yes' },
    ])
    expect(recipeFacts(catalog('series', { with_collection: '10' }), lookup)).toEqual([{ label: 'Type', value: 'Series' }])
  })
})

describe('openFacts', () => {
  const lookup = new Map<number, string>()
  const open = (type: Catalog['type'], params: object) => {
    const row = catalog(type, params)
    return openFacts(row, recipeFacts(row, lookup))
  }

  it('reads every filter a movie recipe leaves unset as Any, Order as most popular', () => {
    expect(open('movie', {})).toEqual([
      { label: 'Genres', value: 'Any' },
      { label: 'Released', value: 'Any time' },
      { label: 'Rating', value: 'Any' },
      { label: 'Votes', value: 'Any' },
      { label: 'Runtime', value: 'Any length' },
      { label: 'Language', value: 'Any' },
      { label: 'Age rating', value: 'Any' },
      { label: 'Streaming service', value: 'Any' },
      { label: 'Production company', value: 'Any' },
      { label: 'Keywords', value: 'Any' },
      { label: 'Order', value: 'Most popular' },
    ])
  })

  it('leaves out what the recipe sets, in either the singular or the plural', () => {
    const labels = open('movie', { with_companies: '420|2', with_keywords: '9715', sort_by: 'vote_average.desc' }).map((f) => f.label)
    expect(labels).not.toContain('Production company')
    expect(labels).not.toContain('Keywords')
    expect(labels).not.toContain('Order')
    expect(labels).toContain('Genres')
  })

  it('reads a series recipe as aired, with networks', () => {
    const labels = open('series', {}).map((f) => f.label)
    expect(labels).toContain('Aired')
    expect(labels).toContain('Network')
    expect(labels).not.toContain('Released')
  })

  it('has none for a TMDB collection row', () => {
    expect(open('movie', { with_collection: '10' })).toEqual([])
  })
})

describe('foldedLine', () => {
  const lookup = new Map<number, string>([
    [28, 'Action'],
    [12, 'Adventure'],
    [18, 'Drama'],
    [99, 'Documentary'],
    [10770, 'TV Movie'],
    [36, 'History'],
  ])

  it('shows the first three phrases, the order first, and counts the rest', () => {
    const line = foldedLine(
      catalog('movie', {
        sort_by: 'vote_average.desc',
        with_genres: '28,12',
        vote_average_gte: 7,
        vote_count_gte: 100,
        with_original_language: 'ja',
      }),
      lookup,
    )
    expect(line).toEqual({ text: 'Highest rated · Action and Adventure · rated 7.0 or more', more: 2 })
  })

  it('counts a genre list of more than two, kept and left out', () => {
    expect(foldedLine(catalog('movie', { with_genres: '28|12|18' }), lookup).text).toBe('3 genres')
    expect(foldedLine(catalog('movie', { without_genres: '99,10770,36' }), lookup).text).toBe('No 3 genres')
    expect(foldedLine(catalog('movie', { without_genres: '99' }), lookup).text).toBe('No Documentary')
  })

  it('is empty for a recipe that sets nothing', () => {
    expect(foldedLine(catalog('movie', {}), lookup)).toEqual({ text: '', more: 0 })
  })
})
