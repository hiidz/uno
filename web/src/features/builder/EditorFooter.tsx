import type { ReactNode } from 'react'

/**
 * The band an editor ends on: what is stopping the save, the way out, and the
 * save itself — DESIGN.md's save bar, shared by both builders.
 *
 * Both builders end the same way, so what varies is the noun, the `status`
 * line, and the labels; `cancelLabel` falls back to "Cancel". The Save button
 * takes the editor's region colour from the `tone-*` class `EditorShell` sets
 * around it.
 */
export function EditorFooter({
  noun,
  saving,
  errorCount,
  onCancel,
  onSubmit,
  status,
  saveLabel,
  cancelLabel,
  saveError,
}: {
  /** What is being saved, named in a failed save's message. */
  noun: string
  saving: boolean
  errorCount: number
  onCancel: () => void
  onSubmit: () => void
  /** DESIGN.md's status text: "No changes yet" / "Unsaved changes" / "N
   *  things need fixing: …". */
  status: ReactNode
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
      {status}
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
