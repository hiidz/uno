import { useMemo, useState } from 'react'
import type { ReactNode } from 'react'
import {
  DndContext,
  KeyboardSensor,
  PointerSensor,
  TouchSensor,
  closestCenter,
  useSensor,
  useSensors,
  type CollisionDetection,
  type DragEndEvent,
} from '@dnd-kit/core'
import {
  SortableContext,
  sortableKeyboardCoordinates,
  useSortable,
  verticalListSortingStrategy,
} from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { TypeBar } from '@/components/TypeBar'
import { Checkbox, FieldNote, Segmented, TextInput } from '@/components/fields'
import { CatalogRefPicker } from './CatalogRefPicker'
import { TILE_SHAPES, type FolderErrors, type FolderFormState } from './collectionForm'
import type { RefOption } from './refs'

/**
 * The folder tree's drag behaviour and the folder card itself.
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
  const sensors = useSensors(
    // Small activation distance so a click on a control inside a row still
    // registers as a click rather than starting a drag.
    useSensor(PointerSensor, { activationConstraint: { distance: 4 } }),
    // Touch takes a hold rather than a distance. A 4px threshold on a finger is
    // met by the start of a scroll, so `PointerSensor` alone made a grip inside
    // a scrolling page either a drag that fired on every swipe or a scroll that
    // never started — which is why the grips can drop `touch-none` for a
    // `touch-manipulation` that lets a quick swipe through.
    useSensor(TouchSensor, { activationConstraint: { delay: 200, tolerance: 6 } }),
    // The only way this tree is operable for anyone who can't drag.
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  )

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

function reorder(ids: string[], from: string, to: string): string[] {
  const fromIndex = ids.indexOf(from)
  const toIndex = ids.indexOf(to)
  if (fromIndex === -1 || toIndex === -1) return ids
  const next = [...ids]
  next.splice(toIndex, 0, ...next.splice(fromIndex, 1))
  return next
}

/** Drag listeners go on the grip alone, never the whole card — a folder card is
 *  full of inputs, and a row-wide drag surface would swallow every click in it.
 *  Typed off `useSortable`'s own return so dnd-kit owns the shape. */
function Grip({
  label,
  sortable,
}: {
  label: string
  sortable: Pick<ReturnType<typeof useSortable>, 'attributes' | 'listeners'>
}) {
  return (
    <button
      type="button"
      aria-label={label}
      className="tap text-dimmer hover:text-dim cursor-grab touch-none text-center leading-none transition-colors active:cursor-grabbing pointer-coarse:touch-manipulation"
      {...sortable.attributes}
      {...sortable.listeners}
    >
      ⠿
    </button>
  )
}

export function FolderCard({
  folder,
  position,
  total,
  errors,
  options,
  optionByID,
  onChange,
  onRemove,
  onAddRef,
  onRemoveRef,
}: {
  folder: FolderFormState
  position: number
  total: number
  /** Undefined until the user has tried to save — same rule as the catalog
   *  builder, which withholds errors until submit. */
  errors: FolderErrors | undefined
  options: RefOption[]
  optionByID: ReadonlyMap<string, RefOption>
  onChange: (update: Partial<FolderFormState>) => void
  onRemove: () => void
  onAddRef: (catalogID: string) => void
  onRemoveRef: (catalogID: string) => void
}) {
  const [picking, setPicking] = useState(false)
  const sortable = useSortable({
    id: folder.key,
    data: { container: FOLDERS } satisfies SortableData,
  })
  const { setNodeRef, transform, transition, isDragging } = sortable

  const label = folder.title.trim() || `folder ${position + 1}`
  // Memoised because it's the picker's `useMemo` dependency — a fresh Set every
  // render would re-filter the whole catalog list on every keystroke anywhere
  // in the card.
  const inFolder = useMemo(() => new Set(folder.catalogIDs), [folder.catalogIDs])

  return (
    <li
      ref={setNodeRef}
      style={{ transform: CSS.Transform.toString(transform), transition }}
      className={`border-line bg-ground flex flex-col gap-4 border p-4 ${
        isDragging ? 'relative z-10 opacity-40' : ''
      }`}
    >
      <div className="flex items-center gap-3">
        <Grip label={`Reorder ${label}`} sortable={sortable} />
        <span className="type-data text-dimmer w-[52px] shrink-0 text-[10.5px]">
          tab {position + 1}/{total}
        </span>
        {/* Sized, not stretched, and the remove button follows it directly:
            a title box spanning the card left Remove marooned at the far
            edge, a hand's width from the folder it removes. */}
        <div className="w-full max-w-[var(--w-entry)] min-w-0">
          <TextInput
            value={folder.title}
            onChange={(title) => onChange({ title })}
            placeholder="Folder title"
            ariaLabel={`Title of folder ${position + 1}`}
            invalid={Boolean(errors?.title)}
          />
        </div>
        <button
          type="button"
          onClick={onRemove}
          className="border-line-hi text-dim hover:text-danger hover:border-danger shrink-0 rounded-[2px] border px-2.5 py-1.5 text-[10.5px] tracking-[0.08em] uppercase transition-colors pointer-coarse:py-2.5"
        >
          Remove
        </button>
      </div>

      {errors?.title && <FieldNote tone="danger">{errors.title}</FieldNote>}

      {/* `max-content` on the first track, not a pixel guess: the tile-shape
          toggle names four shapes and a fixed 260px column clipped "Square"
          to "Sq". The track is now whatever the toggle needs. */}
      <div className="grid gap-x-8 gap-y-4 sm:grid-cols-[max-content_minmax(0,1fr)]">
        <div className="flex flex-col gap-2">
          <label className="type-eyebrow">Tile shape</label>
          <Segmented
            ariaLabel={`Tile shape for ${label}`}
            value={folder.tileShape}
            onChange={(tileShape) => onChange({ tileShape })}
            options={[
              // `''` is its own option, not normalised to POSTER on save: it's
              // a storable value the server accepts, and rewriting it would
              // edit data the user never touched.
              { value: '' as const, label: 'Default' },
              ...TILE_SHAPES.map((shape) => ({
                value: shape,
                label: shape.charAt(0) + shape.slice(1).toLowerCase(),
              })),
            ]}
          />
        </div>

        <div className="flex min-w-0 flex-col gap-4">
          <div className="flex items-end gap-2">
            <div className="flex w-[var(--w-code)] shrink-0 flex-col gap-2">
              <label className="type-eyebrow">Emoji</label>
              <TextInput
                value={folder.coverEmoji}
                onChange={(coverEmoji) => onChange({ coverEmoji })}
                placeholder="🎬"
                maxLength={8}
                ariaLabel={`Cover emoji for ${label}`}
              />
            </div>
            <div className="flex min-w-0 flex-1 flex-col gap-2">
              <label className="type-eyebrow">Cover image</label>
              <TextInput
                value={folder.coverImageURL}
                onChange={(coverImageURL) => onChange({ coverImageURL })}
                placeholder="https://…"
                ariaLabel={`Cover image URL for ${label}`}
              />
            </div>
          </div>
          <Checkbox
            checked={folder.hideTitle}
            onChange={(hideTitle) => onChange({ hideTitle })}
            label="Hide the title above the tiles"
            hint="The tab itself still shows the name."
          />
        </div>
      </div>

      <div className="flex flex-col gap-2">
        <div className="flex items-center gap-3">
          <span className="type-eyebrow flex-1">
            Catalogs{' '}
            <span className="type-data text-dimmer normal-case">
              ({folder.catalogIDs.length})
            </span>
          </span>
          {!picking && (
            <button type="button" onClick={() => setPicking(true)} className="btn-ghost">
              Add catalogs
            </button>
          )}
        </div>

        {errors?.catalogIDs && <FieldNote tone="danger">{errors.catalogIDs}</FieldNote>}

        {folder.catalogIDs.length === 0 ? (
          <p className="type-data text-dimmer m-0 py-1 text-[11px]">
            Empty — add a catalog, or this folder shows nothing.
          </p>
        ) : (
          <SortableContext
            items={folder.catalogIDs.map((id) => refDragID(folder.key, id))}
            strategy={verticalListSortingStrategy}
          >
            {/* Capped: the owner column is right-aligned in the row, and across
                a card-wide row it ended up an inch of empty space from the
                catalog it describes. */}
            <ul className="m-0 flex max-w-[42rem] list-none flex-col p-0">
              {folder.catalogIDs.map((catalogID, index) => (
                <RefRow
                  key={refDragID(folder.key, catalogID)}
                  folderKey={folder.key}
                  catalogID={catalogID}
                  catalogIDs={folder.catalogIDs}
                  position={index}
                  option={optionByID.get(catalogID)}
                  onRemove={() => onRemoveRef(catalogID)}
                />
              ))}
            </ul>
          </SortableContext>
        )}

        {picking && (
          <CatalogRefPicker
            options={options}
            exclude={inFolder}
            onAdd={onAddRef}
            onClose={() => setPicking(false)}
          />
        )}
      </div>
    </li>
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
 */
function RefRow({
  folderKey,
  catalogID,
  catalogIDs,
  position,
  option,
  onRemove,
}: {
  folderKey: string
  catalogID: string
  /** Carried in the drag payload so `onDragEnd` can reorder this folder's list
   *  without reaching back into the form state. */
  catalogIDs: string[]
  position: number
  option: RefOption | undefined
  onRemove: () => void
}) {
  const sortable = useSortable({
    id: refDragID(folderKey, catalogID),
    data: { container: folderKey, ids: catalogIDs },
  })
  const { setNodeRef, transform, transition, isDragging } = sortable

  // Kind and ownership have to exist as text: `TypeBar` carries both visually
  // and is `aria-hidden`.
  const owner = option
    ? `${option.catalog.type === 'movie' ? 'movie' : 'series'} · ${
        option.catalog.owned ? 'you' : 'community'
      }`
    : "can't be saved"

  return (
    <li
      ref={setNodeRef}
      style={{ transform: CSS.Transform.toString(transform), transition }}
      // The owner column is dropped below `sm` and the same words go under the
      // recipe instead. Held as a column at rail-and-pane widths it was 112px
      // against the 56px the catalog's own name was left with — the row spent
      // twice as much width saying "movie · community" as saying which catalog.
      className={`border-line hover:bg-raised grid grid-cols-[16px_3px_24px_minmax(0,1fr)_24px] items-center gap-x-3 border-b py-2 transition-colors sm:grid-cols-[16px_3px_24px_minmax(0,1fr)_7rem_24px] ${
        isDragging ? 'relative z-10 opacity-40' : ''
      }`}
    >
      <Grip label={`Reorder ${option?.name ?? 'unavailable catalog'}`} sortable={sortable} />

      {option ? (
        <TypeBar kind={option.catalog.type} owned={option.catalog.owned} className="min-h-[20px]" />
      ) : (
        <span aria-hidden="true" className="text-danger bg-current w-[3px] self-stretch rounded-[1px]" />
      )}

      <span className="type-data text-dimmer text-[10.5px]">{position + 1}</span>

      {option ? (
        <span className="flex min-w-0 flex-col gap-0.5">
          <span className="truncate text-[12.5px]">{option.name}</span>
          <span className="type-data text-dimmer truncate text-[10.5px]">{option.recipe}</span>
          <span className="type-data text-dimmer truncate text-[10.5px] sm:hidden">{owner}</span>
        </span>
      ) : (
        <span className="flex min-w-0 flex-col gap-0.5">
          <span className="text-danger truncate text-[12.5px]">Unavailable catalog</span>
          <span className="type-data text-dimmer truncate text-[10.5px]">
            Deleted, or made private by its owner
          </span>
          <span className="type-data text-dimmer truncate text-[10.5px] sm:hidden">{owner}</span>
        </span>
      )}

      {/* From `sm`: a fixed track, not `auto`, because these read down the
          folder as a column and sized to their content each row's owner label
          started at a different x. Narrower than that the column is gone and
          the same words sit under the recipe above. */}
      <span className="type-data text-dimmer hidden truncate text-[10.5px] sm:block">{owner}</span>

      <button
        type="button"
        onClick={onRemove}
        className="tap text-dimmer hover:text-danger grid h-6 w-6 place-items-center rounded-[2px] text-[13px] leading-none transition-colors"
      >
        <span aria-hidden="true">×</span>
        <span className="sr-only">
          Remove {option?.name ?? 'this unavailable catalog'} from this folder
        </span>
      </button>
    </li>
  )
}
