import { describe, expect, it } from 'vitest'
import type { Catalog, Collection } from '@/api'
import { catalog, collection, folder } from '@/test/fixtures'
import { buildHomePreview, findFolderPage, toPreviewCollection } from './preview'

const catalogById = new Map<string, Catalog>([
  ['a', catalog({ id: 'a', name: 'Alpha' })],
  ['b', catalog({ id: 'b', name: 'Bravo', type: 'series' })],
])

const collectionById = new Map<string, Collection>([
  ['p1', collection({ id: 'p1', pin_to_top: true })],
  ['p2', collection({ id: 'p2', pin_to_top: true })],
  ['u1', collection({ id: 'u1', folders: [folder({ id: 'f1' })] })],
])

function preview(catalogs: { id: string; showInHome: boolean }[], collections: string[]) {
  return buildHomePreview({ catalogs, collections, catalogById, collectionById })
}

describe('buildHomePreview', () => {
  it('puts pinned collections above the catalog rows and the rest below, each in selection order', () => {
    const screen = preview(
      [
        { id: 'a', showInHome: true },
        { id: 'b', showInHome: false },
      ],
      ['p2', 'u1', 'p1'],
    )
    expect(screen.pinnedCollections.map((c) => c.id)).toEqual(['p2', 'p1'])
    expect(screen.rows.map((r) => r.id)).toEqual(['a'])
    expect(screen.unpinnedCollections.map((c) => c.id)).toEqual(['u1'])
    expect(screen.discoverOnly.map((r) => r.id)).toEqual(['b'])
  })

  it('is empty only with nothing selected, a Discover-only catalog counting as content', () => {
    expect(preview([], []).isEmpty).toBe(true)
    expect(preview([{ id: 'b', showInHome: false }], []).isEmpty).toBe(false)
  })

  it('names a row nothing describes as unavailable', () => {
    expect(preview([{ id: 'gone', showInHome: true }], []).rows).toEqual([
      { id: 'gone', name: 'Unavailable catalog', type: 'movie', missing: true },
    ])
  })
})

describe('toPreviewCollection', () => {
  it('draws a collection nothing describes as a missing, empty row', () => {
    expect(toPreviewCollection('gone', undefined, catalogById)).toMatchObject({
      id: 'gone',
      title: 'Unavailable collection',
      folders: [],
      missing: true,
    })
  })

  it('resolves each folder ref against the catalogs, counting the ones it cannot', () => {
    const saved = collection({
      folders: [
        folder({
          tile_shape: '',
          refs: [
            { catalog_id: 'a', genre: '' },
            { catalog_id: 'a', genre: 'Western' },
            { catalog_id: 'gone', genre: '' },
          ],
        }),
      ],
    })
    const [only] = toPreviewCollection('col1', saved, catalogById).folders
    expect(only.sources.map((s) => [s.key, s.name])).toEqual([
      ['a::', 'Alpha'],
      ['a::Western', 'Alpha'],
      ['gone::', null],
    ])
    expect(only.unresolved).toBe(1)
    expect(only).toMatchObject({ tileShape: 'POSTER', tileShapeAssumed: true })
  })

  it('reads the view mode as Nuvio does', () => {
    expect(toPreviewCollection('c', collection({ view_mode: '' }), catalogById)).toMatchObject({
      viewMode: 'TABBED_GRID',
      viewModeAssumed: false,
    })
    expect(toPreviewCollection('c', collection({ view_mode: 'FOLLOW_LAYOUT' }), catalogById)).toMatchObject({
      viewMode: 'FOLLOW_LAYOUT',
      viewModeAssumed: true,
    })
  })
})

describe('findFolderPage', () => {
  const screen = preview([], ['p1', 'u1'])

  it('finds an open folder by its ids', () => {
    expect(findFolderPage(screen, { collectionId: 'u1', folderId: 'f1' })?.folder.id).toBe('f1')
  })

  it('falls back to home once the folder or its collection is gone', () => {
    expect(findFolderPage(screen, { collectionId: 'u1', folderId: 'deleted' })).toBeNull()
    expect(findFolderPage(screen, { collectionId: 'removed', folderId: 'f1' })).toBeNull()
  })
})
