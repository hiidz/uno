import { closestCenter, type CollisionDetection } from '@dnd-kit/core'

/**
 * How the collection editor's one `DndContext` tells its lists apart. Folder
 * tiles reorder among themselves, and each folder's catalog refs among
 * themselves. Nesting a second `DndContext` inside the first does not isolate
 * them — the outer context still sees the inner drags — so every sortable
 * declares which list it belongs to via dnd-kit's `data`, and
 * `FolderTreeDnd`'s `onDragEnd` reorders only when the dragged item and the one
 * it was dropped on agree. A ref dragged out of its folder is a no-op rather
 * than a mis-drop.
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

/** A ref's drag id, from its form `key` rather than its catalog id: one
 *  catalog can be two refs in a folder, under two genres. Prefixed with the
 *  folder key so `onDragEnd` can recover which list it belongs to. */
export function refDragID(folderKey: string, refKey: string): string {
  return `${folderKey}::${refKey}`
}
