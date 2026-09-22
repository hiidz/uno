import { describe, expect, it, vi } from 'vitest'
import type { Catalog } from '@/api'
import {
  MAX_ENTITY_IDS,
  emptyForm,
  formFromCatalog,
  paramsString,
  parseGenreList,
  serializeGenreList,
  validateForm,
  type CatalogFormState,
} from './catalogForm'

// `@/api`'s barrel pulls in the auth session, which needs a browser `window`.
vi.mock('@/api', () => ({ CATALOG_PROVIDER: 'tmdb' }))
import { companyDetail, sumCollection, sumEntities } from './summary'

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
      randomized: true,
    },
  }
}

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
    expect(sent).toMatchObject({ sort_by: 'name.asc', with_companies: '420', randomized: true })
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
    const saved = { id: '1', name: 'Row', is_public: false, collection_id: null }
    expect(
      formFromCatalog({ ...saved, type: 'movie', params: '{"with_collection":"10"}' } as Catalog)
        .sourceMode,
    ).toBe('collection')
    expect(formFromCatalog({ ...saved, type: 'movie', params: '{}' } as Catalog).sourceMode).toBe(
      'filters',
    )
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

describe('company and keyword cap', () => {
  const ids = (n: number) => Array.from({ length: n }, (_, i) => i + 1).join('|')

  it('allows 20 of each and rejects 21', () => {
    const state: CatalogFormState = {
      ...emptyForm('movie'),
      name: 'Row',
      params: { with_companies: ids(MAX_ENTITY_IDS), with_keywords: ids(MAX_ENTITY_IDS) },
    }
    expect(validateForm(state)).toEqual({})
    state.params = { with_companies: ids(21), with_keywords: ids(21) }
    expect(Object.keys(validateForm(state)).sort()).toEqual(['with_companies', 'with_keywords'])
  })
})

describe('parseGenreList / serializeGenreList', () => {
  it('reads commas as all and pipes as any', () => {
    expect(parseGenreList('420,2')).toEqual({ ids: [420, 2], join: 'and' })
    expect(parseGenreList('420|2')).toEqual({ ids: [420, 2], join: 'or' })
  })

  it('reads empty and missing as no ids', () => {
    expect(parseGenreList(undefined)).toEqual({ ids: [], join: 'and' })
    expect(parseGenreList('')).toEqual({ ids: [], join: 'and' })
  })

  it('drops parts that are not positive ids', () => {
    expect(parseGenreList(' 420 ,,abc,-3,0, 2 ').ids).toEqual([420, 2])
    expect(parseGenreList('x|7|').ids).toEqual([7])
  })

  it('round-trips both joins', () => {
    for (const raw of ['420,2,7', '420|2|7', '99']) {
      const { ids, join } = parseGenreList(raw)
      expect(serializeGenreList(ids, join)).toBe(raw)
    }
  })

  it('writes the join it is given', () => {
    expect(serializeGenreList([1, 2], 'and')).toBe('1,2')
    expect(serializeGenreList([1, 2], 'or')).toBe('1|2')
    expect(serializeGenreList([], 'or')).toBe('')
  })
})

describe('sumEntities', () => {
  it('counts ids and names the join once there are two', () => {
    expect(sumEntities(undefined, 'studio', 'Any studio')).toBe('Any studio')
    expect(sumEntities('420', 'studio', 'Any studio')).toBe('1 studio')
    expect(sumEntities('420|2', 'studio', 'Any studio')).toBe('2 studios, any of them')
    expect(sumEntities('420,2', 'keyword', 'Any keywords')).toBe('2 keywords, all of them')
  })
})

describe('sumCollection', () => {
  it('names the one pick, falling back to its id until the name loads', () => {
    expect(sumCollection(undefined, undefined)).toBe('No collection picked')
    expect(sumCollection('', 'Star Wars Collection')).toBe('No collection picked')
    expect(sumCollection('10', 'Star Wars Collection')).toBe('Star Wars Collection')
    expect(sumCollection('10', undefined)).toBe('Collection 10')
  })
})

describe('companyDetail', () => {
  it('names the country and counts titles of the catalog type', () => {
    expect(companyDetail({ origin_country: 'US', title_count: 176 }, 'movie')).toBe('US · 176 films')
    expect(companyDetail({ origin_country: 'GB', title_count: 1 }, 'movie')).toBe('GB · 1 film')
    expect(companyDetail({ origin_country: 'US', title_count: 11 }, 'series')).toBe('US · 11 series')
  })

  it('leaves out a missing country', () => {
    expect(companyDetail({ origin_country: '', title_count: 9 }, 'movie')).toBe('9 films')
  })
})
