import { useMemo, useState } from 'react'
import type { ReactNode } from 'react'
import { DndContext, closestCenter, type CollisionDetection, type DragEndEvent } from '@dnd-kit/core'
import { SortableContext, useSortable, verticalListSortingStrategy } from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { ChevronDown, Copy, MoreHorizontal, Plus } from 'lucide-react'
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
  collectionID,
  usedInPlaces,
  onChange,
  onMove,
  onRemove,
  onAddRef,
  onCopyRefIntoCollection,
  onRemoveRef,
  onMoveRef,
  onEditRef,
  onCopyRef,
  onAddNewInCollection,
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
  /** Merged: the library plus every scoped catalog this editor already knows
   *  about — see `CollectionEditor`'s `mergedOptionByID`. Scoped catalogs
   *  never appear in `options` (only listed ones are linkable), but they do
   *  need to render once referenced. */
  optionByID: ReadonlyMap<string, RefOption>
  /** This collection's own server id. Absent until the first Save — "copy"
   *  and "new" both need a real collection row to scope a catalog to. */
  collectionID?: string
  /** Home screen plus every folder across every owned collection — only
   *  meaningful for a listed catalog, so the row asks with its own id. */
  usedInPlaces: (catalogID: string) => number
  onChange: (update: Partial<FolderFormState>) => void
  onMove: (direction: -1 | 1) => void
  onRemove: () => void
  onAddRef: (catalogID: string) => void
  /** "Copy" from the picker: adds a fresh scoped copy as a new ref, rather
   *  than linking the listed catalog picked. */
  onCopyRefIntoCollection: (catalogID: string) => void
  onRemoveRef: (catalogID: string) => void
  onMoveRef: (catalogID: string, direction: -1 | 1) => void
  onEditRef: (catalogID: string) => void
  /** "Copy into this collection" offered on an already-linked listed
   *  catalog's own row: replaces this ref with a fresh scoped copy in place,
   *  so the folder's order doesn't change. */
  onCopyRef: (catalogID: string) => void
  onAddNewInCollection: () => void
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
                      usedInPlaces={usedInPlaces}
                      onRemove={() => onRemoveRef(catalogID)}
                      onMove={(direction) => onMoveRef(catalogID, direction)}
                      onEdit={() => onEditRef(catalogID)}
                      onCopyIntoCollection={collectionID ? () => onCopyRef(catalogID) : undefined}
                    />
                  ))}
                </ul>
              </SortableContext>
            )}

            <div className="flex flex-col gap-2 pt-1">
              {!picking && (
                <div className="flex flex-wrap items-center gap-2">
                  <button type="button" onClick={() => setPicking(true)} className="btn-secondary btn-sm self-start">
                    Add catalogs
                  </button>
                  {collectionID && (
                    <button type="button" onClick={onAddNewInCollection} className="btn-secondary btn-sm self-start">
                      New catalog
                    </button>
                  )}
                </div>
              )}
              {picking && (
                <>
                  <p className="type-data text-dimmer m-0 text-[11px] leading-[1.45]">
                    <Icon icon={Plus} size={11} className="mb-px inline" /> links an existing catalog from
                    your library — later edits to it reach every folder that references it.
                    {collectionID && (
                      <>
                        {' '}
                        <Icon icon={Copy} size={11} className="mb-px inline" /> copies it into a fresh
                        catalog only this collection has, which the original can't affect.
                      </>
                    )}
                  </p>
                  <CatalogRefPicker
                    options={options}
                    exclude={inFolder}
                    onAdd={onAddRef}
                    onCopy={collectionID ? onCopyRefIntoCollection : undefined}
                    onClose={() => setPicking(false)}
                    footer={
                      !collectionID ? (
                        <p className="type-data text-dimmer m-0 pt-1 text-[11px] leading-[1.45]">
                          Save this collection to also copy a catalog in or create one new, scoped
                          to it.
                        </p>
                      ) : undefined
                    }
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
 * would 400: `optionByID` merges the library with every scoped catalog this
 * editor already knows about (`CollectionEditor`'s `mergedOptionByID`), which
 * is exactly the closed-graph folder-ref rule `validateFolderRefs` checks
 * server-side.
 *
 * **Quiet Edit** opens the referenced catalog one level down, in a modal over
 * this editor — `CollectionEditor`'s own nested `CatalogEditor`, not the main
 * pane. A scoped catalog is only ever used here, so editing it in place is
 * unambiguous. A *listed* one is a live pointer the same as it always was —
 * editing it here still reaches every other folder and the library — so this
 * row also says how many places that is and offers "Copy into this
 * collection" for whoever wants an independent copy instead.
 */
function RefRow({
  folderKey,
  catalogID,
  catalogIDs,
  position,
  first,
  last,
  option,
  usedInPlaces,
  onRemove,
  onMove,
  onEdit,
  onCopyIntoCollection,
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
  usedInPlaces: (catalogID: string) => number
  onRemove: () => void
  onMove: (direction: -1 | 1) => void
  onEdit: () => void
  /** Present only once this collection has a server id to scope a copy to. */
  onCopyIntoCollection?: () => void
}) {
  const sortable = useSortable({
    id: refDragID(folderKey, catalogID),
    data: { container: folderKey, ids: catalogIDs },
  })
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = sortable

  const kind = option ? (option.catalog.type === 'movie' ? 'movie' : 'series') : "can't be saved"
  const isScoped = option ? option.catalog.collection_id !== null : false
  const places = option && !isScoped ? usedInPlaces(catalogID) : 0

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
            {option.recipe} · {kind}
            {!isScoped && ` · used in ${pluralCount(places, 'place')}`}
          </span>
        </span>
      ) : (
        <span className="flex min-w-0 flex-1 flex-col gap-0.5">
          <span className="text-danger truncate text-[12.5px]">Unavailable catalog</span>
          <span className="type-data text-dimmer truncate text-[10.5px]">Deleted</span>
        </span>
      )}

      <div className="flex shrink-0 items-center gap-0.5">
        {option && (
          <RefMenu
            label={option.name}
            onEdit={onEdit}
            onCopyIntoCollection={!isScoped ? onCopyIntoCollection : undefined}
          />
        )}
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

/**
 * "Edit" and "Copy in" tucked into a "⋯" menu — the same fix `FolderMenu`
 * already applies at the folder level, for the same reason: this row also
 * carries Move up/down and Remove, and five always-visible actions overflow
 * the name/recipe text into unreadable truncation at phone width (measured
 * in a dev-loop check). Move up/down and Remove stay
 * inline — they're the same three actions this row has always had.
 */
function RefMenu({
  label,
  onEdit,
  onCopyIntoCollection,
}: {
  label: string
  onEdit: () => void
  /** Absent for a scoped catalog — nothing else can reference it, so there's
   *  nothing to copy it away from. */
  onCopyIntoCollection?: () => void
}) {
  return (
    <DropdownMenu.Root modal={false}>
      <DropdownMenu.Trigger
        aria-label={`More for ${label}`}
        className="tap text-dimmer hover:bg-line hover:text-ink grid h-7 w-7 shrink-0 place-items-center rounded-[2px] transition-colors"
      >
        <Icon icon={MoreHorizontal} size={14} />
      </DropdownMenu.Trigger>
      <DropdownMenu.Portal>
        <DropdownMenu.Content
          align="end"
          sideOffset={4}
          className="bg-raised-hi border-line-hi z-40 flex w-52 flex-col gap-0.5 rounded-[2px] border p-1.5 shadow-[0_12px_28px_rgba(0,0,0,0.55)]"
        >
          <DropdownMenu.Item
            onSelect={onEdit}
            className="hover:bg-line focus-visible:bg-line text-ink flex items-center rounded-[2px] px-2 py-2 text-left text-[12px] transition-colors"
          >
            Edit
          </DropdownMenu.Item>
          {onCopyIntoCollection && (
            <DropdownMenu.Item
              onSelect={onCopyIntoCollection}
              className="hover:bg-line focus-visible:bg-line text-ink flex items-center rounded-[2px] px-2 py-2 text-left text-[12px] transition-colors"
            >
              Copy into this collection
            </DropdownMenu.Item>
          )}
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  )
}
