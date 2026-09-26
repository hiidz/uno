/**
 * Reordering by id, for every list the app lets you drag or step through —
 * the Home pane's bands, a collection's folders, a folder's catalogs. Order
 * is the value in all of them: array position becomes `sort_order` on the
 * server.
 */

/** Moves the item at `from` to `to`'s position. Returns `ids` unchanged if
 *  either isn't found. */
export function reorder(ids: string[], from: string, to: string): string[] {
  const fromIndex = ids.indexOf(from)
  const toIndex = ids.indexOf(to)
  if (fromIndex === -1 || toIndex === -1) return ids
  const next = [...ids]
  next.splice(toIndex, 0, ...next.splice(fromIndex, 1))
  return next
}

/** Moves the item at `id` one step in `direction`. A no-op at either edge,
 *  returning `ids` itself — callers grey the button instead of relying on this
 *  to clamp silently. */
export function moveByOne(ids: string[], id: string, direction: -1 | 1): string[] {
  const index = ids.indexOf(id)
  const target = index + direction
  if (index === -1 || target < 0 || target >= ids.length) return ids
  const next = [...ids]
  ;[next[index], next[target]] = [next[target], next[index]]
  return next
}

/** `items` in the order of `orderedKeys`. An item whose key isn't listed keeps
 *  its relative place after the listed ones rather than being dropped, and a
 *  listed key with no item is skipped. */
export function orderByKeys<T>(items: T[], orderedKeys: string[], keyOf: (item: T) => string): T[] {
  const byKey = new Map(items.map((item) => [keyOf(item), item]))
  const ordered: T[] = []
  for (const key of orderedKeys) {
    const item = byKey.get(key)
    if (item === undefined) continue
    ordered.push(item)
    byKey.delete(key)
  }
  return [...ordered, ...byKey.values()]
}
