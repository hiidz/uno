import { sendJSON } from './http'
import type { Catalog, CatalogType, Collection, CommunityCopy, TileShape, TMDBKeyStatus } from './types'

/** Publishes a listed catalog as it is saved now, or publishes its update —
 *  `POST .../catalogs/{id}/publish`. An update keeps the publication's id;
 *  publishing again after Unpublish is a new publication. 400s for a
 *  subscribed copy, and 502s when TMDB can't be reached to check the recipe. */
export function publishCatalog(profileIndex: number, catalogID: string): Promise<Catalog> {
  return sendJSON<Catalog>('POST', `/api/p/${profileIndex}/catalogs/${catalogID}/publish`)
}

/** Unpublishes a catalog — `POST .../catalogs/{id}/unpublish`. Its
 *  publication is gone for good: copies other profiles subscribed to become
 *  their own, marked `publisher_unpublished`. */
export function unpublishCatalog(profileIndex: number, catalogID: string): Promise<Catalog> {
  return sendJSON<Catalog>('POST', `/api/p/${profileIndex}/catalogs/${catalogID}/unpublish`)
}

/** `publishCatalog` for a collection: publishes its tree, with every catalog
 *  its folders use, library catalogs included. 400s for a subscribed copy. */
export function publishCollection(profileIndex: number, collectionID: string): Promise<Collection> {
  return sendJSON<Collection>('POST', `/api/p/${profileIndex}/collections/${collectionID}/publish`)
}

export function unpublishCollection(profileIndex: number, collectionID: string): Promise<Collection> {
  return sendJSON<Collection>('POST', `/api/p/${profileIndex}/collections/${collectionID}/unpublish`)
}

/** Subscribe (the UI's Add): a read-only copy of a publication that follows
 *  its updates — `POST .../community/{id}/subscribe`. */
export function subscribe(profileIndex: number, publicationID: string): Promise<CommunityCopy> {
  return sendJSON<CommunityCopy>('POST', `/api/p/${profileIndex}/community/${publicationID}/subscribe`)
}

/** Rewrites this profile's copy of a publication from its current snapshot,
 *  keeping every id — `POST .../community/{id}/update`. 404s once the
 *  publication is unpublished, which makes the copy this profile's own, or
 *  the copy is gone. */
export function updateSubscription(profileIndex: number, publicationID: string): Promise<CommunityCopy> {
  return sendJSON<CommunityCopy>('POST', `/api/p/${profileIndex}/community/${publicationID}/update`)
}

/** Duplicate: a copy of a publication that is the profile's own, with no
 *  subscription — `POST .../community/{id}/duplicate`. */
export function duplicatePublication(profileIndex: number, publicationID: string): Promise<CommunityCopy> {
  return sendJSON<CommunityCopy>('POST', `/api/p/${profileIndex}/community/${publicationID}/duplicate`)
}

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
 *
 * A create always makes a listed catalog: one inside a collection is made only
 * by that collection's save, as a folder's `new` entry.
 */
export interface CatalogPayload {
  type: CatalogType
  name: string
  provider: string
  params: string
}

export const CATALOG_PROVIDER = 'tmdb'

export function createCatalog(profileIndex: number, body: CatalogPayload): Promise<Catalog> {
  return sendJSON<Catalog>('POST', `/api/p/${profileIndex}/catalogs`, body)
}

/**
 * `type` and `provider` are immutable once a catalog exists, and the server
 * enforces both: a `type` that differs from the stored one is a 400, and
 * `provider` is always `"tmdb"`, the only value it accepts. The edit form
 * renders `type` as read-only for the same reason, and a duplicate copies it
 * verbatim, so nothing changes an existing catalog's type.
 */
export function updateCatalog(
  profileIndex: number,
  catalogID: string,
  body: CatalogPayload,
): Promise<Catalog> {
  return sendJSON<Catalog>('PUT', `/api/p/${profileIndex}/catalogs/${catalogID}`, body)
}

/** Hard delete of an owned catalog, cascading to the folder refs that point
 *  at it. A published catalog is unpublished; copies other profiles added
 *  become their own, marked `publisher_unpublished`. */
export function deleteCatalog(profileIndex: number, catalogID: string): Promise<null> {
  return sendJSON<null>('DELETE', `/api/p/${profileIndex}/catalogs/${catalogID}`)
}

/**
 * One entry in a folder payload's ordered catalog list — `vault.
 * FolderCatalogRef`. Either a reference to an existing catalog, or an inline
 * spec for a new one, created scoped to the enclosing collection in the same
 * transaction as the folder write that references it. This is what makes
 * "new inside this collection" atomic with the collection's own save — see
 * `collectionForm.ts`'s `toCollectionPayload` and `CollectionEditor.tsx`'s
 * draft catalogs.
 *
 * `genre` narrows this one reference to a genre by name; omitted means
 * unfiltered.
 */
export type FolderCatalogRef = (
  | { catalog_id: string }
  | { new: { key: string; type: CatalogType; name: string; provider: string; params: string } }
) & { genre?: string }

/**
 * One folder inside a collection payload — `vault.FolderData`.
 *
 * `id` is the whole upsert protocol in one field: absent means "insert a new
 * folder", present means "update this existing one". A folder that exists
 * server-side but is *omitted from the array* is **deleted**, cascading to its
 * catalog refs. There is no soft-delete and no per-folder endpoint; the array
 * is the truth.
 *
 * `catalogs` is ordered — its index becomes `folder_catalogs.sort_order`. An
 * existing catalog may repeat within one folder under different genres, never
 * under the same one: `folder_catalogs` is
 * `PRIMARY KEY (folder_id, catalog_id, genre)`, and `CollectionForm.Validate`
 * rejects the repeat as a 400. The same catalog in two *different* folders is
 * fine and is a supported thing to want.
 */
interface FolderPayload {
  id?: string
  title: string
  tile_shape: TileShape | ''
  hide_title: boolean
  cover_emoji: string
  cover_image_url: string
  focus_gif_url: string
  focus_gif_enabled: boolean
  hero_backdrop_url: string
  hero_video_url: string
  title_logo_url: string
  catalogs: FolderCatalogRef[]
}

/**
 * One entry in a collection payload's `catalog_edits` — `vault.ScopedCatalogEdit`.
 * The new name and recipe for a catalog already scoped to this collection,
 * written in the same transaction as the rest of the save; the only way such
 * a catalog is written once it exists. `type` and `provider` must match the
 * stored row.
 */
interface ScopedCatalogEdit {
  id: string
  type: CatalogType
  provider: string
  name: string
  params: string
}

/**
 * The create/update payload — `vault.CollectionForm`.
 *
 * The whole tree in one request: `POST`/`PUT` replace the collection, its
 * folders, every folder's catalog refs and the edits to its scoped catalogs in
 * a single transaction, so there is one dirty state and one save button rather
 * than a save per folder or per catalog. `catalog_edits` is always `[]` on
 * create, which has no scoped catalogs yet.
 */
export interface CollectionPayload {
  title: string
  view_mode: string
  show_all_tab: boolean
  backdrop_image_url: string
  focus_glow_enabled: boolean
  folders: FolderPayload[]
  catalog_edits: ScopedCatalogEdit[]
}

export function createCollection(
  profileIndex: number,
  body: CollectionPayload,
): Promise<Collection> {
  return sendJSON<Collection>('POST', `/api/p/${profileIndex}/collections`, body)
}

/** Atomically copies a collection this profile already owns: listed folder
 *  refs stay references, each distinct scoped catalog becomes a fresh scoped
 *  copy — `POST .../collections/{id}/duplicate`. 404s if the source isn't
 *  owned by this profile. */
export function duplicateCollection(
  profileIndex: number,
  collectionID: string,
): Promise<Collection> {
  return sendJSON<Collection>('POST', `/api/p/${profileIndex}/collections/${collectionID}/duplicate`)
}

/**
 * Full upsert of the whole tree. Owned only — the server matches on
 * `id = ? AND owner_id = ?` and 404s otherwise.
 *
 * Two ways this 400s that the form has to prevent rather than report, since
 * the body is plain text with no field name in it: a folder `id` that doesn't
 * belong to this collection, and a folder ref to a catalog that isn't this
 * profile's own and either listed or scoped to this collection
 * (`validateFolderRefs`). See `collectionForm.ts`.
 */
export function updateCollection(
  profileIndex: number,
  collectionID: string,
  body: CollectionPayload,
): Promise<Collection> {
  return sendJSON<Collection>('PUT', `/api/p/${profileIndex}/collections/${collectionID}`, body)
}

/** Hard delete of an owned collection, cascading to its folders and their
 *  catalog refs. As with a catalog, copies other profiles took survive. */
export function deleteCollection(profileIndex: number, collectionID: string): Promise<null> {
  return sendJSON<null>('DELETE', `/api/p/${profileIndex}/collections/${collectionID}`)
}

/** Saves the signed-in account's TMDB key, replacing any it had, once TMDB
 *  accepts it. A key of the wrong shape, or one TMDB refuses, is a 400 in
 *  words; TMDB unreachable is a 502. Nothing is saved on either. */
export function saveTMDBKey(key: string): Promise<TMDBKeyStatus> {
  return sendJSON<TMDBKeyStatus>('PUT', '/api/account/tmdb-key', { key })
}

/** Removes the signed-in account's TMDB key. */
export function removeTMDBKey(): Promise<null> {
  return sendJSON<null>('DELETE', '/api/account/tmdb-key')
}
