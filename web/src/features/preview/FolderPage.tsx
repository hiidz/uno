import { useMemo, useState } from 'react'
import type { ReactNode } from 'react'
import { tmdbKind } from '@/api'
import type { PreviewItem, TMDBKind } from '@/api'
import { plural } from '@/lib/plural'
import {
  ALL_TAB,
  ALL_TAB_TILE_CAP,
  folderTabs,
  sourceLabel,
  interleaveTiles,
  type PreviewCollection,
  type PreviewFolder,
  type PreviewSource,
} from './model'
import { CONTENT_TILE_SHAPE, Note, TileGrid, TilesNote, TileStrip, noTiles } from './tiles'
import type { CatalogTiles } from './tiles'

/**
 * What a folder tile opens: one folder, and the catalogs inside it.
 *
 * `view_mode` is a **collection-level** setting that applies to every folder in
 * it, and it governs this page only — `TABBED_GRID` gives one tab per catalog
 * over a grid, `ROWS` stacks one row per catalog the way home does.
 *
 * Sibling folders are absent: the folder tile you pressed is the whole subject
 * of this page, in both modes.
 *
 * Shared by the Home pane's Preview view and the collection builder. The two
 * differ only in what they can say *about* a catalog — Home knows whether a
 * selected row has since left the library — so that goes through `chrome`
 * rather than being read from Home state here.
 */
export interface PreviewChrome {
  /** An extra note beside a catalog's name. Home uses it to mark a catalog that
   *  has left the library; the builder has nothing to add and omits it. */
  note?: (catalogID: string) => ReactNode
}

export function FolderPage({
  collection,
  folder,
  tiles,
  chrome,
  onBack,
  backLabel = '← Home',
}: {
  collection: PreviewCollection
  folder: PreviewFolder
  /** Tiles for every source in the folder, keyed by `PreviewSource.key`. Both layouts
   *  need the same set — `ROWS` draws them all at once, `TABBED_GRID` shows one
   *  at a time but its "All" tab spans the lot — so the caller fetches once and
   *  switching tabs never triggers a new call. */
  tiles: ReadonlyMap<string, CatalogTiles>
  chrome: PreviewChrome
  onBack: () => void
  backLabel?: string
}) {
  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center gap-2.5">
        <button
          type="button"
          onClick={onBack}
          className="type-data border-line-hi text-dim hover:text-ink hover:border-dim rounded-[2px] border px-2.5 py-1 text-[10px] tracking-[0.05em] uppercase transition-colors pointer-coarse:py-2"
        >
          {backLabel}
        </button>
        <span className="type-data text-dimmer text-[10px]">
          {collection.title || 'Untitled collection'} /{' '}
          <span className="text-dim">{folder.title || 'Untitled folder'}</span>
        </span>
        {folder.hideTitle && <Note>title hidden on the tile</Note>}
      </div>

      {folder.sources.length === 0 ? (
        <p className="type-data text-dimmer m-0 text-[11px]">
          This folder has no catalogs, so it opens empty.
        </p>
      ) : collection.viewMode === 'TABBED_GRID' ? (
        <TabbedCatalogs
          folder={folder}
          showAllTab={collection.showAllTab}
          tiles={tiles}
          chrome={chrome}
        />
      ) : (
        <CatalogRowsInFolder folder={folder} tiles={tiles} chrome={chrome} />
      )}

      {collection.viewModeAssumed && folder.sources.length > 0 && (
        <p className="type-data text-dimmer m-0 text-[10px]">
          Shown as rows — the app decides the real layout.
        </p>
      )}
    </div>
  )
}

/**
 * `TABBED_GRID`: one tab per **catalog in the folder**, over a grid of that
 * catalog's content. The tabs are per catalog, not per folder — `view_mode`
 * describes how a folder's catalogs are presented once you are inside it.
 */
function TabbedCatalogs({
  folder,
  showAllTab,
  tiles,
  chrome,
}: {
  folder: PreviewFolder
  showAllTab: boolean
  tiles: ReadonlyMap<string, CatalogTiles>
  chrome: PreviewChrome
}) {
  const tabs = folderTabs(folder, showAllTab)
  const [openKey, setOpenKey] = useState(tabs[0]?.key ?? ALL_TAB)
  // A catalog removed from the folder while this is open must not leave this
  // pointing at a tab that no longer exists.
  const open = tabs.some((t) => t.key === openKey) ? openKey : (tabs[0]?.key ?? ALL_TAB)

  const source = folder.sources.find((s) => s.key === open)
  const allUnresolved = folder.sources.length > 0 && folder.unresolved === folder.sources.length

  // The All tab's own tiles: every source's page, merged round-robin and
  // capped. Nothing specifies how Nuvio itself merges a folder's sources, so
  // this order is a guess and is labelled as such below.
  const [allTiles, allKinds] = useMemo((): [CatalogTiles, Map<PreviewItem, TMDBKind>] => {
    const perSource = folder.sources.map((s) => ({
      kind: s.type ? tmdbKind(s.type) : undefined,
      items: tiles.get(s.key)?.items ?? [],
    }))
    const loaded = folder.sources.map((s) => tiles.get(s.key)).filter((t) => t !== undefined)
    const merged = interleaveTiles(perSource, ALL_TAB_TILE_CAP)
    const all: CatalogTiles = {
      items: merged.items,
      // A shuffling source anywhere in the folder makes the merged view a
      // sample too, so the caveat has to propagate rather than be per-tab.
      randomized: loaded.some((t) => t.randomized),
      isLoading: loaded.some((t) => t.isLoading),
      // Only a total failure counts: with one source down out of eight the tab
      // still has content, and "couldn't load" over a full grid is wrong.
      isError: loaded.length > 0 && loaded.every((t) => t.isError),
    }
    return [all, merged.kinds]
  }, [folder.sources, tiles])

  return (
    <div className="flex flex-col gap-3">
      {tabs.length > 1 && (
        <div className="border-line flex gap-1 overflow-x-auto overscroll-x-contain border-b pb-2">
          {tabs.map((tab) => (
            <Tab
              key={tab.key}
              label={tab.label}
              active={tab.key === open}
              onClick={() => setOpenKey(tab.key)}
            />
          ))}
        </div>
      )}

      {source ? (
        <>
          <SourceHeading source={source} tiles={tiles.get(source.key)} chrome={chrome} />
          {source.name === null ? (
            <UnresolvedSource />
          ) : (
            <TileGrid
              shape={CONTENT_TILE_SHAPE}
              tiles={tiles.get(source.key) ?? noTiles()}
              kind={source.type ? tmdbKind(source.type) : undefined}
            />
          )}
        </>
      ) : allUnresolved ? (
        // Nothing in the folder resolves, so there is nothing to merge and a
        // grid here would claim content the folder doesn't have.
        <UnresolvedSource all />
      ) : (
        <>
          {/* The All tab has no source heading to hang `TilesNote` off, so it
              sits above the grid — otherwise a folder whose sources all return
              nothing renders as unexplained blank space. */}
          <div className="flex items-center gap-2.5">
            <span className="type-eyebrow">Everything in this folder</span>
            <TilesNote tiles={allTiles} />
          </div>
          {/* The merge interleaves sources that can be a mix of movies and
              series, so `kind` is a per-item lookup rather than one value
              applied to every tile in the grid. */}
          <TileGrid
            shape={CONTENT_TILE_SHAPE}
            tiles={allTiles}
            kind={(item) => allKinds.get(item)}
          />
          {/* The one place in the preview that merges more than one catalog,
              and Nuvio's merge order for a folder is unspecified (folders
              aren't an addon concept), so the caveat is stated outright. */}
          <p className="type-data text-dimmer m-0 text-[10px]">
            A sample from {folder.sources.length} {plural(folder.sources.length, 'catalog')}
            {folder.unresolved > 0 && `, ${folder.unresolved} of them unavailable`} · your TV may
            order these differently
            {allTiles.randomized && ' · one of them shuffles, so it will differ'}
          </p>
        </>
      )}
    </div>
  )
}

/** `ROWS`: every catalog in the folder as its own row, the same shape home
 *  uses. Each row is exactly one catalog, so nothing here is merged. */
function CatalogRowsInFolder({
  folder,
  tiles,
  chrome,
}: {
  folder: PreviewFolder
  tiles: ReadonlyMap<string, CatalogTiles>
  chrome: PreviewChrome
}) {
  return (
    <div className="flex flex-col gap-6">
      {folder.sources.map((source) => (
        <section key={source.key} className="flex flex-col gap-2">
          <SourceHeading source={source} tiles={tiles.get(source.key)} chrome={chrome} />
          {source.name === null ? (
            <UnresolvedSource />
          ) : (
            <TileStrip
              shape={CONTENT_TILE_SHAPE}
              tiles={tiles.get(source.key) ?? noTiles()}
              kind={source.type ? tmdbKind(source.type) : undefined}
            />
          )}
        </section>
      ))}
    </div>
  )
}

function SourceHeading({
  source,
  tiles,
  chrome,
}: {
  source: PreviewSource
  tiles?: CatalogTiles
  chrome: PreviewChrome
}) {
  return (
    <div className="flex flex-wrap items-center gap-x-2.5 gap-y-1">
      <h3 className="m-0 min-w-0 truncate text-[13px] font-medium">
        {sourceLabel(source)}
      </h3>
      {chrome.note?.(source.id)}
      {/* An unresolvable source has no recipe to run, so it has no tile state
          either — `UnresolvedSource` below says what happened instead. */}
      {source.name !== null && tiles && <TilesNote tiles={tiles} />}
    </div>
  )
}

/** A folder can reference a catalog that has since been deleted. The
 *  reference keeps its slot and says what happened rather than vanishing,
 *  matching how the List view draws a detached row. */
function UnresolvedSource({ all }: { all?: boolean }) {
  return (
    <p className="type-data text-dimmer m-0 text-[10px]">
      {all
        ? 'Every catalog in this folder was deleted, so there is nothing to show. They stay in the folder until they are removed.'
        : 'This catalog was deleted. It stays in the folder until it is removed.'}
    </p>
  )
}

function Tab({
  label,
  active,
  onClick,
}: {
  label: string
  active: boolean
  onClick: () => void
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      aria-pressed={active}
      title={label}
      className={`type-data flex shrink-0 items-center gap-1.5 rounded-[2px] border px-2.5 py-1 text-[11px] transition-colors pointer-coarse:py-2 ${
        active ? 'border-line-hi bg-raised-hi text-ink' : 'text-dim hover:text-ink border-transparent'
      }`}
    >
      <span className="max-w-[180px] truncate">{label}</span>
    </button>
  )
}
