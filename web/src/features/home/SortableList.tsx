import type { ReactNode } from 'react'
import {
  DndContext,
  KeyboardSensor,
  PointerSensor,
  closestCenter,
  useSensor,
  useSensors,
  type DragEndEvent,
} from '@dnd-kit/core'
import {
  SortableContext,
  sortableKeyboardCoordinates,
  useSortable,
  verticalListSortingStrategy,
} from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'

/**
 * Vertical drag-to-reorder for the Home pane's two lists. Order *is* the value
 * — there is no sort field in the payload, array position becomes
 * `sort_order` at write time — so this is the pane's central interaction rather
 * than a convenience.
 *
 * Two sensors:
 *  - Pointer, with a small activation distance so a click on a button inside a
 *    row still registers as a click rather than starting a drag.
 *  - Keyboard, the only way this list is operable for anyone who can't drag.
 */
export function SortableList({
  ids,
  onReorder,
  children,
}: {
  ids: string[]
  onReorder: (orderedIds: string[]) => void
  children: ReactNode
}) {
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 4 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  )

  function handleDragEnd(event: DragEndEvent) {
    const { active, over } = event
    if (!over || active.id === over.id) return
    const from = ids.indexOf(String(active.id))
    const to = ids.indexOf(String(over.id))
    if (from === -1 || to === -1) return
    const next = [...ids]
    next.splice(to, 0, ...next.splice(from, 1))
    onReorder(next)
  }

  return (
    <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={handleDragEnd}>
      <SortableContext items={ids} strategy={verticalListSortingStrategy}>
        {children}
      </SortableContext>
    </DndContext>
  )
}

/**
 * Renders one draggable row. The drag listeners go on the grip alone, not the
 * whole row, so the buttons a row carries stay clickable.
 */
export function SortableRow({
  id,
  label,
  children,
}: {
  id: string
  /** Announced to screen readers on the grip, e.g. "Reorder Trending Sci-Fi". */
  label: string
  children: ReactNode
}) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({
    id,
  })

  return (
    <div
      ref={setNodeRef}
      style={{ transform: CSS.Transform.toString(transform), transition }}
      className={`border-line bg-ground hover:bg-raised grid grid-cols-[16px_3px_minmax(0,1fr)_auto_24px] items-center gap-x-3 border-b py-3 transition-colors ${
        isDragging ? 'relative z-10 opacity-40' : ''
      }`}
    >
      <button
        type="button"
        aria-label={label}
        className="text-dimmer hover:text-dim cursor-grab touch-none text-center leading-none transition-colors active:cursor-grabbing"
        {...attributes}
        {...listeners}
      >
        ⠿
      </button>
      {children}
    </div>
  )
}
