import { useState } from 'react'
import type { ReactNode } from 'react'
import { useSortable } from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { Tv } from 'lucide-react'
import { tmdbKind } from '@/api'
import { Grip, MoveDownButton, MoveUpButton } from '@/components/dnd'
import { Segmented } from '@/components/fields'
import { Icon } from '@/components/Icon'
import { ListState } from '@/components/ListState'
import { MoreMenu, MoreMenuItem } from '@/components/MoreMenu'
import { PaneSign, SignLibraryButton } from '@/components/PaneSign'
import { describeCollection } from '@/features/library/collection'
import { recipeLine, typeLabel } from '@/features/library/recipe'
import type { PreviewCollection, PreviewFolder } from '@/features/preview/model'
import { noTiles, TileRun, TILE_ASPECT } from '@/features/preview/tiles'
import { rowStickers } from '@/features/sharing/sharingState'
import { SharingStickers } from '@/features/sharing/SharingStickers'
import { ordinal } from '@/lib/ordinal'
import { showFirstAction } from './changes'
import { HomePreview } from './HomePreview'
import type { PreviewRow } from './preview'
import { SortableList } from './SortableList'
import { useCatalogTiles } from './useCatalogTiles'
import { useHomeEdits, useHomePreview, useHomeSelection } from './useHomeSelection'

export type HomeView = 'list' | 'preview'

/**
 * What is actually on the profile's home screen: selected collections (each a
 * row whose tiles are its folders) and selected catalogs (each a row of
 * content), both ordered. Collections shown first — Show first, a pending
 * edit in each collection row's menu, like Move to Discover for a catalog —
 * sit above the catalog rows, the rest below; Preview is where that order is
 * visible. This view
 * groups by Nuvio's own three bands too — see `HomeList` — because the
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
    <main className="tone-home flex flex-1 flex-col lg:min-h-0 lg:overflow-y-auto">
      {/* The padding is split between this band and the content below rather
          than sitting on `main`, because a sticky child of a padded parent
          leaves a gap above it that the content then scrolls through.

          It is the home screen's sign: Nuvio yellow, because everything under it
          is what Nuvio shows. */}
      <PaneSign className="flex-wrap gap-x-4 gap-y-2">
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
        <SignLibraryButton onClick={onShowLibrary} title="Back up to the library" className="ml-auto lg:hidden" />
      </PaneSign>

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
            is an instruction to go add something, Preview's is the screen Nuvio
            shows when there's nothing to show. So each branch owns it. */}
        <ListState
          isLoading={home.isLoading || !home.ready}
          error={home.error}
          onRetry={home.retry}
          loadingLabel="Loading your home screen…"
          errorLabel="Couldn't load your home screen."
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
              ? 'bg-sign-ink text-nuvio-yellow'
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
 * The running order, in Nuvio's own three groups: pinned collections, then
 * catalog rows, then the remaining collections. Positions count straight
 * through all three, so a row's number is its place in Nuvio — the same rule
 * `preview.ts` draws and `changes.ts` speaks in. A row moves only within its
 * own group: reordering hands `HomeSelectionContext` only that group's ids,
 * which reconstructs the full list itself (see `reorderWithinBand`).
 */
function HomeList({ compact }: { compact: boolean }) {
  const home = useHomeSelection()
  const preview = useHomePreview()
  const tiles = useCatalogTiles(preview.rows.map((row) => row.id))

  if (preview.isEmpty) {
    return (
      <p className="text-dim m-0 max-w-[48ch] py-2 text-[15px] leading-[1.5]">
        Nothing on your home screen yet. Tap + beside anything in the Library.
      </p>
    )
  }

  // Nuvio's own order, for continuous numbering — see the module comment.
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
        <Group label="Shown first">
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

      <Group label="Catalogs">
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
        <Group label="After the catalogs">
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

function Group({ label, children }: { label: string; children: ReactNode }) {
  return (
    <section className="flex flex-col gap-1">
      <div className="border-line-hi border-b pb-2.5">
        <h2 className="text-ink m-0 text-[16px] font-bold">{label}</h2>
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
        <Grip label={`Reorder ${name}, ${ordinal(position)} on your home screen`} sortable={{ attributes, listeners }} />
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
        <span className="sr-only">{ordinal(position)} on your home screen</span>
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

/** One name for the action everywhere, matching the add button's own "Remove
 *  from home" (see DESIGN.md's add button spec). Danger-styled when the item is
 *  detached, since removing it there can't be undone. */
function RemoveFromHomeItem({ detached, onSelect }: { detached: boolean; onSelect: () => void }) {
  return (
    <MoreMenuItem danger={detached} reason={detached ? "can't be undone" : undefined} onSelect={onSelect}>
      Remove from home
    </MoreMenuItem>
  )
}

/** Every flag a Home row carries beside its kind: its Community sticker,
 *  Unpublished, and Push to Nuvio while a push would change what Nuvio holds
 *  for it (`rowStickers`). A catalog and a collection never share an id. */
function HomeFlags({ id }: { id: string }) {
  const home = useHomeSelection()
  const row = home.catalogById.get(id) ?? home.collectionById.get(id)
  return row ? <SharingStickers stickers={rowStickers(row, home.waitingForPush.has(id))} /> : null
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
        <span className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1">
          <span className="max-w-full truncate text-[16.5px] font-bold">{row.name}</span>
          {catalog && (
            <span className="stk stk-kind shrink-0">{typeLabel(catalog.type)}</span>
          )}
          <HomeFlags id={row.id} />
          {detached && <DetachedTag />}
        </span>
        <span className="text-dim truncate text-[13.5px]">{detail || 'No filters'}</span>
        <div className="mt-1.5">{children}</div>
      </RowBody>

      <MoreMenu label={row.name}>
        <MoreMenuItem onSelect={() => home.toggleShowInHome(row.id)}>Move to Discover</MoreMenuItem>
        <RemoveFromHomeItem detached={detached} onSelect={() => home.removeCatalog(row.id)} />
      </MoreMenu>
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
        <span className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1">
          <span className="max-w-full truncate text-[16.5px] font-bold">{collection.title}</span>
          <span className="stk stk-kind shrink-0">Collection</span>
          <HomeFlags id={collection.id} />
          {detached && <DetachedTag />}
        </span>
        <span className="text-dim truncate text-[13.5px]">{detail}</span>
        <div className="mt-1.5">{children}</div>
      </RowBody>

      <MoreMenu label={collection.title}>
        <ShowFirstItem collection={collection} />
        <RemoveFromHomeItem detached={detached} onSelect={() => home.removeCollection(collection.id)} />
      </MoreMenu>
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
 * `show_in_home = false` catalogs: reachable in Nuvio from Discover, but no
 * home row — so they sit outside the numbered running order entirely, never
 * as a fourth group. See `internal/addon/addon.go`'s `buildManifest`, which
 * enforces this by marking the catalog's genre filter `isRequired`.
 */
function DiscoverTray({ rows }: { rows: PreviewRow[] }) {
  const home = useHomeSelection()

  return (
    <section className="flex flex-col gap-2">
      <div className="border-line-hi border-b pb-2.5">
        <h2 className="text-ink m-0 text-[16px] font-bold">Not on home</h2>
      </div>
      <ul className="flex flex-col">
        {rows.map((row) => {
          const detached = home.isDetached(row.id)
          return (
            <li
              key={row.id}
              className="border-line flex items-center justify-between gap-3 border-b py-3"
            >
              <div className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1">
                <span className="max-w-full truncate text-[15px] font-semibold">{row.name}</span>
                <HomeFlags id={row.id} />
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

/** A collection row's Show first: a pending edit, like Move to Discover for a
 *  catalog, that moves the row to the other collection group until Push. */
function ShowFirstItem({ collection }: { collection: PreviewCollection }) {
  const home = useHomeEdits()
  return (
    <MoreMenuItem onSelect={() => home.togglePinToTop(collection.id)}>
      {showFirstAction(collection.pinned)}
    </MoreMenuItem>
  )
}
