import type { CatalogType, PreviewItem, TileShape, TMDBKind } from '@/api'

/**
 * The shape of a collection, its folders, and the catalogs they reference —
 * the structure every preview renders, independent of where it was read from.
 *
 * Two sources build these: the Home pane, from a saved `Collection` on the
 * selection response, and the collection builder, from the form state of a
 * collection that may not exist yet. Both then render the same components, so
 * the two previews can't drift apart in what they claim a layout will do.
 *
 * **The screen this models has two levels.** A collection is **one row** on
 * home, and its tiles are its **folders** — drawn from folder metadata (cover
 * emoji, title, `tile_shape`), never from catalog content. Clicking a folder
 * tile opens *a folder page*, scoped to that one folder, where the collection's
 * `view_mode` decides how the folder's **catalogs** are laid out: `TABBED_GRID`
 * gives one tab per catalog (plus "All" when `show_all_tab`) over a grid;
 * `ROWS` stacks one row per catalog, the same shape as home.
 */

/**
 * `collections.view_mode` is a bare `string` on the wire, not an enum, so an
 * unrecognised value is representable and has to be handled rather than cast.
 * `FOLLOW_LAYOUT` means "use whatever the app is globally set to" — a value
 * Uno cannot know, so the renderer picks a shape and says it is guessing.
 *
 * It is a **collection-level** setting that applies to every folder in the
 * collection; it describes how a folder's catalogs are laid out once you're
 * inside it, not how the collection itself sits on home.
 */
export type PreviewViewMode = 'TABBED_GRID' | 'ROWS' | 'FOLLOW_LAYOUT'

/** One catalog referenced by a folder. This is the folder page's content spine:
 *  each source is a row (`ROWS`) or a tab (`TABBED_GRID`). `name === null`
 *  means the reference couldn't be resolved — see `unresolved` below. */
export interface PreviewSource {
  /** Unique within its folder — the tab value and the key its tiles are filed
   *  under. Not the catalog id: one catalog can be two sources in a folder,
   *  under two genres. */
  key: string
  /** The referenced catalog's id. */
  id: string
  name: string | null
  type: CatalogType | null
  /** The recipe to run for this source's tiles, or `''` when unresolved. */
  params: string
  /** The genre this reference is narrowed to, or `''` when unfiltered. */
  genre: string
}

export interface PreviewFolder {
  id: string
  title: string
  /** `hide_title` suppresses the title text under the folder's **tile** — the
   *  literal reading of Nuvio's own field description ("Hide the tile title
   *  text"). The folder still has a title, and preview still needs to name it
   *  somewhere, so the renderer moves it into the tile's accessible name
   *  rather than dropping it. */
  hideTitle: boolean
  /** The shape of this folder's own tile in its collection's row. Folders in
   *  one collection can disagree, so a row can be ragged — drawn as-is, since
   *  the raggedness is the thing being previewed. */
  tileShape: TileShape
  /** True when the wire value was `''` and `POSTER` was assumed. */
  tileShapeAssumed: boolean
  coverEmoji: string
  /** The folder tile's cover art, loaded for real. Empty string when unset, in
   *  which case the tile falls back to the cover emoji, then to the title. */
  coverImageUrl: string
  sources: PreviewSource[]
  /** How many of `sources` couldn't be resolved to a catalog. A folder can
   *  reference a catalog that has since been deleted. */
  unresolved: number
}

export interface PreviewCollection {
  id: string
  title: string
  /** `pin_to_top` — hoists the whole collection row to the top of **home**,
   *  above the catalog rows, not merely to the front of the collections. */
  pinned: boolean
  viewMode: PreviewViewMode
  /** True when `view_mode` was empty or a value this build doesn't know. */
  viewModeAssumed: boolean
  /** Adds an "All" tab to a `TABBED_GRID` folder page, merging every catalog in
   *  the folder into one grid. Meaningless in `ROWS`, where every catalog is
   *  already on the page. */
  showAllTab: boolean
  /** `backdrop_image_url` is set. Noted as text, not loaded. */
  hasBackdrop: boolean
  folders: PreviewFolder[]
  /** Nothing on hand describes this collection — it's in neither the library
   *  nor the selection response, so there's no layout to draw. The row exists
   *  because it's still selected.
   *
   *  **Not the same as detached.** A collection that's left the library but is
   *  still on the selection response resolves here and renders normally. That
   *  case is `isDetached` on the Home selection, and the renderer asks it
   *  separately — see `DetachedNote` in `HomePreview.tsx`.
   *
   *  Never true for a collection built from builder form state: the form *is*
   *  the description. */
  missing: boolean
}

export function normalizeTileShape(shape: TileShape | ''): {
  shape: TileShape
  assumed: boolean
} {
  // The one real `collections_json` sample sets `tileShape` explicitly on every
  // folder, so `''` is untested on the Nuvio side. Poster is the safer guess,
  // and the renderer marks it as a guess rather than presenting it as fact.
  if (shape === 'POSTER' || shape === 'LANDSCAPE' || shape === 'SQUARE') {
    return { shape, assumed: false }
  }
  return { shape: 'POSTER', assumed: true }
}

export function normalizeViewMode(mode: string): {
  mode: PreviewViewMode
  assumed: boolean
} {
  if (mode === 'TABBED_GRID' || mode === 'ROWS') return { mode, assumed: false }
  // `FOLLOW_LAYOUT` and an empty/unknown value land in the same place: Uno
  // can't read the app's global layout setting, so it picks one and flags it.
  return { mode: 'FOLLOW_LAYOUT', assumed: true }
}

const KIND_LABEL: Record<CatalogType, string> = { movie: 'Movie', series: 'Series' }

/**
 * A folder source's name as Nuvio shows it on a folder page, as a tab and as a
 * row title alike: `<Catalog name> (<Kind>)`, then ` • <Genre>` when the
 * reference is narrowed to one. The genre is what tells two sources of the
 * same catalog apart.
 */
export function sourceLabel(source: PreviewSource): string {
  if (source.name === null || source.type === null) return 'Unavailable catalog'
  const base = `${source.name} (${KIND_LABEL[source.type]})`
  return source.genre ? `${base} • ${source.genre}` : base
}

/**
 * The tabs a `TABBED_GRID` folder page shows: **one per source in the folder**,
 * with the synthetic "All" tab in front when the collection sets
 * `show_all_tab`. The tabs are per source, not per folder — `view_mode`
 * describes how a folder's catalogs are presented once you're inside it.
 */
export function folderTabs(
  folder: PreviewFolder,
  showAllTab: boolean,
): { key: string; label: string }[] {
  const tabs = folder.sources.map((source) => ({
    key: source.key,
    label: sourceLabel(source),
  }))
  return showAllTab ? [{ key: ALL_TAB, label: 'All' }, ...tabs] : tabs
}

/** The synthetic "All" tab's key. The only view in the preview that merges more
 *  than one catalog, so the only place Nuvio's unknown merge order matters. */
export const ALL_TAB = '__all__'

/**
 * How many tiles the "All" tab renders.
 *
 * Every other view is naturally bounded at one TMDB page (20). All is not — a
 * folder can carry 8+ sources, which is 160 tiles and 160 remote images in
 * one grid.
 */
export const ALL_TAB_TILE_CAP = 20

/**
 * Merges each source's tiles round-robin and caps the result.
 *
 * Round-robin rather than concatenation so the cap doesn't turn "everything in
 * this folder" into "the first catalog in this folder" — with 8 sources and a
 * cap of 20, concatenating would show source 1 and nothing else.
 *
 * This ordering is Uno's own guess: folders aren't an addon concept, so nothing
 * specifies how Nuvio merges a folder's sources. The UI has to label it as a
 * guess wherever it renders.
 *
 * De-duplicates on kind and `tmdb_id` together: two catalogs in one folder can
 * surface the same title (overlapping filters), and the same poster twice in
 * one grid reads as a rendering bug rather than as two sources agreeing. The
 * kind is part of the key because TMDB numbers movies and TV separately, so a
 * movie and a series can share an id and still be different titles.
 *
 * The merge can span sources of different `CatalogType`, so the output has no
 * single kind to hand `TileGrid`; `kinds` carries each merged item's kind
 * instead.
 */
export function interleaveTiles(
  perSource: readonly { kind: TMDBKind | undefined; items: readonly PreviewItem[] }[],
  cap: number = ALL_TAB_TILE_CAP,
): { items: PreviewItem[]; kinds: Map<PreviewItem, TMDBKind> } {
  const items: PreviewItem[] = []
  const kinds = new Map<PreviewItem, TMDBKind>()
  const seen = new Set<string>()
  const deepest = perSource.reduce((n, source) => Math.max(n, source.items.length), 0)

  for (let round = 0; round < deepest && items.length < cap; round++) {
    for (const source of perSource) {
      if (items.length >= cap) break
      const item = source.items[round]
      if (!item) continue
      const key = `${source.kind}:${item.tmdb_id}`
      if (seen.has(key)) continue
      seen.add(key)
      items.push(item)
      if (source.kind) kinds.set(item, source.kind)
    }
  }
  return { items, kinds }
}

/** The recipes a folder's sources need tiles for, filed under each source's
 *  `key`. Unresolved sources have no recipe to run, so they're dropped rather
 *  than queried. */
export function folderRecipes(
  folder: PreviewFolder,
): { id: string; type: CatalogType; params: string; genre: string }[] {
  return folder.sources
    .filter((source) => source.type !== null)
    .map((source) => ({ id: source.key, type: source.type!, params: source.params, genre: source.genre }))
}
