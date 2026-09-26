import { useState } from 'react'
import { useMutation } from '@tanstack/react-query'
import { checkImport } from '@/api'
import type { ImportCheck, ImportMatch, ImportResult } from '@/api'
import { Segmented, Select } from '@/components/fields'
import { Modal, ModalBody, ModalFooter, ModalHeader } from '@/components/Modal'
import { typeLabel } from '@/features/library/recipe'
import { pluralCount } from '@/lib/plural'
import { choicesForAll, reuseMap, type ReuseChoice, type ReuseChoices } from './reuse'
import { useImport } from './useImport'

/** The largest file the dialog reads. The twin of `maxBundleBodyBytes` in
 *  `internal/api/bundle.go`, the most either import route accepts; change one
 *  and you must change the other. */
const MAX_BUNDLE_BYTES = 4 << 20 // 4 MiB

/**
 * Imports a bundle file as new, private catalogs and collections, in three
 * steps: pick a file, review the catalogs it shares a recipe with ones you
 * already have, import.
 *
 * Every catalog is imported as a copy unless the review says otherwise.
 * Importing the same file twice gives two sets.
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
    <Modal open={open} onClose={onClose} labelledBy="import-title" width="560px">
      <ModalHeader>
        <h2 id="import-title" className="type-display m-0 text-[18px]">
          Import
        </h2>
      </ModalHeader>
      {open && <ImportFlow profileIndex={profileIndex} onClose={onClose} onImported={onImported} />}
    </Modal>
  )
}

interface Review {
  fileName: string
  bundle: unknown
  check: ImportCheck
  choices: ReuseChoices
}

/** One opening of the dialog, mounted only while it is open so every opening
 *  starts back at the file picker. */
function ImportFlow({
  profileIndex,
  onClose,
  onImported,
}: {
  profileIndex: number
  onClose: () => void
  onImported: (result: ImportResult) => void
}) {
  const [review, setReview] = useState<Review | null>(null)
  const [fileError, setFileError] = useState<string | null>(null)
  const checking = useMutation({
    mutationFn: (bundle: unknown) => checkImport(profileIndex, bundle),
  })
  const importing = useImport(profileIndex)

  async function pick(file: File) {
    setFileError(null)
    checking.reset()
    if (file.size > MAX_BUNDLE_BYTES) {
      setFileError(`${file.name} is over 4 MiB, the most an import can carry.`)
      return
    }
    let bundle: unknown
    try {
      bundle = JSON.parse(await file.text())
    } catch (err) {
      setFileError(`${file.name} isn't valid JSON: ${(err as Error).message}`)
      return
    }
    checking.mutate(bundle, {
      onSuccess: (check) =>
        setReview({ fileName: file.name, bundle, check, choices: choicesForAll(check.matches, false) }),
    })
  }

  function write() {
    if (!review) return
    importing.mutate(
      { bundle: review.bundle, reuse: reuseMap(review.check.matches, review.choices) },
      { onSuccess: onImported },
    )
  }

  return (
    <>
      <ModalBody>
        {review ? (
          <ReviewStep
            review={review}
            onChoices={(choices) => setReview({ ...review, choices })}
          />
        ) : (
          <PickStep
            checking={checking.isPending}
            error={fileError ?? (checking.error && `This file can't be imported: ${checking.error.message}`)}
            onPick={(file) => void pick(file)}
          />
        )}
        {importing.error && (
          <p
            role="alert"
            className="callout-danger type-data mt-4"
          >
            Couldn't import: {importing.error.message}
          </p>
        )}
      </ModalBody>
      <ModalFooter>
        <button type="button" onClick={onClose} className="btn-ghost">
          Cancel
        </button>
        {review && (
          <button
            type="button"
            onClick={write}
            disabled={importing.isPending}
            className="btn-primary"
          >
            {importing.isPending ? 'Importing…' : 'Import'}
          </button>
        )}
      </ModalFooter>
    </>
  )
}

function PickStep({
  checking,
  error,
  onPick,
}: {
  checking: boolean
  error: string | null
  onPick: (file: File) => void
}) {
  return (
    <div className="flex flex-col gap-4">
      <p className="text-dim m-0 text-[13px] leading-relaxed">
        Pick an Uno export file. Everything in it is added to your library as new, private
        catalogs and collections.
      </p>
      {/* The input stays in the accessibility tree, visually hidden inside
          its label, so the label is what is seen and clicked while the
          input still takes focus and a file. */}
      <label className="btn-secondary has-[:focus-visible]:outline-ink w-fit cursor-pointer has-[:focus-visible]:outline-2 has-[:focus-visible]:outline-offset-2">
        {checking ? 'Checking…' : 'Choose a file…'}
        <input
          type="file"
          accept=".json,application/json"
          disabled={checking}
          className="sr-only"
          onChange={(event) => {
            const file = event.target.files?.[0]
            // Cleared so picking the same file again still fires a change.
            event.target.value = ''
            if (file) onPick(file)
          }}
        />
      </label>
      {error && (
        <p
          role="alert"
          className="callout-danger type-data break-words"
        >
          {error}
        </p>
      )}
    </div>
  )
}

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
        <strong className="text-ink">{review.fileName}</strong> holds{' '}
        {pluralCount(check.catalogs, 'catalog')}, {pluralCount(check.collections, 'collection')}{' '}
        and {pluralCount(check.folders, 'folder')}.
      </p>

      {matches.length > 0 && (
        <>
          <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
            <p className="text-dim m-0 flex-1 basis-[240px] text-[13px] leading-relaxed">
              {matches.length === 1
                ? 'One of these has the same recipe as a catalog you already have.'
                : `${matches.length} of these have the same recipe as catalogs you already have.`}
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
        </>
      )}
    </div>
  )
}

/**
 * One matched catalog. The wording follows where the catalog sits in the
 * file: a top-level catalog reused is simply not imported, while one of a
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
