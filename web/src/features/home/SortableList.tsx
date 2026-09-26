import type { ReactNode } from 'react'
import { DndContext, closestCenter, type DragEndEvent } from '@dnd-kit/core'
import { SortableContext, verticalListSortingStrategy } from '@dnd-kit/sortable'
import { useDragSensors } from '@/components/dnd'
import { reorder } from '@/lib/order'

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
