import { TriangleAlert } from 'lucide-react'
import { Icon } from '@/components/Icon'
import { pluralCount } from '@/lib/plural'

/**
 * The band an editor ends on: what is stopping the save, the way out, and the
 * save itself — DESIGN.md's save bar, shared by both builders.
 *
 * Both builders end the same way, so what varies is the noun, what the status
 * line names, and the labels; `cancelLabel` falls back to "Cancel". The Save
 * button takes the editor's region colour from the `tone-*` class
 * `EditorShell` sets around it.
 */
export function EditorFooter({
  noun,
  saving,
  errorCount,
  errorLabels,
  dirty,
  notes = [],
  onCancel,
  onSubmit,
  saveLabel,
  cancelLabel,
  saveError,
}: {
  /** What is being saved, named in a failed save's message. */
  noun: string
  saving: boolean
  errorCount: number
  /** Where the errors are, named in the status line — empty until errors are
   *  on show. */
  errorLabels: string[]
  dirty: boolean
  /** What else the next save does, as dim clauses after the status — "2
   *  folders will be deleted". */
  notes?: string[]
  onCancel: () => void
  onSubmit: () => void
  /** DESIGN.md's Save bar reads "Save" (or "Save collection"). */
  saveLabel: string
  /** Overrides the quiet button's label — the collection editor's own save bar
   *  reads "Discard changes" (DESIGN.md's Collection editor spec), not
   *  "Cancel". Both still route through `onCancel`, i.e. this editor's own
   *  `onRequestClose` and the shared `EditorGuard`'s leave-with-unsaved-changes
   *  prompt behind it — a second, editor-specific confirm isn't added here. */
  cancelLabel?: string
  /**
   * Plain-text body of a server rejection of the last save. Both builders
   * mirror every server rule, so this should be unreachable, and a 400 that
   * arrives anyway has no field to hang it on. It sits here, on its own line
   * above the buttons, because this band is the one part of the editor that
   * stays on screen: under the form it scrolled out of sight, and a save that
   * failed looked like a press that did nothing.
   */
  saveError?: string | null
}) {
  return (
    <>
      {saveError && (
        <p role="alert" className="callout-danger type-data basis-full">
          Couldn't save this {noun}: {saveError}
        </p>
      )}
      {/* DESIGN.md's status text: "N things need fixing: …", else "Unsaved
          changes" or "No changes yet". */}
      {errorCount > 0 ? (
        <span className="ed-status is-error">
          <Icon icon={TriangleAlert} size={16} />
          {pluralCount(errorCount, 'thing')} {errorCount === 1 ? 'needs' : 'need'} fixing
          {errorLabels.length > 0 && `: ${errorLabels.join(', ')}`}
        </span>
      ) : (
        <span className={dirty ? 'ed-status' : 'ed-status is-muted'}>
          {dirty ? 'Unsaved changes' : 'No changes yet'}
          {notes.map((note) => (
            <span key={note} className="text-dim">
              {' '}
              · {note}
            </span>
          ))}
        </span>
      )}
      <button type="button" onClick={onCancel} className="btn-ghost">
        {cancelLabel ?? 'Cancel'}
      </button>
      <button
        type="button"
        onClick={onSubmit}
        disabled={saving}
        aria-disabled={!saving && errorCount > 0}
        className="btn-primary"
      >
        {saving ? 'Saving…' : saveLabel}
      </button>
    </>
  )
}
