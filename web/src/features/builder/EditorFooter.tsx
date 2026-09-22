import type { ReactNode } from 'react'
import { plural } from '@/lib/plural'

/**
 * The band an editor ends on: what is stopping the save, the way out, and the
 * save itself — DESIGN.md's save bar, shared by both builders.
 *
 * Both builders end the same way, so what varies is the noun and (for the
 * catalog editor's own credit-row status line) the `status` slot. `status`
 * and `saveLabel` are additive and optional: omitted, this renders exactly as
 * it did before either builder was restyled to DESIGN.md — the collection
 * editor keeps that default until its own phase.
 */
export function EditorFooter({
  mode,
  noun,
  saving,
  showErrors,
  errorCount,
  onCancel,
  onSubmit,
  status,
  saveLabel,
  cancelLabel,
  saveError,
}: {
  mode: 'edit' | 'duplicate'
  /** What is being saved, for the create button's default label. */
  noun: string
  saving: boolean
  /** Errors are on show only from the first submit, so the note arrives with
   *  the field highlighting it is explaining rather than ahead of it. */
  showErrors: boolean
  errorCount: number
  onCancel: () => void
  onSubmit: () => void
  /** DESIGN.md's status text ("No changes yet" / "Unsaved changes" / "N
   *  things need fixing: …"), replacing the plain "Fix the highlighted N
   *  fields." line below when given. */
  status?: ReactNode
  /** Overrides the primary button's label outright — DESIGN.md's Save bar
   *  always reads "Save" (or "Save collection"), never "Create catalog". */
  saveLabel?: string
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
        <p
          role="alert"
          className="type-data text-danger border-danger m-0 basis-full border-l-2 pl-3 text-[11px] leading-[1.45]"
        >
          Couldn't save this {noun}: {saveError}
        </p>
      )}
      {status ??
        (showErrors && errorCount > 0 && (
          <span className="type-data text-danger mr-auto text-[10.5px]">
            Fix the highlighted {plural(errorCount, 'field')}.
          </span>
        ))}
      <button type="button" onClick={onCancel} className="btn-quiet">
        {cancelLabel ?? 'Cancel'}
      </button>
      <button
        type="button"
        onClick={onSubmit}
        disabled={saving}
        aria-disabled={!saving && errorCount > 0}
        className="btn-primary"
      >
        {saving ? 'Saving…' : (saveLabel ?? (mode === 'edit' ? 'Save changes' : `Create ${noun}`))}
      </button>
    </>
  )
}
