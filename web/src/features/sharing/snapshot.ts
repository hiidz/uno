import type { Catalog, Collection, PublicationDetail, SnapshotCatalog, SnapshotFolder } from '@/api'
import { recipeLine } from '@/features/library/recipe'
import type { GenreLookups } from '@/features/library/useLibrary'

/** A snapshot catalog in the shape the rest of the SPA reads a catalog in:
 *  its `key` as its id, and its params as the JSON string a `Catalog`
 *  carries. It is never written anywhere. */
export function asCatalog(catalog: SnapshotCatalog): Catalog {
  return {
    id: catalog.key,
    type: catalog.type,
    name: catalog.name,
    provider: catalog.provider,
    params: JSON.stringify(catalog.params),
    collection_id: null,
    created_at: '',
    updated_at: '',
    show_in_home: false,
    publication: null,
    subscription: null,
    publisher_unpublished: false,
  }
}

/** A snapshot catalog's recipe as the one line the rail shows. */
export function snapshotRecipeLine(catalog: SnapshotCatalog, genres: GenreLookups): string {
  return recipeLine(asCatalog(catalog), catalog.type === 'movie' ? genres.movie : genres.tv)
}

/** A catalog publication's one catalog, as a catalog; undefined for a
 *  snapshot that holds none. */
export function snapshotCatalog(detail: PublicationDetail): Catalog | undefined {
  const catalog = detail.snapshot.catalogs?.[0]
  return catalog && asCatalog(catalog)
}

/** A collection publication's snapshot as a `Collection` with its catalogs,
 *  keyed by snapshot key, so the preview and the folder list read it the way
 *  they read a saved collection. `null` for a catalog publication. */
export function snapshotAsCollection(detail: PublicationDetail): Collection | null {
  const snapshot = detail.snapshot.collection
  if (!snapshot) return null
  return {
    id: detail.id,
    title: snapshot.title,
    pin_to_top: false,
    view_mode: snapshot.view_mode,
    show_all_tab: snapshot.show_all_tab,
    backdrop_image_url: snapshot.backdrop_image_url,
    focus_glow_enabled: snapshot.focus_glow_enabled,
    created_at: detail.published_at,
    updated_at: detail.updated_at,
    publication: null,
    subscription: null,
    publisher_unpublished: false,
    folders: (snapshot.folders ?? []).map((folder) => asFolder(folder)),
    catalogs: (detail.snapshot.catalogs ?? []).map(asCatalog),
  }
}

function asFolder(folder: SnapshotFolder): NonNullable<Collection['folders']>[number] {
  return {
    id: folder.key,
    title: folder.title,
    tile_shape: folder.tile_shape,
    hide_title: folder.hide_title,
    cover_emoji: folder.cover_emoji,
    cover_image_url: folder.cover_image_url,
    focus_gif_url: folder.focus_gif_url,
    focus_gif_enabled: folder.focus_gif_enabled,
    hero_backdrop_url: folder.hero_backdrop_url,
    hero_video_url: folder.hero_video_url,
    title_logo_url: folder.title_logo_url,
    refs: (folder.refs ?? []).map((ref) => ({ catalog_id: ref.catalog, genre: ref.genre })),
  }
}
