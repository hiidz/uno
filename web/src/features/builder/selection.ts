import type { EditorTarget } from './target'

/**
 * What selecting a library row does to the pane.
 *
 * - `open`: another row — its editor takes the pane, through the discard guard.
 * - `close`: the row already open, from `lg` — the same guarded close as ×.
 * - `scroll`: the row already open, stacked below `lg` — tapping it is how you
 *   ask to be taken back to its editor, so it only scrolls.
 *
 * Selecting the open row never re-seeds the form from its saved state.
 */
export type SelectionAction = 'open' | 'close' | 'scroll'

export function selectionAction(open: EditorTarget | null, next: EditorTarget, stacked: boolean): SelectionAction {
  if (!isOpen(open, next)) return 'open'
  return stacked ? 'scroll' : 'close'
}

function isOpen(open: EditorTarget | null, next: EditorTarget): boolean {
  return open?.kind === next.kind && open.id === next.id
}
