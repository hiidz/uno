import type { ReactNode } from 'react'
import { DndContext, closestCenter, type DragEndEvent } from '@dnd-kit/core'
import { SortableContext, useSortable, verticalListSortingStrategy } from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { Grip, reorder, useDragSensors } from '@/components/dnd'

/**
 * Vertical drag-to-reorder for the Home pane's two lists. Order *is* the value
 * — there is no sort field in the payload, array position becomes
 * `sort_order` at write time — so this is the pane's central interaction rather
 * than a convenience. The sensors are `useDragSensors` — see there for why
 * each of the three is needed.
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
  const sensors = useDragSensors()

  function handleDragEnd(event: DragEndEvent) {
    const { active, over } = event
    if (!over || active.id === over.id) return
    onReorder(reorder(ids, String(active.id), String(over.id)))
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
      <Grip label={label} sortable={{ attributes, listeners }} />
      {children}
    </div>
  )
}
