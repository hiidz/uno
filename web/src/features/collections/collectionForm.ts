import type { Collection, CollectionPayload, Folder, TileShape } from '@/api'

/**
 * The collection builder's form model, and the client-side mirror of what the
 * server rejects.
 *
 * Same premise as `catalogForm.ts`: every `400` out of the collection handlers
 * is `http.Error(w, err.Error(), …)` — plain text, no field name in a
 * machine-readable position — so the rules live here too, and a server 400 that
 * gets through means this mirror has drifted.
 *
 * The tree makes two rules structural that the catalog form didn't have to
 * think about:
 *
 *  - **A folder's identity is its position in the array plus an optional `id`.**
 *    Absent `id` = insert; present = update; *omitted from the array* = deleted
 *    server-side, cascading to its catalog refs. So removing a folder in the
 *    editor is a destructive pending change, and `removedFolders` exists to let
 *    the UI say so before the save rather than after.
 *  - **`key` is local and never sent.** New folders have no server id, but React
 *    and dnd-kit both need a stable handle from the moment one is added, and
 *    two brand-new folders must not collide. `id` is the wire; `key` is the
 *    session.
 */

/** `''` is a real, storable value meaning "unset" — the server's `Validate`
 *  explicitly allows it, and Preview treats it as an assumed `POSTER` rather
 *  than a fact. Modelled as its own option instead of being normalised away, so
 *  a round-trip through this form doesn't quietly rewrite stored data. */
export type FolderTileShape = TileShape | ''

/** Mirrors `validViewModes` in `internal/vault/validation.go`. No `''`: every
 *  collection has a view mode, and "unset" already has a name in that enum —
 *  `FOLLOW_LAYOUT`, which is what the app does with an empty one anyway.
 *  Union-typed rather than validated, so an invalid view mode is
 *  unrepresentable and `validateCollectionForm` doesn't have to check it. */
export type CollectionViewMode = 'TABBED_GRID' | 'ROWS' | 'FOLLOW_LAYOUT'

/** Ordered as the select lists them, so the default reads first. */
export const VIEW_MODES: CollectionViewMode[] = ['FOLLOW_LAYOUT', 'TABBED_GRID', 'ROWS']

export const VIEW_MODE_LABELS: Record<CollectionViewMode, string> = {
  FOLLOW_LAYOUT: 'Follow layout',
  TABBED_GRID: 'Tabbed Grids',
  ROWS: 'Rows',
}
export const TILE_SHAPES: TileShape[] = ['POSTER', 'LANDSCAPE', 'SQUARE']

/** As in `catalogForm.ts`: no `create`, because a collection is titled into
 *  existence by its own dialog and saved before the editor opens. */
export type BuilderMode = 'edit' | 'duplicate'

export interface FolderFormState {
  /** Stable for the lifetime of this form only — React keys and dnd-kit ids.
   *  Never sent; see the module comment. */
  key: string
  /** The server's folder id, absent on a folder that doesn't exist yet. */
  id?: string
  title: string
  tileShape: FolderTileShape
  hideTitle: boolean
  coverEmoji: string
  coverImageURL: string
  /** Ordered — index becomes `folder_catalogs.sort_order`. */
  catalogIDs: string[]
}

export interface CollectionFormState {
  title: string
  isPublic: boolean
  pinToTop: boolean
  viewMode: CollectionViewMode
  showAllTab: boolean
  backdropImageURL: string
  folders: FolderFormState[]
}

let folderKeySeq = 0

/** A counter, not a random id: these never leave the tab and never persist, so
 *  uniqueness within one form is the only requirement. */
export function nextFolderKey(): string {
  folderKeySeq += 1
  return `f${folderKeySeq}`
}

export function newFolder(): FolderFormState {
  return {
    key: nextFolderKey(),
    title: '',
    tileShape: '',
    hideTitle: false,
    coverEmoji: '',
    coverImageURL: '',
    catalogIDs: [],
  }
}

export function emptyCollectionForm(): CollectionFormState {
  return {
    title: '',
    isPublic: false,
    pinToTop: false,
    viewMode: 'FOLLOW_LAYOUT',
    showAllTab: false,
    backdropImageURL: '',
    folders: [],
  }
}

function toViewMode(raw: string): CollectionViewMode {
  // `view_mode` is a bare string on the wire, so an empty or unknown value is
  // representable. Both land on `FOLLOW_LAYOUT`: it is what the app falls back
  // to for them, so the form shows what the collection already does rather
  // than a fourth state meaning "whatever this string was".
  return (VIEW_MODES as string[]).includes(raw) ? (raw as CollectionViewMode) : 'FOLLOW_LAYOUT'
}

function toTileShape(raw: string): FolderTileShape {
  return (TILE_SHAPES as string[]).includes(raw) ? (raw as FolderTileShape) : ''
}

function folderFromWire(folder: Folder, mode: BuilderMode): FolderFormState {
  return {
    key: nextFolderKey(),
    // A duplicate is a brand-new collection: carrying folder ids across would
    // point at rows belonging to the *source*, which the server rejects
    // ("folder … does not belong to this collection"). Stripping them is what
    // turns the clone into inserts.
    id: mode === 'duplicate' ? undefined : folder.id,
    title: folder.title,
    tileShape: toTileShape(folder.tile_shape),
    hideTitle: folder.hide_title,
    coverEmoji: folder.cover_emoji,
    coverImageURL: folder.cover_image_url,
    // `?? []` is a guard, not a live case: the Go side runs `catalog_ids`
    // through `orEmpty`. It stays because `getList` coerces only the top-level
    // response, never nested arrays like this one.
    catalogIDs: folder.catalog_ids ?? [],
  }
}

export interface SeedResult {
  state: CollectionFormState
  /** Catalog ids dropped because this profile can't reference them. Non-empty
   *  only when duplicating; see `formFromCollection`. */
  droppedRefs: string[]
}

/**
 * Seed the builder from an existing collection.
 *
 * `accessibleCatalogIDs` is the set this profile may reference — the library,
 * which is exactly `owned ∪ is_public` and therefore exactly the server's
 * `owner_id = ? OR is_public = 1`. Passing it matters for **duplicate**: a
 * community collection can reference a catalog whose owner has since made it
 * private, and cloning that verbatim would `400` with a message naming a uuid
 * the user has never seen. So duplicate clones partially, and the caller is
 * expected to say which refs went.
 *
 * **Edit keeps unresolvable refs.** They're pre-existing data, and dropping
 * them silently would delete rows the user never asked to touch —
 * `validateCollectionForm` flags them instead.
 */
export function formFromCollection(
  collection: Collection,
  mode: BuilderMode,
  accessibleCatalogIDs: ReadonlySet<string>,
): SeedResult {
  const folders = (collection.folders ?? []).map((f) => folderFromWire(f, mode))

  const droppedRefs: string[] = []
  if (mode === 'duplicate') {
    for (const folder of folders) {
      const keep = folder.catalogIDs.filter((id) => accessibleCatalogIDs.has(id))
      for (const id of folder.catalogIDs) {
        if (!accessibleCatalogIDs.has(id)) droppedRefs.push(id)
      }
      folder.catalogIDs = keep
    }
  }

  return {
    state: {
      title: mode === 'duplicate' ? `${collection.title} (copy)` : collection.title,
      // Never born public — publishing is a deliberate act, not something
      // inherited from whoever you forked. Same rule as the catalog copy.
      isPublic: mode === 'duplicate' ? false : collection.is_public,
      pinToTop: collection.pin_to_top,
      viewMode: toViewMode(collection.view_mode),
      showAllTab: collection.show_all_tab,
      backdropImageURL: collection.backdrop_image_url,
      folders,
    },
    droppedRefs,
  }
}

export interface FolderErrors {
  title?: string
  catalogIDs?: string
}

export interface CollectionErrors {
  title?: string
  /** Keyed by folder `key`, not `id` — a new folder has no id and still needs
   *  to be able to carry an error. */
  folders: Record<string, FolderErrors>
}

export function countErrors(errors: CollectionErrors): number {
  let n = errors.title ? 1 : 0
  for (const folder of Object.values(errors.folders)) {
    if (folder.title) n += 1
    if (folder.catalogIDs) n += 1
  }
  return n
}

/**
 * Mirrors `CollectionForm.Validate()` plus the two rejections that live outside
 * it, in `UpdateUserCollection`/`validateCatalogAccess`.
 *
 * What is *not* checked here, because the types make it unrepresentable:
 * `view_mode` is the server's own enum and `tile_shape` is that enum plus `''`,
 * and a folder `id` only ever comes from a collection this form loaded, so
 * "folder does not belong to this collection" can't be constructed.
 *
 * **`accessibleCatalogIDs` is an exact mirror, not an approximation.** The
 * builder's only source of catalogs is the library — `GET /api/p/{i}/catalogs`
 * ∪ `GET /api/catalogs`, i.e. owned ∪ public — and `validateAccess` is
 * `owner_id = ? OR is_public = 1` over the same rows. So here, unlike in the
 * Home pane, "not in the library" and "the server will reject this" are one
 * condition: no selection endpoint supplies a third source of rows.
 */
export function validateCollectionForm(
  state: CollectionFormState,
  accessibleCatalogIDs: ReadonlySet<string>,
): CollectionErrors {
  const errors: CollectionErrors = { title: undefined, folders: {} }

  if (!state.title.trim()) errors.title = 'Give this collection a title.'

  for (const folder of state.folders) {
    const folderErrors: FolderErrors = {}

    if (!folder.title.trim()) folderErrors.title = 'Every folder needs a title.'

    const unavailable = folder.catalogIDs.filter((id) => !accessibleCatalogIDs.has(id))
    // A repeat inside one folder violates `PRIMARY KEY (folder_id, catalog_id)`
    // — a 500, not a 400. The picker makes it unrepresentable by omitting ids
    // already in the folder; this is the backstop.
    const repeated = folder.catalogIDs.filter((id, i) => folder.catalogIDs.indexOf(id) !== i)

    if (unavailable.length > 0) {
      folderErrors.catalogIDs =
        unavailable.length === 1
          ? 'One catalog here is no longer available. Remove it to save.'
          : `${unavailable.length} catalogs here are no longer available. Remove them to save.`
    } else if (repeated.length > 0) {
      folderErrors.catalogIDs = 'This folder lists the same catalog twice.'
    }

    if (folderErrors.title || folderErrors.catalogIDs) {
      errors.folders[folder.key] = folderErrors
    }
  }

  return errors
}

/**
 * The folders that saving `current` would delete server-side.
 *
 * Only folders that already exist can be "removed" in the destructive sense —
 * dropping a folder that was never saved costs nothing. Returned as the
 * original rows so the UI can name which folders are about to disappear.
 */
export function removedFolders(
  initial: CollectionFormState,
  current: CollectionFormState,
): FolderFormState[] {
  const keptIDs = new Set(current.folders.map((f) => f.id).filter(Boolean))
  return initial.folders.filter((f) => f.id && !keptIDs.has(f.id))
}

export function toCollectionPayload(state: CollectionFormState): CollectionPayload {
  return {
    title: state.title.trim(),
    is_public: state.isPublic,
    pin_to_top: state.pinToTop,
    view_mode: state.viewMode,
    show_all_tab: state.showAllTab,
    backdrop_image_url: state.backdropImageURL.trim(),
    folders: state.folders.map((folder) => ({
      // Omitted rather than sent as null: `FolderData.ID` is `*uuid.UUID` with
      // `omitempty`, so an absent key is what marks a folder as new.
      ...(folder.id ? { id: folder.id } : {}),
      title: folder.title.trim(),
      tile_shape: folder.tileShape,
      hide_title: folder.hideTitle,
      cover_emoji: folder.coverEmoji.trim(),
      cover_image_url: folder.coverImageURL.trim(),
      catalog_ids: folder.catalogIDs,
    })),
  }
}

/** Structural equality over everything that reaches the wire, so the dialog can
 *  tell an untouched form from an edited one without diffing by hand. `key` is
 *  excluded — it's session-local and moving a folder doesn't change it. */
export function isSameCollection(a: CollectionFormState, b: CollectionFormState): boolean {
  return JSON.stringify(toCollectionPayload(a)) === JSON.stringify(toCollectionPayload(b))
}
