import { describe, expect, it } from 'vitest'
import type { SnapshotChange } from '@/api'
import { changeSummary, changeWords } from './changeWords'

const genres = { movie: new Map([[27, 'Horror']]), tv: new Map<number, string>() }

const REMOVED_FOLDER: SnapshotChange = { op: 'removed', kind: 'folder', name: '80s' }
const REMOVED_CATALOG: SnapshotChange = { op: 'removed', kind: 'catalog', name: 'Retro', folder: '80s' }
const ADDED_FOLDER: SnapshotChange = { op: 'added', kind: 'folder', name: 'Classics' }
const ADDED_CATALOG: SnapshotChange = { op: 'added', kind: 'catalog', name: 'Heat', folder: 'Classics' }
const CHANGED_RECIPE: SnapshotChange = {
  op: 'changed',
  kind: 'catalog',
  aspect: 'recipe',
  name: 'Horror',
  catalog: {
    key: 'k',
    name: 'Horror',
    type: 'movie',
    provider: 'tmdb',
    params: { with_genres: '27', sort_by: 'vote_average.desc' },
  },
}

describe('changeWords', () => {
  it('words what a folder and a catalog lose and gain, naming the folder', () => {
    expect(changeWords(REMOVED_FOLDER, genres)).toBe('Removed folder “80s”')
    expect(changeWords(REMOVED_CATALOG, genres)).toBe('Removed “Retro” from “80s”')
    expect(changeWords(ADDED_FOLDER, genres)).toBe('Added folder “Classics”')
    expect(changeWords(ADDED_CATALOG, genres)).toBe('Added “Heat” to “Classics”')
  })

  it('names the genre a folder narrows a catalog to, and a catalog without a folder', () => {
    expect(changeWords({ ...REMOVED_CATALOG, genre: 'War' }, genres)).toBe('Removed “Retro” (War) from “80s”')
    expect(changeWords({ op: 'added', kind: 'catalog', name: 'Popular' }, genres)).toBe('Added “Popular”')
    expect(changeWords({ op: 'removed', kind: 'catalog', name: 'Popular' }, genres)).toBe('Removed “Popular”')
  })

  it('shows a changed catalog by its new recipe line, never by what changed in it', () => {
    expect(changeWords(CHANGED_RECIPE, genres)).toBe('“Horror” now: Highest rated · Horror')
    expect(changeWords({ ...CHANGED_RECIPE, was: 'Scary' }, genres)).toBe('“Horror” (was “Scary”) now: Highest rated · Horror')
    expect(changeWords({ ...CHANGED_RECIPE, catalog: { ...CHANGED_RECIPE.catalog!, params: {} } }, genres)).toBe(
      '“Horror” now: no filters',
    )
  })

  it('words a rename, and the order, art and settings as one line each', () => {
    expect(changeWords({ op: 'changed', kind: 'catalog', aspect: 'name', name: 'B', was: 'A' }, genres)).toBe('Renamed “A” to “B”')
    expect(changeWords({ op: 'changed', kind: 'folder', aspect: 'name', name: 'Family', was: 'Kids' }, genres)).toBe(
      'Renamed folder “Kids” to “Family”',
    )
    expect(changeWords({ op: 'changed', kind: 'collection', aspect: 'name', name: 'Weekend' }, genres)).toBe(
      'Renamed the collection to “Weekend”',
    )
    expect(changeWords({ op: 'changed', kind: 'collection', aspect: 'order' }, genres)).toBe('Folder order changed')
    expect(changeWords({ op: 'changed', kind: 'collection', aspect: 'settings' }, genres)).toBe('Collection settings changed')
    expect(changeWords({ op: 'changed', kind: 'folder', aspect: 'art', name: 'Kids' }, genres)).toBe('Art changed on “Kids”')
    expect(changeWords({ op: 'changed', kind: 'folder', aspect: 'catalog_order', name: 'Kids' }, genres)).toBe(
      'Catalog order changed in “Kids”',
    )
  })

  it('says nothing for an item it has no words for', () => {
    expect(changeWords({ op: 'changed', kind: 'folder' }, genres)).toBe('')
  })
})

describe('changeSummary', () => {
  it('counts what a list holds, removals first, as the list orders them', () => {
    expect(changeSummary([REMOVED_FOLDER, REMOVED_CATALOG, ADDED_FOLDER, ADDED_CATALOG, ADDED_CATALOG, CHANGED_RECIPE])).toBe(
      '1 folder removed · 1 catalog removed · 1 folder added · 2 catalogs added · 1 catalog changed',
    )
  })

  it('counts every other change together, and alone says change', () => {
    const order: SnapshotChange = { op: 'changed', kind: 'collection', aspect: 'order' }
    const art: SnapshotChange = { op: 'changed', kind: 'folder', aspect: 'art', name: 'Kids' }
    expect(changeSummary([ADDED_CATALOG, order, art])).toBe('1 catalog added · 2 other changes')
    expect(changeSummary([order])).toBe('1 change')
    expect(changeSummary([order, art])).toBe('2 changes')
  })

  it('is empty for no changes', () => {
    expect(changeSummary([])).toBe('')
  })
})
