/**
 * Turns the Home pane's pending state into the shape of the home *screen*, for
 * the Preview view.
 *
 * Pure: everything here derives from state the pane already holds, so flipping
 * List → Preview needs no endpoint and no fetch.
 *
 * *Home* is one page, three bands: collections pinned to the top, then the
 * catalog rows, then the remaining collections. The collection/folder structure
 * itself lives in `features/preview/model.ts`, shared with the collection
 * builder so the two previews can't disagree about what a layout will do; this
 * module only assembles home out of it.
 */

import type { Catalog, CatalogType, Collection } from '@/api'
import {
  normalizeTileShape,
  normalizeViewMode,
  type PreviewCollection,
  type PreviewFolder,
  type PreviewSource,
} from '@/features/preview/model'
import type { HomeCatalogEntry, HomeCollectionEntry } from './pending'

export interface PreviewRow {
  id: string
  name: string
  type: CatalogType
  /** Same meaning, and the same warning, as `PreviewCollection.missing`: this
   *  is "nothing describes it", not "no longer in the library". It drives the
   *  placeholder name only. */
  missing: boolean
}

/** Named for the screen, not the component — `HomePreview.tsx` renders this. */
export interface HomeScreenPreview {
  /** Band 1: collections with `pin_to_top`, above everything else on home. */
  pinnedCollections: PreviewCollection[]
  /** Band 2: rows down the home screen — selected catalogs with `show_in_home`. */
  rows: PreviewRow[]
  /** Band 3: the remaining collection rows, in selection order. */
  unpinnedCollections: PreviewCollection[]
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
  const { shape, assumed } = normalizeTileShape(folder.tile_shape)

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
    tileShape: shape,
    tileShapeAssumed: assumed,
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
      viewModeAssumed: true,
      showAllTab: false,
      hasBackdrop: false,
      folders: [],
      missing: true,
    }
  }

  const { mode, assumed } = normalizeViewMode(collection.view_mode)

  return {
    id,
    title: collection.title,
    pinned: collection.pin_to_top,
    viewMode: mode,
    viewModeAssumed: assumed,
    showAllTab: collection.show_all_tab,
    hasBackdrop: collection.backdrop_image_url !== '',
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
    missing: catalog === undefined,
  }
}

/**
 * Builds the whole preview in one pass.
 *
 * Ordering rule for home: Show first (`pin_to_top`) lifts a collection row
 * above the catalog rows entirely — Nuvio's own field description is "pin to
 * top of home screen", so it outranks the catalogs-then-collections default
 * rather than merely sorting within the collections. The pin is the pending
 * one each entry carries, not the one last pushed. Selection order is
 * preserved inside each band, so pinning moves a row between bands without
 * discarding the order the user just dragged.
 */
export function buildHomePreview({
  catalogs,
  collections,
  catalogById,
  collectionById,
}: {
  catalogs: HomeCatalogEntry[]
  collections: HomeCollectionEntry[]
  catalogById: ReadonlyMap<string, Catalog>
  collectionById: ReadonlyMap<string, Collection>
}): HomeScreenPreview {
  const previewCollections = collections.map((entry) => ({
    ...toPreviewCollection(entry.id, collectionById.get(entry.id), catalogById),
    pinned: entry.pinToTop,
  }))

  const rows: PreviewRow[] = []
  const discoverOnly: PreviewRow[] = []
  for (const entry of catalogs) {
    const row = toRow(entry, catalogById.get(entry.id))
    ;(entry.showInHome ? rows : discoverOnly).push(row)
  }

  return {
    pinnedCollections: previewCollections.filter((c) => c.pinned),
    rows,
    unpinnedCollections: previewCollections.filter((c) => !c.pinned),
    discoverOnly,
    // Discover-only catalogs still count as content: this is "nothing selected
    // at all", the first-run state the colour bars are for — not "no rows on
    // home", which `HomeScreen` reports separately.
    isEmpty: catalogs.length === 0 && collections.length === 0,
  }
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
  const collection = [...preview.pinnedCollections, ...preview.unpinnedCollections].find(
    (c) => c.id === target.collectionId,
  )
  if (!collection) return null

  const folder = collection.folders.find((f) => f.id === target.folderId)
  if (!folder) return null

  return { collection, folder }
}
