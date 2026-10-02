import type { EditorTarget } from './target'

/**
 * What selecting a library row does to the pane.
 *
 * - `open`: another row — its editor takes the pane, through the discard guard.
 * - `close`: the row already open — the same guarded close as ×. Below `lg`
 *   the open row sits under the editor layer, so this only happens from `lg`.
 *
 * Selecting the open row never re-seeds the form from its saved state.
 */
export type SelectionAction = 'open' | 'close'

export function selectionAction(open: EditorTarget | null, next: EditorTarget): SelectionAction {
  return isOpen(open, next) ? 'close' : 'open'
}

function isOpen(open: EditorTarget | null, next: EditorTarget): boolean {
  return open?.kind === next.kind && open.id === next.id
}

/** Below `lg` the Home pane stays mounted under an open editor's layer; from
 *  `lg` an editor takes its place. */
export function homeMounted(target: EditorTarget | null, stacked: boolean): boolean {
  return target === null || stacked
}
