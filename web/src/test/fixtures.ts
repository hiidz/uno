import type { Catalog, Collection, Folder, SelectedCatalog } from '@/api'

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
    owner_id: '',
    is_public: false,
    collection_id: null,
    created_at: '',
    updated_at: '',
    linked: false,
    ...overrides,
  }
}

export function selectedCatalog(overrides: Partial<SelectedCatalog> = {}): SelectedCatalog {
  return { ...catalog(overrides), show_in_home: true, ...overrides }
}

export function folder(overrides: Partial<Folder> = {}): Folder {
  return {
    id: 'f1',
    collection_id: 'col1',
    title: 'Folder',
    sort_order: 0,
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
    owner_id: '',
    is_public: false,
    pin_to_top: false,
    view_mode: 'ROWS',
    show_all_tab: false,
    backdrop_image_url: '',
    focus_glow_enabled: true,
    created_at: '',
    updated_at: '',
    version: 1,
    pushed_version: null,
    linked: false,
    folders: [],
    catalogs: [],
    ...overrides,
  }
}
