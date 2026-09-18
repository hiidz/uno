import type { Catalog, Collection, CollectionPayload, Folder, FolderCatalogRef, TileShape } from '@/api'

/** Prefix marking a `FolderRefState.catalogID` as a client-only
 *  draft — staged locally by "copy into this collection"/"new inside this
 *  collection" (`CollectionEditor.tsx`), not yet written to the DB.
 *  `toCollectionPayload` resolves one into an inline `new` spec, which is
 *  what makes those actions atomic with this collection's own save: nothing
 *  is written until then, so discarding instead of saving leaves no row
 *  behind. See docs/frontend.md's "Three sources for a folder's catalog". */
export const DRAFT_ID_PREFIX = 'draft:'

export function isDraftCatalogID(id: string): boolean {
  return id.startsWith(DRAFT_ID_PREFIX)
}

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
  focusGIFURL: string
  focusGIFEnabled: boolean
  heroBackdropURL: string
  heroVideoURL: string
  titleLogoURL: string
  /** Ordered — index becomes `folder_catalogs.sort_order`. */
  refs: FolderRefState[]
}

/**
 * One catalog reference in a folder. A catalog can appear more than once in
 * one folder, each time under a different genre (`folder_catalogs` is
 * `PRIMARY KEY (folder_id, catalog_id, genre)`), so neither the catalog id nor
 * the pair is a stable handle while editing — the genre changes under the
 * user. `key` is that handle: React keys, dnd-kit ids and every per-ref
 * action. Never sent, like a folder's `key`.
 */
export interface FolderRefState {
  key: string
  /** A real catalog id, or a `draft:` one — see `DRAFT_ID_PREFIX`. */
  catalogID: string
  /** A genre name from the catalog's genre options, or `''` for unfiltered. */
  genre: string
}

export interface CollectionFormState {
  title: string
  isPublic: boolean
  pinToTop: boolean
  viewMode: CollectionViewMode
  showAllTab: boolean
  backdropImageURL: string
  focusGlowEnabled: boolean
  folders: FolderFormState[]
}

let folderKeySeq = 0

/** A counter, not a random id: these never leave the tab and never persist, so
 *  uniqueness within one form is the only requirement. */
export function nextFolderKey(): string {
  folderKeySeq += 1
  return `f${folderKeySeq}`
}

let refKeySeq = 0

export function newRef(catalogID: string, genre = ''): FolderRefState {
  refKeySeq += 1
  return { key: `r${refKeySeq}`, catalogID, genre }
}

/** True when `folder` already holds `catalogID` under `genre` — the pair the
 *  primary key forbids repeating. */
export function hasRef(folder: FolderFormState, catalogID: string, genre: string): boolean {
  return folder.refs.some((ref) => ref.catalogID === catalogID && ref.genre === genre)
}

/** `refs` in the order of `orderedKeys`, dropping any key not among them. */
export function reorderRefs(refs: FolderRefState[], orderedKeys: string[]): FolderRefState[] {
  const byKey = new Map(refs.map((ref) => [ref.key, ref]))
  return orderedKeys.map((key) => byKey.get(key)).filter((ref) => ref !== undefined)
}

/** The two focus flags start on: Nuvio reads an absent flag as on, and the
 *  schema defaults them to 1 to match. */
export function newFolder(): FolderFormState {
  return {
    key: nextFolderKey(),
    title: '',
    tileShape: '',
    hideTitle: false,
    coverEmoji: '',
    coverImageURL: '',
    focusGIFURL: '',
    focusGIFEnabled: true,
    heroBackdropURL: '',
    heroVideoURL: '',
    titleLogoURL: '',
    refs: [],
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
    focusGlowEnabled: true,
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

function folderFromWire(folder: Folder): FolderFormState {
  return {
    key: nextFolderKey(),
    id: folder.id,
    title: folder.title,
    tileShape: toTileShape(folder.tile_shape),
    hideTitle: folder.hide_title,
    coverEmoji: folder.cover_emoji,
    coverImageURL: folder.cover_image_url,
    focusGIFURL: folder.focus_gif_url,
    focusGIFEnabled: folder.focus_gif_enabled,
    heroBackdropURL: folder.hero_backdrop_url,
    heroVideoURL: folder.hero_video_url,
    titleLogoURL: folder.title_logo_url,
    // `?? []` is a guard, not a live case: the Go side runs `refs` through
    // `orEmpty`. It stays because `getList` coerces only the top-level
    // response, never nested arrays like this one.
    refs: (folder.refs ?? []).map((ref) => newRef(ref.catalog_id, ref.genre)),
  }
}

/**
 * Seed the builder from an existing collection for editing.
 *
 * There's no `mode` any more: duplicating a collection is now a single
 * atomic server call (`DuplicateCollection`) that returns a
 * brand-new row with its own folders and scoped-catalog copies already in
 * place, so the result opens through this same edit-mode seed rather than a
 * pre-filled, not-yet-saved form. Unresolvable refs (pre-existing data on a
 * row this profile can't fully reach) are kept rather than dropped —
 * `validateCollectionForm` flags them instead of silently deleting rows the
 * user never asked to touch.
 */
export function formFromCollection(collection: Collection): CollectionFormState {
  return {
    title: collection.title,
    isPublic: collection.is_public,
    pinToTop: collection.pin_to_top,
    viewMode: toViewMode(collection.view_mode),
    showAllTab: collection.show_all_tab,
    backdropImageURL: collection.backdrop_image_url,
    focusGlowEnabled: collection.focus_glow_enabled,
    folders: (collection.folders ?? []).map(folderFromWire),
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
 * builder's only source of catalogs is the library — `GET /api/p/{i}/catalogs`,
 * exactly this profile's own listed catalogs — and the server's
 * `validateFolderRefs` checks the same closed-graph rule. So here, unlike in
 * the Home pane, "not in the library" and "the server will reject this" are
 * one condition: no selection endpoint supplies a third source of rows.
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

    // A draft (staged locally, not yet a row) is always "accessible" — it
    // doesn't exist yet for the library to have excluded.
    const unavailable = folder.refs.filter(
      (ref) => !isDraftCatalogID(ref.catalogID) && !accessibleCatalogIDs.has(ref.catalogID),
    )
    // The same catalog under the same genre twice breaks
    // `PRIMARY KEY (folder_id, catalog_id, genre)`; `CollectionForm.Validate`
    // rejects it as a 400. The picker and "Add another genre" never produce
    // one, but switching a ref's genre to one its twin already has does.
    const repeated = folder.refs.filter((ref, i) =>
      folder.refs.some((other, j) => j < i && other.catalogID === ref.catalogID && other.genre === ref.genre),
    )

    if (unavailable.length > 0) {
      folderErrors.catalogIDs =
        unavailable.length === 1
          ? 'One catalog here is no longer available. Remove it to save.'
          : `${unavailable.length} catalogs here are no longer available. Remove them to save.`
    } else if (repeated.length > 0) {
      folderErrors.catalogIDs = 'This folder lists the same catalog with the same genre twice.'
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

/**
 * `localCatalogs` resolves a draft id into its inline `new` spec — omitted
 * (defaulting to an empty map) by `isSameCollection`'s dirty-check below,
 * which only needs structural presence to differ, not a draft's exact
 * content: a draft is never in `baseline`, so adding one already changes
 * `refs`' membership regardless of how it's serialized.
 */
export function toCollectionPayload(
  state: CollectionFormState,
  localCatalogs: ReadonlyMap<string, Catalog> = new Map(),
): CollectionPayload {
  return {
    title: state.title.trim(),
    is_public: state.isPublic,
    pin_to_top: state.pinToTop,
    view_mode: state.viewMode,
    show_all_tab: state.showAllTab,
    backdrop_image_url: state.backdropImageURL.trim(),
    focus_glow_enabled: state.focusGlowEnabled,
    folders: state.folders.map((folder) => ({
      // Omitted rather than sent as null: `FolderData.ID` is `*uuid.UUID` with
      // `omitempty`, so an absent key is what marks a folder as new.
      ...(folder.id ? { id: folder.id } : {}),
      title: folder.title.trim(),
      tile_shape: folder.tileShape,
      hide_title: folder.hideTitle,
      cover_emoji: folder.coverEmoji.trim(),
      cover_image_url: folder.coverImageURL.trim(),
      focus_gif_url: folder.focusGIFURL.trim(),
      focus_gif_enabled: folder.focusGIFEnabled,
      hero_backdrop_url: folder.heroBackdropURL.trim(),
      hero_video_url: folder.heroVideoURL.trim(),
      title_logo_url: folder.titleLogoURL.trim(),
      catalogs: folder.refs.map(({ catalogID, genre }): FolderCatalogRef => {
        const draft = isDraftCatalogID(catalogID) ? localCatalogs.get(catalogID) : undefined
        // The draft id is the `new` spec's key: every ref to one draft, in any
        // folder and under any genre, resolves to the one catalog Save creates.
        const ref: FolderCatalogRef = draft
          ? {
              new: {
                key: catalogID,
                type: draft.type,
                name: draft.name,
                provider: draft.provider,
                params: draft.params,
              },
            }
          : { catalog_id: catalogID }
        // Omitted rather than sent empty, so an unfiltered ref serializes the
        // same way whether or not it ever had a genre.
        return genre ? { ...ref, genre } : ref
      }),
    })),
  }
}

/** Structural equality over everything that reaches the wire, so the dialog can
 *  tell an untouched form from an edited one without diffing by hand. `key` is
 *  excluded — it's session-local and moving a folder doesn't change it. */
export function isSameCollection(a: CollectionFormState, b: CollectionFormState): boolean {
  return JSON.stringify(toCollectionPayload(a)) === JSON.stringify(toCollectionPayload(b))
}
