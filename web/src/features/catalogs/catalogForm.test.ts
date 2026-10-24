import { describe, expect, it, vi } from 'vitest'
import { catalog } from '@/test/fixtures'
import {
  MAX_ENTITY_IDS,
  NEW_CATALOG_KIND,
  emptyForm,
  formFromCatalog,
  isSameCatalog,
  newCatalogForm,
  newCatalogKindError,
  paramsString,
  sourcesFor,
  toPayload,
  validateForm,
  withType,
  type CatalogFormState,
} from './catalogForm'

// `@/api`'s barrel pulls in the auth session, which needs a browser `window`.
vi.mock('@/api', () => ({ CATALOG_PROVIDER: 'tmdb' }))

/** A movie form with a filter in every section, several of them invalid. */
function filledForm(): CatalogFormState {
  return {
    ...emptyForm('movie'),
    name: 'Row',
    dateMode: 'rolling',
    params: {
      sort_by: 'name.asc',
      with_genres: '28',
      vote_average_gte: 8,
      vote_average_lte: 2,
      with_original_language: 'en',
      certification: 'PG',
      with_watch_providers: '8',
      with_companies: '420',
      with_keywords: '9715',
      without_companies: '2',
      without_keywords: '180547',
      randomized: true,
    },
  }
}

describe('scope', () => {
  const scoped = formFromCatalog(catalog({ name: 'Scoped', collection_id: 'col1' }))

  it('is not part of the payload, nor of what makes an edit', () => {
    expect(scoped.collectionID).toBe('col1')
    expect(toPayload(scoped)).not.toHaveProperty('collection_id')
    expect(isSameCatalog(scoped, { ...scoped, collectionID: null })).toBe(true)
    expect(isSameCatalog(scoped, { ...scoped, name: 'Renamed' })).toBe(false)
  })
})

describe('source mode', () => {
  it('sends only the collection and shuffle in collection mode', () => {
    const state = filledForm()
    state.params.with_collection = '10'
    state.sourceMode = 'collection'
    expect(JSON.parse(paramsString(state))).toEqual({ with_collection: '10', randomized: true })
  })

  it('sends every filter but the collection in filters mode', () => {
    const state = filledForm()
    state.params.with_collection = '10'
    const sent = JSON.parse(paramsString(state)) as Record<string, unknown>
    expect(sent).toMatchObject({
      sort_by: 'name.asc',
      with_companies: '420',
      without_companies: '2',
      without_keywords: '180547',
      randomized: true,
    })
    expect(sent).not.toHaveProperty('with_collection')
  })

  it('keeps both sides in form state across a switch', () => {
    const state = filledForm()
    state.params.with_collection = '10'
    state.sourceMode = 'collection'
    state.sourceMode = 'filters'
    expect(JSON.parse(paramsString(state))).toMatchObject({ with_companies: '420' })
    state.sourceMode = 'collection'
    expect(JSON.parse(paramsString(state))).toEqual({ with_collection: '10', randomized: true })
  })

  it('reports no errors for the filters collection mode drops', () => {
    const state = filledForm()
    expect(Object.keys(validateForm(state)).sort()).toEqual([
      'certification_country',
      'sort_by',
      'vote_average',
      'watch_region',
      'within_days',
    ])
    state.params.with_collection = '10'
    state.sourceMode = 'collection'
    expect(validateForm(state)).toEqual({})
    state.name = ''
    expect(Object.keys(validateForm(state))).toEqual(['name'])
  })

  it('requires a collection in collection mode', () => {
    const state: CatalogFormState = { ...emptyForm('movie'), name: 'Row', sourceMode: 'collection' }
    expect(Object.keys(validateForm(state))).toEqual(['with_collection'])
    state.params = { with_collection: '10' }
    expect(validateForm(state)).toEqual({})
  })

  it('reads a saved collection back as collection mode', () => {
    expect(formFromCatalog(catalog({ params: '{"with_collection":"10"}' })).sourceMode).toBe('collection')
    expect(formFromCatalog(catalog({ params: '{}' })).sourceMode).toBe('filters')
  })

  it('never sends a collection on a series catalog', () => {
    const state: CatalogFormState = {
      ...emptyForm('series'),
      params: { with_collection: '10', with_genres: '18' },
    }
    const sent = JSON.parse(paramsString(state)) as Record<string, unknown>
    expect(sent).toMatchObject({ with_genres: '18' })
    expect(sent).not.toHaveProperty('with_collection')
  })
})

describe('networks', () => {
  const ids = (n: number) => Array.from({ length: n }, (_, i) => i + 1).join('|')

  it('sends networks on a series catalog and never on a movie catalog', () => {
    const series: CatalogFormState = { ...emptyForm('series'), params: { with_networks: '213|49' } }
    expect(JSON.parse(paramsString(series))).toEqual({ with_networks: '213|49' })
    const movie: CatalogFormState = { ...emptyForm('movie'), params: { with_networks: '213|49' } }
    expect(JSON.parse(paramsString(movie))).not.toHaveProperty('with_networks')
  })

  it('allows 20 networks and rejects 21, on series only', () => {
    const state: CatalogFormState = {
      ...emptyForm('series'),
      name: 'Row',
      params: { with_networks: ids(MAX_ENTITY_IDS) },
    }
    expect(validateForm(state)).toEqual({})
    state.params = { with_networks: ids(21) }
    expect(Object.keys(validateForm(state))).toEqual(['with_networks'])
    expect(validateForm({ ...state, type: 'movie' })).toEqual({})
  })
})

describe('company and keyword cap', () => {
  const ids = (n: number) => Array.from({ length: n }, (_, i) => i + 1).join('|')

  it('allows 20 of each and rejects 21', () => {
    const state: CatalogFormState = {
      ...emptyForm('movie'),
      name: 'Row',
      params: {
        with_companies: ids(MAX_ENTITY_IDS),
        with_keywords: ids(MAX_ENTITY_IDS),
        without_companies: ids(MAX_ENTITY_IDS),
        without_keywords: ids(MAX_ENTITY_IDS),
      },
    }
    expect(validateForm(state)).toEqual({})
    state.params = {
      with_companies: ids(21),
      with_keywords: ids(21),
      without_companies: ids(21),
      without_keywords: ids(21),
    }
    expect(Object.keys(validateForm(state)).sort()).toEqual([
      'with_companies',
      'with_keywords',
      'without_companies',
      'without_keywords',
    ])
  })
})

describe('validateForm: the other server rules a form can reach', () => {
  const named = (update: Partial<CatalogFormState>): CatalogFormState => ({ ...emptyForm('movie'), name: 'Row', ...update })

  it('holds the name to 60 characters, counted as characters', () => {
    expect(validateForm(named({ name: '日'.repeat(60) }))).toEqual({})
    expect(validateForm(named({ name: '日'.repeat(61) })).name).toBe('Keep the name to 60 characters or fewer.')
  })

  it('keeps a rating between 0 and 10 and the counts non-negative', () => {
    expect(validateForm(named({ params: { vote_average_gte: 0, vote_average_lte: 10 } }))).toEqual({})
    expect(validateForm(named({ params: { vote_average_lte: 10.5 } })).vote_average).toBe('Pick a number from 0 to 10.')
    expect(validateForm(named({ params: { vote_average_gte: -1 } })).vote_average).toBe('Pick a number from 0 to 10.')
    expect(validateForm(named({ params: { vote_count_gte: -5 } })).vote_count).toBe('Numbers can’t be negative.')
    expect(validateForm(named({ params: { with_runtime_gte: -1 } })).with_runtime).toBe('Numbers can’t be negative.')
  })

  it('wants fixed dates that are real and in order, for the type in use', () => {
    const fixed = (params: CatalogFormState['params'], type: CatalogFormState['type'] = 'movie') =>
      validateForm(named({ type, dateMode: 'fixed', params })).date_range
    expect(fixed({ primary_release_date_gte: '2020-01-01', primary_release_date_lte: '2020-01-01' })).toBeUndefined()
    expect(fixed({ primary_release_date_gte: '2021-01-01', primary_release_date_lte: '2020-01-01' })).toBe(
      'The start date is after the end date.',
    )
    expect(fixed({ primary_release_date_gte: '2020-02-30' })).toBe('Pick real dates.')
    expect(fixed({ primary_release_date_lte: '275760-09-13' })).toBe('Pick real dates.')
    expect(fixed({ first_air_date_gte: '2021-01-01', first_air_date_lte: '2020-01-01' }, 'series')).toBe(
      'The start date is after the end date.',
    )
    // The other type's dates are not sent, so they raise nothing.
    expect(fixed({ first_air_date_gte: '2021-01-01', first_air_date_lte: '2020-01-01' })).toBeUndefined()
  })
})

describe('the New catalog dialog\'s kind', () => {
  it('offers a TMDB collection for movies only, and a TMDB list for both', () => {
    expect(sourcesFor('movie').map((s) => s.label)).toEqual(['Filters', 'TMDB collection', 'TMDB list'])
    expect(sourcesFor('series').map((s) => s.label)).toEqual(['Filters', 'TMDB list'])
  })

  it('keeps a list source across types, unpicking a list the new type lacks', () => {
    expect(withType({ type: 'movie', source: 'list', list: 'now_playing' }, 'series')).toEqual({
      type: 'series',
      source: 'list',
      list: undefined,
    })
    expect(withType({ type: 'movie', source: 'list', list: 'popular' }, 'series').list).toBe('popular')
  })

  it('creates nothing from a list source until a list is picked, and sends that list alone', () => {
    expect(newCatalogKindError({ type: 'series', source: 'list' })).toBe('Pick a list.')
    const form = { ...newCatalogForm({ type: 'series', source: 'list', list: 'airing_today' }), name: 'New' }
    expect(toPayload(form).params).toBe('{"tmdb_list":"airing_today"}')
    expect(toPayload({ ...form, params: { ...form.params, with_genres: '18', randomized: true } }).params).toBe(
      '{"tmdb_list":"airing_today","randomized":true}',
    )
  })

  it('reads a saved list back as a list row', () => {
    expect(formFromCatalog(catalog({ type: 'series', params: '{"tmdb_list":"popular"}' })).sourceMode).toBe('list')
    expect(validateForm({ ...newCatalogForm({ type: 'movie', source: 'list' }), name: 'Row' }).tmdb_list).toBe(
      'Pick a list.',
    )
  })

  it('falls back to Filters when the type can\'t be made from the source', () => {
    const picked = { type: 'movie' as const, source: 'collection' as const, collection: '263' }
    expect(withType(picked, 'series')).toEqual({ type: 'series', source: 'filters', collection: '263', list: undefined })
    expect(withType({ ...NEW_CATALOG_KIND, source: 'filters' }, 'series').source).toBe('filters')
  })

  it('creates nothing from a TMDB collection source until one is picked', () => {
    expect(newCatalogKindError(NEW_CATALOG_KIND)).toBeUndefined()
    expect(newCatalogKindError({ type: 'movie', source: 'collection' })).toBe('Pick a TMDB collection.')
    expect(newCatalogKindError({ type: 'movie', source: 'collection', collection: '263' })).toBeUndefined()
  })

  it('makes a catalog complete from the start', () => {
    expect(toPayload({ ...newCatalogForm(NEW_CATALOG_KIND), name: 'Row' }).params).toBe('{}')
    const collection = toPayload({ ...newCatalogForm({ type: 'movie', source: 'collection', collection: '263' }), name: 'Batman' })
    expect(collection.params).toBe('{"with_collection":"263"}')
    expect(newCatalogForm(NEW_CATALOG_KIND, 'coll-1').collectionID).toBe('coll-1')
  })
})
