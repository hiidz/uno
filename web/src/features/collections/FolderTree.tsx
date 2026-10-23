import { useMemo } from 'react'
import type { ReactNode } from 'react'
import { DndContext, type Announcements, type DragEndEvent } from '@dnd-kit/core'
import { SortableContext, useSortable, verticalListSortingStrategy } from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { ChevronDown, ChevronUp, TriangleAlert } from 'lucide-react'
import { Grip, useDragSensors } from '@/components/dnd'
import { IconButton } from '@/components/IconButton'
import { Icon } from '@/components/Icon'
import { shapeClass } from '@/features/preview/heroArt'
import { thumbnail } from '@/lib/tmdbImage'
import { reorder } from '@/lib/order'
import { pluralCount } from '@/lib/plural'
import { folderLabel, type FolderFormState } from './collectionForm'
import { FOLDERS, containerOf, reorderDropped, withinContainer, type ReorderableData, type SortableData } from './folderDnd'

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
  itemName: (id: string) => string,
): Announcements {
  function describe(id: string | number): string {
    const raw = String(id)
    const separator = raw.indexOf('::')
    if (separator === -1) return `folder “${folderName(raw)}”`
    return `catalog “${itemName(raw.slice(separator + 2))}”`
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
  itemName,
  onReorderFolders,
  children,
}: {
  folderKeys: string[]
  /** Looks up a folder's display name from its `key` — used only to word the
   *  drag-and-drop live-region announcements. */
  folderName: (folderKey: string) => string
  /** Looks up a folder catalog's display name from its catalog id, or one
   *  of its genres' from the ref `key` — same. */
  itemName: (id: string) => string
  onReorderFolders: (orderedKeys: string[]) => void
  children: ReactNode
}) {
  const sensors = useDragSensors()
  const announcements = useMemo(
    () => buildAnnouncements(folderName, itemName),
    [folderName, itemName],
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

    reorderDropped(active.data.current as ReorderableData, activeID, overID)
  }

  return (
    <DndContext
      sensors={sensors}
      collisionDetection={withinContainer}
      onDragEnd={handleDragEnd}
      accessibility={{ announcements }}
    >
      <SortableContext items={folderKeys} strategy={verticalListSortingStrategy}>
        {children}
      </SortableContext>
    </DndContext>
  )
}

/**
 * The folder list: one row per folder in its running order, its cover as a
 * small thumbnail (the Preview beside the editor draws the tiles). One is
 * always selected, and its folder (`detail`) opens directly under its row.
 */
export function FolderList({
  folders,
  selectedKey,
  errorKeys,
  detail,
  onSelect,
  onMove,
}: {
  folders: FolderFormState[]
  selectedKey: string | null
  /** Folders with a validation error, marked on their row so a problem in a
   *  folder that isn't open is still visible. */
  errorKeys: ReadonlySet<string>
  /** The selected folder's settings and catalogs. */
  detail: ReactNode
  onSelect: (key: string) => void
  onMove: (key: string, direction: -1 | 1) => void
}) {
  return (
    <ul className="fold-list">
      {folders.map((folder, index) => (
        <FolderItem
          key={folder.key}
          folder={folder}
          position={index}
          total={folders.length}
          selected={folder.key === selectedKey}
          hasError={errorKeys.has(folder.key)}
          detail={detail}
          onSelect={() => onSelect(folder.key)}
          onMove={(direction) => onMove(folder.key, direction)}
        />
      ))}
    </ul>
  )
}

function FolderItem({
  folder,
  position,
  total,
  selected,
  hasError,
  detail,
  onSelect,
  onMove,
}: {
  folder: FolderFormState
  position: number
  total: number
  selected: boolean
  hasError: boolean
  detail: ReactNode
  onSelect: () => void
  onMove: (direction: -1 | 1) => void
}) {
  const sortable = useSortable({
    id: folder.key,
    data: { container: FOLDERS } satisfies SortableData,
  })
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = sortable
  const name = folder.title.trim() || 'Untitled folder'
  const label = folderLabel(folder, position)

  return (
    <li
      ref={setNodeRef}
      style={{ transform: CSS.Transform.toString(transform), transition }}
      className={isDragging ? 'relative z-10 opacity-40' : undefined}
    >
      <div className={`fold-row ${selected ? 'is-selected' : ''}`}>
        <Grip label={`Reorder ${label}`} sortable={{ attributes, listeners }} />
        <span className="type-data text-dimmer text-meta w-[18px] shrink-0 text-right tabular-nums">{position + 1}</span>
        {/* drift-ok: the row's own button, holding its thumbnail, name and count */}
        <button
          type="button"
          onClick={onSelect}
          aria-pressed={selected}
          aria-label={`${name}, ${pluralCount(folder.refs.length, 'catalog')}${hasError ? ', needs fixing' : ''}`}
          className="fold-row-btn"
        >
          <FolderThumb folder={folder} />
          <span className="flex min-w-0 flex-col">
            <span className="fold-row-name text-body flex items-center gap-1 font-semibold [overflow-wrap:anywhere]">
              {hasError && <Icon icon={TriangleAlert} size={14} className="text-danger shrink-0" />}
              {name}
            </span>
            <span className="type-data text-dimmer text-meta">{pluralCount(folder.refs.length, 'catalog')}</span>
          </span>
        </button>
        {selected && <FolderMoveArrows label={label} position={position} total={total} onMove={onMove} />}
      </div>
      {selected && detail}
    </li>
  )
}

/** A folder's cover, small and at its tile shape, centred in the same slot
 *  whatever the shape: its image, else its emoji, else an empty box. */
function FolderThumb({ folder }: { folder: FolderFormState }) {
  const image = folder.coverImageURL.trim()
  return (
    <span aria-hidden="true" className="fold-thumb-slot">
      {/* drift-ok: the cover emoji is sized to its thumbnail */}
      <span className={`fold-thumb ${shapeClass(folder.tileShape)} text-[18px] leading-none`}>
        {image ? (
          <img {...thumbnail(image, '2.75rem')} alt="" loading="lazy" className="h-full w-full object-cover" />
        ) : (
          folder.coverEmoji
        )}
      </span>
    </span>
  )
}

/** ↑ and ↓ on the selected row: the same reorder as dragging it, for
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
    <span className="flex shrink-0 gap-0.5 pointer-coarse:gap-3">
      <IconButton
        variant="plain"
        size="sm"
        icon={ChevronUp}
        label={`Move ${label} earlier${position === 0 ? ', already first' : ''}`}
        disabled={position === 0}
        onClick={() => onMove(-1)}
      />
      <IconButton
        variant="plain"
        size="sm"
        icon={ChevronDown}
        label={`Move ${label} later${position === total - 1 ? ', already last' : ''}`}
        disabled={position === total - 1}
        onClick={() => onMove(1)}
      />
    </span>
  )
}
