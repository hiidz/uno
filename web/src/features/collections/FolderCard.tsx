import { useMemo, useState } from 'react'
import type { ReactNode } from 'react'
import { DndContext, closestCenter, type CollisionDetection, type DragEndEvent } from '@dnd-kit/core'
import { SortableContext, useSortable, verticalListSortingStrategy } from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { ChevronDown, MoreHorizontal } from 'lucide-react'
import { DropdownMenu } from 'radix-ui'
import { Grip, MoveDownButton, MoveUpButton, reorder, useDragSensors } from '@/components/dnd'
import { FieldNote, Segmented, TextInput } from '@/components/fields'
import { Icon } from '@/components/Icon'
import { ordinal } from '@/lib/ordinal'
import { pluralCount } from '@/lib/plural'
import { CatalogRefPicker } from './CatalogRefPicker'
import { TILE_SHAPES, type FolderErrors, type FolderFormState, type FolderTileShape } from './collectionForm'
import type { RefOption } from './refs'

/**
 * The folder tree's drag behaviour and the folder running-order row itself —
 * DESIGN.md's "Folder running order" and "Open folder body".
 *
 * **One `DndContext`, several `SortableContext`s.** Folders reorder among
 * themselves; each folder's catalog refs reorder among themselves. Nesting a
 * second `DndContext` inside the first does not isolate them — the outer
 * context still sees the inner drags. So every sortable declares which list it
 * belongs to via dnd-kit's `data`, and `onDragEnd` reorders only when the
 * dragged item and the one it was dropped on agree. A ref dragged out of its
 * folder is a no-op rather than a mis-drop.
 *
 * Moving a ref *between* folders is not supported: remove and re-add instead.
 */

const FOLDERS = '__folders__'

interface SortableData {
  container: string
}

function containerOf(node: { data: { current?: unknown } } | null | undefined): string | null {
  const data = node?.data.current as SortableData | undefined
  return data?.container ?? null
}

/**
 * Rank only the droppables in the dragged item's own list.
 *
 * Load-bearing. Folders and every ref row share one `DndContext`, and plain
 * `closestCenter` ranks all of them together — so dragging a tall folder card
 * past its neighbour usually finds a *ref row inside* that neighbour to be the
 * nearest center. `onDragEnd` then sees two different containers, refuses to
 * guess, and the folder springs back. Narrowing the candidate set here leaves
 * that container check as a backstop rather than the mechanism.
 */
const withinContainer: CollisionDetection = (args) => {
  const container = containerOf(args.active)
  return closestCenter({
    ...args,
    droppableContainers: args.droppableContainers.filter(
      (candidate) => containerOf(candidate) === container,
    ),
  })
}

/** Composite so the same catalog in two folders yields two distinct drag ids —
 *  which the schema explicitly allows. */
function refDragID(folderKey: string, catalogID: string): string {
  return `${folderKey}::${catalogID}`
}

export function FolderTreeDnd({
  folderKeys,
  onReorderFolders,
  onReorderRefs,
  children,
}: {
  folderKeys: string[]
  onReorderFolders: (orderedKeys: string[]) => void
  onReorderRefs: (folderKey: string, orderedCatalogIDs: string[]) => void
  children: ReactNode
}) {
  const sensors = useDragSensors()

  function handleDragEnd(event: DragEndEvent) {
    const { active, over } = event
    if (!over || active.id === over.id) return

    const container = containerOf(active)
    // A drop onto a different list is ignored rather than guessed at.
    if (!container || container !== containerOf(over)) return

    const activeID = String(active.id)
    const overID = String(over.id)

    if (container === FOLDERS) {
      onReorderFolders(reorder(folderKeys, activeID, overID))
      return
    }

    const ids = (active.data.current as (SortableData & { ids: string[] }) | undefined)?.ids
    if (!ids) return
    const ordered = reorder(
      ids.map((id) => refDragID(container, id)),
      activeID,
      overID,
    ).map((dragID) => dragID.slice(container.length + 2))
    onReorderRefs(container, ordered)
  }

  return (
    <DndContext sensors={sensors} collisionDetection={withinContainer} onDragEnd={handleDragEnd}>
      <SortableContext items={folderKeys} strategy={verticalListSortingStrategy}>
        {children}
      </SortableContext>
    </DndContext>
  )
}

const SHAPE_LABEL: Record<FolderTileShape, string> = {
  '': 'poster',
  POSTER: 'poster',
  LANDSCAPE: 'landscape',
  SQUARE: 'square',
}

export function FolderCard({
  folder,
  position,
  total,
  open,
  onToggleOpen,
  errors,
  options,
  optionByID,
  onChange,
  onMove,
  onRemove,
  onAddRef,
  onRemoveRef,
  onMoveRef,
}: {
  folder: FolderFormState
  position: number
  total: number
  /** One folder open at a time (DESIGN.md's "One folder is open at a time") —
   *  held by the editor, not here, the same shape the catalog editor's
   *  collapsible sections use. */
  open: boolean
  onToggleOpen: () => void
  /** Undefined until the user has tried to save — same rule as the catalog
   *  builder, which withholds errors until submit. */
  errors: FolderErrors | undefined
  options: RefOption[]
  optionByID: ReadonlyMap<string, RefOption>
  onChange: (update: Partial<FolderFormState>) => void
  onMove: (direction: -1 | 1) => void
  onRemove: () => void
  onAddRef: (catalogID: string) => void
  onRemoveRef: (catalogID: string) => void
  onMoveRef: (catalogID: string, direction: -1 | 1) => void
}) {
  const [picking, setPicking] = useState(false)
  const sortable = useSortable({
    id: folder.key,
    data: { container: FOLDERS } satisfies SortableData,
  })
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = sortable

  const label = folder.title.trim() || `folder ${position + 1}`
  // Memoised because it's the picker's `useMemo` dependency — a fresh Set every
  // render would re-filter the whole catalog list on every keystroke anywhere
  // in the card.
  const inFolder = useMemo(() => new Set(folder.catalogIDs), [folder.catalogIDs])

  return (
    <li
      ref={setNodeRef}
      style={{ transform: CSS.Transform.toString(transform), transition }}
      className={`border-line border-b ${isDragging ? 'bg-raised relative z-10 opacity-40' : ''}`}
    >
      <div className="flex items-center gap-3 py-2.5 pr-2 pl-1">
        <div className="flex shrink-0 items-center gap-1">
          <Grip label={`Reorder ${label}`} sortable={{ attributes, listeners }} />
          <MoveUpButton
            label={`Move ${label} up${position === 0 ? ', already first' : ''}`}
            disabled={position === 0}
            onClick={() => onMove(-1)}
          />
          <MoveDownButton
            label={`Move ${label} down${position === total - 1 ? ', already last' : ''}`}
            disabled={position === total - 1}
            onClick={() => onMove(1)}
          />
        </div>

        <span className="type-data text-dim w-10 shrink-0 text-right text-[11px] tabular-nums">
          {ordinal(position + 1)}
        </span>

        <button
          type="button"
          onClick={onToggleOpen}
          aria-expanded={open}
          className="hover:bg-raised flex min-w-0 flex-1 items-center gap-2 rounded-[2px] py-1 pr-1 pl-1.5 text-left transition-colors"
        >
          <span aria-hidden="true" className="w-5 shrink-0 text-center text-[16px] leading-none">
            {folder.coverEmoji || '📁'}
          </span>
          <span className="truncate text-[13px] font-medium">{label}</span>
          <span className="type-data text-dimmer ml-auto hidden shrink-0 truncate text-[10.5px] sm:block">
            {SHAPE_LABEL[folder.tileShape]} tiles · {pluralCount(folder.catalogIDs.length, 'catalog')}
          </span>
          <Icon
            icon={ChevronDown}
            size={16}
            className={`shrink-0 transition-transform ${open ? 'text-ink rotate-180' : 'text-dim'}`}
          />
        </button>

        <FolderMenu label={label} open={open} onToggleOpen={onToggleOpen} onRemove={onRemove} />
      </div>

      {errors?.title && !open && (
        <div className="pb-2 pl-16">
          <FieldNote tone="danger">{errors.title}</FieldNote>
        </div>
      )}

      {open && (
        <div className="flex flex-col gap-5 pb-5 pl-16">
          <div className="flex flex-col gap-1.5">
            <label className="type-eyebrow">Folder title</label>
            <TextInput
              value={folder.title}
              onChange={(title) => onChange({ title })}
              placeholder="Folder title"
              ariaLabel={`Title of folder ${position + 1}`}
              invalid={Boolean(errors?.title)}
            />
            {errors?.title && <FieldNote tone="danger">{errors.title}</FieldNote>}
          </div>

          <div className="flex flex-col gap-1.5">
            <span className="type-eyebrow">Hide the title</span>
            <Segmented
              ariaLabel={`Hide the title above ${label}'s tiles`}
              value={folder.hideTitle ? 'hide' : 'show'}
              onChange={(value) => onChange({ hideTitle: value === 'hide' })}
              options={[
                { value: 'show', label: 'Show it' },
                { value: 'hide', label: 'Hide it' },
              ]}
            />
            <FieldNote>The tab itself still shows the name.</FieldNote>
          </div>

          <div className="flex flex-col gap-1.5">
            <span className="type-eyebrow">Tile shape</span>
            <div className="choices" role="group" aria-label={`Tile shape for ${label}`}>
              {/* `''` is its own option, not normalised to POSTER on save: it's
                  a storable value the server accepts, and rewriting it would
                  edit data the user never touched. */}
              <button
                type="button"
                className="choice"
                aria-pressed={folder.tileShape === ''}
                onClick={() => onChange({ tileShape: '' })}
              >
                Default
              </button>
              {TILE_SHAPES.map((shape) => (
                <button
                  key={shape}
                  type="button"
                  className="choice"
                  aria-pressed={folder.tileShape === shape}
                  onClick={() => onChange({ tileShape: shape })}
                >
                  {shape.charAt(0) + shape.slice(1).toLowerCase()}
                </button>
              ))}
            </div>
          </div>

          <div className="flex items-end gap-3">
            <div className="flex w-[var(--w-code)] shrink-0 flex-col gap-1.5">
              <label className="type-eyebrow">Cover emoji</label>
              <TextInput
                value={folder.coverEmoji}
                onChange={(coverEmoji) => onChange({ coverEmoji })}
                placeholder="🎬"
                maxLength={8}
                ariaLabel={`Cover emoji for ${label}`}
              />
            </div>
            <div className="flex min-w-0 flex-1 flex-col gap-1.5">
              <label className="type-eyebrow">Cover image</label>
              <TextInput
                value={folder.coverImageURL}
                onChange={(coverImageURL) => onChange({ coverImageURL })}
                placeholder="https://…"
                ariaLabel={`Cover image URL for ${label}`}
              />
            </div>
          </div>

          <div className="flex flex-col gap-2 pt-1">
            <span className="type-eyebrow">
              Catalogs in this folder{' '}
              <span className="type-data text-dimmer normal-case">
                ({folder.catalogIDs.length})
              </span>
            </span>

            {errors?.catalogIDs && <FieldNote tone="danger">{errors.catalogIDs}</FieldNote>}

            {folder.catalogIDs.length === 0 ? (
              <p className="type-data text-dimmer m-0 py-1 text-[11px]">
                This folder needs at least one catalog to show anything on your TV.
              </p>
            ) : (
              <SortableContext
                items={folder.catalogIDs.map((id) => refDragID(folder.key, id))}
                strategy={verticalListSortingStrategy}
              >
                <ul className="border-line m-0 flex list-none flex-col border-t p-0">
                  {folder.catalogIDs.map((catalogID, index) => (
                    <RefRow
                      key={refDragID(folder.key, catalogID)}
                      folderKey={folder.key}
                      catalogID={catalogID}
                      catalogIDs={folder.catalogIDs}
                      position={index}
                      first={index === 0}
                      last={index === folder.catalogIDs.length - 1}
                      option={optionByID.get(catalogID)}
                      onRemove={() => onRemoveRef(catalogID)}
                      onMove={(direction) => onMoveRef(catalogID, direction)}
                    />
                  ))}
                </ul>
              </SortableContext>
            )}

            <div className="flex flex-col gap-2 pt-1">
              {!picking && (
                <button type="button" onClick={() => setPicking(true)} className="btn-secondary btn-sm self-start">
                  Add catalogs
                </button>
              )}
              {picking && (
                <>
                  <p className="type-data text-dimmer m-0 text-[11px] leading-[1.45]">
                    Adds an existing catalog from your library. It stays linked — later edits to
                    the original reach every folder that references it.
                  </p>
                  <CatalogRefPicker
                    options={options}
                    exclude={inFolder}
                    onAdd={onAddRef}
                    onClose={() => setPicking(false)}
                  />
                </>
              )}
            </div>
          </div>

          <button type="button" onClick={onRemove} className="btn-quiet self-start">
            Remove this folder
          </button>
        </div>
      )}
    </li>
  )
}

function FolderMenu({
  label,
  open,
  onToggleOpen,
  onRemove,
}: {
  label: string
  open: boolean
  onToggleOpen: () => void
  onRemove: () => void
}) {
  return (
    <DropdownMenu.Root modal={false}>
      <DropdownMenu.Trigger
        aria-label={`More for ${label}`}
        className="tap text-dimmer hover:bg-line hover:text-ink grid h-8 w-8 shrink-0 place-items-center rounded-[2px] transition-colors"
      >
        <Icon icon={MoreHorizontal} size={16} />
      </DropdownMenu.Trigger>
      <DropdownMenu.Portal>
        <DropdownMenu.Content
          align="end"
          sideOffset={4}
          className="bg-raised-hi border-line-hi z-40 flex w-60 flex-col gap-0.5 rounded-[2px] border p-1.5 shadow-[0_12px_28px_rgba(0,0,0,0.55)]"
        >
          <DropdownMenu.Item
            onSelect={onToggleOpen}
            className="hover:bg-line focus-visible:bg-line text-ink flex items-center rounded-[2px] px-2 py-2 text-left text-[12px] transition-colors"
          >
            {open ? 'Close folder' : 'Open folder'}
          </DropdownMenu.Item>
          <DropdownMenu.Separator className="bg-line my-1 h-px" />
          <DropdownMenu.Item
            onSelect={onRemove}
            className="hover:bg-line focus-visible:bg-line text-danger flex items-center rounded-[2px] px-2 py-2 text-left text-[12px] transition-colors"
          >
            Remove folder
          </DropdownMenu.Item>
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  )
}

/**
 * One catalog reference. Order is the value here — index becomes
 * `folder_catalogs.sort_order` — so the position is stated as well as draggable.
 *
 * An option this profile can't resolve is the *same* condition as a save that
 * would 400: the builder's only source of catalogs is the library, and the
 * library predicate and `validateAccess`'s predicate are identical. (The Home
 * pane can't collapse the two that way — its selection endpoint supplies rows
 * the library doesn't have.)
 *
 * **No Edit here, on purpose.** DESIGN.md's folder-catalog row carries a quiet
 * Edit that opens the folder's own copy of the catalog one level down — that
 * assumes the folder-catalogs-as-private-copies model this migration phase
 * explicitly doesn't build (see the migration plan's §2 and Phase 5 scope
 * note). Today a folder ref is a live pointer at the same catalog row
 * everywhere it's used, so an "Edit" here would open the real catalog editor
 * and silently change it for every other folder and the library too — DESIGN's
 * copy note text off. Rather than ship copy that implies isolation this app
 * doesn't have, editing a folder's catalog stays where it already is: select
 * it from the library rail.
 */
function RefRow({
  folderKey,
  catalogID,
  catalogIDs,
  position,
  first,
  last,
  option,
  onRemove,
  onMove,
}: {
  folderKey: string
  catalogID: string
  /** Carried in the drag payload so `onDragEnd` can reorder this folder's list
   *  without reaching back into the form state. */
  catalogIDs: string[]
  position: number
  first: boolean
  last: boolean
  option: RefOption | undefined
  onRemove: () => void
  onMove: (direction: -1 | 1) => void
}) {
  const sortable = useSortable({
    id: refDragID(folderKey, catalogID),
    data: { container: folderKey, ids: catalogIDs },
  })
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = sortable

  const owner = option
    ? `${option.catalog.type === 'movie' ? 'movie' : 'series'} · ${
        option.catalog.owned ? 'you' : 'community'
      }`
    : "can't be saved"

  return (
    <li
      ref={setNodeRef}
      style={{ transform: CSS.Transform.toString(transform), transition }}
      className={`border-line flex items-center gap-3 border-b py-2.5 ${
        isDragging ? 'bg-raised relative z-10 opacity-40' : ''
      }`}
    >
      <Grip label={`Reorder ${option?.name ?? 'unavailable catalog'}`} sortable={{ attributes, listeners }} />
      <span className="type-data text-dimmer w-5 shrink-0 text-center text-[11px] tabular-nums">
        {position + 1}
      </span>

      {!option && (
        <span aria-hidden="true" className="text-danger bg-current w-[3px] shrink-0 self-stretch rounded-[1px]" />
      )}

      {option ? (
        <span className="flex min-w-0 flex-1 flex-col gap-0.5">
          <span className="truncate text-[12.5px] font-medium">{option.name}</span>
          <span className="type-data text-dimmer truncate text-[10.5px]">
            {option.recipe} · {owner}
          </span>
        </span>
      ) : (
        <span className="flex min-w-0 flex-1 flex-col gap-0.5">
          <span className="text-danger truncate text-[12.5px]">Unavailable catalog</span>
          <span className="type-data text-dimmer truncate text-[10.5px]">
            Deleted, or made private by its owner
          </span>
        </span>
      )}

      <div className="flex shrink-0 items-center gap-0.5">
        <MoveUpButton
          label={`Move ${option?.name ?? 'this catalog'} up${first ? ', already first' : ''}`}
          disabled={first}
          onClick={() => onMove(-1)}
        />
        <MoveDownButton
          label={`Move ${option?.name ?? 'this catalog'} down${last ? ', already last' : ''}`}
          disabled={last}
          onClick={() => onMove(1)}
        />
        <button type="button" onClick={onRemove} className="btn-quiet px-2 text-[11px]">
          Remove
        </button>
      </div>
    </li>
  )
}
