import { describe, expect, it } from 'vitest'
import type { SnapshotChange } from '@/api'
import {
  catalogMarkWords,
  factChangeNote,
  firstUses,
  folderEntryMark,
  folderMarkWords,
  pageBlockMark,
  recipeChanges,
  renamedFrom,
  updateMarks,
  updateSummary,
} from './updateMarks'

describe('recipeChanges', () => {
  it('lists each filter whose value differs, against its open value when it comes or goes', () => {
    const was = [
      { label: 'Type', value: 'Movies' },
      { label: 'Released', value: '1980–1989' },
      { label: 'Production company', value: 'Any' },
      { label: 'Order', value: 'Most popular' },
      { label: 'Left-out keyword', value: 'gore' },
    ]
    const now = [
      { label: 'Type', value: 'Movies' },
      { label: 'Released', value: '1975–1989' },
      { label: 'Production companies', value: 'Studio Ghibli or Pixar' },
      { label: 'Order', value: 'Highest rated' },
    ]
    expect(recipeChanges(was, now)).toEqual([
      { label: 'Released', value: '1975–1989', was: '1980–1989', list: null },
      { label: 'Production companies', value: 'Studio Ghibli or Pixar', was: 'Any', list: null },
      { label: 'Order', value: 'Highest rated', was: 'Most popular', list: null },
      { label: 'Left-out keyword', value: '', was: 'gore', list: null },
    ])
  })

  it('reads a list both sides name item by item: what it gained and lost, its join and its region', () => {
    const was = [
      { label: 'Genres', value: 'Action and Drama', list: { items: ['Action', 'Drama'], join: 'and' as const, region: '' } },
      {
        label: 'Streaming services',
        value: 'Netflix or Hulu in United States',
        list: { items: ['Netflix', 'Hulu'], join: 'or' as const, region: 'United States' },
      },
    ]
    const now = [
      {
        label: 'Genres',
        value: 'Action, Drama or War',
        list: { items: ['Action', 'Drama', 'War'], join: 'or' as const, region: '' },
      },
      {
        label: 'Streaming service',
        value: 'Netflix in United Kingdom',
        list: { items: ['Netflix'], join: 'or' as const, region: 'United Kingdom' },
      },
    ]
    const [genres, streaming] = recipeChanges(was, now)
    expect(factChangeNote(genres!)).toBe('added War · now matches any of them, was all of them')
    expect(factChangeNote(streaming!)).toBe('removed Hulu · was in United States')
    expect(streaming!.label).toBe('Streaming service')
  })

  it('says only the old value when a list kept its items, or a single value changed', () => {
    const list = (region: string) => ({ items: ['Netflix'], join: 'and' as const, region })
    const [moved] = recipeChanges(
      [{ label: 'Streaming service', value: 'Netflix in Japan', list: list('Japan') }],
      [{ label: 'Streaming service', value: 'Netflix', list: list('') }],
    )
    expect(factChangeNote(moved!)).toBe('was in Japan')
    const [placed] = recipeChanges(
      [{ label: 'Streaming service', value: 'Netflix', list: list('') }],
      [{ label: 'Streaming service', value: 'Netflix in Japan', list: list('Japan') }],
    )
    expect(factChangeNote(placed!)).toBe('was anywhere')
    expect(factChangeNote({ label: 'Rating', value: '7.0 or more', was: '6.5–9.5', list: null })).toBe('was 6.5–9.5')
    expect(factChangeNote({ label: 'Left-out keyword', value: 'gore', was: '', list: null })).toBe('was Any')
  })
})

const changes: SnapshotChange[] = [
  { op: 'removed', kind: 'catalog', key: 'retro', name: 'Retro', folder_key: 'fa', folder: '80s', genre: 'Horror' },
  { op: 'removed', kind: 'folder', key: 'fb', name: 'Kids' },
  { op: 'removed', kind: 'catalog', key: 'cars', name: 'Cars', folder_key: 'fb', folder: 'Kids' },
  { op: 'added', kind: 'folder', key: 'fc', name: 'Classics' },
  { op: 'added', kind: 'catalog', key: 'heat', name: 'Heat', folder_key: 'fc', folder: 'Classics' },
  { op: 'added', kind: 'catalog', key: 'alien', name: 'Alien', folder_key: 'fa', folder: '80s' },
  { op: 'added', kind: 'catalog', key: 'stray', name: 'Stray' },
  { op: 'changed', kind: 'collection', aspect: 'name', name: 'Weekend', was: 'Friday' },
  { op: 'changed', kind: 'collection', aspect: 'settings' },
  { op: 'changed', kind: 'folder', aspect: 'name', key: 'fa', name: 'Eighties', was: '80s' },
  { op: 'changed', kind: 'folder', aspect: 'art', key: 'fa', name: 'Eighties' },
  { op: 'changed', kind: 'folder', aspect: 'catalog_order', key: 'fa', name: 'Eighties' },
  { op: 'changed', kind: 'folder', aspect: 'order', key: 'fa', name: 'Eighties' },
  {
    op: 'changed',
    kind: 'catalog',
    aspect: 'recipe',
    key: 'alien',
    name: 'Alien II',
    was: 'Alien',
    catalog: { key: 'alien', name: 'Alien II', type: 'movie', provider: 'tmdb', params: { sort_by: 'vote_average.desc' } },
    was_catalog: { key: 'alien', name: 'Alien', type: 'movie', provider: 'tmdb', params: { sort_by: 'popularity.desc' } },
  },
  { op: 'changed', kind: 'catalog', aspect: 'name', key: 'heat', name: 'Heat 2', was: 'Heat' },
  { op: 'changed', kind: 'collection', aspect: 'order' },
]

describe('updateMarks', () => {
  const marks = updateMarks(changes)

  it('places each change on what the new version shows; a catalog leaving with its folder goes with it', () => {
    expect(marks.folders.get('fc')).toEqual({ added: true, was: '', art: false, catalogOrder: false })
    expect(marks.folders.get('fa')).toEqual({ added: false, was: '80s', art: true, catalogOrder: true })
    expect([...marks.added]).toEqual(['fc/heat', 'fa/alien'])
    expect(marks.removed).toEqual(new Map([['fa', ['Retro (Horror)']]]))
    expect(marks.removedFolders).toEqual(['Kids'])
    expect(marks.catalogs.get('alien')?.was).toBe('Alien')
    expect(marks.catalogs.get('alien')?.recipe?.aspect).toBe('recipe')
    expect(marks.catalogs.get('heat')).toEqual({ was: 'Heat', recipe: null })
    expect(marks.collection).toEqual({ was: 'Friday', settings: true, order: true })
  })

  it('words a folder’s mark and a catalog’s', () => {
    expect(folderMarkWords(marks.folders.get('fc'))).toBe('new')
    expect(folderMarkWords(marks.folders.get('fa'))).toBe('renamed · was 80s · new art · catalog order changed')
    expect(folderMarkWords(undefined)).toBe('')
    expect(catalogMarkWords(marks.catalogs.get('alien'), true, true)).toBe('added · renamed · was Alien · filters changed')
    expect(catalogMarkWords(marks.catalogs.get('alien'), false, false)).toBe('renamed · filters changed, see above')
    expect(catalogMarkWords(undefined, false, true)).toBe('')
  })

  it('sums a collection’s update in one line, each part jumping to its mark', () => {
    expect(updateSummary(marks, 'collection')).toEqual([
      { text: 'renamed, was Friday', target: 'update-renamed' },
      { text: '1 new folder', target: 'update-folder-fc' },
      { text: '1 folder renamed', target: 'update-folder-fa' },
      { text: 'new art on 1 folder', target: 'update-folder-fa' },
      { text: 'catalog order changed in 1 folder', target: 'update-folder-fa' },
      { text: '1 folder removed', target: 'update-removed' },
      { text: '1 catalog added', target: 'update-folder-fa' },
      { text: '1 catalog removed', target: 'update-folder-fa' },
      { text: '2 catalogs changed', target: 'update-catalog-alien' },
      { text: 'folder order changed', target: null },
      { text: 'view settings changed', target: null },
    ])
    expect(renamedFrom(marks, 'collection')).toBe('Friday')
  })

  it('sums a catalog publication’s update as its rename and its filters', () => {
    const catalog = updateMarks([changes[13]!])
    expect(updateSummary(catalog, 'catalog')).toEqual([
      { text: 'renamed, was Alien', target: 'update-renamed' },
      { text: 'filters changed', target: 'update-catalog-alien' },
    ])
    expect(updateSummary(updateMarks([changes[14]!]), 'catalog')).toEqual([{ text: 'renamed, was Heat', target: 'update-renamed' }])
    expect(renamedFrom(updateMarks([]), 'catalog')).toBe('')
    expect(updateSummary(updateMarks([]), 'collection')).toEqual([])
  })
})

describe('block marks', () => {
  const marks = updateMarks(changes)

  it('marks a changed catalog in full where the new version first uses it, and in words elsewhere', () => {
    expect(folderEntryMark(marks, 'fa', 'alien', true)).toEqual({
      words: 'added · renamed · was Alien · filters changed',
      recipe: marks.catalogs.get('alien')!.recipe,
      id: 'update-catalog-alien',
      startOpen: true,
    })
    expect(folderEntryMark(marks, 'fc', 'alien', false)).toEqual({
      words: 'renamed · filters changed, see above',
      recipe: null,
      id: undefined,
      startOpen: false,
    })
    expect(folderEntryMark(marks, 'fc', 'heat', true)).toEqual({ words: 'added · renamed · was Heat', recipe: null, id: 'update-catalog-heat', startOpen: false })
    expect(folderEntryMark(null, 'fa', 'alien', true)).toBeUndefined()
  })

  it('marks a catalog publication’s own tiles, with no words', () => {
    expect(pageBlockMark(marks, 'alien')).toEqual({ words: '', recipe: marks.catalogs.get('alien')!.recipe, id: 'update-catalog-alien', startOpen: true })
    expect(pageBlockMark(marks, 'nothing')).toBeUndefined()
    expect(pageBlockMark(null, 'alien')).toBeUndefined()
  })

  it('finds each catalog’s first ref in folder order', () => {
    const folders = [
      { id: 'fa', refs: [{ catalog_id: 'alien' }, { catalog_id: 'retro' }] },
      { id: 'fb', refs: [{ catalog_id: 'retro' }, { catalog_id: 'heat' }] },
      { id: 'fc', refs: null },
    ]
    expect([...firstUses(folders)]).toEqual(['fa/0', 'fa/1', 'fb/1'])
  })
})
