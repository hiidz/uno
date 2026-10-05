import { TriangleAlert } from 'lucide-react'
import { Icon } from '@/components/Icon'
import { SignStepButton, type SignStep } from '@/components/PaneSign'
import { pluralCount } from '@/lib/plural'

interface EditorFooterProps {
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
  /** The subject's next step with Community — Publish…, Unpublish… — between
   *  the status line and Close. Absent where there is none to take. */
  step?: SignStep
  /** DESIGN.md's Save bar reads "Save", and "Done" in a collection's nested
   *  catalog editor, whose save only stages the edit. */
  saveLabel: string
  /**
   * Plain-text body of a server rejection of the last save. Both builders
   * mirror every server rule, so this should be unreachable, and a 400 that
   * arrives anyway has no field to hang it on. It sits here, on its own line
   * above the buttons, because this band is the one part of the editor that
   * stays on screen: under the form it scrolled out of sight, and a save that
   * failed looked like a press that did nothing.
   */
  saveError?: string | null
}

/**
 * The band an editor ends on: what is stopping the save, the way out, and the
 * save itself — DESIGN.md's save bar, shared by both builders.
 *
 * Both builders end the same way, so what varies is the noun, what the status
 * line names, and Save's label. The quiet button reads Close in every editor
 * and leaves through `onCancel`, the editor's own `onRequestClose`, so unsaved
 * edits get the same "Discard unsaved changes?" every other exit asks. The
 * Save button takes the editor's region colour from the `tone-*` class
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
  step,
  saveLabel,
  saveError,
}: EditorFooterProps) {
  return (
    <>
      <SaveError noun={noun} error={saveError} />
      <SaveStatus errorCount={errorCount} errorLabels={errorLabels} dirty={dirty} notes={notes} />
      <SignStepButton step={step} />
      <button type="button" onClick={onCancel} className="btn-ghost">
        Close
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

/** A save the server rejected, on its own line above the buttons. */
function SaveError({ noun, error }: { noun: string; error: string | null | undefined }) {
  if (!error) return null
  return (
    <p role="alert" className="callout-danger type-data basis-full">
      {`Couldn't save this ${noun}: ${error}`}
    </p>
  )
}

interface SaveStatusProps {
  errorCount: number
  errorLabels: string[]
  dirty: boolean
  notes: string[]
}

/** DESIGN.md's status text: "N things need fixing: …", else "Unsaved
 *  changes" or "No changes yet", with the notes after it. */
function SaveStatus({ errorCount, errorLabels, dirty, notes }: SaveStatusProps) {
  if (errorCount > 0) return <ErrorStatus count={errorCount} labels={errorLabels} />
  return (
    <span className={dirty ? 'ed-status' : 'ed-status is-muted'}>
      {dirty ? 'Unsaved changes' : 'No changes yet'}
      {notes.map((note) => (
        <span key={note} className="text-dim">
          {' '}
          · {note}
        </span>
      ))}
    </span>
  )
}

function ErrorStatus({ count, labels }: { count: number; labels: string[] }) {
  return (
    <span className="ed-status is-error">
      <Icon icon={TriangleAlert} size={16} />
      {pluralCount(count, 'thing')} {count === 1 ? 'needs' : 'need'} fixing
      {labels.length > 0 && `: ${labels.join(', ')}`}
    </span>
  )
}
