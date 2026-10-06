import { useSortable } from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { Grip } from '@/components/dnd'
import { MoreMenu, MoreMenuItem, MoreMenuSeparator } from '@/components/MoreMenu'
import { Toast } from '@/components/Toast'
import { useHomeSelection } from '@/features/home/useHomeSelection'
import { kindStickers } from '@/features/sharing/sharingState'
import { SharingStickers } from '@/features/sharing/SharingStickers'
import { pluralCount } from '@/lib/plural'
import type { FolderUnit, RefGroup } from './collectionForm'
import { dragID, type ReorderableData } from './folderDnd'
import { GenreSplit } from './GenreSplit'
import { isLinked, type RefOption } from './refs'
import { useCopyToLibrary, type CopyToLibrary } from './useCopyToLibrary'

export interface CatalogRowProps {
  folderKey: string
  group: RefGroup
  /** The folder's catalog ids in order, carried in the drag payload with
   *  `onReorder` so a drop reorders the folder without reaching back into the
   *  form state. */
  catalogIDs: string[]
  position: number
  option: RefOption | undefined
  unit: FolderUnit
  usedInFolders(catalogID: string): number
  onReorder(catalogIDs: string[]): void
  onAddGenre(genre: string): void
  onRemoveRef(refKey: string): void
  onReorderGenres(refKeys: string[]): void
  onMove(direction: -1 | 1): void
  onRemove(): void
  onEdit(): void
  onCopyToLibrary: CopyToLibrary
  /** This catalog moved to a catalog of its own, which only this collection
   *  has, every genre of it kept. */
  onUnlink(): void
}

/**
 * One catalog in a folder, drawn once however many of its refs the folder
 * holds: each ref is one Nuvio tab (or row), and `GenreSplit` under the name
 * shows them as genre chips. Order is the value here — a catalog's refs sit
 * side by side, and their index becomes `folder_catalogs.sort_order` — so the
 * position is stated as well as draggable.
 *
 * An option this profile can't resolve is the *same* condition as a save that
 * would 400: `optionByID` merges the library with every scoped catalog this
 * editor already knows about (`CollectionEditor`'s `mergedOptionByID`), which
 * is exactly the closed-graph folder-ref rule `validateFolderRefs` checks
 * server-side.
 *
 * **Quiet Edit is scoped-catalog only.** It opens the referenced catalog one
 * level down, in a modal over this editor — `CollectionEditor`'s own nested
 * `CatalogEditor`, not the main pane — and a scoped catalog is only ever
 * used here, so editing it in place is unambiguous. A *listed* one is a live
 * pointer the same as it always was — editing it here would silently reach
 * every other folder and the library too — so this row has no inline edit
 * for it at all: it states how many places it's used. Editing a listed
 * catalog directly is the library rail's job.
 */
export function CatalogRow(props: CatalogRowProps) {
  const { folderKey, group, catalogIDs, position, option } = props
  const data: ReorderableData = { container: folderKey, ids: catalogIDs, reorder: props.onReorder }
  const sortable = useSortable({ id: dragID(folderKey, group.catalogID), data })
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = sortable
  const copyToLibrary = useCopyToLibrary(option, props.onCopyToLibrary)
  const name = option?.name ?? 'unavailable catalog'

  return (
    <li
      ref={setNodeRef}
      style={{ transform: CSS.Transform.toString(transform), transition }}
      className={`run-row ${isDragging ? 'bg-raised relative z-10 opacity-40' : ''}`}
    >
      <div className="run-ctl self-start pt-0.5">
        <Grip label={`Reorder ${name}`} sortable={{ attributes, listeners }} />
        <span className="type-data text-dimmer w-8 shrink-0 text-right text-[12.5px] tabular-nums">
          {position + 1}
        </span>
      </div>

      <div className="flex min-w-0 items-start gap-3">
        {option ? (
          <CatalogBody
            folderKey={folderKey}
            group={group}
            option={option}
            unit={props.unit}
            usedInFolders={props.usedInFolders}
            onAddGenre={props.onAddGenre}
            onRemoveRef={props.onRemoveRef}
            onReorderGenres={props.onReorderGenres}
          />
        ) : (
          <UnavailableBody />
        )}

        <div className="flex shrink-0 items-center gap-1">
          <InlineAction option={option} onEdit={props.onEdit} onRemove={props.onRemove} />
          <Toast toast={copyToLibrary.toast} />
          <CatalogMenu
            label={option?.name ?? 'this catalog'}
            first={position === 0}
            last={position === catalogIDs.length - 1}
            onMove={props.onMove}
            onRemove={props.onRemove}
            onCopyToLibrary={copyToLibrary.copy}
            copyAvailable={copyToLibrary.available}
            onUnlink={props.onUnlink}
            unlinkAvailable={isLinked(option)}
          />
        </div>
      </div>
    </li>
  )
}

interface CatalogBodyProps {
  folderKey: string
  group: RefGroup
  option: RefOption
  unit: FolderUnit
  usedInFolders(catalogID: string): number
  onAddGenre(genre: string): void
  onRemoveRef(refKey: string): void
  onReorderGenres(refKeys: string[]): void
}

/** The catalog's name, how many tabs it makes once it makes more than one,
 *  its recipe and where else it's used, then its genres. */
function CatalogBody({ folderKey, group, option, unit, usedInFolders, onAddGenre, onRemoveRef, onReorderGenres }: CatalogBodyProps) {
  return (
    <span className="flex min-w-0 flex-1 flex-col gap-0.5">
      <span className="flex min-w-0 flex-wrap items-center gap-2">
        <span className="truncate text-[13px] font-medium">{option.name}</span>
        <SharingStickers stickers={kindStickers(option.catalog)} />
        <UnitCount count={group.refs.length} unit={unit} />
      </span>
      <span className="type-data text-dimmer text-[12.5px] leading-[1.45]">
        {option.recipe}
        <UsedIn option={option} usedInFolders={usedInFolders} />
      </span>
      <GenreSplit
        folderKey={folderKey}
        group={group}
        catalog={option.catalog}
        name={option.name}
        unit={unit}
        onAddGenre={onAddGenre}
        onRemoveRef={onRemoveRef}
        onReorder={onReorderGenres}
      />
    </span>
  )
}

/** "3 tabs": how many Nuvio shows of this one catalog, said once there's
 *  more than one. */
function UnitCount({ count, unit }: { count: number; unit: FolderUnit }) {
  if (count < 2) return null
  return (
    <span className="border-catalog/55 text-catalog shrink-0 rounded-full border px-[7px] py-[4px] text-[10.5px] leading-none font-bold tracking-[0.06em] uppercase">
      {pluralCount(count, unit)}
    </span>
  )
}

/** " · used in 2 places" for a library catalog, counting the home screen
 *  here rather than in `usedInFolders`, so a change to the selection
 *  re-renders the rows that state it and not the workspace. Nothing for one
 *  that lives in this collection. */
function UsedIn({ option, usedInFolders }: { option: RefOption; usedInFolders(catalogID: string): number }) {
  const home = useHomeSelection()
  if (option.catalog.collection_id !== null) return null
  const places = usedInFolders(option.id) + (home.hasCatalog(option.id) ? 1 : 0)
  return <>{` · used in ${pluralCount(places, 'place')}`}</>
}

function UnavailableBody() {
  return (
    <>
      <span aria-hidden="true" className="text-danger bg-current w-[3px] shrink-0 self-stretch rounded-[1px]" />
      <span className="flex min-w-0 flex-1 flex-col gap-0.5">
        <span className="text-danger truncate text-[13px]">Unavailable catalog</span>
        <span className="type-data text-dimmer truncate text-[12.5px]">Deleted</span>
      </span>
    </>
  )
}

/** The row's one inline action: Edit for a catalog that lives in this
 *  collection, Remove for one that can't be resolved, nothing for a library
 *  one. */
function InlineAction({ option, onEdit, onRemove }: { option: RefOption | undefined; onEdit(): void; onRemove(): void }) {
  if (!option) {
    return (
      <button type="button" onClick={onRemove} className="btn-ghost px-2 text-[12px]">
        Remove
      </button>
    )
  }
  if (option.catalog.collection_id === null) return null
  return (
    <button type="button" onClick={onEdit} className="btn-ghost px-2 text-[12px]">
      Edit
    </button>
  )
}

interface CatalogMenuProps {
  label: string
  first: boolean
  last: boolean
  onMove(direction: -1 | 1): void
  onRemove(): void
  /** A library copy of the catalog as this editor holds it, staged edits
   *  included. */
  onCopyToLibrary(): void
  /** True for a catalog that lives in this collection, the only kind with a
   *  library copy to make. */
  copyAvailable: boolean
  onUnlink(): void
  /** True for a library catalog, the only kind there is to unlink. */
  unlinkAvailable: boolean
}

/**
 * Everything on a catalog row besides Edit and its genres: Edit is the row's
 * one inline action, and dragging the grip (pointer, touch or keyboard) is
 * the main way to reorder, so Move up/down and Remove wait behind "⋯" and the
 * name keeps the row's width at phone size. Each acts on the whole catalog,
 * every genre of it.
 */
function CatalogMenu({
  label,
  first,
  last,
  onMove,
  onRemove,
  onCopyToLibrary,
  copyAvailable,
  onUnlink,
  unlinkAvailable,
}: CatalogMenuProps) {
  return (
    <MoreMenu label={label}>
      <MoreMenuItem disabled={first} onSelect={() => onMove(-1)}>
        Move up
      </MoreMenuItem>
      <MoreMenuItem disabled={last} onSelect={() => onMove(1)}>
        Move down
      </MoreMenuItem>
      <CopyIntoLibraryItem available={copyAvailable} onSelect={onCopyToLibrary} />
      <UnlinkItem available={unlinkAvailable} onSelect={onUnlink} />
      <MoreMenuSeparator />
      <MoreMenuItem onSelect={onRemove}>Remove from folder</MoreMenuItem>
    </MoreMenu>
  )
}

/** Copy into library, in its own group, for a catalog that lives in this
 *  collection; nothing for any other. */
function CopyIntoLibraryItem({ available, onSelect }: { available: boolean; onSelect: () => void }) {
  if (!available) return null
  return (
    <>
      <MoreMenuSeparator />
      <MoreMenuItem onSelect={onSelect}>Copy into library</MoreMenuItem>
    </>
  )
}

/** Unlink from library, in its own group, for a library catalog; nothing for
 *  any other. */
function UnlinkItem({ available, onSelect }: { available: boolean; onSelect: () => void }) {
  if (!available) return null
  return (
    <>
      <MoreMenuSeparator />
      <MoreMenuItem onSelect={onSelect}>Unlink from library</MoreMenuItem>
    </>
  )
}
