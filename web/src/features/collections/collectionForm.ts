import type {
  Catalog,
  CatalogType,
  Collection,
  CollectionPayload,
  Folder,
  FolderCatalogRef,
  TileShape,
  ViewMode,
} from '@/api'
import {
  MAX_NAME_LENGTH,
  characterCount,
  formFromCatalog,
  isSameCatalog,
  toPayload as toCatalogPayload,
  type CatalogFormState,
} from '@/features/catalogs/catalogForm'
import type { PreviewCollection, PreviewFolder, PreviewSource } from '@/features/preview/model'
import type { RefOption } from './refs'

/** Prefix marking a `FolderRefState.catalogID` as a client-only
 *  draft — staged locally by "new inside this collection"
 *  (`CollectionEditor.tsx`), not yet written to the DB.
 *  `toCollectionPayload` resolves one into an inline `new` spec, which is
 *  what makes that action atomic with this collection's own save: nothing
 *  is written until then, so discarding instead of saving leaves no row
 *  behind. See docs/frontend.md's "Two sources for a folder's catalog". */
export const DRAFT_ID_PREFIX = 'draft:'

export function isDraftCatalogID(id: string): boolean {
  return id.startsWith(DRAFT_ID_PREFIX)
}

/**
 * The collection builder's form model, and the collection rules a form can
 * reach, checked here as the server checks them.
 *
 * Same premise as `catalogForm.ts`: every `400` out of the collection handlers
 * is `http.Error(w, err.Error(), …)` — plain text, no field name in a
 * machine-readable position — so those rules live here too, and a server 400
 * on one of them means this copy has drifted. The rules no control can break
 * (a genre's length, the cover emoji's, a catalog edit's repeats) are left to
 * the server.
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

/** Ordered as the editor's choices list them, so the default reads first. */
export const VIEW_MODES: ViewMode[] = ['TABBED_GRID', 'ROWS']

export const VIEW_MODE_LABELS: Record<ViewMode, string> = {
  TABBED_GRID: 'Tabbed Grids',
  ROWS: 'Rows',
}

/** The collection's Appearance shelf folded: "Tabbed Grids · All tab ·
 *  background image · glow on". The All tab counts only where Tabbed Grids
 *  has tabs to add it to. */
export function appearanceSummary(
  state: Pick<CollectionFormState, 'viewMode' | 'showAllTab' | 'backdropImageURL' | 'focusGlowEnabled'>,
): string {
  const parts = [VIEW_MODE_LABELS[state.viewMode]]
  if (state.viewMode === 'TABBED_GRID' && state.showAllTab) parts.push('All tab')
  if (state.backdropImageURL.trim()) parts.push('background image')
  if (state.focusGlowEnabled) parts.push('glow on')
  return parts.join(' · ')
}

export const TILE_SHAPES: TileShape[] = ['POSTER', 'LANDSCAPE', 'SQUARE']

export interface FolderFormState {
  /** Stable for the lifetime of this form only — React keys and dnd-kit ids.
   *  Never sent; see the module comment. */
  key: string
  /** The server's folder id, absent on a folder that doesn't exist yet. */
  id?: string
  title: string
  tileShape: TileShape
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

/**
 * A pending edit to a catalog already scoped to this collection, made in the
 * nested catalog editor. Part of the form rather than written on the spot, so
 * it lands with the collection's own Save (as `catalog_edits`) and a discarded
 * form never wrote it. `type` and `provider` ride along because the server
 * checks the recipe before it reads the row.
 */
interface CatalogEditState {
  type: CatalogType
  provider: string
  name: string
  params: string
}

export interface CollectionFormState {
  title: string
  /** Typed to the server's enum, so an invalid view mode is unrepresentable
   *  and `validateCollectionForm` doesn't have to check it. */
  viewMode: ViewMode
  showAllTab: boolean
  backdropImageURL: string
  focusGlowEnabled: boolean
  folders: FolderFormState[]
  /** Keyed by catalog id. An edit that would leave its catalog as saved is
   *  removed rather than kept, so undoing one leaves the form clean. */
  catalogEdits: Record<string, CatalogEditState>
}

let folderKeySeq = 0

/** A counter, not a random id: these never leave the tab and never persist, so
 *  uniqueness within one form is the only requirement. */
function nextFolderKey(): string {
  folderKeySeq += 1
  return `f${folderKeySeq}`
}

let refKeySeq = 0

export function newRef(catalogID: string, genre = ''): FolderRefState {
  refKeySeq += 1
  return { key: `r${refKeySeq}`, catalogID, genre }
}

/** True when `refs` already hold `catalogID` under `genre` — the pair the
 *  primary key forbids repeating. */
export function hasRef(refs: FolderRefState[], catalogID: string, genre: string): boolean {
  return refs.some((ref) => ref.catalogID === catalogID && ref.genre === genre)
}

/** The two focus flags start on: Nuvio reads an absent flag as on, and the
 *  schema defaults them to 1 to match. */
export function newFolder(): FolderFormState {
  return {
    key: nextFolderKey(),
    title: '',
    tileShape: 'POSTER',
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
    viewMode: 'TABBED_GRID',
    showAllTab: false,
    backdropImageURL: '',
    focusGlowEnabled: true,
    folders: [],
    catalogEdits: {},
  }
}

function folderFromWire(folder: Folder): FolderFormState {
  return {
    key: nextFolderKey(),
    id: folder.id,
    title: folder.title,
    tileShape: folder.tile_shape,
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
 * Seed the editor from a saved collection — one picked in the rail, or a
 * duplicate's finished copy. Unresolvable refs (pre-existing data on a row
 * this profile can't fully reach) are kept rather than dropped —
 * `validateCollectionForm` flags them instead of silently deleting rows the
 * user never asked to touch.
 */
export function formFromCollection(collection: Collection): CollectionFormState {
  return {
    title: collection.title,
    viewMode: collection.view_mode,
    showAllTab: collection.show_all_tab,
    backdropImageURL: collection.backdrop_image_url,
    focusGlowEnabled: collection.focus_glow_enabled,
    folders: (collection.folders ?? []).map(groupedFolderFromWire),
    catalogEdits: {},
  }
}

/**
 * `state` with a pending edit to `saved` taken from the nested catalog
 * editor's `form`, or with that edit dropped when `form` would leave the
 * catalog as `saved` has it. The comparison runs on forms (`isSameCatalog`),
 * never on the stored `params` string, which the form re-serializes: a stored
 * string with its keys in another order would otherwise read as an edit.
 * `saved` is the row as the collection editor opened it, not the latest local
 * copy, so editing a catalog and then editing it back clears the edit.
 */
export function withCatalogEdit(
  state: CollectionFormState,
  saved: Catalog,
  form: CatalogFormState,
): CollectionFormState {
  const catalogEdits = { ...state.catalogEdits }
  if (isSameCatalog(formFromCatalog(saved), form)) {
    delete catalogEdits[saved.id]
  } else {
    const payload = toCatalogPayload(form)
    catalogEdits[saved.id] = {
      type: payload.type,
      provider: payload.provider,
      name: payload.name,
      params: payload.params,
    }
  }
  return { ...state, catalogEdits }
}

/** The most folders a collection holds and refs a folder holds, the server's
 *  `maxFoldersPerCollection` and `maxRefsPerFolder`. A collection stored past
 *  either still loads; it saves once trimmed back under them. */
export const MAX_FOLDERS = 10
export const MAX_REFS_PER_FOLDER = 20

export interface FolderErrors {
  title?: string
  catalogIDs?: string
  /** The first of the folder's media addresses that isn't one the server takes. */
  media?: string
}

export interface CollectionErrors {
  title?: string
  /** The background image's address, when it isn't one the server takes. */
  backdrop?: string
  /** Set while the collection holds more than `MAX_FOLDERS` folders. */
  folderCount?: string
  /** Keyed by folder `key`, not `id` — a new folder has no id and still needs
   *  to be able to carry an error. */
  folders: Record<string, FolderErrors>
}

/** The collection's own errors, not its folders': its title and its folder
 *  count. */
export function ownErrors(errors: CollectionErrors): string[] {
  const own: string[] = []
  for (const error of [errors.title, errors.backdrop, errors.folderCount]) {
    if (error !== undefined) own.push(error)
  }
  return own
}

export function countErrors(errors: CollectionErrors): number {
  let n = ownErrors(errors).length
  for (const folder of Object.values(errors.folders)) {
    if (folder.title) n += 1
    if (folder.catalogIDs) n += 1
    if (folder.media) n += 1
  }
  return n
}

/**
 * The rules of `CollectionForm.Validate()` that a form can reach, plus the
 * folder-ref rule that lives outside it, in `UpdateUserCollection`'s
 * `validateFolderRefs`.
 *
 * What is *not* checked here, because the types make it unrepresentable:
 * `view_mode` and `tile_shape` are the server's own enums, and a folder `id`
 * only ever comes from a collection this form loaded, so "folder does not
 * belong to this collection" can't be constructed.
 *
 * **`accessibleCatalogIDs` is an exact mirror, not an approximation.** The
 * editor's catalogs are the library — `GET /api/p/{i}/catalogs`, exactly
 * this profile's own listed catalogs — plus the ones scoped to this
 * collection that it already knows about, and the server's
 * `validateFolderRefs` checks the same closed-graph rule. So here, unlike in
 * the Home pane, "not accessible" and "the server will reject this" are one
 * condition: no selection endpoint supplies another source of rows.
 */
export function validateCollectionForm(
  state: CollectionFormState,
  accessibleCatalogIDs: ReadonlySet<string>,
): CollectionErrors {
  const errors: CollectionErrors = { title: undefined, folders: {} }

  errors.title = titleProblem(state.title, 'Give this collection a title.')
  errors.backdrop = mediaProblem('Background image', state.backdropImageURL)
  errors.folderCount = folderCountError(state.folders.length)

  for (const folder of state.folders) {
    const folderErrors: FolderErrors = {}

    folderErrors.title = titleProblem(folder.title, 'Every folder needs a title.')
    folderErrors.media = folderMediaProblem(folder)

    // A draft (staged locally, not yet a row) is always "accessible" — it
    // doesn't exist yet for the library to have excluded.
    const unavailable = folder.refs.filter(
      (ref) => !isDraftCatalogID(ref.catalogID) && !accessibleCatalogIDs.has(ref.catalogID),
    )
    // The same catalog under the same genre twice breaks
    // `PRIMARY KEY (folder_id, catalog_id, genre)`; `CollectionForm.Validate`
    // rejects it as a 400. The picker and a catalog's genre dropdown never
    // produce one, so this backstops the server's check.
    const repeated = folder.refs.filter((ref, i) =>
      folder.refs.some((other, j) => j < i && other.catalogID === ref.catalogID && other.genre === ref.genre),
    )

    folderErrors.catalogIDs = refsError(unavailable.length, repeated.length, folder.refs.length)

    if (hasFolderErrors(folderErrors)) {
      errors.folders[folder.key] = folderErrors
    }
  }

  return errors
}

function hasFolderErrors(errors: FolderErrors): boolean {
  return Boolean(errors.title || errors.catalogIDs || errors.media)
}

/** Why a title can't save: empty (`empty` says so), or past the server's
 *  length. */
function titleProblem(title: string, empty: string): string | undefined {
  const trimmed = title.trim()
  if (!trimmed) return empty
  if (characterCount(trimmed) > MAX_NAME_LENGTH) return `Keep the title to ${MAX_NAME_LENGTH} characters or fewer.`
  return undefined
}

/** The most characters a media address may hold: the server's
 *  `maxMediaURLLen`. */
const MAX_MEDIA_URL_LENGTH = 2048

/** Why `raw`, a media address, can't save, or undefined when it is empty or
 *  one the server takes: an absolute http or https address. Every one of
 *  these is pushed into Nuvio and read by its clients. */
function mediaProblem(label: string, raw: string): string | undefined {
  const address = raw.trim()
  if (!address) return undefined
  if (characterCount(address) > MAX_MEDIA_URL_LENGTH) return `${label} address is too long.`
  if (!isWebAddress(address)) return `${label} must be a web address starting with http:// or https://.`
  return undefined
}

function isWebAddress(address: string): boolean {
  try {
    const url = new URL(address)
    return (url.protocol === 'http:' || url.protocol === 'https:') && url.host !== ''
  } catch {
    return false
  }
}

/** The first of a folder's media addresses that can't save. */
function folderMediaProblem(folder: FolderFormState): string | undefined {
  for (const [label, key] of FOLDER_ADDRESSES) {
    const problem = mediaProblem(label, folder[key])
    if (problem) return problem
  }
  return undefined
}

const FOLDER_ADDRESSES = [
  ['Cover image', 'coverImageURL'],
  ['Focus GIF', 'focusGIFURL'],
  ['Hero backdrop', 'heroBackdropURL'],
  ['Hero video', 'heroVideoURL'],
  ['Title logo', 'titleLogoURL'],
] as const satisfies readonly (readonly [string, keyof FolderFormState])[]

/** Why a collection of `count` folders can't save, while it holds more than
 *  `MAX_FOLDERS`. */
function folderCountError(count: number): string | undefined {
  if (count <= MAX_FOLDERS) return undefined
  return `A collection holds at most ${MAX_FOLDERS} folders. Remove ${count - MAX_FOLDERS} to save.`
}

/** Why a folder's catalogs can't save, the first that applies: `unavailable`
 *  of them gone from the library, `repeated` ones under the same genre twice,
 *  or `total` refs past `MAX_REFS_PER_FOLDER`. */
function refsError(unavailable: number, repeated: number, total: number): string | undefined {
  if (unavailable === 1) return 'One catalog here is no longer available. Remove it to save.'
  if (unavailable > 1) return `${unavailable} catalogs here are no longer available. Remove them to save.`
  if (repeated > 0) return 'This folder lists the same catalog with the same genre twice.'
  if (total > MAX_REFS_PER_FOLDER) {
    return `A folder holds at most ${MAX_REFS_PER_FOLDER} catalogs, one split by genre counting once a genre. Remove ${total - MAX_REFS_PER_FOLDER} to save.`
  }
  return undefined
}

/** A folder's name for labels and announcements: its title, or its place in
 *  the row while it has none. */
export function folderLabel(folder: FolderFormState, position: number): string {
  return folder.title.trim() || `folder ${position + 1}`
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
    catalog_edits: Object.entries(state.catalogEdits).map(([id, edit]) => ({
      id,
      type: edit.type,
      provider: edit.provider,
      name: edit.name,
      params: edit.params,
    })),
  }
}

/** Structural equality over everything that reaches the wire, so the editor can
 *  tell an untouched form from an edited one without diffing by hand. `key` is
 *  excluded — it's session-local and moving a folder doesn't change it. */
export function isSameCollection(a: CollectionFormState, b: CollectionFormState): boolean {
  return JSON.stringify(toCollectionPayload(a)) === JSON.stringify(toCollectionPayload(b))
}

/**
 * The form's own state as a previewable collection, for the editor's Preview
 * panel.
 *
 * A folder's identity here is its form `key`, not its server `id`: a folder
 * that hasn't been saved yet has no id, and `CollectionMeta` still has to be
 * able to count it.
 *
 * An id `optionByID` can't resolve becomes an unresolved source — the same
 * shape Home uses for a catalog that's since been deleted. In this form that
 * state is also a validation error, so the form names it on the folder; the
 * preview only has to avoid claiming content that isn't there.
 */
export function previewFromForm(
  state: CollectionFormState,
  optionByID: ReadonlyMap<string, RefOption>,
): PreviewCollection {
  const folders: PreviewFolder[] = state.folders.map((folder) => {
    const sources: PreviewSource[] = folder.refs.map((ref) => {
      const option = optionByID.get(ref.catalogID)
      return {
        // The ref's own key, not the catalog/genre pair: the form can briefly
        // hold a repeated pair, which is a validation error rather than a
        // state the preview may collide on.
        key: ref.key,
        name: option?.name ?? null,
        type: option?.catalog.type ?? null,
        params: option?.catalog.params ?? '',
        genre: ref.genre,
      }
    })

    return {
      id: folder.key,
      title: folder.title,
      hideTitle: folder.hideTitle,
      tileShape: folder.tileShape,
      coverEmoji: folder.coverEmoji,
      coverImageUrl: folder.coverImageURL,
      sources,
      unresolved: sources.filter((s) => s.name === null).length,
    }
  })

  return {
    // Never rendered — the row's identity is the form, not a stored row.
    id: '',
    title: state.title,
    // Show first is Home's, pushed from there; this preview draws the
    // collection's own layout, which it doesn't change.
    pinned: false,
    viewMode: state.viewMode,
    showAllTab: state.showAllTab,
    folders,
    // The form is the description, so there is always something to draw.
    missing: false,
  }
}

/** What one folder entry is in Nuvio, by the collection's view mode: a tab in
 *  a `TABBED_GRID` folder page, a row in a `ROWS` one. */
export type FolderUnit = 'tab' | 'row'

export function folderUnit(viewMode: ViewMode): FolderUnit {
  if (viewMode === 'ROWS') return 'row'
  return 'tab'
}

/** One catalog's refs in a folder, in their order: the folder editor's line
 *  for that catalog, one Nuvio tab or row per ref. */
export interface RefGroup {
  catalogID: string
  refs: FolderRefState[]
}

/** `refs` gathered per catalog, each catalog where it first appears. */
export function refGroups(refs: FolderRefState[]): RefGroup[] {
  const groups = new Map<string, RefGroup>()
  for (const ref of refs) {
    const group = groups.get(ref.catalogID)
    if (group) group.refs.push(ref)
    else groups.set(ref.catalogID, { catalogID: ref.catalogID, refs: [ref] })
  }
  return [...groups.values()]
}

/** `groups` back to one ordered ref list. */
export function flatRefs(groups: RefGroup[]): FolderRefState[] {
  const refs: FolderRefState[] = []
  for (const group of groups) refs.push(...group.refs)
  return refs
}

/** `refs` with each catalog's refs side by side, where that catalog first
 *  appears. A folder loads this way, and every edit keeps it so, because the
 *  editor draws a catalog once with its genres under it. */
export function groupedByCatalog(refs: FolderRefState[]): FolderRefState[] {
  return flatRefs(refGroups(refs))
}

/** A saved folder as the editor holds it: each catalog's refs side by side,
 *  where that catalog first appears, so a folder saved interleaved is drawn
 *  as one line per catalog and stored grouped by the next Save. */
function groupedFolderFromWire(folder: Folder): FolderFormState {
  const form = folderFromWire(folder)
  return { ...form, refs: groupedByCatalog(form.refs) }
}
