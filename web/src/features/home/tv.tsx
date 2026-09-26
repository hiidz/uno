import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Tabs } from 'radix-ui'
import { ArrowLeft, ChevronLeft, ChevronRight } from 'lucide-react'
import { tmdbKind } from '@/api'
import type { PreviewItem, TMDBKind } from '@/api'
import { Icon } from '@/components/Icon'
import {
  ALL_TAB,
  ALL_TAB_TILE_CAP,
  folderTabs,
  sourceLabel,
  interleaveTiles,
  type PreviewCollection,
  type PreviewFolder,
  type PreviewSource,
} from '@/features/preview/model'
import {
  TILES_PER_PAGE,
  noTiles,
  resolveTileKind,
  type CatalogTiles,
  type TileKind,
} from '@/features/preview/tiles'
import type { FolderPageTarget, PreviewRow } from './preview'

/**
 * The Home pane's TV preview: a framed 16:9 picture in Roboto, DESIGN.md's
 * "TV preview". Deliberately its own component tree, not a restyle of
 * `features/preview/tiles.tsx`/`FolderPage.tsx` — those stay in Uno's own
 * Figtree for their unframed consumers, and `cqw` sizing only means anything
 * against `.tv-bezel`'s own `container-type`, so sharing the classes across a
 * non-framed consumer would silently fall back to viewport units there. The
 * collection editor's docked row is framed, so it draws with these.
 *
 * **Uno pins nothing onto the screen — the Clean Preview amendment.** No
 * shuffled-row note, no stopped-sharing note (that warning stays in the
 * library), no All-tab merge-order caveat, no "follows the app" caveat. An
 * unresolved source or an empty result just renders as an empty row, the way
 * the real TV would show it, rather than an explanation Uno pins onto the
 * picture. The one caveat kept — placeholder tiles behind a poster that
 * hasn't loaded yet, or is missing — is the tone cycle below, not text.
 */

const TILE_TONES = ['bg-tile-a', 'bg-tile-b', 'bg-tile-c'] as const

function tone(index: number): string {
  return TILE_TONES[index % TILE_TONES.length]
}

/**
 * One real title. A flat tone (cycled a/b/c along the strip) shows through
 * until the poster loads, or in its place if there is none — DESIGN.md's
 * accepted placeholder treatment, not a Uno note.
 *
 * Linked when the caller knows the kind, exactly like the workspace's own
 * `ContentTile` — opens in a new tab so it doesn't discard the pane's state.
 */
function TVPosterTile({
  item,
  kind,
  toneIndex,
}: {
  item: PreviewItem
  kind?: TileKind
  toneIndex: number
}) {
  const resolvedKind = resolveTileKind(kind, item)
  const name = item.year ? `${item.title} (${item.year})` : item.title
  const face = (
    <>
      <span className={`art ${tone(toneIndex)}`}>
        {item.poster && <img src={item.poster} alt="" loading="lazy" />}
      </span>
      <span className="cap">{item.title}</span>
      {item.year && <span className="cap cap-year">{item.year}</span>}
    </>
  )

  if (!resolvedKind) {
    return (
      <span title={name} className="tv-tile tv-poster">
        {face}
      </span>
    )
  }

  return (
    <a
      href={`https://www.themoviedb.org/${resolvedKind}/${item.tmdb_id}`}
      target="_blank"
      rel="noopener noreferrer"
      title={`${name} — open on TMDB`}
      className="tv-tile tv-poster"
    >
      {face}
    </a>
  )
}

/** The loading placeholder — a bare toned tile, no caption. Count matches one
 *  TMDB page so the row doesn't reflow when real tiles land. */
function TVPlaceholderTile({ toneIndex }: { toneIndex: number }) {
  return (
    <span className="tv-tile tv-poster">
      <span className={`art ${tone(toneIndex)}`} />
    </span>
  )
}

/**
 * A row's own scroll container and the two buttons that drive it — the "see
 * more" affordance a real row carries, not a Uno addition. A whole TMDB page
 * is fetched either way (`useCatalogTiles`); this is what makes every one of
 * those tiles reachable instead of the row silently clipping whatever didn't
 * fit the frame.
 *
 * **Swiping is touch-only, and native.** `overflow-x` on `.tv-strip` is the
 * whole of it: a finger swipes the row like any native scroller. A mouse gets
 * no drag gesture — pressing on a poster link or cover image starts the
 * browser's own drag of that link or image — so a mouse scrolls a row with
 * the chevrons, a horizontal wheel or trackpad, or Shift+wheel.
 *
 * `canPrev`/`canNext` come from the scroll container's own position
 * (`scrollLeft` against `scrollWidth`/`clientWidth`), tracked via a `scroll`
 * listener and a `ResizeObserver` — the latter because tiles arrive after
 * the row first mounts and widen `scrollWidth` with no `scroll` event of
 * their own.
 */
function useRowScroll() {
  const ref = useRef<HTMLDivElement>(null)
  const [bounds, setBounds] = useState({ canPrev: false, canNext: false })

  const measure = useCallback(() => {
    const el = ref.current
    if (!el) return
    const canPrev = el.scrollLeft > 1
    const canNext = el.scrollLeft < el.scrollWidth - el.clientWidth - 1
    // Returning `prev` untouched lets React skip the render mid-scroll.
    setBounds((prev) =>
      prev.canPrev === canPrev && prev.canNext === canNext ? prev : { canPrev, canNext },
    )
  }, [])

  useEffect(() => {
    const el = ref.current
    if (!el) return
    measure()
    el.addEventListener('scroll', measure, { passive: true })
    const ro = new ResizeObserver(measure)
    ro.observe(el)
    return () => {
      el.removeEventListener('scroll', measure)
      ro.disconnect()
    }
  }, [measure])

  const scrollBy = useCallback((direction: -1 | 1) => {
    const el = ref.current
    if (!el) return
    // Past either end this is a no-op — `scrollBy` clamps rather than wraps.
    el.scrollBy({ left: direction * el.clientWidth * 0.85, behavior: 'smooth' })
  }, [])

  return { stripRef: ref, ...bounds, scrollBy }
}

function RowButton({
  direction,
  onClick,
  label,
}: {
  direction: -1 | 1
  onClick: () => void
  label: string
}) {
  return (
    <button type="button" onClick={onClick} aria-label={label} className="tv-row-btn">
      <Icon icon={direction === 1 ? ChevronRight : ChevronLeft} />
    </button>
  )
}

/** The pair of scroll buttons beside a row's title, each rendered only while
 *  it would do something — a row at its start shows no dead "back" button,
 *  matching the chevron a real row's title carries. */
function RowNav({
  canPrev,
  canNext,
  onScroll,
  name,
}: {
  canPrev: boolean
  canNext: boolean
  onScroll: (direction: -1 | 1) => void
  name: string
}) {
  if (!canPrev && !canNext) return null
  return (
    <div className="tv-row-nav">
      {canPrev && <RowButton direction={-1} onClick={() => onScroll(-1)} label={`Scroll ${name} back`} />}
      {canNext && <RowButton direction={1} onClick={() => onScroll(1)} label={`See more of ${name}`} />}
    </div>
  )
}

/**
 * The shared body of a strip or grid: real tiles once they land, toned
 * placeholders until then. A settled, empty result draws nothing at all — on
 * the real TV an empty catalog is an empty row, not an explanation.
 */
function TVTiles({
  tiles,
  kind,
  gridClassName,
  scroll,
}: {
  tiles: CatalogTiles
  kind?: TileKind
  /** `fp-grid` inside a folder page's tabbed view; omitted draws a strip. */
  gridClassName?: string
  /** The strip's own scroll container. Only ever passed
   *  alongside a strip — a grid doesn't scroll sideways. */
  scroll?: ReturnType<typeof useRowScroll>
}) {
  const className = gridClassName ?? 'tv-strip'

  if (!tiles.isLoading && !tiles.isError && tiles.items.length === 0) return null

  if (tiles.items.length === 0) {
    return (
      <div ref={scroll?.stripRef} aria-hidden="true" className={className}>
        {Array.from({ length: TILES_PER_PAGE }, (_, i) => (
          <TVPlaceholderTile key={i} toneIndex={i} />
        ))}
      </div>
    )
  }

  return (
    <div ref={scroll?.stripRef} className={className}>
      {tiles.items.map((item, i) => (
        <TVPosterTile
          key={`${resolveTileKind(kind, item)}:${item.tmdb_id}`}
          item={item}
          kind={kind}
          toneIndex={i}
        />
      ))}
    </div>
  )
}

/**
 * A folder, drawn as a tile in its collection's row — cover emoji, then the
 * folder's cover art over it once loaded, in its own `tile_shape`. Always
 * Placeholder Tile C underneath, per DESIGN.md's `tv-folder-tile` token —
 * unlike a poster strip, a folder tile's tone isn't cycled.
 */
function TVFolderTile({ folder, onOpen }: { folder: PreviewFolder; onOpen: () => void }) {
  const shapeClass =
    folder.tileShape === 'LANDSCAPE' ? 'landscape' : folder.tileShape === 'SQUARE' ? 'square' : 'poster'
  const name = folder.title || 'Untitled folder'

  const face = (
    <>
      <span className="art">
        {folder.coverEmoji && <span aria-hidden="true">{folder.coverEmoji}</span>}
        {folder.coverImageUrl && <img src={folder.coverImageUrl} alt="" loading="lazy" />}
      </span>
      {/* `hide_title` hides the tile's own caption on the real TV too — the
          slot stays, empty, so the row's baseline doesn't jump between
          folders that hide their title and folders that don't. */}
      {folder.hideTitle ? (
        <span className="cap is-hidden" aria-hidden="true">
          ·
        </span>
      ) : (
        <span className="cap">{name}</span>
      )}
    </>
  )

  return (
    <button
      type="button"
      onClick={onOpen}
      aria-label={`Open ${name}`}
      className={`tv-tile tv-folder ${shapeClass}`}
    >
      {face}
    </button>
  )
}

/** A collection's row on the TV: its tiles are its folders, never catalog
 *  content — a folder with no catalogs, or a collection with no folders,
 *  simply draws an empty strip, matching a real empty row rather than
 *  explaining itself. */
export function TVCollectionRow({
  collection,
  onOpenFolder,
}: {
  collection: PreviewCollection
  onOpenFolder: (target: FolderPageTarget) => void
}) {
  const scroll = useRowScroll()
  return (
    <div className="tv-band">
      <div className="tv-row-head">
        <p className="tv-row-title">{collection.title}</p>
        <RowNav
          canPrev={scroll.canPrev}
          canNext={scroll.canNext}
          onScroll={scroll.scrollBy}
          name={collection.title}
        />
      </div>
      <div ref={scroll?.stripRef} className="tv-strip">
        {collection.folders.map((folder) => (
          <TVFolderTile
            key={folder.id}
            folder={folder}
            onOpen={() => onOpenFolder({ collectionId: collection.id, folderId: folder.id })}
          />
        ))}
      </div>
    </div>
  )
}

export function TVCatalogRow({ row, tiles }: { row: PreviewRow; tiles: CatalogTiles }) {
  const scroll = useRowScroll()
  return (
    <div className="tv-band">
      <div className="tv-row-head">
        <p className="tv-row-title">{row.name}</p>
        <RowNav canPrev={scroll.canPrev} canNext={scroll.canNext} onScroll={scroll.scrollBy} name={row.name} />
      </div>
      <TVTiles tiles={tiles} kind={tmdbKind(row.type)} scroll={scroll} />
    </div>
  )
}

/**
 * A folder's page, opened from its tile — DESIGN.md's "TV folder page
 * (signature)". `view_mode` decides the body: `TABBED_GRID` and
 * `FOLLOW_LAYOUT` both render as tabs (the latter forcing the "All" tab
 * first, Nuvio's own default, per the Follows the app's layout amendment);
 * `ROWS` stacks every catalog as its own row.
 *
 * The back arrow lives beside the folder's own title, inside the screen —
 * the Back Like the Remote amendment. `onBack` is the one way this asks to
 * close; Escape and the browser's own Back are wired by the caller, which
 * also owns the history entry that makes Back work at all.
 */
export function TVFolderPage({
  collection,
  folder,
  tiles,
  onBack,
  backLabel = 'Back to the home screen',
}: {
  collection: PreviewCollection
  folder: PreviewFolder
  tiles: ReadonlyMap<string, CatalogTiles>
  onBack: () => void
  /** The back arrow's accessible name — where `onBack` actually lands. */
  backLabel?: string
}) {
  const name = folder.title || 'Untitled folder'
  const tabbed = collection.viewMode === 'TABBED_GRID' || collection.viewMode === 'FOLLOW_LAYOUT'

  return (
    <>
      <p className="fp-title">
        <button
          type="button"
          className="fp-back"
          onClick={onBack}
          aria-label={backLabel}
        >
          <Icon icon={ArrowLeft} />
        </button>
        {folder.coverEmoji && <span aria-hidden="true">{folder.coverEmoji} </span>}
        {name}
      </p>
      {folder.sources.length > 0 &&
        (tabbed ? (
          <TVTabbedCatalogs
            folder={folder}
            showAllTab={collection.viewMode === 'FOLLOW_LAYOUT' ? true : collection.showAllTab}
            tiles={tiles}
          />
        ) : (
          <TVRowsInFolder folder={folder} tiles={tiles} />
        ))}
    </>
  )
}

function TVTabbedCatalogs({
  folder,
  showAllTab,
  tiles,
}: {
  folder: PreviewFolder
  showAllTab: boolean
  tiles: ReadonlyMap<string, CatalogTiles>
}) {
  const tabs = useMemo(() => folderTabs(folder, showAllTab), [folder, showAllTab])
  const [openKey, setOpenKey] = useState(tabs[0]?.key ?? ALL_TAB)
  // A catalog removed from the folder while this tab is open must not leave
  // this pointing at a tab that no longer exists.
  const open = tabs.some((t) => t.key === openKey) ? openKey : (tabs[0]?.key ?? ALL_TAB)

  const source = folder.sources.find((s) => s.key === open)

  const [allTiles, allKinds] = useMemo((): [CatalogTiles, Map<PreviewItem, TMDBKind>] => {
    const perSource = folder.sources.map((s) => ({
      kind: s.type ? tmdbKind(s.type) : undefined,
      items: tiles.get(s.key)?.items ?? [],
    }))
    const loaded = folder.sources.map((s) => tiles.get(s.key)).filter((t) => t !== undefined)
    const merged = interleaveTiles(perSource, ALL_TAB_TILE_CAP)
    const all: CatalogTiles = {
      items: merged.items,
      randomized: loaded.some((t) => t.randomized),
      isLoading: loaded.some((t) => t.isLoading),
      isError: loaded.length > 0 && loaded.every((t) => t.isError),
    }
    return [all, merged.kinds]
  }, [folder.sources, tiles])

  return (
    <Tabs.Root value={open} onValueChange={setOpenKey}>
      {tabs.length > 1 && (
        <Tabs.List className="fp-tabs" aria-label={`Tabs in ${folder.title || 'this folder'}`}>
          {tabs.map((tab) => (
            <Tabs.Trigger key={tab.key} value={tab.key} className="fp-tab">
              {tab.label}
            </Tabs.Trigger>
          ))}
        </Tabs.List>
      )}
      {source ? (
        <TVTiles
          tiles={tiles.get(source.key) ?? noTiles()}
          kind={source.type ? tmdbKind(source.type) : undefined}
          gridClassName="fp-grid"
        />
      ) : (
        <TVTiles
          tiles={allTiles}
          kind={(item) => allKinds.get(item)}
          gridClassName="fp-grid"
        />
      )}
    </Tabs.Root>
  )
}

function TVRowsInFolder({
  folder,
  tiles,
}: {
  folder: PreviewFolder
  tiles: ReadonlyMap<string, CatalogTiles>
}) {
  return (
    <>
      {folder.sources.map((source) => (
        <TVFolderSourceRow key={source.key} source={source} tiles={tiles.get(source.key) ?? noTiles()} />
      ))}
    </>
  )
}

function TVFolderSourceRow({
  source,
  tiles,
}: {
  source: PreviewSource
  tiles: CatalogTiles
}) {
  const scroll = useRowScroll()
  const name = sourceLabel(source)
  return (
    <div className="tv-band">
      <div className="tv-row-head">
        <p className="tv-row-title">{name}</p>
        <RowNav canPrev={scroll.canPrev} canNext={scroll.canNext} onScroll={scroll.scrollBy} name={name} />
      </div>
      <TVTiles tiles={tiles} kind={source.type ? tmdbKind(source.type) : undefined} scroll={scroll} />
    </div>
  )
}

/** The caption above the frame, in Uno's own Figtree, and the layout name
 *  beside it. Outside the screen, so it's Uno's own words about the
 *  picture, not pinned onto it. */
export const FOLDER_LAYOUT_LABEL: Record<PreviewCollection['viewMode'], string> = {
  TABBED_GRID: 'Tabbed grids, one tab per catalog',
  ROWS: 'Rows, one per catalog',
  FOLLOW_LAYOUT: "Follows the app's layout",
}
