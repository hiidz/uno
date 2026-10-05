import { describe, expect, it } from 'vitest'
import type { Catalog, Collection } from '@/api'
import { catalog, collection, folder } from '@/test/fixtures'
import { bandItemId, buildHomePreview, findFolderPage, homeCollections, toPreviewCollection } from './preview'
import type { HomeEntry } from './pending'

const catalogById = new Map<string, Catalog>([
  ['a', catalog({ id: 'a', name: 'Alpha' })],
  ['b', catalog({ id: 'b', name: 'Bravo', type: 'series' })],
])

const collectionById = new Map<string, Collection>([
  ['p1', collection({ id: 'p1', pin_to_top: true })],
  ['p2', collection({ id: 'p2', pin_to_top: true })],
  ['u1', collection({ id: 'u1', folders: [folder({ id: 'f1' })] })],
])

function preview(...rows: HomeEntry[]) {
  return buildHomePreview({ rows, catalogById, collectionById })
}

const pinned = (id: string): HomeEntry => ({ kind: 'collection', id, pinToTop: true })
const unpinned = (id: string): HomeEntry => ({ kind: 'collection', id, pinToTop: false })
const shown = (id: string): HomeEntry => ({ kind: 'catalog', id, showInHome: true })
const discover = (id: string): HomeEntry => ({ kind: 'catalog', id, showInHome: false })

describe('buildHomePreview', () => {
  it('puts pinned collections above the home rows, catalogs and collections mixed in Home order', () => {
    const screen = preview(pinned('p2'), unpinned('u1'), shown('a'), discover('b'), pinned('p1'))
    expect(screen.pinnedCollections.map((c) => c.id)).toEqual(['p2', 'p1'])
    expect(screen.home.map(bandItemId)).toEqual(['u1', 'a'])
    expect(screen.rows.map((r) => r.id)).toEqual(['a'])
    expect(homeCollections(screen).map((c) => c.id)).toEqual(['u1'])
    expect(screen.discoverOnly.map((r) => r.id)).toEqual(['b'])
  })

  it('draws the pending Show first, not the one last pushed', () => {
    const screen = preview(unpinned('p1'), pinned('u1'))
    expect(screen.pinnedCollections.map((c) => c.id)).toEqual(['u1'])
    expect(homeCollections(screen).map((c) => c.id)).toEqual(['p1'])
  })

  it('is empty only with nothing selected, a Discover-only catalog counting as content', () => {
    expect(preview().isEmpty).toBe(true)
    expect(preview(discover('b')).isEmpty).toBe(false)
  })

  it('names a row nothing describes as unavailable', () => {
    expect(preview(shown('gone')).rows).toEqual([{ id: 'gone', name: 'Unavailable catalog', type: 'movie', missing: true }])
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
  const screen = preview(pinned('p1'), unpinned('u1'))

  it('finds an open folder by its ids', () => {
    expect(findFolderPage(screen, { collectionId: 'u1', folderId: 'f1' })?.folder.id).toBe('f1')
  })

  it('falls back to home once the folder or its collection is gone', () => {
    expect(findFolderPage(screen, { collectionId: 'u1', folderId: 'deleted' })).toBeNull()
    expect(findFolderPage(screen, { collectionId: 'removed', folderId: 'f1' })).toBeNull()
  })
})
