import type { PreviewCollection } from './model'

/**
 * The collection-level settings that change the layout but can't be drawn on a
 * collection's row, stated as text.
 *
 * `view_mode` and `show_all_tab` both describe a *folder page*, so they are
 * named here and demonstrated one level down — open a folder tile to see them.
 *
 * Shared so the Home pane and the collection builder describe the same settings
 * in the same words: two different sentences for one `view_mode` would read as
 * two different behaviours.
 *
 * **It also carries what a folder tile can't say for itself.** A tile is 92px
 * of emoji or cover art; facts about it — that some of its catalogs no longer
 * resolve, that its shape was assumed rather than set — sit in the tile's
 * `title`, which no touch screen ever opens. Summed across the row here, they
 * are on the page for everyone.
 */
export function CollectionMeta({ collection }: { collection: PreviewCollection }) {
  if (collection.missing) return null

  const notes: string[] = []

  // `FOLLOW_LAYOUT` names no layout Uno can honour, so the preview draws the
  // app's own default — tabs, All first — and the note says that's a stand-in.
  if (collection.folders.length > 0 && collection.viewModeAssumed) notes.push('previewed as tabs')

  // Summed over the row rather than stated per tile: the tiles sit side by side
  // and a caption under each one saying "2 unavailable" would be the same
  // sentence three times.
  const unresolved = collection.folders.reduce((total, folder) => total + folder.unresolved, 0)
  if (unresolved > 0) {
    notes.push(`${unresolved} ${unresolved === 1 ? 'catalog is' : 'catalogs are'} unavailable`)
  }

  if (collection.folders.some((folder) => folder.tileShapeAssumed)) {
    notes.push('no tile shape set — shown as posters')
  }

  if (notes.length === 0) return null
  return <p className="type-data text-dim m-0 text-[12px] leading-[17px]">{notes.join(' · ')}</p>
}
