import type { ImportMatch, ImportResult } from '@/api'
import { Segmented, Select } from '@/components/fields'
import { Modal, ModalBody, ModalFooter, ModalHeader } from '@/components/Modal'
import { typeLabel } from '@/features/library/recipe'
import { pluralCount } from '@/lib/plural'
import { JsonField } from './JsonField'
import { choicesForAll, type ReuseChoice, type ReuseChoices } from './reuse'
import { PASTED_JSON } from './text'
import { useImportFlow, type Review } from './useImportFlow'

/**
 * Imports a bundle as new, private catalogs and collections: paste its JSON,
 * press Import. Import stops to ask only when the bundle holds
 * catalogs with the same recipe as ones you already have, and asks under the
 * field, which stays editable.
 *
 * Every catalog is imported as a copy unless the choices say otherwise.
 * Importing the same bundle twice gives two sets.
 */
export function ImportDialog({
  open,
  profileIndex,
  onClose,
  onImported,
}: {
  open: boolean
  profileIndex: number
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
      {open && <ImportFlow profileIndex={profileIndex} onClose={onClose} onImported={onImported} />}
    </Modal>
  )
}

/** One opening of the dialog, mounted only while it is open so every opening
 *  starts with an empty field. */
function ImportFlow({
  profileIndex,
  onClose,
  onImported,
}: {
  profileIndex: number
  onClose: () => void
  onImported: (result: ImportResult) => void
}) {
  const flow = useImportFlow(profileIndex, onImported)
  const busy = flow.checking || flow.importing

  return (
    <>
      <ModalBody>
        <div className="flex flex-col gap-4">
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
          {flow.review && <ReviewStep review={flow.review} onChoices={flow.setChoices} />}
        </div>
        {flow.importError && (
          <p role="alert" className="callout-danger type-data mt-4">
            Couldn't import: {flow.importError}
          </p>
        )}
      </ModalBody>
      <ModalFooter>
        <button type="button" onClick={onClose} className="btn-ghost">
          Cancel
        </button>
        <button
          type="button"
          onClick={flow.runImport}
          disabled={busy || !flow.hasText}
          className="btn-primary"
        >
          {importLabel(flow.checking, flow.importing)}
        </button>
      </ModalFooter>
    </>
  )
}

function importLabel(checking: boolean, importing: boolean) {
  if (checking) return 'Checking…'
  return importing ? 'Importing…' : 'Import'
}

type Flow = ReturnType<typeof useImportFlow>

/** The line under the input: the bundle's own refusal, else the server's;
 *  else, after Validate, that it parsed. */
function outcomeMessage(inputError: string | null, checkError: string | null): string | null {
  if (inputError) return inputError
  return checkError && `This JSON can't be imported: ${checkError}`
}

function Outcome({ flow }: { flow: Flow }) {
  const error = outcomeMessage(flow.inputError, flow.checkError)
  if (error) {
    return (
      <p role="alert" className="callout-danger type-data break-words">
        {error}
      </p>
    )
  }
  if (!flow.valid) return null
  return <p className="text-dim type-data m-0 text-[12.5px]">Valid JSON.</p>
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
        ariaLabel="Export JSON"
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

/** What the check found: the bundle's counts, then one row per catalog that
 *  matches one already in the library. */
function ReviewStep({
  review,
  onChoices,
}: {
  review: Review
  onChoices: (choices: ReuseChoices) => void
}) {
  const { check, choices } = review
  const matches = check.matches

  return (
    <div className="flex flex-col gap-4">
      <p className="text-dim m-0 text-[13px] leading-relaxed break-words">
        <strong className="text-ink">{PASTED_JSON}</strong> holds {pluralCount(check.catalogs, 'catalog')},{' '}
        {pluralCount(check.collections, 'collection')} and {pluralCount(check.folders, 'folder')}.
      </p>

      <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
        <p className="text-dim m-0 flex-1 basis-[240px] text-[13px] leading-relaxed">
          {matches.length === 1
            ? 'One of these has the same recipe as a catalog you already have.'
            : `${matches.length} of these have the same recipe as catalogs you already have.`}{' '}
          Choose what Import does with each.
        </p>
        <div className="flex shrink-0">
          <button
            type="button"
            className="btn-ghost"
            onClick={() => onChoices(choicesForAll(matches, false))}
          >
            Copy all
          </button>
          <button
            type="button"
            className="btn-ghost"
            onClick={() => onChoices(choicesForAll(matches, true))}
          >
            Use existing for all
          </button>
        </div>
      </div>
      <ul className="m-0 flex list-none flex-col p-0">
        {matches.map((match) => (
          <MatchRow
            key={match.key}
            match={match}
            choice={choices[match.key]}
            onChange={(choice) => onChoices({ ...choices, [match.key]: choice })}
          />
        ))}
      </ul>
    </div>
  )
}

/**
 * One matched catalog. The wording follows where the catalog sits in the
 * bundle: a top-level catalog reused is simply not imported, while one of a
 * collection's own reused makes that collection reference your library
 * catalog in its place.
 */
function MatchRow({
  match,
  choice,
  onChange,
}: {
  match: ImportMatch
  choice: ReuseChoice
  onChange: (choice: ReuseChoice) => void
}) {
  const scoped = match.scope === 'scoped'
  const kind = typeLabel(match.type)

  return (
    <li className="border-line flex min-w-0 flex-col gap-2 border-t py-3">
      <div className="flex min-w-0 flex-col gap-0.5">
        <span className="text-ink text-[13px] break-words">{match.name}</span>
        <span className="type-data text-dimmer text-[12.5px] break-words">
          {scoped ? `${kind} · in the collection ${match.collection}` : kind}
        </span>
      </div>
      <Segmented
        ariaLabel={`What to do with ${match.name}`}
        value={choice.useExisting ? 'existing' : 'copy'}
        onChange={(value) => onChange({ ...choice, useExisting: value === 'existing' })}
        options={[
          { value: 'copy', label: 'Import a copy' },
          { value: 'existing', label: scoped ? 'Use my existing one' : 'Skip, I already have it' },
        ]}
      />
      {choice.useExisting &&
        (match.existing.length > 1 ? (
          <Select
            ariaLabel={`Which of your catalogs to use for ${match.name}`}
            value={choice.existingID}
            onChange={(existingID) => onChange({ ...choice, existingID })}
            options={match.existing.map((e) => ({ value: e.id, label: e.name }))}
          />
        ) : (
          <span className="type-data text-dim text-[12.5px] break-words">
            Uses {match.existing[0].name} from your library
          </span>
        ))}
      {choice.useExisting && scoped && (
        <span className="type-data text-dimmer text-[12.5px] leading-[1.45]">
          The collection will share your library catalog, so later edits to it show up there
          too.
        </span>
      )}
    </li>
  )
}
