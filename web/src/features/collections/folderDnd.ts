import { closestCenter, type CollisionDetection } from '@dnd-kit/core'
import { reorder } from '@/lib/order'

/**
 * How the collection editor's one `DndContext` tells its lists apart. Folder
 * tiles reorder among themselves, each folder's catalogs among themselves,
 * and each catalog's genres among themselves. Nesting a second `DndContext`
 * inside the first does not isolate them — the outer context still sees the
 * inner drags — so every sortable
 * declares which list it belongs to via dnd-kit's `data`, and
 * `FolderTreeDnd`'s `onDragEnd` reorders only when the dragged item and the one
 * it was dropped on agree. A ref dragged out of its folder is a no-op rather
 * than a mis-drop. A folder's catalogs and a catalog's genres carry their own
 * `reorder` in that data (`ReorderableData`), so `onDragEnd` hands the new
 * order straight back to the list it came from.
 */

export const FOLDERS = '__folders__'

export interface SortableData {
  container: string
}

export function containerOf(node: { data: { current?: unknown } } | null | undefined): string | null {
  const data = node?.data.current as SortableData | undefined
  return data?.container ?? null
}

/**
 * Rank only the droppables in the dragged item's own list.
 *
 * Load-bearing. Folders and every ref row share one `DndContext`, and plain
 * `closestCenter` ranks all of them together — so a folder tile dragged down
 * toward the open folder's catalogs finds a *ref row* to be the nearest center.
 * `onDragEnd` then sees two different containers, refuses to guess, and the
 * folder springs back. Narrowing the candidate set here leaves
 * that container check as a backstop rather than the mechanism.
 */
export const withinContainer: CollisionDetection = (args) => {
  const container = containerOf(args.active)
  return closestCenter({
    ...args,
    droppableContainers: args.droppableContainers.filter(
      (candidate) => containerOf(candidate) === container,
    ),
  })
}

/** An item's drag id: its `id` in list `container`, prefixed with the
 *  container so ids stay unique across the one `DndContext` and `onDragEnd`
 *  can strip the prefix again. A folder's catalogs sort by catalog id (each
 *  catalog is one line), and one catalog's genres by ref `key`, because one
 *  catalog can be several refs in a folder. */
export function dragID(container: string, id: string): string {
  return `${container}::${id}`
}

/** The list one catalog's genres sort in: its own, inside the folder's. */
export function genreContainer(folderKey: string, catalogID: string): string {
  return `${folderKey}/${catalogID}`
}

/** What a sortable in a folder's lists carries besides its container: the
 *  list's ids in order, and where the reordered ids go. */
export interface ReorderableData extends SortableData {
  ids: string[]
  reorder(orderedIDs: string[]): void
}

/** A drop of `activeID` over `overID` in `data`'s list, handed to its
 *  `reorder`. */
export function reorderDropped(data: ReorderableData, activeID: string, overID: string): void {
  const prefix = data.container.length + 2
  const ordered = reorder(
    data.ids.map((id) => dragID(data.container, id)),
    activeID,
    overID,
  )
  data.reorder(ordered.map((id) => id.slice(prefix)))
}
