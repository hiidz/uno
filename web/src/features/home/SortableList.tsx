import type { ReactNode } from 'react'
import { DndContext, closestCenter, type Announcements, type DragEndEvent, type UniqueIdentifier } from '@dnd-kit/core'
import { SortableContext, verticalListSortingStrategy } from '@dnd-kit/sortable'
import { useDragSensors } from '@/components/dnd'
import { ordinal } from '@/lib/ordinal'
import { reorder } from '@/lib/order'

/**
 * Vertical drag-to-reorder for the Home pane's two lists. Order *is* the value
 * — there is no sort field in the payload, array position becomes
 * `sort_order` at write time — so this is the pane's central interaction rather
 * than a convenience. The sensors are `useDragSensors` — see there for why
 * each of the three is needed.
 *
 * A drag's live region names the row and the place it would take, in the
 * numbering the rows show, instead of dnd-kit's raw ids.
 */
export function SortableList({
  ids,
  nameOf,
  positionOf,
  onReorder,
  children,
}: {
  ids: string[]
  nameOf: (id: string) => string
  /** The row's number on the home screen, counted across both lists. */
  positionOf: (id: string) => number
  onReorder: (orderedIds: string[]) => void
  children: ReactNode
}) {
  const sensors = useDragSensors()
  const place = (id: UniqueIdentifier) => `${ordinal(positionOf(String(id)))} on your home screen`
  const name = (id: UniqueIdentifier) => `“${nameOf(String(id))}”`
  const announcements: Announcements = {
    onDragStart: ({ active }) => `Picked up ${name(active.id)}, ${place(active.id)}.`,
    onDragOver: ({ active, over }) =>
      over ? `${name(active.id)} would be ${place(over.id)}.` : `${name(active.id)} is outside the list.`,
    onDragEnd: ({ active, over }) =>
      over ? `${name(active.id)} dropped, now ${place(over.id)}.` : `${name(active.id)} dropped; it stays ${place(active.id)}.`,
    onDragCancel: ({ active }) => `Moving ${name(active.id)} cancelled; it stays ${place(active.id)}.`,
  }

  function handleDragEnd(event: DragEndEvent) {
    const { active, over } = event
    if (!over || active.id === over.id) return
    onReorder(reorder(ids, String(active.id), String(over.id)))
  }

  return (
    <DndContext
      sensors={sensors}
      collisionDetection={closestCenter}
      onDragEnd={handleDragEnd}
      accessibility={{ announcements }}
    >
      <SortableContext items={ids} strategy={verticalListSortingStrategy}>
        {children}
      </SortableContext>
    </DndContext>
  )
}
