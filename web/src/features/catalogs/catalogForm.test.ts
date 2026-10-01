import { describe, expect, it, vi } from 'vitest'
import { catalog } from '@/test/fixtures'
import {
  MAX_ENTITY_IDS,
  emptyForm,
  formFromCatalog,
  isSameCatalog,
  paramsString,
  toPayload,
  validateForm,
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

  it('is not part of the payload', () => {
    expect(scoped.collectionID).toBe('col1')
    expect(toPayload(scoped)).not.toHaveProperty('collection_id')
  })

  it('still makes a staged Move to library an edit', () => {
    expect(isSameCatalog(scoped, { ...scoped })).toBe(true)
    expect(isSameCatalog(scoped, { ...scoped, collectionID: null })).toBe(false)
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
