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
  return describeFolders((collection.folders ?? []).map((f) => f.title))
}

/** "N folders" or "N folders · Title, Title, …" for folders titled `titles`, in order. */
export function describeFolders(titles: string[] | null): string {
  const names = titles ?? []
  const count = pluralCount(names.length, 'folder')
  return names.length === 0 ? count : `${count} · ${names.join(', ')}`
}
