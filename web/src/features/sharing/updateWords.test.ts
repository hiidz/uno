import { describe, expect, it } from 'vitest'
import type { SnapshotChange, SnapshotFolder } from '@/api'
import { groupByFolder, recipeChanges, takeRows, type UpdateSection } from './updateWords'

function folder(title: string, catalogs: string[] = []): SnapshotFolder {
  return { key: title, title, refs: catalogs.map((catalog) => ({ catalog, genre: '' })) } as unknown as SnapshotFolder
}

const recipe = (key: string) => ({ key, name: key, type: 'movie' as const, provider: 'tmdb', params: {} })

const update: SnapshotChange[] = [
  { op: 'removed', kind: 'catalog', name: 'Gore classics', folder: '80s' },
  { op: 'removed', kind: 'folder', name: 'Kids' },
  { op: 'removed', kind: 'catalog', name: 'Cars night', folder: 'Kids' },
  { op: 'added', kind: 'folder', name: 'Cult' },
  { op: 'added', kind: 'catalog', name: 'Heat vibes', folder: 'Cult' },
  { op: 'added', kind: 'catalog', name: 'Ronin cuts', folder: 'Cult' },
  { op: 'changed', kind: 'folder', aspect: 'name', name: 'Eighties', was: '80s' },
  { op: 'changed', kind: 'catalog', aspect: 'recipe', name: 'Retro slashers', catalog: recipe('rs'), was_catalog: recipe('rs') },
]

const newVersion = [folder('Eighties', ['rs']), folder('Streaming'), folder('Cult')]

describe('groupByFolder', () => {
  it('sets each change under its folder as the new version names it, in the new version’s order', () => {
    const sections = groupByFolder(update, newVersion)
    expect(sections.map((s) => s.folder)).toEqual(['Eighties', 'Cult', 'Kids'])
    expect(sections[0]).toMatchObject({
      notes: ['renamed · was 80s'],
      rows: [
        { name: 'Gore classics', status: 'removed' },
        { name: 'Retro slashers', status: 'changed' },
      ],
    })
  })

  it('counts a whole folder’s catalogs on its own line instead of listing them', () => {
    const sections = groupByFolder(update, newVersion)
    expect(sections.find((s) => s.folder === 'Cult')).toEqual({ folder: 'Cult', notes: ['new folder · 2 catalogs'], rows: [] })
    expect(sections.find((s) => s.folder === 'Kids')).toEqual({ folder: 'Kids', notes: ['folder removed · 1 catalog'], rows: [] })
  })

  it('puts the collection’s own changes first, under no folder', () => {
    const sections = groupByFolder(
      [
        { op: 'changed', kind: 'folder', aspect: 'art', name: 'Cult' },
        { op: 'changed', kind: 'collection', aspect: 'name', name: 'Late night', was: 'Night shift' },
        { op: 'changed', kind: 'collection', aspect: 'order' },
      ],
      newVersion,
    )
    expect(sections[0]).toEqual({
      folder: '',
      notes: [],
      rows: [
        expect.objectContaining({ name: 'Late night', status: 'renamed · was Night shift' }),
        expect.objectContaining({ name: 'Folder order', status: 'changed' }),
      ],
    })
    expect(sections[1]).toMatchObject({ folder: 'Cult', notes: ['new art'] })
  })

  it('reads a catalog published on its own under no folder, and a renamed catalog with its old name', () => {
    const sections = groupByFolder([{ op: 'changed', kind: 'catalog', aspect: 'name', name: 'B', was: 'A' }])
    expect(sections).toEqual([{ folder: '', notes: [], rows: [expect.objectContaining({ name: 'B', status: 'renamed · was A' })] }])
  })

  it('names the genre a folder narrows a catalog to', () => {
    const [section] = groupByFolder([{ op: 'added', kind: 'catalog', name: 'Heat', folder: 'Eighties', genre: 'Crime' }], newVersion)
    expect(section.rows[0].name).toBe('Heat (Crime)')
  })
})

describe('takeRows', () => {
  const sections: UpdateSection[] = groupByFolder(update, newVersion)

  it('keeps the first lines in their folders and counts the rest', () => {
    const { shown, hidden } = takeRows(sections, 2)
    expect(shown).toEqual([{ ...sections[0], rows: [sections[0].rows[0]] }])
    expect(hidden).toBe(3)
  })

  it('keeps everything under the limit', () => {
    expect(takeRows(sections, Infinity)).toEqual({ shown: sections, hidden: 0 })
  })
})

describe('recipeChanges', () => {
  it('lists each filter whose value differs, against its open value when it comes or goes', () => {
    const was = [
      { label: 'Type', value: 'Movies' },
      { label: 'Released', value: '1980–1989' },
      { label: 'Production company', value: 'Any' },
      { label: 'Order', value: 'Most popular' },
    ]
    const now = [
      { label: 'Type', value: 'Movies' },
      { label: 'Released', value: '1975–1989' },
      { label: 'Production companies', value: 'Studio Ghibli or Pixar' },
      { label: 'Order', value: 'Highest rated' },
    ]
    expect(recipeChanges(was, now)).toEqual([
      { label: 'Released', value: '1975–1989', was: '1980–1989' },
      { label: 'Production companies', value: 'Studio Ghibli or Pixar', was: 'Any' },
      { label: 'Order', value: 'Highest rated', was: 'Most popular' },
    ])
  })
})
