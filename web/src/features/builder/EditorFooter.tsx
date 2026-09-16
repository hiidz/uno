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
}) {
  return (
    <>
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

/**
 * A server 400 on save, under the form.
 *
 * Both builders mirror every server rule, so this should be unreachable — and
 * a 400 that arrives anyway is plain text with no field to hang it on, which
 * is why it renders as a banner rather than against an input.
 */
export function SaveError({ noun, message }: { noun: string; message: string | null }) {
  if (!message) return null
  return (
    <p className="type-data text-danger border-danger mt-6 border-l-2 pl-3 text-[11px] leading-[1.45]">
      Couldn't save this {noun}: {message}
    </p>
  )
}
