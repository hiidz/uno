import type { Collection } from '@/api'
import { pluralCount } from '@/lib/plural'

/**
 * "N folders" or "N folders · Title, Title, …" — the summary the Library
 * rail and the Home pane both show for a collection row.
 *
 * `undefined` is a selected collection whose owner has since deleted it: the
 * selection response still names the id, but nothing describes it any more.
 * A row from the library itself is never `undefined` — it's a `Collection`
 * by definition — so callers with one in hand just pass it through.
 */
export function describeCollection(collection: Collection | undefined): string {
  if (!collection) return 'no longer available'
  const folders = collection.folders ?? []
  const count = pluralCount(folders.length, 'folder')
  if (folders.length === 0) return count
  return `${count} · ${folders.map((f) => f.title).join(', ')}`
}
