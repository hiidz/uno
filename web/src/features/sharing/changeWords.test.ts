import { describe, expect, it } from 'vitest'
import type { SnapshotCatalog, SnapshotChange } from '@/api'
import { changeCount, groupChanges, takeLines } from './changeWords'

const genres = { movie: new Map([[27, 'Horror']]), tv: new Map<number, string>() }

const REMOVED_FOLDER: SnapshotChange = { op: 'removed', kind: 'folder', name: 'Kids' }
const REMOVED_CATALOG: SnapshotChange = { op: 'removed', kind: 'catalog', name: 'Gore classics', folder: '80s' }
const ADDED_FOLDER: SnapshotChange = { op: 'added', kind: 'folder', name: 'Cult' }
const ADDED_CATALOG: SnapshotChange = { op: 'added', kind: 'catalog', name: 'Predator picks', folder: 'Streaming' }

function catalogWith(params: Record<string, unknown>): SnapshotCatalog {
  return { key: 'k', name: 'Seed Ghibli', type: 'movie', provider: 'tmdb', params }
}

function recipeChange(
  was: Record<string, unknown>,
  now: Record<string, unknown>,
  extra: Partial<SnapshotChange> = {},
): SnapshotChange {
  return {
    op: 'changed',
    kind: 'catalog',
    aspect: 'recipe',
    name: 'Seed Ghibli',
    catalog: catalogWith(now),
    was_catalog: catalogWith(was),
    ...extra,
  }
}

const lines = (changes: SnapshotChange[]) =>
  groupChanges(changes, genres).map((group) => [group.label, group.lines.map((line) => line.text)])

describe('groupChanges', () => {
  it('groups lines under Removed, Added and Changed in the server’s order', () => {
    expect(
      lines([
        REMOVED_FOLDER,
        REMOVED_CATALOG,
        ADDED_FOLDER,
        ADDED_CATALOG,
        { op: 'changed', kind: 'collection', aspect: 'order' },
      ]),
    ).toEqual([
      ['Removed', ['Folder “Kids”', '“Gore classics” from “80s”']],
      ['Added', ['Folder “Cult”', '“Predator picks” to “Streaming”']],
      ['Changed', ['Folder order']],
    ])
  })

  it('folds the catalogs of a folder removed or added whole into its line', () => {
    const inKids = { ...REMOVED_CATALOG, folder: 'Kids' }
    const inCult: SnapshotChange[] = ['a', 'b', 'c'].map((name) => ({ op: 'added', kind: 'catalog', name, folder: 'Cult' }))
    expect(lines([REMOVED_FOLDER, inKids, { ...inKids, name: 'Other' }, ADDED_FOLDER, ...inCult, ADDED_CATALOG])).toEqual([
      ['Removed', ['Folder “Kids” · 2 catalogs']],
      ['Added', ['Folder “Cult” · 3 catalogs', '“Predator picks” to “Streaming”']],
    ])
    expect(lines([REMOVED_FOLDER, { ...inKids, name: 'One' }])[0][1]).toEqual(['Folder “Kids” · 1 catalog'])
  })

  it('names the genre a folder narrows a catalog to, and a catalog with no folder', () => {
    expect(lines([{ ...REMOVED_CATALOG, genre: 'War' }, { op: 'added', kind: 'catalog', name: 'Popular' }])).toEqual([
      ['Removed', ['“Gore classics” (War) from “80s”']],
      ['Added', ['“Popular”']],
    ])
  })

  it('lists what differs in a changed recipe, Production companies and Production company as one fact', () => {
    const groups = groupChanges(
      [
        recipeChange(
          { sort_by: 'popularity.desc', with_companies: '1,2', with_genres: '27' },
          { sort_by: 'vote_average.desc', with_companies: '1', with_genres: '27', with_keywords: '5' },
          { was: 'Ghibli' },
        ),
      ],
      genres,
    )
    expect(groups).toEqual([
      {
        label: 'Changed',
        lines: [
          {
            text: '“Seed Ghibli”',
            notes: ['Name: “Ghibli” → “Seed Ghibli”', 'Production companies: 2 → 1', 'Keyword added', 'Order: Most popular → Highest rated'],
          },
        ],
      },
    ])
  })

  it('says a fact that came or went as "any", and falls back when it can tell nothing', () => {
    const notes = (change: SnapshotChange) => groupChanges([change], genres)[0].lines[0].notes
    expect(notes(recipeChange({}, { vote_average_gte: 7 }))).toEqual(['Rating: any → 7.0 or more'])
    expect(notes(recipeChange({ vote_average_gte: 7 }, {}))).toEqual(['Rating: 7.0 or more → any'])
    expect(notes(recipeChange({ with_keywords: '5' }, { with_keywords: '6' }))).toEqual(['Filters changed'])
    expect(notes({ ...recipeChange({}, {}), was_catalog: undefined })).toEqual(['Filters changed'])
  })

  it('words a rename, and the collection, folder and catalog changes, one line each', () => {
    expect(
      lines([
        { op: 'changed', kind: 'collection', aspect: 'name', name: 'Weekend', was: 'Night' },
        { op: 'changed', kind: 'collection', aspect: 'settings' },
        { op: 'changed', kind: 'folder', aspect: 'name', name: 'Family', was: 'Kids' },
        { op: 'changed', kind: 'folder', aspect: 'art', name: 'Kids' },
        { op: 'changed', kind: 'folder', aspect: 'catalog_order', name: 'Kids' },
        { op: 'changed', kind: 'catalog', aspect: 'name', name: 'B', was: 'A' },
      ]),
    ).toEqual([
      [
        'Changed',
        [
          'Collection name: “Night” → “Weekend”',
          'Collection settings',
          'Folder name: “Kids” → “Family”',
          'Art on “Kids”',
          'Catalog order in “Kids”',
          '“B”',
        ],
      ],
    ])
    const rename: SnapshotChange = { op: 'changed', kind: 'catalog', aspect: 'name', name: 'B', was: 'A' }
    expect(groupChanges([rename], genres)[0].lines[0].notes).toEqual(['Name: “A” → “B”'])
  })

  it('leaves out an item it has no words for', () => {
    expect(groupChanges([{ op: 'changed', kind: 'folder' }], genres)).toEqual([])
  })
})

describe('changeCount', () => {
  it('counts the lines, with a folder’s catalogs inside its line', () => {
    expect(changeCount([REMOVED_FOLDER, { ...REMOVED_CATALOG, folder: 'Kids' }, ADDED_CATALOG])).toBe(2)
    expect(changeCount([])).toBe(0)
  })
})

describe('takeLines', () => {
  const groups = groupChanges([REMOVED_FOLDER, REMOVED_CATALOG, ADDED_FOLDER, ADDED_CATALOG], genres)

  it('keeps the first lines across groups and counts the rest', () => {
    const { shown, hidden } = takeLines(groups, 3)
    expect(shown.map((group) => group.lines.length)).toEqual([2, 1])
    expect(hidden).toBe(1)
  })

  it('shows everything for no limit', () => {
    expect(takeLines(groups, Infinity)).toEqual({ shown: groups, hidden: 0 })
  })
})
