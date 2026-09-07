import { sendJSON } from './http'
import type { Catalog, CatalogType, Collection, TileShape } from './types'

/**
 * The create/update payload — `vault.CatalogForm`.
 *
 * `provider` is required by the backend but never shown to the user: it is
 * always `"tmdb"`, the only provider with a params shape or any validation, and
 * the backend rejects anything else.
 *
 * There is no `endpoint` field: the backend derives the TMDB path from `type`
 * server-side and never reads a client-supplied one, which is what keeps the
 * addon's outbound requests from being pointed anywhere by whoever wrote the
 * row.
 *
 * `params` is a JSON-encoded *string*, not a nested object.
 */
export interface CatalogPayload {
  type: CatalogType
  name: string
  provider: string
  params: string
  is_public: boolean
}

export const CATALOG_PROVIDER = 'tmdb'

export function createCatalog(profileIndex: number, body: CatalogPayload): Promise<Catalog> {
  return sendJSON<Catalog>('POST', `/api/p/${profileIndex}/catalogs`, body)
}

/**
 * `type` and `provider` are immutable once a catalog exists — but that is a
 * **UI-enforced rule, not a backend one**: `PUT` still accepts and writes
 * both. The edit form renders `type` as read-only and re-sends the
 * catalog's existing values, so the rule holds as long as every caller goes
 * through the form. Duplicate is the supported way to "change a catalog's type".
 */
export function updateCatalog(
  profileIndex: number,
  catalogID: string,
  body: CatalogPayload,
): Promise<Catalog> {
  return sendJSON<Catalog>('PUT', `/api/p/${profileIndex}/catalogs/${catalogID}`, body)
}

/** Hard delete. Removes the row for **every profile using it**, not just this
 *  one — the confirmation copy has to say so. */
export function deleteCatalog(profileIndex: number, catalogID: string): Promise<null> {
  return sendJSON<null>('DELETE', `/api/p/${profileIndex}/catalogs/${catalogID}`)
}

/**
 * One folder inside a collection payload — `vault.FolderData`.
 *
 * `id` is the whole upsert protocol in one field: absent means "insert a new
 * folder", present means "update this existing one". A folder that exists
 * server-side but is *omitted from the array* is **deleted**, cascading to its
 * catalog refs. There is no soft-delete and no per-folder endpoint; the array
 * is the truth.
 *
 * `catalog_ids` is ordered — its index becomes `folder_catalogs.sort_order`.
 * It must not repeat an id *within one folder*: `folder_catalogs` is
 * `PRIMARY KEY (folder_id, catalog_id)`, so a repeat is a constraint violation
 * (a 500, not a 400). The same catalog in two *different* folders is fine and
 * is a supported thing to want.
 */
export interface FolderPayload {
  id?: string
  title: string
  tile_shape: TileShape | ''
  hide_title: boolean
  cover_emoji: string
  cover_image_url: string
  catalog_ids: string[]
}

/**
 * The create/update payload — `vault.CollectionForm`.
 *
 * The whole tree in one request: `POST`/`PUT` replace the collection, its
 * folders, and every folder's catalog refs in a single transaction, so there is
 * one dirty state and one save button rather than a save per folder.
 *
 * Cosmetic Nuvio fields (`focus_glow_enabled` on the collection;
 * `focus_gif_url`, `hero_video_url`, `title_logo_url` and friends on folders)
 * are unmodelled — there is no editor for them. They survive an update anyway,
 * because the `UPDATE` statements name only the modelled columns.
 */
export interface CollectionPayload {
  title: string
  is_public: boolean
  pin_to_top: boolean
  view_mode: string
  show_all_tab: boolean
  backdrop_image_url: string
  folders: FolderPayload[]
}

export function createCollection(
  profileIndex: number,
  body: CollectionPayload,
): Promise<Collection> {
  return sendJSON<Collection>('POST', `/api/p/${profileIndex}/collections`, body)
}

/**
 * Full upsert of the whole tree. Owned only — the server matches on
 * `id = ? AND owner_id = ?` and 404s otherwise.
 *
 * Two ways this 400s that the form has to prevent rather than report, since
 * the body is plain text with no field name in it: a folder `id` that doesn't
 * belong to this collection, and a `catalog_ids` entry the profile can't
 * reference (`owner_id = ? OR is_public = 1`). See `collectionForm.ts`.
 */
export function updateCollection(
  profileIndex: number,
  collectionID: string,
  body: CollectionPayload,
): Promise<Collection> {
  return sendJSON<Collection>('PUT', `/api/p/${profileIndex}/collections/${collectionID}`, body)
}

/** Hard delete, cascading to folders and their catalog refs. Removes the
 *  collection for **every profile using it**, same as a catalog. */
export function deleteCollection(profileIndex: number, collectionID: string): Promise<null> {
  return sendJSON<null>('DELETE', `/api/p/${profileIndex}/collections/${collectionID}`)
}
