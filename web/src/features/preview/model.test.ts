import { describe, expect, it } from 'vitest'
import {
  folderRecipes,
  folderTabs,
  interleaveTiles,
  sourceLabel,
  type PreviewFolder,
  type PreviewSource,
} from './model'

function source(overrides: Partial<PreviewSource>): PreviewSource {
  return { key: 'k', id: 'c1', name: 'Popular', type: 'movie', params: '{}', genre: '', ...overrides }
}

describe('sourceLabel', () => {
  it('names a source as Nuvio does, with the genre only when set', () => {
    expect(sourceLabel(source({}))).toBe('Popular (Movie)')
    expect(sourceLabel(source({ type: 'series', genre: 'War & Politics' }))).toBe('Popular (Series) • War & Politics')
  })

  it('reads an unresolved source as unavailable', () => {
    expect(sourceLabel(source({ name: null, type: null }))).toBe('Unavailable catalog')
  })
})

describe('folderTabs', () => {
  it('gives one catalog under two genres two distinct tabs', () => {
    const folder = {
      sources: [source({ key: 'a', genre: 'Western' }), source({ key: 'b', genre: 'War' })],
    } as PreviewFolder
    expect(folderTabs(folder, true)).toEqual([
      { key: '__all__', label: 'All' },
      { key: 'a', label: 'Popular (Movie) • Western' },
      { key: 'b', label: 'Popular (Movie) • War' },
    ])
  })
})

describe('folderRecipes', () => {
  it('files each recipe under its source key and carries its genre', () => {
    const folder = {
      sources: [source({ key: 'a', genre: 'Western' }), source({ key: 'b', name: null, type: null })],
    } as PreviewFolder
    expect(folderRecipes(folder)).toEqual([{ id: 'a', type: 'movie', params: '{}', genre: 'Western' }])
  })
})

describe('interleaveTiles', () => {
  it('keeps a movie and a series that share a TMDB id, each with its own kind', () => {
    const movie = { tmdb_id: 550, title: 'Fight Club', year: '1999' }
    const series = { tmdb_id: 550, title: 'Other', year: '2001' }
    const merged = interleaveTiles([
      { kind: 'movie', items: [movie] },
      { kind: 'tv', items: [series] },
    ])
    expect(merged.items).toEqual([movie, series])
    expect(merged.kinds.get(series)).toBe('tv')
  })

  it('drops a title a second source of the same kind repeats', () => {
    const a = { tmdb_id: 1, title: 'A', year: '' }
    const merged = interleaveTiles([
      { kind: 'movie', items: [a] },
      { kind: 'movie', items: [{ ...a }] },
    ])
    expect(merged.items).toEqual([a])
  })
})
