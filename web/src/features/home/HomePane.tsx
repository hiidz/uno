import { useMemo, useState } from 'react'
import type { ReactNode } from 'react'
import { useSortable } from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { ChevronDown, ChevronUp, MoreHorizontal } from 'lucide-react'
import type { LucideIcon } from 'lucide-react'
import { DropdownMenu } from 'radix-ui'
import { tmdbKind } from '@/api'
import { Grip } from '@/components/dnd'
import { Segmented } from '@/components/fields'
import { Icon } from '@/components/Icon'
import { ListState } from '@/components/ListState'
import { describeCollection } from '@/features/library/collection'
import { describeRecipe } from '@/features/library/recipe'
import type { PreviewCollection, PreviewFolder } from '@/features/preview/model'
import { noTiles, TileRun, TILE_ASPECT } from '@/features/preview/tiles'
import type { CatalogTiles } from '@/features/preview/tiles'
import { ordinal } from '@/lib/ordinal'
import { HomePreview } from './HomePreview'
import { buildHomePreview } from './preview'
import type { PreviewRow } from './preview'
import { SortableList } from './SortableList'
import { useCatalogTiles } from './useCatalogTiles'
import { useHomeSelection } from './useHomeSelection'

export type HomeView = 'list' | 'preview'

/**
 * What is actually on the profile's home screen: selected collections (each a
 * row whose tiles are its folders) and selected catalogs (each a row of
 * content), both ordered. Collections with `pin_to_top` sit above the catalog
 * rows, the rest below — Preview is where that order is visible. This view
 * groups by the TV's own three bands too — see `HomeList` — because the
 * running order's numbering only makes sense in that order.
 *
 * Called "your home screen", never "selection" — that's the schema's word.
 * Nothing here writes to the server; every edit is pending until Push.
 *
 * Two views of one state. List is where every edit happens; Preview draws the
 * same state as the shape of the screen it becomes. Both read from the same
 * client state, so the switch costs no fetch. View mode is in-page state, not
 * routed.
 *
 * **The view is held above this component**, because the pane it lives in is
 * shared with the editors: opening one unmounts this, and a view kept here
 * would silently drop back to List every time someone edited a catalog and came
 * back. Closing an editor should return the screen you left.
 */
export function HomePane({
  view,
  onViewChange,
  onShowLibrary,
}: {
  view: HomeView
  onViewChange: (view: HomeView) => void
  /** Scroll back up to the rail. Below `lg` only, where the two are stacked
   *  into one page. Not an exit — nothing here is unmounted by it. */
  onShowLibrary: () => void
}) {
  const home = useHomeSelection()
  // Not part of the pushed state — a display preference for this view, not an
  // edit to the home screen. Held here, above `HomeList`, for the same reason
  // `view` is: it must survive `HomeList` unmounting under an editor.
  const [compact, setCompact] = useState(false)
  const isEmpty = home.catalogs.length === 0 && home.collections.length === 0

  return (
    <main className="flex flex-1 flex-col lg:min-h-0 lg:overflow-y-auto">
      {/* Pinned under the app header below `lg`, for the same reason the
          editors' header is: it names the region the page has just scrolled to
          and carries the way back up out of it. The padding is split between
          this band and the content below rather than sitting on `main`, because
          a sticky child of a padded parent leaves a gap above it that the
          content then scrolls through. Above `lg` the two halves add back up to
          the `p-6` and `gap-8` that were here before. */}
      <div className="border-line bg-ground sticky top-[var(--app-h)] z-20 flex flex-wrap items-center gap-x-4 gap-y-2 border-b px-4 py-3 lg:static lg:items-baseline lg:border-0 lg:bg-transparent lg:px-6 lg:pt-6 lg:pb-0">
        {/* Where focus lands when the page scrolls here — see `stacked.ts`. */}
        <h1
          tabIndex={-1}
          data-landing
          className="type-display m-0 text-[17px] outline-none lg:text-[21px]"
        >
          Your home screen
        </h1>
        <ViewSwitch view={view} onChange={onViewChange} />
        {/* Only on the Home screen tab, and only once there's something to
            show strips for — DESIGN.md's Home-screen pane spec. */}
        {view === 'list' && !isEmpty && (
          <div className="flex items-center gap-2">
            <span className="type-eyebrow">Rows</span>
            <Segmented
              ariaLabel="Row display"
              value={compact ? 'compact' : 'strips'}
              onChange={(value) => setCompact(value === 'compact')}
              options={[
                { value: 'strips', label: 'Strips' },
                { value: 'compact', label: 'Compact' },
              ]}
            />
          </div>
        )}
        <button
          type="button"
          onClick={onShowLibrary}
          title="Back up to the library"
          className="tap type-data border-line-hi text-dim hover:text-ink hover:border-dim ml-auto flex h-7 shrink-0 items-center gap-1 rounded-[2px] border px-2 text-[10px] tracking-[0.06em] uppercase transition-colors lg:hidden"
        >
          <span aria-hidden="true" className="text-[11px] leading-none">
            ↑
          </span>
          Library
        </button>
      </div>

      <div className="flex flex-col px-4 py-4 lg:px-6 lg:pt-8 lg:pb-6">
        {/* Loading and error are shared — both views need the same state before
            they can render anything. Empty is *not* shared: List's empty state
            is an instruction to go add something, Preview's is the screen a TV
            shows when there's nothing to show. So each branch owns it. */}
        <ListState
          isLoading={home.isLoading || !home.ready}
          error={home.error}
          isEmpty={false}
          loadingLabel="Loading your home screen…"
          errorLabel="Couldn't load your home screen."
          emptyLabel={null}
        >
          {view === 'list' ? <HomeList compact={compact} /> : <HomePreview />}
        </ListState>
      </div>
    </main>
  )
}

function ViewSwitch({ view, onChange }: { view: HomeView; onChange: (view: HomeView) => void }) {
  return (
    <div className="border-line-hi flex overflow-hidden rounded-[2px] border">
      {(['list', 'preview'] as const).map((option) => (
        <button
          key={option}
          type="button"
          onClick={() => onChange(option)}
          aria-pressed={view === option}
          className={`type-data px-3 py-1.5 text-[10.5px] tracking-[0.08em] uppercase transition-colors pointer-coarse:py-2.5 ${
            view === option ? 'bg-raised-hi text-ink' : 'text-dim hover:text-ink'
          }`}
        >
          {option}
        </button>
      ))}
    </div>
  )
}

/**
 * The running order, in the TV's own three groups: pinned collections, then
 * catalog rows, then the remaining collections. Positions count straight
 * through all three, so a row's number is its place on the TV — the same rule
 * `preview.ts` draws and `changes.ts` speaks in. A row moves only within its
 * own group: reordering hands `HomeSelectionContext` only that group's ids,
 * which reconstructs the full list itself (see `reorderWithinBand`).
 */
function HomeList({ compact }: { compact: boolean }) {
  const home = useHomeSelection()

  const preview = useMemo(
    () =>
      buildHomePreview({
        catalogs: home.catalogs,
        collections: home.collections,
        catalogById: home.catalogById,
        collectionById: home.collectionById,
      }),
    [home.catalogs, home.collections, home.catalogById, home.collectionById],
  )

  const tiles = useCatalogTiles(preview.rows.map((row) => row.id))

  if (preview.isEmpty) {
    return (
      <p className="type-data text-dimmer m-0 py-2 text-[11px]">
        Nothing here yet. Add catalogs and collections from the sidebar.
      </p>
    )
  }

  // The TV's own order, for continuous numbering — see the module comment.
  const order = [
    ...preview.pinnedCollections.map((c) => `collection:${c.id}`),
    ...preview.rows.map((r) => `catalog:${r.id}`),
    ...preview.unpinnedCollections.map((c) => `collection:${c.id}`),
  ]
  const positionOf = (kind: 'collection' | 'catalog', id: string) =>
    order.indexOf(`${kind}:${id}`) + 1

  return (
    <div className="flex max-w-[var(--w-form)] flex-col gap-8">
      {preview.pinnedCollections.length > 0 && (
        <Group
          label="Shown first"
          note="Pinned collections. Your TV puts them above every other row."
        >
          <SortableList
            ids={preview.pinnedCollections.map((c) => c.id)}
            onReorder={(ids) => home.reorderCollections('pinned', ids)}
          >
            <ol className="flex flex-col">
              {preview.pinnedCollections.map((collection, i, group) => (
                <CollectionRow
                  key={collection.id}
                  collection={collection}
                  position={positionOf('collection', collection.id)}
                  first={i === 0}
                  last={i === group.length - 1}
                  groupWord="the pinned collections"
                  compact={compact}
                  onMove={(direction) => home.moveCollection(collection.id, direction)}
                />
              ))}
            </ol>
          </SortableList>
        </Group>
      )}

      <Group label="Catalogs" note="Rows of posters.">
        {preview.rows.length === 0 ? (
          <EmptyBlock>No catalogs on your home screen.</EmptyBlock>
        ) : (
          <SortableList ids={preview.rows.map((r) => r.id)} onReorder={home.reorderCatalogs}>
            <ol className="flex flex-col">
              {preview.rows.map((row, i, group) => (
                <CatalogRow
                  key={row.id}
                  row={row}
                  tiles={tiles.get(row.id) ?? noTiles()}
                  position={positionOf('catalog', row.id)}
                  first={i === 0}
                  last={i === group.length - 1}
                  groupWord="the catalogs"
                  compact={compact}
                  onMove={(direction) => home.moveCatalog(row.id, direction)}
                />
              ))}
            </ol>
          </SortableList>
        )}
      </Group>

      {preview.unpinnedCollections.length > 0 && (
        <Group
          label="After the catalogs"
          note="Collections that aren’t pinned. Your TV puts them below your catalog rows."
        >
          <SortableList
            ids={preview.unpinnedCollections.map((c) => c.id)}
            onReorder={(ids) => home.reorderCollections('unpinned', ids)}
          >
            <ol className="flex flex-col">
              {preview.unpinnedCollections.map((collection, i, group) => (
                <CollectionRow
                  key={collection.id}
                  collection={collection}
                  position={positionOf('collection', collection.id)}
                  first={i === 0}
                  last={i === group.length - 1}
                  groupWord="the other collections"
                  compact={compact}
                  onMove={(direction) => home.moveCollection(collection.id, direction)}
                />
              ))}
            </ol>
          </SortableList>
        </Group>
      )}

      {preview.discoverOnly.length > 0 && <DiscoverTray rows={preview.discoverOnly} />}
    </div>
  )
}

function Group({ label, note, children }: { label: string; note: string; children: ReactNode }) {
  return (
    <section className="flex flex-col gap-2">
      <div className="border-line-hi flex flex-col gap-0.5 border-b pb-2">
        <span className="type-eyebrow">{label}</span>
        <p className="type-data text-dimmer m-0 text-[10.5px]">{note}</p>
      </div>
      {children}
    </section>
  )
}

/* -------------------------------------------------------------------------- */
/* The running-order row (signature)                                          */
/* -------------------------------------------------------------------------- */

/**
 * Grip, ↑, ↓, the position against the name, a credit line, a strip of whole
 * tiles, then ⋯ — DESIGN.md's "Running-order row (signature)". Shared by
 * catalog and collection rows; only the body and the strip differ.
 */
function HomeRow({
  id,
  name,
  position,
  first,
  last,
  groupWord,
  compact,
  onMoveUp,
  onMoveDown,
  body,
  strip,
  menu,
}: {
  id: string
  name: string
  position: number
  first: boolean
  last: boolean
  /** Named in a disabled ↑/↓'s accessible name — "already first of the
   *  catalogs" — since a row moves only within its own group. */
  groupWord: string
  compact: boolean
  onMoveUp: () => void
  onMoveDown: () => void
  body: ReactNode
  strip: ReactNode
  menu: ReactNode
}) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({
    id,
  })

  return (
    <li
      ref={setNodeRef}
      style={{ transform: CSS.Transform.toString(transform), transition }}
      className={`border-line grid grid-cols-[32px_40px_minmax(0,1fr)_32px] items-start gap-x-3 border-b py-4 pr-3 pl-1 transition-colors ${
        isDragging ? 'relative z-10 opacity-40' : ''
      }`}
    >
      <div className="flex flex-col gap-0.5">
        <Grip label={`Reorder ${name}, ${ordinal(position)} on your TV`} sortable={{ attributes, listeners }} />
        <RowIconButton
          icon={ChevronUp}
          label={`Move ${name} up${first ? `, already first of ${groupWord}` : ''}`}
          disabled={first}
          onClick={onMoveUp}
        />
        <RowIconButton
          icon={ChevronDown}
          label={`Move ${name} down${last ? `, already last of ${groupWord}` : ''}`}
          disabled={last}
          onClick={onMoveDown}
        />
      </div>

      <span className="type-data text-dim pt-2 text-right text-[11px] tabular-nums">
        {ordinal(position)}
      </span>

      <div className="flex min-w-0 flex-col gap-1">
        {body}
        {!compact && strip}
      </div>

      {menu}
    </li>
  )
}

function RowIconButton({
  icon,
  label,
  disabled,
  onClick,
}: {
  icon: LucideIcon
  label: string
  disabled: boolean
  onClick: () => void
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      aria-label={label}
      title={label}
      className={`tap grid h-8 w-8 place-items-center rounded-[2px] transition-colors ${
        disabled
          ? 'text-dimmer cursor-not-allowed'
          : 'text-dim hover:bg-line hover:text-ink active:bg-line-hi'
      }`}
    >
      <Icon icon={icon} size={14} />
    </button>
  )
}

/**
 * The row's ⋯ menu: "Move to Discover" for a catalog row only, then "Take off
 * TV" — one name for the action everywhere, matching the add button's own
 * "Take off TV" (see DESIGN.md's add button spec). Danger-styled when the item
 * is detached, since removing it there can't be undone.
 */
function RowMenu({
  name,
  showMoveToDiscover,
  onMoveToDiscover,
  onTakeOffTV,
  detached,
}: {
  name: string
  showMoveToDiscover: boolean
  onMoveToDiscover?: () => void
  onTakeOffTV: () => void
  detached: boolean
}) {
  return (
    <DropdownMenu.Root modal={false}>
      <DropdownMenu.Trigger
        aria-label={`More for ${name}`}
        className="tap text-dimmer hover:bg-line hover:text-ink grid h-8 w-8 place-items-center rounded-[2px] transition-colors"
      >
        <Icon icon={MoreHorizontal} size={16} />
      </DropdownMenu.Trigger>
      <DropdownMenu.Portal>
        <DropdownMenu.Content
          align="end"
          sideOffset={4}
          className="bg-raised-hi border-line-hi z-40 flex w-60 flex-col gap-0.5 rounded-[2px] border p-1.5 shadow-[0_12px_28px_rgba(0,0,0,0.55)]"
        >
          {showMoveToDiscover && onMoveToDiscover && (
            <RowMenuItem label="Move to Discover" onSelect={onMoveToDiscover} />
          )}
          <RowMenuItem
            label="Take off TV"
            danger={detached}
            reason={detached ? "can't be undone" : undefined}
            onSelect={onTakeOffTV}
          />
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  )
}

function RowMenuItem({
  label,
  danger,
  reason,
  onSelect,
}: {
  label: string
  danger?: boolean
  reason?: string
  onSelect: () => void
}) {
  return (
    <DropdownMenu.Item
      onSelect={onSelect}
      className={`hover:bg-line focus-visible:bg-line flex items-center justify-between gap-2 rounded-[2px] px-2 py-2 text-left text-[12px] transition-colors ${
        danger ? 'text-danger' : 'text-ink'
      }`}
    >
      <span>{label}</span>
      {reason && <span className="type-data text-dimmer text-[10px]">{reason}</span>}
    </DropdownMenu.Item>
  )
}

function DetachedTag() {
  return (
    <span
      className="text-series shrink-0"
      title="Its owner deleted it or made it private. It still works here, but you can't edit it."
    >
      · unavailable
    </span>
  )
}

/* -------------------------------------------------------------------------- */
/* Catalog and collection rows                                                */
/* -------------------------------------------------------------------------- */

function CatalogRow({
  row,
  tiles,
  position,
  first,
  last,
  groupWord,
  compact,
  onMove,
}: {
  row: PreviewRow
  tiles: CatalogTiles
  position: number
  first: boolean
  last: boolean
  groupWord: string
  compact: boolean
  onMove: (direction: -1 | 1) => void
}) {
  const home = useHomeSelection()
  const catalog = home.catalogById.get(row.id)
  const detached = home.isDetached(row.id)
  const detail = catalog
    ? describeRecipe(catalog, home.genres[tmdbKind(catalog.type)]).join(' · ')
    : ''

  return (
    <HomeRow
      id={row.id}
      name={row.name}
      position={position}
      first={first}
      last={last}
      groupWord={groupWord}
      compact={compact}
      onMoveUp={() => onMove(-1)}
      onMoveDown={() => onMove(1)}
      body={
        <>
          <span className="truncate text-[13px] font-medium">{row.name}</span>
          <span className="type-data text-dimmer flex min-w-0 items-baseline gap-1.5 text-[10.5px]">
            <span className="truncate">{detail || 'no filters'}</span>
            {detached && <DetachedTag />}
          </span>
        </>
      }
      strip={<TileRun tiles={tiles} width={64} height={96} wrap={false} />}
      menu={
        <RowMenu
          name={row.name}
          showMoveToDiscover
          onMoveToDiscover={() => home.toggleShowInHome(row.id)}
          onTakeOffTV={() => home.removeCatalog(row.id)}
          detached={detached}
        />
      }
    />
  )
}

function CollectionRow({
  collection,
  position,
  first,
  last,
  groupWord,
  compact,
  onMove,
}: {
  collection: PreviewCollection
  position: number
  first: boolean
  last: boolean
  groupWord: string
  compact: boolean
  onMove: (direction: -1 | 1) => void
}) {
  const home = useHomeSelection()
  const detached = home.isDetached(collection.id)
  const detail = describeCollection(home.collectionById.get(collection.id))

  return (
    <HomeRow
      id={collection.id}
      name={collection.title}
      position={position}
      first={first}
      last={last}
      groupWord={groupWord}
      compact={compact}
      onMoveUp={() => onMove(-1)}
      onMoveDown={() => onMove(1)}
      body={
        <>
          <span className="truncate text-[13px] font-medium">{collection.title}</span>
          <span className="type-data text-dimmer flex min-w-0 items-baseline gap-1.5 text-[10.5px]">
            <span className="truncate">{detail}</span>
            {detached && <DetachedTag />}
          </span>
        </>
      }
      strip={<FolderStrip folders={collection.folders} />}
      menu={
        <RowMenu
          name={collection.title}
          showMoveToDiscover={false}
          onTakeOffTV={() => home.removeCollection(collection.id)}
          detached={detached}
        />
      }
    />
  )
}

/** A collection's strip: its folders, at the 64–171×96 sizes DESIGN.md's
 *  running-order row spec gives each shape — narrower than `FolderTile`'s own
 *  92px-high preview tile, so this draws its own box rather than reusing it. */
function FolderStrip({ folders }: { folders: PreviewFolder[] }) {
  if (folders.length === 0) return null
  return (
    <div className="flex gap-2 overflow-hidden">
      {folders.map((folder) => (
        <FolderStripTile key={folder.id} folder={folder} />
      ))}
    </div>
  )
}

function FolderStripTile({ folder }: { folder: PreviewFolder }) {
  const height = 96
  const width = height * TILE_ASPECT[folder.tileShape]
  const name = folder.title || 'Untitled folder'

  return (
    <span
      style={{ width: `${width}px`, height: `${height}px` }}
      title={name}
      className="bg-raised border-line relative grid shrink-0 place-items-center overflow-hidden rounded-[2px] border px-1"
    >
      {folder.coverEmoji ? (
        <span aria-hidden="true" className="text-[18px] leading-none">
          {folder.coverEmoji}
        </span>
      ) : (
        <span aria-hidden="true" className="type-data text-dimmer text-center text-[8px] leading-tight">
          {name}
        </span>
      )}
      {folder.coverImageUrl && (
        <img
          src={folder.coverImageUrl}
          alt=""
          loading="lazy"
          className="absolute inset-0 h-full w-full object-cover"
        />
      )}
    </span>
  )
}

/* -------------------------------------------------------------------------- */
/* Discover tray                                                              */
/* -------------------------------------------------------------------------- */

/**
 * `show_in_home = false` catalogs: reachable on the TV from Discover, but no
 * home row — so they sit outside the numbered running order entirely, never
 * as a fourth group. See `internal/addon/addon.go`'s `buildManifest`, which
 * enforces this by marking the catalog's genre filter `isRequired`.
 */
function DiscoverTray({ rows }: { rows: PreviewRow[] }) {
  const home = useHomeSelection()

  return (
    <section className="flex flex-col gap-2">
      <div className="border-line-hi flex flex-col gap-0.5 border-b pb-2">
        <span className="type-eyebrow">Not on home</span>
        <p className="type-data text-dimmer m-0 text-[10.5px]">
          In Discover only. Still reachable on your TV, just not as a home row.
        </p>
      </div>
      <ul className="flex flex-col">
        {rows.map((row) => {
          const detached = home.isDetached(row.id)
          return (
            <li
              key={row.id}
              className="border-line flex items-center justify-between gap-3 border-b py-3"
            >
              <div className="flex min-w-0 flex-col gap-0.5">
                <span className="truncate text-[12.5px]">{row.name}</span>
                {detached && <DetachedTag />}
              </div>
              <button
                type="button"
                onClick={() => home.toggleShowInHome(row.id)}
                className="btn-secondary btn-sm shrink-0"
              >
                Move to home
              </button>
            </li>
          )
        })}
      </ul>
    </section>
  )
}

function EmptyBlock({ children }: { children: ReactNode }) {
  return <p className="type-data text-dimmer m-0 py-3 text-[11px]">{children}</p>
}
