import type { Catalog, Collection, CommunityFolder, CommunityItem, Folder } from '@/api'

/**
 * Complete wire rows for tests, so a test names only the fields it is about
 * and a field added to the type fails here, once, rather than hiding behind a
 * partial cast in every test that builds one.
 */

export function catalog(overrides: Partial<Catalog> = {}): Catalog {
  return {
    id: 'c1',
    type: 'movie',
    name: 'Popular',
    provider: 'tmdb',
    params: '{}',
    collection_id: null,
    created_at: '',
    updated_at: '',
    show_in_home: false,
    publication: null,
    subscription: null,
    publisher_unpublished: false,
    ...overrides,
  }
}

export function folder(overrides: Partial<Folder> = {}): Folder {
  return {
    id: 'f1',
    title: 'Folder',
    tile_shape: 'POSTER',
    hide_title: false,
    cover_emoji: '',
    cover_image_url: '',
    focus_gif_url: '',
    focus_gif_enabled: true,
    hero_backdrop_url: '',
    hero_video_url: '',
    title_logo_url: '',
    refs: [],
    ...overrides,
  }
}

export function collection(overrides: Partial<Collection> = {}): Collection {
  return {
    id: 'col1',
    title: 'Collection',
    pin_to_top: false,
    view_mode: 'ROWS',
    show_all_tab: false,
    backdrop_image_url: '',
    focus_glow_enabled: true,
    created_at: '',
    updated_at: '',
    publication: null,
    subscription: null,
    publisher_unpublished: false,
    folders: [],
    catalogs: [],
    ...overrides,
  }
}

export function communityItem(overrides: Partial<CommunityItem> = {}): CommunityItem {
  return {
    id: 'p1',
    kind: 'catalog',
    title: 'Popular',
    catalog_count: 1,
    folder_count: 0,
    subscriber_count: 0,
    published_at: '2026-09-20T10:00:00Z',
    updated_at: '2026-09-20T10:00:00Z',
    subscribed: false,
    update_available: false,
    catalog_names: ['Popular'],
    folders: [],
    catalog: { key: 'k1', name: 'Popular', type: 'movie', provider: 'tmdb', params: {} },
    ...overrides,
  }
}

/** A listed collection's folder: a poster tile with no cover. */
export function communityFolder(title: string, overrides: Partial<CommunityFolder> = {}): CommunityFolder {
  return { title, tile_shape: 'POSTER', cover_emoji: '', cover_image_url: '', ...overrides }
}
