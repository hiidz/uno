import { useMemo, useState } from 'react'
import type { ReactNode } from 'react'
import { useSortable } from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { ArrowUp, MoreHorizontal, Tv } from 'lucide-react'
import { DropdownMenu } from 'radix-ui'
import { tmdbKind } from '@/api'
import { Grip, MoveDownButton, MoveUpButton } from '@/components/dnd'
import { Segmented } from '@/components/fields'
import { Icon } from '@/components/Icon'
import { ListState } from '@/components/ListState'
import { describeCollection } from '@/features/library/collection'
import { recipeLine } from '@/features/library/recipe'
import type { PreviewCollection, PreviewFolder } from '@/features/preview/model'
import { noTiles, TileRun, TILE_ASPECT } from '@/features/preview/tiles'
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
  profileLabel,
  view,
  onViewChange,
  onShowLibrary,
}: {
  /** Which profile's TV this is — "Profile 1 · Dev" — named in the intro. */
  profileLabel: string
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
    <main className="tone-tv flex flex-1 flex-col lg:min-h-0 lg:overflow-y-auto">
      {/* Pinned under the app header below `lg`, for the same reason the
          editors' header is: it names the region the page has just scrolled to
          and carries the way back up out of it. The padding is split between
          this band and the content below rather than sitting on `main`, because
          a sticky child of a padded parent leaves a gap above it that the
          content then scrolls through.

          It is the home screen's sign: TV yellow, because everything under it
          is what the TV shows. */}
      <div className="sign sticky top-[var(--app-h)] z-20 flex-wrap gap-x-4 gap-y-2 px-4 py-2.5 lg:static lg:min-h-[80px] lg:px-6 lg:py-4">
        <Icon icon={Tv} size={24} className="hidden shrink-0 lg:block" />
        {/* Where focus lands when the page scrolls here — see `stacked.ts`. */}
        <h1
          tabIndex={-1}
          data-landing
          className="type-sign m-0 text-[16px] leading-tight outline-none lg:text-[25px]"
        >
          Your home screen
        </h1>
        {/* On the sign above `lg`. Below it the sign is pinned and has room for
            one line, so the switch moves to the row underneath. */}
        <div className="hidden lg:block">
          <ViewSwitch view={view} onChange={onViewChange} />
        </div>
        <button
          type="button"
          onClick={onShowLibrary}
          title="Back up to the library"
          className="tap sign-btn-outline ml-auto lg:hidden"
        >
          <Icon icon={ArrowUp} size={15} />
          Library
        </button>
      </div>

      <div
        className={`flex max-w-[calc(var(--w-home)+3rem)] flex-wrap items-center gap-x-5 gap-y-3 px-4 pt-5 lg:px-6 ${
          view === 'list' ? '' : 'lg:hidden'
        }`}
      >
        <div className="lg:hidden">
          <Segmented
            ariaLabel="Home screen view"
            value={view}
            onChange={onViewChange}
            options={[
              { value: 'list', label: 'List' },
              { value: 'preview', label: 'Preview' },
            ]}
          />
        </div>
        {view === 'list' && (
          <p className="m-0 min-w-[16rem] flex-1 text-[15px] leading-[1.5]">
            What {profileLabel} shows on the TV, top to bottom. Drag a row or use its arrows to
            reorder, then push.
          </p>
        )}
        {/* Only once there's something to show strips for — DESIGN.md's
            Home-screen pane spec. */}
        {view === 'list' && !isEmpty && (
          <div className="flex items-center gap-2.5">
            <span className="type-label">Rows</span>
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
      </div>

      <div className="flex flex-col px-4 py-5 lg:px-6 lg:pt-6 lg:pb-8">
        {/* Loading and error are shared — both views need the same state before
            they can render anything. Empty is *not* shared: List's empty state
            is an instruction to go add something, Preview's is the screen a TV
            shows when there's nothing to show. So each branch owns it. */}
        <ListState
          isLoading={home.isLoading || !home.ready}
          error={home.error}
          onRetry={home.retry}
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
    <div className="flex gap-0.5 rounded-full p-[3px] shadow-[inset_0_0_0_1.5px_var(--uno-sign-ink)]">
      {(['list', 'preview'] as const).map((option) => (
        <button
          key={option}
          type="button"
          onClick={() => onChange(option)}
          aria-pressed={view === option}
          className={`rounded-full px-3.5 py-1 text-[13px] font-bold capitalize transition-colors pointer-coarse:py-2.5 ${
            view === option
              ? 'bg-sign-ink text-tv-yellow'
              : 'text-sign-ink hover:bg-sign-ink/12'
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
      <p className="text-dim m-0 max-w-[48ch] py-2 text-[15px] leading-[1.5]">
        Nothing on your TV yet. Tap the empty circle beside a catalog or collection in the sidebar
        to put it here.
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
    <div className="flex max-w-[var(--w-home)] flex-col gap-8">
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
                  onMove={(direction) => home.moveCollection(collection.id, direction)}
                >
                  {!compact && <FolderStrip folders={collection.folders} />}
                </CollectionRow>
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
                  position={positionOf('catalog', row.id)}
                  first={i === 0}
                  last={i === group.length - 1}
                  groupWord="the catalogs"
                  onMove={(direction) => home.moveCatalog(row.id, direction)}
                >
                  {!compact && (
                    <TileRun
                      tiles={tiles.get(row.id) ?? noTiles()}
                      width={84}
                      height={126}
                      wrap={false}
                    />
                  )}
                </CatalogRow>
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
                  onMove={(direction) => home.moveCollection(collection.id, direction)}
                >
                  {!compact && <FolderStrip folders={collection.folders} />}
                </CollectionRow>
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
    <section className="flex flex-col gap-1">
      <div className="border-line-hi flex flex-col gap-1 border-b pb-2.5">
        <h2 className="text-ink m-0 text-[16px] font-bold">{label}</h2>
        <p className="text-dim m-0 text-[13.5px]">{note}</p>
      </div>
      {children}
    </section>
  )
}

/* -------------------------------------------------------------------------- */
/* The running-order row                                                      */
/* -------------------------------------------------------------------------- */

/**
 * Grip, ↑, ↓, the position as a yellow sticker, the name and its summary line, a
 * strip of whole tiles, then ⋯. The frame draws the grip, the ↑/↓ pair and the
 * position; a catalog or collection row composes the rest into its four-column
 * grid as a `RowBody` and a `RowMenu`.
 */
function HomeRow({
  id,
  name,
  position,
  first,
  last,
  groupWord,
  onMoveUp,
  onMoveDown,
  children,
}: {
  id: string
  name: string
  position: number
  first: boolean
  last: boolean
  /** Named in a disabled ↑/↓'s accessible name — "already first of the
   *  catalogs" — since a row moves only within its own group. */
  groupWord: string
  onMoveUp: () => void
  onMoveDown: () => void
  children: ReactNode
}) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({
    id,
  })

  return (
    <li
      ref={setNodeRef}
      style={{ transform: CSS.Transform.toString(transform), transition }}
      className={`border-line grid grid-cols-[32px_32px_minmax(0,1fr)_34px] items-start gap-x-3 border-b py-4 pr-1 pl-0 transition-colors ${
        isDragging ? 'relative z-10 opacity-40' : ''
      }`}
    >
      <div className="flex flex-col gap-0.5">
        <Grip label={`Reorder ${name}, ${ordinal(position)} on your TV`} sortable={{ attributes, listeners }} />
        <MoveUpButton
          label={`Move ${name} up${first ? `, already first of ${groupWord}` : ''}`}
          disabled={first}
          onClick={onMoveUp}
        />
        <MoveDownButton
          label={`Move ${name} down${last ? `, already last of ${groupWord}` : ''}`}
          disabled={last}
          onClick={onMoveDown}
        />
      </div>

      <span className="relative mt-1">
        <span aria-hidden="true" className="pos-sticker">
          {position}
        </span>
        <span className="sr-only">{ordinal(position)} on your TV</span>
      </span>

      {children}
    </li>
  )
}

/** The row's third grid column: the name, the summary line, and the strip when
 *  one is composed in. */
function RowBody({ children }: { children: ReactNode }) {
  return <div className="flex min-w-0 flex-col gap-1.5 pt-0.5">{children}</div>
}

/** The row's ⋯ menu: the trigger and the popover, holding whichever items the
 *  row composes into it. */
function RowMenu({ name, children }: { name: string; children: ReactNode }) {
  return (
    <DropdownMenu.Root modal={false}>
      <DropdownMenu.Trigger
        aria-label={`More for ${name}`}
        className="tap text-dim hover:bg-raised-hi hover:text-ink grid h-8 w-8 place-items-center rounded-full transition-colors"
      >
        <Icon icon={MoreHorizontal} size={16} />
      </DropdownMenu.Trigger>
      <DropdownMenu.Portal>
        <DropdownMenu.Content
          align="end"
          sideOffset={4}
          className="bg-raised-hi border-line-hi z-40 flex w-60 flex-col gap-0.5 rounded-xl border p-1.5"
        >
          {children}
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  )
}

/** One name for the action everywhere, matching the add button's own "Take off
 *  TV" (see DESIGN.md's add button spec). Danger-styled when the item is
 *  detached, since removing it there can't be undone. */
function TakeOffTVItem({ detached, onSelect }: { detached: boolean; onSelect: () => void }) {
  return (
    <RowMenuItem
      label="Take off TV"
      danger={detached}
      reason={detached ? "can't be undone" : undefined}
      onSelect={onSelect}
    />
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
      className={`hover:bg-line focus-visible:bg-line flex items-center justify-between gap-2 rounded-lg px-2.5 py-2 text-left text-[14px] font-medium transition-colors ${
        danger ? 'text-danger' : 'text-ink'
      }`}
    >
      <span>{label}</span>
      {reason && <span className="text-dimmer text-[12.5px]">{reason}</span>}
    </DropdownMenu.Item>
  )
}

function DetachedTag() {
  return (
    <span
      className="stk stk-danger shrink-0"
      title="Deleted. It still works here, but you can't edit it."
    >
      Unavailable
    </span>
  )
}

/* -------------------------------------------------------------------------- */
/* Catalog and collection rows                                                */
/* -------------------------------------------------------------------------- */

function CatalogRow({
  row,
  position,
  first,
  last,
  groupWord,
  onMove,
  children,
}: {
  row: PreviewRow
  position: number
  first: boolean
  last: boolean
  groupWord: string
  onMove: (direction: -1 | 1) => void
  /** The row's strip of poster tiles, in Strips mode. */
  children: ReactNode
}) {
  const home = useHomeSelection()
  const catalog = home.catalogById.get(row.id)
  const detached = home.isDetached(row.id)
  const detail = catalog
    ? recipeLine(catalog, home.genres[tmdbKind(catalog.type)])
    : ''

  return (
    <HomeRow
      id={row.id}
      name={row.name}
      position={position}
      first={first}
      last={last}
      groupWord={groupWord}
      onMoveUp={() => onMove(-1)}
      onMoveDown={() => onMove(1)}
    >
      <RowBody>
        <span className="flex min-w-0 items-center gap-2">
          <span className="truncate text-[16.5px] font-bold">{row.name}</span>
          {catalog && (
            <span className="stk stk-kind shrink-0">
              {catalog.type === 'movie' ? 'Movies' : 'Series'}
            </span>
          )}
          {detached && <DetachedTag />}
        </span>
        <span className="text-dim truncate text-[13.5px]">{detail || 'No filters'}</span>
        <div className="mt-1.5">{children}</div>
      </RowBody>

      <RowMenu name={row.name}>
        <RowMenuItem label="Move to Discover" onSelect={() => home.toggleShowInHome(row.id)} />
        <TakeOffTVItem detached={detached} onSelect={() => home.removeCatalog(row.id)} />
      </RowMenu>
    </HomeRow>
  )
}

function CollectionRow({
  collection,
  position,
  first,
  last,
  groupWord,
  onMove,
  children,
}: {
  collection: PreviewCollection
  position: number
  first: boolean
  last: boolean
  groupWord: string
  onMove: (direction: -1 | 1) => void
  /** The collection's strip of folder tiles, in Strips mode. */
  children: ReactNode
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
      onMoveUp={() => onMove(-1)}
      onMoveDown={() => onMove(1)}
    >
      <RowBody>
        <span className="flex min-w-0 items-center gap-2">
          <span className="truncate text-[16.5px] font-bold">{collection.title}</span>
          <span className="stk stk-kind shrink-0">Collection</span>
          {detached && <DetachedTag />}
        </span>
        <span className="text-dim truncate text-[13.5px]">{detail}</span>
        <div className="mt-1.5">{children}</div>
      </RowBody>

      <RowMenu name={collection.title}>
        <TakeOffTVItem detached={detached} onSelect={() => home.removeCollection(collection.id)} />
      </RowMenu>
    </HomeRow>
  )
}

/** A collection's strip: its folders at the catalog rows' 126px poster height,
 *  each as wide as its own tile shape. */
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
  const height = 126
  const width = height * TILE_ASPECT[folder.tileShape]
  const name = folder.title || 'Untitled folder'

  return (
    <span
      style={{ width: `${width}px`, height: `${height}px` }}
      title={name}
      className="bg-raised-hi relative grid shrink-0 place-items-center overflow-hidden rounded-md px-1"
    >
      {folder.coverEmoji ? (
        <span aria-hidden="true" className="text-[18px] leading-none">
          {folder.coverEmoji}
        </span>
      ) : (
        <span aria-hidden="true" className="type-data text-dimmer text-center text-[10px] leading-tight">
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
      <div className="border-line-hi flex flex-col gap-1 border-b pb-2.5">
        <h2 className="text-ink m-0 text-[16px] font-bold">Not on home</h2>
        <p className="text-dim m-0 text-[13.5px]">
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
                <span className="truncate text-[15px] font-semibold">{row.name}</span>
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
  return <p className="text-dim m-0 py-3 text-[14px]">{children}</p>
}
