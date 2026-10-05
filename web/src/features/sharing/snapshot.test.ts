import { describe, expect, it } from 'vitest'
import type { PublicationDetail } from '@/api'
import { catalog, communityItem } from '@/test/fixtures'
import { catalogItem } from './listing'
import { kindSticker } from './sharingState'
import { asCatalog, snapshotAsCollection, snapshotCatalog, snapshotRecipeLine } from './snapshot'

const genres = { movie: new Map([[27, 'Horror']]), tv: new Map([[10765, 'Sci-Fi & Fantasy']]) }

function detail(snapshot: PublicationDetail['snapshot']): PublicationDetail {
  return { ...communityItem({ id: 'pub', kind: 'collection', catalog: null }), unpublished: false, snapshot }
}

describe('asCatalog', () => {
  it('reads a snapshot catalog as a catalog keyed by its key', () => {
    expect(asCatalog({ key: 'k', name: 'N', type: 'series', provider: 'tmdb', params: { with_genres: '10765' } })).toMatchObject({
      id: 'k',
      name: 'N',
      type: 'series',
      params: '{"with_genres":"10765"}',
      collection_id: null,
      publication: null,
      subscription: null,
    })
  })

  it('describes a recipe with the lookup for its type', () => {
    expect(snapshotRecipeLine({ key: 'k', name: 'N', type: 'movie', provider: 'tmdb', params: { with_genres: '27' } }, genres)).toBe('Horror')
    expect(snapshotRecipeLine({ key: 'k', name: 'N', type: 'series', provider: 'tmdb', params: { with_genres: '10765' } }, genres)).toBe(
      'Sci-Fi & Fantasy',
    )
  })
})

describe('snapshotAsCollection', () => {
  it('is null for a catalog publication', () => {
    expect(snapshotAsCollection(detail({ format: 'uno-publication', version: 1, catalogs: [] }))).toBeNull()
  })

  it('reads a collection snapshot as a collection, refs by key', () => {
    const result = snapshotAsCollection(
      detail({
        format: 'uno-publication',
        version: 1,
        catalogs: [{ key: 'c1', name: 'Scares', type: 'movie', provider: 'tmdb', params: {} }],
        collection: {
          title: 'Night',
          view_mode: 'ROWS',
          show_all_tab: false,
          backdrop_image_url: '',
          focus_glow_enabled: true,
          folders: [
            {
              key: 'f1',
              title: 'Folder',
              tile_shape: 'POSTER',
              hide_title: false,
              cover_emoji: '🔪',
              cover_image_url: '',
              focus_gif_url: '',
              focus_gif_enabled: false,
              hero_backdrop_url: '',
              hero_video_url: '',
              title_logo_url: '',
              refs: [{ catalog: 'c1', genre: 'Horror' }],
            },
            { ...emptyFolder(), key: 'f2', refs: null },
          ],
        },
      }),
    )
    expect(result).toMatchObject({
      id: 'pub',
      title: 'Night',
      view_mode: 'ROWS',
      folders: [
        { id: 'f1', collection_id: 'pub', sort_order: 0, cover_emoji: '🔪', refs: [{ catalog_id: 'c1', genre: 'Horror' }] },
        { id: 'f2', sort_order: 1, refs: [] },
      ],
      catalogs: [{ id: 'c1', name: 'Scares' }],
    })
  })

  it('reads a snapshot with null lists', () => {
    const result = snapshotAsCollection(
      detail({
        format: 'uno-publication',
        version: 1,
        catalogs: null,
        collection: { title: 'T', view_mode: '', show_all_tab: false, backdrop_image_url: '', focus_glow_enabled: false, folders: null },
      }),
    )
    expect(result).toMatchObject({ folders: [], catalogs: [] })
  })
})

describe('listing', () => {
  const horror = catalog({ id: 'h', name: 'Horror', params: '{"with_genres":"27"}' })
  const scifi = catalog({ id: 's', name: 'Sci-fi', type: 'series', params: '{"with_genres":"10765"}' })

  it('lists a catalog by name over its recipe line', () => {
    const kind = kindSticker('movie')
    expect(catalogItem(horror, genres, kind)).toEqual({ key: 'h', name: 'Horror', line: 'Horror', sticker: kind })
    expect(catalogItem(scifi, genres)).toMatchObject({ line: 'Sci-Fi & Fantasy' })
  })
})

function emptyFolder() {
  return {
    key: '',
    title: '',
    tile_shape: '' as const,
    hide_title: false,
    cover_emoji: '',
    cover_image_url: '',
    focus_gif_url: '',
    focus_gif_enabled: false,
    hero_backdrop_url: '',
    hero_video_url: '',
    title_logo_url: '',
  }
}

describe('snapshotCatalog', () => {
  it('reads a catalog publication’s one catalog, or none', () => {
    expect(snapshotCatalog(detail({ format: 'uno-publication', version: 1, catalogs: [{ key: 'k', name: 'N', type: 'movie', provider: 'tmdb', params: {} }] }))).toMatchObject({ id: 'k' })
    expect(snapshotCatalog(detail({ format: 'uno-publication', version: 1, catalogs: null }))).toBeUndefined()
    expect(snapshotCatalog(detail({ format: 'uno-publication', version: 1, catalogs: [] }))).toBeUndefined()
  })
})
