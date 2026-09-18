import { describe, expect, it, vi } from 'vitest'
import { folderRecipes, folderTabs, sourceLabel, type PreviewFolder, type PreviewSource } from './model'

// `@/api`'s barrel reaches the auth session, which touches `window` at import
// time; the model needs only this one pure helper from it.
vi.mock('@/api', () => ({ tmdbKind: (type: string) => (type === 'series' ? 'tv' : 'movie') }))

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
