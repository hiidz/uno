import { describe, expect, it } from 'vitest'
import {
  folderRecipes,
  folderTabs,
  interleaveTiles,
  normalizeTileShape,
  normalizeViewMode,
  sourceLabel,
  type PreviewFolder,
  type PreviewSource,
} from './model'

function source(overrides: Partial<PreviewSource>): PreviewSource {
  return { key: 'k', name: 'Popular', type: 'movie', params: '{}', genre: '', ...overrides }
}

function folderOf(sources: PreviewSource[]): PreviewFolder {
  return {
    id: 'f1',
    title: 'Folder',
    hideTitle: false,
    tileShape: 'POSTER',
    tileShapeAssumed: false,
    coverEmoji: '',
    coverImageUrl: '',
    sources,
    unresolved: 0,
  }
}

const tile = (tmdb_id: number) => ({ tmdb_id, title: `T${tmdb_id}`, year: '' })

describe('normalizeTileShape', () => {
  it('keeps a known shape and reads an empty one as an assumed poster', () => {
    expect(normalizeTileShape('LANDSCAPE')).toEqual({ shape: 'LANDSCAPE', assumed: false })
    expect(normalizeTileShape('')).toEqual({ shape: 'POSTER', assumed: true })
  })
})

describe('normalizeViewMode', () => {
  it('reads an empty or unknown mode as Tabbed Grids, and flags only Follow Layout as a guess', () => {
    expect(normalizeViewMode('ROWS')).toEqual({ mode: 'ROWS', assumed: false })
    expect(normalizeViewMode('')).toEqual({ mode: 'TABBED_GRID', assumed: false })
    expect(normalizeViewMode('SIDEWAYS')).toEqual({ mode: 'TABBED_GRID', assumed: false })
    expect(normalizeViewMode('FOLLOW_LAYOUT')).toEqual({ mode: 'FOLLOW_LAYOUT', assumed: true })
  })
})

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
    const folder = folderOf([source({ key: 'a', genre: 'Western' }), source({ key: 'b', genre: 'War' })])
    expect(folderTabs(folder, true)).toEqual([
      { key: '__all__', label: 'All' },
      { key: 'a', label: 'Popular (Movie) • Western' },
      { key: 'b', label: 'Popular (Movie) • War' },
    ])
  })

  it('leaves out the All tab unless the collection asks for it', () => {
    expect(folderTabs(folderOf([source({ key: 'a' })]), false)).toEqual([{ key: 'a', label: 'Popular (Movie)' }])
  })
})

describe('folderRecipes', () => {
  it('files each recipe under its source key and carries its genre', () => {
    const folder = folderOf([source({ key: 'a', genre: 'Western' }), source({ key: 'b', name: null, type: null })])
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

  it('takes one tile from each source in turn, so the cap leaves every source a share', () => {
    const merged = interleaveTiles(
      [
        { kind: 'movie', items: [tile(1), tile(2), tile(3)] },
        { kind: 'movie', items: [tile(10)] },
        { kind: 'movie', items: [tile(20), tile(21)] },
      ],
      5,
    )
    expect(merged.items.map((item) => item.tmdb_id)).toEqual([1, 10, 20, 2, 21])
  })
})
