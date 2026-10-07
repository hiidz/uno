/**
 * Turns the Home pane's pending state into the shape of the home *screen*, for
 * the Preview view.
 *
 * Pure: everything here derives from state the pane already holds, so flipping
 * List → Preview needs no endpoint and no fetch.
 *
 * *Home* is one page, two bands: collections pinned to the top, then the home
 * rows, catalogs and collections mixed in the order the user put them. The collection/folder structure
 * itself lives in `features/preview/model.ts`, shared with the collection
 * builder so the two previews can't disagree about what a layout will do; this
 * module only assembles home out of it.
 */

import type { Catalog, CatalogType, Collection } from '@/api'
import type { PreviewCollection, PreviewFolder, PreviewSource } from '@/features/preview/model'
import { bandOf } from './pending'
import type { HomeBand, HomeCatalogEntry, HomeEntry } from './pending'

export interface PreviewRow {
  id: string
  name: string
  type: CatalogType
}

/** One home row: a catalog's, or a collection's. */
export type HomeBandItem =
  | { kind: 'catalog'; row: PreviewRow }
  | { kind: 'collection'; collection: PreviewCollection }

/** Named for the screen, not the component — `HomePreview.tsx` renders this. */
export interface HomeScreenPreview {
  /** Band 1: collections with `pin_to_top`, above everything else on home. */
  pinnedCollections: PreviewCollection[]
  /** Band 2: the home rows down the screen — catalogs with `show_in_home` and
   *  the other collections, mixed, in Home order. */
  home: HomeBandItem[]
  /** The catalogs among `home`, in order: the rows whose tiles Preview fetches. */
  rows: PreviewRow[]
  /** Selected, but `show_in_home: false` — Discover only, no home row. Not a
   *  band: it renders as a labelled group beneath the screen. */
  discoverOnly: PreviewRow[]
  isEmpty: boolean
}

/** Which folder page is open. Held as ids, never indices, so a folder removed
 *  in the List view falls back to home instead of pointing at a different
 *  folder — see `findFolderPage`. */
export interface FolderPageTarget {
  collectionId: string
  folderId: string
}

function toFolder(
  folder: NonNullable<Collection['folders']>[number],
  catalogById: ReadonlyMap<string, Catalog>,
): PreviewFolder {
  // `?? []` is a guard, not a live case: the Go side runs `refs` through
  // `orEmpty`. It stays because `getList` coerces only the top-level response,
  // never nested arrays like this one.
  const sources: PreviewSource[] = (folder.refs ?? []).map(({ catalog_id: id, genre }) => {
    const catalog = catalogById.get(id)
    return {
      // Unique within a saved folder: `folder_catalogs` is
      // `PRIMARY KEY (folder_id, catalog_id, genre)`.
      key: `${id}::${genre}`,
      name: catalog?.name ?? null,
      type: catalog?.type ?? null,
      params: catalog?.params ?? '',
      genre,
    }
  })

  return {
    id: folder.id,
    title: folder.title,
    hideTitle: folder.hide_title,
    tileShape: folder.tile_shape,
    coverEmoji: folder.cover_emoji,
    coverImageUrl: folder.cover_image_url,
    sources,
    unresolved: sources.filter((s) => s.name === null).length,
  }
}

/** A saved collection as a previewable one, its refs resolved against
 *  `catalogById`. `undefined` is a selected id nothing describes. Also what
 *  Community previews a community collection with. */
export function toPreviewCollection(
  id: string,
  collection: Collection | undefined,
  catalogById: ReadonlyMap<string, Catalog>,
): PreviewCollection {
  if (!collection) {
    return {
      id,
      title: 'Unavailable collection',
      pinned: false,
      viewMode: 'ROWS',
      showAllTab: false,
      folders: [],
      missing: true,
    }
  }

  return {
    id,
    title: collection.title,
    pinned: collection.pin_to_top,
    viewMode: collection.view_mode,
    showAllTab: collection.show_all_tab,
    // Same guard as `catalog_ids` above.
    folders: (collection.folders ?? []).map((f) => toFolder(f, catalogById)),
    missing: false,
  }
}

function toRow(entry: HomeCatalogEntry, catalog: Catalog | undefined): PreviewRow {
  return {
    id: entry.id,
    name: catalog?.name ?? 'Unavailable catalog',
    type: catalog?.type ?? 'movie',
  }
}

/**
 * Builds the whole preview in one pass.
 *
 * Ordering rule for home: Show first (`pin_to_top`) lifts a collection row
 * above every other row — Nuvio's own field description is "pin to top of
 * home screen". The pin is the pending one each entry carries, not the one
 * last pushed. Home order is preserved inside each band, so pinning moves a
 * row between bands without discarding the order the user just dragged.
 */
export function buildHomePreview({
  rows,
  catalogById,
  collectionById,
}: {
  rows: HomeEntry[]
  catalogById: ReadonlyMap<string, Catalog>
  collectionById: ReadonlyMap<string, Collection>
}): HomeScreenPreview {
  // Discover-only catalogs still count as content: `isEmpty` is "nothing
  // selected at all", the first-run state the colour bars are for — not "no
  // rows on home", which `HomeScreen` reports separately.
  const preview: HomeScreenPreview = { pinnedCollections: [], home: [], rows: [], discoverOnly: [], isEmpty: rows.length === 0 }
  for (const entry of rows) {
    placeInPreview(preview, bandOf(entry), toBandItem(entry, catalogById, collectionById))
  }
  return preview
}

/** Puts `item` in `preview` by its band: a home row, a pinned collection, or
 *  a Discover-only catalog. */
function placeInPreview(preview: HomeScreenPreview, band: HomeBand, item: HomeBandItem): void {
  if (band === 'home') {
    preview.home.push(item)
    if (item.kind === 'catalog') preview.rows.push(item.row)
  } else if (item.kind === 'collection') {
    preview.pinnedCollections.push(item.collection)
  } else {
    preview.discoverOnly.push(item.row)
  }
}

/** One Home entry as the preview draws it. */
function toBandItem(
  entry: HomeEntry,
  catalogById: ReadonlyMap<string, Catalog>,
  collectionById: ReadonlyMap<string, Collection>,
): HomeBandItem {
  if (entry.kind === 'catalog') return { kind: 'catalog', row: toRow(entry, catalogById.get(entry.id)) }
  const collection = toPreviewCollection(entry.id, collectionById.get(entry.id), catalogById)
  return { kind: 'collection', collection: { ...collection, pinned: entry.pinToTop } }
}

/**
 * Resolves an open folder page, or `null` when the target no longer exists.
 *
 * The target is `{collectionId, folderId}` rather than indices, so a collection
 * removed or a folder deleted in the List view collapses back to home instead
 * of silently rendering a *different* folder now at the same position.
 */
export function findFolderPage(
  preview: HomeScreenPreview,
  target: FolderPageTarget,
): { collection: PreviewCollection; folder: PreviewFolder } | null {
  const collection = [...preview.pinnedCollections, ...homeCollections(preview)].find(
    (c) => c.id === target.collectionId,
  )
  if (!collection) return null

  const folder = collection.folders.find((f) => f.id === target.folderId)
  if (!folder) return null

  return { collection, folder }
}

/** The collections among `preview`'s home rows, in order. */
export function homeCollections(preview: HomeScreenPreview): PreviewCollection[] {
  return preview.home.flatMap(function collectionOf(item) {
    return item.kind === 'collection' ? [item.collection] : []
  })
}

/** A home row's id: its catalog's or its collection's. */
export function bandItemId(item: HomeBandItem): string {
  if (item.kind === 'catalog') return item.row.id
  return item.collection.id
}
