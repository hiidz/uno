import { useEffect, useRef } from 'react'
import type { ImportResult } from '@/api'
import { Modal, ModalBody, ModalFooter, ModalHeader } from '@/components/Modal'
import type { GenreLookups } from '@/features/library/useLibrary'
import { andList } from '@/lib/list'
import { pluralCount } from '@/lib/plural'
import { ImportReview } from './ImportReview'
import { JsonField } from './JsonField'
import { importTally } from './reuse'
import type { ImportProblem } from './problem'
import { useImportFlow } from './useImportFlow'

/**
 * Imports a bundle as new, private catalogs and collections: paste its JSON,
 * press Import, and the dialog lists everything the bundle holds in the
 * field's place, with a choice on each row that matches a collection title or
 * a catalog's filters you already have. The next press imports what the
 * button names; Edit JSON brings the field back.
 *
 * Everything is imported unless the choices say otherwise.
 */
export function ImportDialog({
  open,
  profileIndex,
  genres,
  onClose,
  onImported,
}: {
  open: boolean
  profileIndex: number
  /** The genre names a catalog's filters line reads. */
  genres: GenreLookups
  onClose: () => void
  /** Called once the import is written and the library has refetched. */
  onImported: (result: ImportResult) => void
}) {
  return (
    <Modal open={open} onClose={onClose} labelledBy="import-title" width="760px">
      <ModalHeader>
        <h2 id="import-title" className="type-display m-0 text-[18px]">
          Import
        </h2>
      </ModalHeader>
      {open && (
        <ImportFlow profileIndex={profileIndex} genres={genres} onClose={onClose} onImported={onImported} />
      )}
    </Modal>
  )
}

/** One opening of the dialog, mounted only while it is open so every opening
 *  starts with an empty field. */
function ImportFlow({
  profileIndex,
  genres,
  onClose,
  onImported,
}: {
  profileIndex: number
  genres: GenreLookups
  onClose: () => void
  onImported: (result: ImportResult) => void
}) {
  const flow = useImportFlow(profileIndex, onImported)
  const busy = flow.checking || flow.importing
  const tally = flow.review && importTally(flow.review.check, flow.review.choices, flow.review.skip)

  return (
    <>
      <ModalBody>
        <div className="flex flex-col gap-4">
          {flow.review ? (
            <ImportReview
              review={flow.review}
              genres={genres}
              busy={busy}
              onEdit={flow.editAgain}
              onChoices={flow.setChoices}
              onSkip={flow.setSkip}
            />
          ) : (
            <>
              <p className="text-dim m-0 text-[13px] leading-relaxed">
                Paste the JSON of an Uno export. Everything in it is added to your library as new,
                private catalogs and collections.
              </p>
              <PasteBox
                text={flow.pasted}
                busy={busy}
                invalid={flow.inputError !== null}
                onEdit={flow.edit}
                onValidate={flow.validate}
              />
              <Outcome flow={flow} />
            </>
          )}
        </div>
        {flow.importError && <ErrorLine className="mt-4" problem={flow.importError} />}
      </ModalBody>
      <ModalFooter>
        <button type="button" onClick={onClose} className="btn-ghost">
          Cancel
        </button>
        <button
          type="button"
          onClick={flow.runImport}
          disabled={busy || !flow.hasText || isEmpty(tally)}
          className="btn-primary"
        >
          {importLabel(flow.checking, flow.importing, tally)}
        </button>
      </ModalFooter>
    </>
  )
}

type Tally = ReturnType<typeof importTally>

function isEmpty(tally: Tally | null): boolean {
  return tally !== null && tally.catalogs + tally.collections === 0
}

/** "Import" until the check has listed the bundle; from then on, what the
 *  next press adds with the choices as they stand. */
function importLabel(checking: boolean, importing: boolean, tally: Tally | null) {
  if (checking) return 'Checking…'
  if (importing) return 'Importing…'
  return tallyLabel(tally)
}

function tallyLabel(tally: Tally | null): string {
  if (!tally) return 'Import'
  const { catalogs, collections } = tally
  const parts = [
    catalogs > 0 && pluralCount(catalogs, 'catalog'),
    collections > 0 && pluralCount(collections, 'collection'),
  ].filter((part) => part !== false)
  if (parts.length === 0) return 'Nothing to import'
  return `Import ${andList(parts)}`
}

type Flow = ReturnType<typeof useImportFlow>

/** The line under the input: the bundle's own refusal, else the server's;
 *  else, after Validate, that it parsed. */
function Outcome({ flow }: { flow: Flow }) {
  const problem = flow.inputError ?? flow.checkError
  if (problem) return <ErrorLine problem={problem} />
  if (!flow.valid) return null
  return <p className="text-dim type-data m-0 text-[12.5px]">Valid JSON.</p>
}

/** An error the dialog shows after a press: what happened, with the technical
 *  text dimmer beneath. It takes focus when it appears, which scrolls it into
 *  view and has it read out. */
function ErrorLine({ className = '', problem }: { className?: string; problem: ImportProblem }) {
  const ref = useRef<HTMLDivElement>(null)
  useEffect(() => ref.current?.focus(), [])
  return (
    <div
      ref={ref}
      role="alert"
      tabIndex={-1}
      className={`callout-danger flex flex-col gap-1 break-words outline-none ${className}`}
    >
      <p className="m-0">{problem.headline}</p>
      {problem.detail && <p className="type-data text-dimmer m-0 text-[12.5px]">{problem.detail}</p>}
    </div>
  )
}

function PasteBox({
  text,
  busy,
  invalid,
  onEdit,
  onValidate,
}: {
  text: string
  busy: boolean
  invalid: boolean
  onEdit: (text: string) => void
  onValidate: () => void
}) {
  return (
    <div className="flex flex-col items-start gap-3">
      <JsonField
        value={text}
        readOnly={busy}
        invalid={invalid}
        ariaLabel="Import JSON"
        placeholder={'{ "format": …'}
        onChange={onEdit}
      />
      <button
        type="button"
        onClick={onValidate}
        disabled={busy || text.trim() === ''}
        className="btn-secondary"
      >
        Validate JSON
      </button>
    </div>
  )
}
