import { useMemo } from 'react'
import type { ReactNode } from 'react'
import { DndContext, type Announcements, type DragEndEvent } from '@dnd-kit/core'
import { SortableContext, rectSortingStrategy, useSortable } from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { ChevronLeft, ChevronRight, TriangleAlert } from 'lucide-react'
import { Grip, RowIconButton, useDragSensors } from '@/components/dnd'
import { Icon } from '@/components/Icon'
import { TILE_ASPECT } from '@/features/preview/tiles'
import { reorder } from '@/lib/order'
import { pluralCount } from '@/lib/plural'
import { folderLabel, type FolderFormState } from './collectionForm'
import { FOLDERS, containerOf, refDragID, withinContainer, type SortableData } from './folderDnd'

/**
 * The folder tree's drag behaviour and the folder tile strip — DESIGN.md's
 * "Folder strip". `FolderTreeDnd` is the one `DndContext` both the tiles and
 * the open folder's catalog rows sort inside; see `folderDnd.ts` for how it
 * keeps the lists apart. Moving a ref *between* folders is not supported:
 * remove and re-add instead.
 */

/**
 * dnd-kit's default live-region announcements read out the raw drag id
 * ("Draggable item f1 was moved over droppable area f2") — meaningless to a
 * screen reader user, since ids are session-local keys or catalog uuids, not
 * anything shown on screen. This translates every id back to the name
 * already visible for it before handing it to the drag's `aria-live` region.
 */
function buildAnnouncements(
  folderName: (folderKey: string) => string,
  refName: (refKey: string) => string,
): Announcements {
  function describe(id: string | number): string {
    const raw = String(id)
    const separator = raw.indexOf('::')
    if (separator === -1) return `folder “${folderName(raw)}”`
    return `catalog “${refName(raw.slice(separator + 2))}”`
  }
  return {
    onDragStart: ({ active }) => `Picked up ${describe(active.id)}.`,
    onDragOver: ({ active, over }) =>
      over
        ? `${describe(active.id)} is now over ${describe(over.id)}.`
        : `${describe(active.id)} is no longer over a droppable area.`,
    onDragEnd: ({ active, over }) =>
      over
        ? `${describe(active.id)} was dropped over ${describe(over.id)}.`
        : `${describe(active.id)} was dropped.`,
    onDragCancel: ({ active }) => `Reordering ${describe(active.id)} was cancelled.`,
  }
}

export function FolderTreeDnd({
  folderKeys,
  folderName,
  refName,
  onReorderFolders,
  onReorderRefs,
  children,
}: {
  folderKeys: string[]
  /** Looks up a folder's display name from its `key` — used only to word the
   *  drag-and-drop live-region announcements. */
  folderName: (folderKey: string) => string
  /** Looks up a catalog ref's display name from its `key` — same. */
  refName: (refKey: string) => string
  onReorderFolders: (orderedKeys: string[]) => void
  onReorderRefs: (folderKey: string, orderedRefKeys: string[]) => void
  children: ReactNode
}) {
  const sensors = useDragSensors()
  const announcements = useMemo(
    () => buildAnnouncements(folderName, refName),
    [folderName, refName],
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
    <DndContext
      sensors={sensors}
      collisionDetection={withinContainer}
      onDragEnd={handleDragEnd}
      accessibility={{ announcements }}
    >
      <SortableContext items={folderKeys} strategy={rectSortingStrategy}>
        {children}
      </SortableContext>
    </DndContext>
  )
}

/**
 * The folder strip: each folder drawn at its own `tile_shape`, the way Nuvio
 * draws the collection's row. One is always selected, and its contents open
 * beneath the strip in `FolderDetail`.
 */
export function FolderTiles({
  folders,
  selectedKey,
  errorKeys,
  onSelect,
  onMove,
}: {
  folders: FolderFormState[]
  selectedKey: string | null
  /** Folders with a validation error, marked on their tile so a problem in a
   *  folder that isn't open is still visible. */
  errorKeys: ReadonlySet<string>
  onSelect: (key: string) => void
  onMove: (key: string, direction: -1 | 1) => void
}) {
  return (
    <ul className="fold-tiles">
      {folders.map((folder, index) => (
        <FolderTileItem
          key={folder.key}
          folder={folder}
          position={index}
          total={folders.length}
          selected={folder.key === selectedKey}
          hasError={errorKeys.has(folder.key)}
          onSelect={() => onSelect(folder.key)}
          onMove={(direction) => onMove(folder.key, direction)}
        />
      ))}
    </ul>
  )
}

const TILE_H = 104

function FolderTileItem({
  folder,
  position,
  total,
  selected,
  hasError,
  onSelect,
  onMove,
}: {
  folder: FolderFormState
  position: number
  total: number
  selected: boolean
  hasError: boolean
  onSelect: () => void
  onMove: (direction: -1 | 1) => void
}) {
  const sortable = useSortable({
    id: folder.key,
    data: { container: FOLDERS } satisfies SortableData,
  })
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = sortable
  const width = TILE_H * TILE_ASPECT[folder.tileShape]
  const name = folder.title.trim() || 'Untitled folder'

  return (
    <li
      ref={setNodeRef}
      style={{ transform: CSS.Transform.toString(transform), transition, width }}
      className={`fold-tile ${selected ? 'is-selected' : ''} ${isDragging ? 'relative z-10 opacity-40' : ''}`}
    >
      <button
        type="button"
        onClick={onSelect}
        aria-pressed={selected}
        aria-label={`${name}, ${pluralCount(folder.refs.length, 'catalog')}${hasError ? ', needs fixing' : ''}`}
        className="fold-tile-btn"
      >
        <span className="fold-tile-face" style={{ height: TILE_H }}>
          {folder.coverEmoji ? (
            <span aria-hidden="true" className="text-[26px] leading-none">
              {folder.coverEmoji}
            </span>
          ) : (
            <span aria-hidden="true" className="text-dimmer line-clamp-2 px-2 text-center text-[12.5px] leading-tight">
              {name}
            </span>
          )}
          {/* Layered over the emoji/title rather than replacing them, so a
              cover URL that 404s or is slow leaves a legible tile. */}
          {folder.coverImageURL.trim() && (
            <img src={folder.coverImageURL} alt="" loading="lazy" className="absolute inset-0 h-full w-full object-cover" />
          )}
        </span>
        <span className="fold-tile-name">
          {hasError && <Icon icon={TriangleAlert} size={12} className="text-danger shrink-0" />}
          <span className="truncate">{name}</span>
        </span>
        <span className="type-data text-dimmer text-[12.5px] tabular-nums">
          {pluralCount(folder.refs.length, 'catalog')}
        </span>
      </button>
      <span className="fold-tile-grip">
        <Grip label={`Reorder ${folderLabel(folder, position)}`} sortable={{ attributes, listeners }} />
      </span>
      {selected && (
        <FolderMoveArrows label={folderLabel(folder, position)} position={position} total={total} onMove={onMove} />
      )}
    </li>
  )
}

/** ← and → under the selected tile: the same reorder as dragging it, for
 *  anyone who can't. */
function FolderMoveArrows({
  label,
  position,
  total,
  onMove,
}: {
  label: string
  position: number
  total: number
  onMove: (direction: -1 | 1) => void
}) {
  return (
    <div className="fold-tile-move">
      <RowIconButton
        icon={ChevronLeft}
        label={`Move ${label} left${position === 0 ? ', already first' : ''}`}
        disabled={position === 0}
        onClick={() => onMove(-1)}
      />
      <RowIconButton
        icon={ChevronRight}
        label={`Move ${label} right${position === total - 1 ? ', already last' : ''}`}
        disabled={position === total - 1}
        onClick={() => onMove(1)}
      />
    </div>
  )
}
