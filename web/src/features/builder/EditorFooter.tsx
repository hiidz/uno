import { plural } from '@/lib/plural'

/**
 * The band an editor ends on: what is stopping the save, the way out, and the
 * save itself.
 *
 * Both builders end the same way, so what varies is the noun — the button
 * commits to "Create catalog" or "Create collection", never a bare "Create".
 */
export function EditorFooter({
  mode,
  noun,
  saving,
  showErrors,
  errorCount,
  onCancel,
  onSubmit,
}: {
  mode: 'edit' | 'duplicate'
  /** What is being saved, for the create button's label. */
  noun: string
  saving: boolean
  /** Errors are on show only from the first submit, so the note arrives with
   *  the field highlighting it is explaining rather than ahead of it. */
  showErrors: boolean
  errorCount: number
  onCancel: () => void
  onSubmit: () => void
}) {
  return (
    <>
      {showErrors && errorCount > 0 && (
        <span className="type-data text-danger mr-auto text-[10.5px]">
          Fix the highlighted {plural(errorCount, 'field')}.
        </span>
      )}
      <button type="button" onClick={onCancel} className="btn-ghost">
        Cancel
      </button>
      <button type="button" onClick={onSubmit} disabled={saving} className="btn-primary">
        {saving ? 'Saving…' : mode === 'edit' ? 'Save changes' : `Create ${noun}`}
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
