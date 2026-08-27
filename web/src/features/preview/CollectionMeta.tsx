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
 */
export function CollectionMeta({ collection }: { collection: PreviewCollection }) {
  if (collection.missing) return null

  const notes: string[] = []

  if (collection.folders.length > 0) {
    if (collection.viewModeAssumed) {
      // Covers both `FOLLOW_LAYOUT` and an empty/unrecognised `view_mode`:
      // either way the collection names no layout Uno can honour, so rows is a
      // guess and the note says so without claiming which case it hit.
      notes.push("folders open as rows — the app's own layout setting decides, and Uno can't read it")
    } else if (collection.viewMode === 'TABBED_GRID') {
      notes.push(
        collection.showAllTab
          ? 'folders open as a tabbed grid, with an All tab'
          : 'folders open as a tabbed grid',
      )
    } else {
      notes.push('folders open as rows')
    }
  }

  if (collection.hasBackdrop) notes.push('has a backdrop image (not loaded here)')

  if (notes.length === 0) return null
  return <p className="type-data text-dimmer m-0 text-[10px]">{notes.join(' · ')}</p>
}
