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
import { recipeLine } from '@/features/library/recipe'
import type { PreviewCollection } from '@/features/preview/model'
import { COLLECTION_KIND, homeStickers, kindStickers } from '@/features/sharing/sharingState'
import { SharingStickers } from '@/features/sharing/SharingStickers'
import { ordinal } from '@/lib/ordinal'
import { showFirstAction } from './changes'
import { CollectionDetail, DetailLine } from './FolderChips'
import { HomePreview } from './HomePreview'
import { bandItemId } from './preview'
import type { HomeBandItem, PreviewRow } from './preview'
import { SortableList } from './SortableList'
import { useHomeEdits, useHomePreview, useHomeSelection } from './useHomeSelection'

export type HomeView = 'list' | 'preview'

/**
 * What is actually on the profile's home screen: selected collections (each a
 * row whose tiles are its folders) and selected catalogs (each a row of
 * content), both ordered. Pinned collections — Pin, a pending edit in each
 * collection row's menu, like Move to Discover for a catalog — sit above the
 * catalog rows, the rest below; Preview is where that order is
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

      <div className="px-4 pt-5 lg:hidden">
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
          {view === 'list' ? <HomeList /> : <HomePreview />}
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
 * The running order, in Nuvio's own two groups: pinned collections, then the
 * home rows, catalogs and collections mixed. Positions count straight through
 * both, so a row's number is its place in Nuvio — the same rule `preview.ts`
 * draws and `changes.ts` speaks in. A row moves only within its own group:
 * reordering hands `HomeSelectionContext` only that group's ids, which
 * reconstructs the full list itself (see `reorderWithinBand`).
 */
function HomeList() {
  const home = useHomeSelection()
  const preview = useHomePreview()

  if (preview.isEmpty) {
    return (
      <p className="text-dim m-0 max-w-[48ch] py-2 text-[15px] leading-[1.5]">
        Nothing on your home screen yet. Add rows from the Library with +.
      </p>
    )
  }

  // Nuvio's own order, for continuous numbering — see the module comment.
  const order = [...preview.pinnedCollections.map((c) => c.id), ...preview.home.map(bandItemId)]
  const positionOf = (id: string) => order.indexOf(id) + 1

  return (
    <div className="flex flex-col gap-8">
      {preview.pinnedCollections.length > 0 && (
        <Group label="Pinned">
          <SortableList
            ids={preview.pinnedCollections.map((c) => c.id)}
            onReorder={(ids) => home.reorderBand('pinned', ids)}
          >
            <ol className="flex flex-col">
              {preview.pinnedCollections.map((collection, i, group) => (
                <CollectionRow
                  key={collection.id}
                  collection={collection}
                  position={positionOf(collection.id)}
                  first={i === 0}
                  last={i === group.length - 1}
                  groupWord="the pinned collections"
                  onMove={(direction) => home.moveRow(collection.id, direction)}
                />
              ))}
            </ol>
          </SortableList>
        </Group>
      )}

      <Group label="Rows">
        {preview.home.length === 0 ? (
          <EmptyBlock>No rows on your home screen.</EmptyBlock>
        ) : (
          <SortableList ids={preview.home.map(bandItemId)} onReorder={(ids) => home.reorderBand('home', ids)}>
            <ol className="flex flex-col">
              {preview.home.map((item, i, group) => (
                <HomeBandRow
                  key={bandItemId(item)}
                  item={item}
                  position={positionOf(bandItemId(item))}
                  first={i === 0}
                  last={i === group.length - 1}
                  onMove={(direction) => home.moveRow(bandItemId(item), direction)}
                />
              ))}
            </ol>
          </SortableList>
        )}
      </Group>

      {preview.discoverOnly.length > 0 && <DiscoverTray rows={preview.discoverOnly} />}
    </div>
  )
}

/** One of the home rows: a catalog's row or a collection's, numbered and
 *  moved within the home rows. */
function HomeBandRow({
  item,
  position,
  first,
  last,
  onMove,
}: {
  item: HomeBandItem
  position: number
  first: boolean
  last: boolean
  onMove: (direction: -1 | 1) => void
}) {
  if (item.kind === 'catalog') {
    return (
      <CatalogRow row={item.row} position={position} first={first} last={last} groupWord="the rows" onMove={onMove} />
    )
  }
  return (
    <CollectionRow
      collection={item.collection}
      position={position}
      first={first}
      last={last}
      groupWord="the rows"
      onMove={onMove}
    />
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
 * One line of the running order: the grip and the position as a yellow
 * sticker at the left, the name over its summary line or folder tiles, then
 * ↑ and ↓ side by side and ⋯ at the right, with the stickers on their own
 * line below `sm`. A catalog or collection row hands in its `RowBody` as
 * `children` and its ⋯ menu as `menu`.
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
  menu,
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
  menu: ReactNode
  children: ReactNode
}) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({
    id,
  })

  return (
    <li
      ref={setNodeRef}
      style={{ transform: CSS.Transform.toString(transform), transition }}
      className={`border-line grid grid-cols-[auto_minmax(0,1fr)_auto_auto] items-center gap-x-2 gap-y-1 border-b py-3 pr-1 pl-0 transition-colors sm:gap-x-3 ${
        isDragging ? 'relative z-10 opacity-40' : ''
      }`}
    >
      <div className="col-start-1 row-start-1 flex items-center gap-2.5">
        <Grip label={`Reorder ${name}, ${ordinal(position)} on your home screen`} sortable={{ attributes, listeners }} />
        <span className="relative">
          <span aria-hidden="true" className="pos-sticker">
            {position}
          </span>
          <span className="sr-only">{ordinal(position)} on your home screen</span>
        </span>
      </div>

      {children}

      {/* On touch each button's `.tap` box grows to 44px, so the pair spreads
          apart rather than let one swallow the other's taps. */}
      <div className="col-start-3 row-start-1 flex gap-0.5 pointer-coarse:gap-3">
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

      <div className="col-start-4 row-start-1 flex">{menu}</div>
    </li>
  )
}

/** The row's name, its stickers and its detail: the summary line, or a
 *  collection row's folder tiles. From `sm` they are the second grid column:
 *  the name with its stickers on one wrapping line, over the detail. Below it
 *  the body dissolves into the row's grid, so the name keeps the first line
 *  beside ↑, ↓ and ⋯, and the stickers and the detail each take a full-width
 *  line under them. */
function RowBody({ name, stickers, detail }: { name: string; stickers: ReactNode; detail: ReactNode }) {
  return (
    <div className="contents sm:flex sm:min-w-0 sm:flex-col sm:gap-1">
      <span className="contents sm:flex sm:min-w-0 sm:flex-wrap sm:items-center sm:gap-x-2 sm:gap-y-1">
        <span className="min-w-0 truncate text-[16.5px] font-bold sm:max-w-full">{name}</span>
        <span className="col-[2/-1] flex flex-wrap items-center gap-x-2 gap-y-1 sm:contents">{stickers}</span>
      </span>
      {detail}
    </div>
  )
}

/** One name for the action everywhere, matching the add button's own "Remove
 *  from home" (see DESIGN.md's add button spec). */
function RemoveFromHomeItem({ onSelect }: { onSelect: () => void }) {
  return <MoreMenuItem onSelect={onSelect}>Remove from home</MoreMenuItem>
}

/** The flag a Home row carries beside its kind: To push while a push would
 *  change what Nuvio holds for it (`homeStickers`). */
function HomeFlags({ id }: { id: string }) {
  const home = useHomeSelection()
  return <SharingStickers stickers={homeStickers(home.waitingForPush.has(id))} />
}

/** A catalog row's kind, Movies or Series, once the catalog is known. */
function CatalogKind({ id }: { id: string }) {
  return <SharingStickers stickers={kindStickers(useHomeSelection().catalogById.get(id))} />
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
}: {
  row: PreviewRow
  position: number
  first: boolean
  last: boolean
  groupWord: string
  onMove: (direction: -1 | 1) => void
}) {
  const home = useHomeSelection()
  const catalog = home.catalogById.get(row.id)
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
      menu={
        <MoreMenu label={row.name}>
          <MoreMenuItem onSelect={() => home.toggleShowInHome(row.id)}>Move to Discover</MoreMenuItem>
          <RemoveFromHomeItem onSelect={() => home.removeCatalog(row.id)} />
        </MoreMenu>
      }
    >
      <RowBody
        name={row.name}
        stickers={
          <>
            <CatalogKind id={row.id} />
            <HomeFlags id={row.id} />
          </>
        }
        detail={<DetailLine text={detail || 'No filters'} />}
      />
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
}: {
  collection: PreviewCollection
  position: number
  first: boolean
  last: boolean
  groupWord: string
  onMove: (direction: -1 | 1) => void
}) {
  const home = useHomeSelection()
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
      menu={
        <MoreMenu label={collection.title}>
          <PinItem collection={collection} />
          <RemoveFromHomeItem onSelect={() => home.removeCollection(collection.id)} />
        </MoreMenu>
      }
    >
      <RowBody
        name={collection.title}
        stickers={
          <>
            <SharingStickers stickers={[COLLECTION_KIND]} />
            <HomeFlags id={collection.id} />
          </>
        }
        detail={<CollectionDetail collection={collection} summary={detail} />}
      />
    </HomeRow>
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
  return (
    <section className="flex flex-col gap-2">
      <div className="border-line-hi border-b pb-2.5">
        <h2 className="text-ink m-0 text-[16px] font-bold">Only in Discover</h2>
      </div>
      <ul className="flex flex-col">
        {rows.map((row) => (
          <TrayRow key={row.id} row={row} />
        ))}
      </ul>
    </section>
  )
}

/** One Discover-only catalog: its name with its kind and flags, and Move to
 *  home. */
function TrayRow({ row }: { row: PreviewRow }) {
  const home = useHomeSelection()
  return (
    <li className="border-line flex items-center justify-between gap-3 border-b py-3">
      <div className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1">
        <span className="max-w-full truncate text-[15px] font-semibold">{row.name}</span>
        <CatalogKind id={row.id} />
        <HomeFlags id={row.id} />
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
}

function EmptyBlock({ children }: { children: ReactNode }) {
  return <p className="text-dim m-0 py-3 text-[14px]">{children}</p>
}

/** A collection row's Pin or Unpin: a pending edit, like Move to Discover for
 *  a catalog, that moves the row to the other collection group until Push. */
function PinItem({ collection }: { collection: PreviewCollection }) {
  const home = useHomeEdits()
  return (
    <MoreMenuItem onSelect={() => home.togglePinToTop(collection.id)}>
      {showFirstAction(collection.pinned)}
    </MoreMenuItem>
  )
}
