import { useEffect, useId, useRef, useState } from 'react'
import { useMutation } from '@tanstack/react-query'
import { exportBundle } from '@/api'
import { Checkbox } from '@/components/fields'
import { Modal, ModalBody, ModalFooter, ModalHeader } from '@/components/Modal'
import type { LibraryCatalog, LibraryCollection } from '@/features/library/useLibrary'
import { downloadJSON, exportFilename } from './download'
import { bundleText, copyText } from './text'

/** Focuses a box, selects its text and brings it into view. */
function reveal(box: HTMLTextAreaElement | null) {
  if (!box) return
  box.focus({ preventScroll: true })
  box.select()
  box.scrollIntoView({ block: 'nearest' })
}

/**
 * Picks catalogs and collections from the library and downloads them as one
 * bundle file or copies them as text. A collection carries every catalog its
 * folders use, so a listed catalog a picked collection references is exported
 * with it whether or not its own box is ticked.
 */
export function ExportDialog({
  open,
  profileIndex,
  catalogs,
  collections,
  preselected,
  onClose,
}: {
  open: boolean
  profileIndex: number
  catalogs: LibraryCatalog[]
  collections: LibraryCollection[]
  /** The id of the row open in the pane, ticked when the dialog opens. */
  preselected: string | null
  onClose: () => void
}) {
  return (
    <Modal open={open} onClose={onClose} labelledBy="export-title" width="480px">
      <ModalHeader>
        <h2 id="export-title" className="type-display m-0 text-[18px]">
          Export
        </h2>
      </ModalHeader>
      {open && (
        <ExportForm
          profileIndex={profileIndex}
          catalogs={catalogs}
          collections={collections}
          preselected={preselected}
          onClose={onClose}
        />
      )}
    </Modal>
  )
}

/** The picks for one opening of the dialog, mounted only while it is open so
 *  every opening starts from the open row alone. */
function ExportForm({
  profileIndex,
  catalogs,
  collections,
  preselected,
  onClose,
}: {
  profileIndex: number
  catalogs: LibraryCatalog[]
  collections: LibraryCollection[]
  preselected: string | null
  onClose: () => void
}) {
  const [picked, setPicked] = useState<ReadonlySet<string>>(
    () => new Set(preselected ? [preselected] : []),
  )
  const { request, copied, fallback, clearFallback } = useExporter(profileIndex, onClose)

  const catalogIDs = catalogs.filter((c) => picked.has(c.id)).map((c) => c.id)
  const collectionIDs = collections.filter((c) => picked.has(c.id)).map((c) => c.id)
  const empty = catalogIDs.length === 0 && collectionIDs.length === 0

  function pick(ids: string[], on: boolean) {
    clearFallback()
    setPicked((current) => {
      const next = new Set(current)
      for (const id of ids) {
        if (on) next.add(id)
        else next.delete(id)
      }
      return next
    })
  }

  return (
    <>
      <ModalBody>
        <div className="flex flex-col gap-5">
          <PickGroup
            label="Catalogs"
            rows={catalogs.map((c) => ({ id: c.id, name: c.name }))}
            picked={picked}
            onPick={pick}
          />
          <PickGroup
            label="Collections"
            rows={collections.map((c) => ({ id: c.id, name: c.title }))}
            picked={picked}
            onPick={pick}
          />
          <p className="type-data text-dimmer m-0 text-[12.5px]">
            Collections include the catalogs they use.
          </p>
          {fallback !== null && <CopyFallback text={fallback} />}
          {request.error && (
            <p role="alert" className="callout-danger type-data">
              Couldn't export: {request.error.message}
            </p>
          )}
        </div>
      </ModalBody>
      <ExportFooter
        onClose={onClose}
        disabled={empty || request.isPending}
        running={request.isPending ? request.variables.as : null}
        copied={copied}
        onRun={(as) =>
          request.mutate({ body: { catalog_ids: catalogIDs, collection_ids: collectionIDs }, as })
        }
      />
    </>
  )
}

function PickGroup({
  label,
  rows,
  picked,
  onPick,
}: {
  label: string
  rows: Array<{ id: string; name: string }>
  picked: ReadonlySet<string>
  onPick: (ids: string[], on: boolean) => void
}) {
  const labelID = useId()
  const ids = rows.map((row) => row.id)
  const count = ids.filter((id) => picked.has(id)).length

  return (
    <div role="group" aria-labelledby={labelID} className="flex min-w-0 flex-col gap-2">
      <div className="border-line flex items-center gap-2 border-b pb-2">
        <span id={labelID} className="type-label flex-1">
          {label}
        </span>
        <span className="type-data text-dimmer text-[12px]">
          {count} of {rows.length}
        </span>
        <button type="button" className="btn-ghost" onClick={() => onPick(ids, true)}>
          All
        </button>
        <button type="button" className="btn-ghost" onClick={() => onPick(ids, false)}>
          None
        </button>
      </div>
      {rows.length === 0 ? (
        <p className="type-data text-dimmer m-0 text-[12.5px]">None in your library.</p>
      ) : (
        rows.map((row) => (
          <Checkbox
            key={row.id}
            checked={picked.has(row.id)}
            onChange={(on) => onPick([row.id], on)}
            label={row.name}
          />
        ))
      )}
    </div>
  )
}

/** What one export run produced. A text run has already tried the clipboard. */
type ExportOutcome =
  | { as: 'file'; bundle: unknown }
  | { as: 'text'; text: string; copied: boolean }

interface ExportRequest {
  body: { catalog_ids: string[]; collection_ids: string[] }
  as: 'file' | 'text'
}

/** One export request. A text run hands the clipboard the request's own
 *  promise, so a browser that ties copying to the click keeps the click across
 *  the wait. A failed request rejects either way; a failed copy does not. */
async function runExport(profileIndex: number, { body, as }: ExportRequest): Promise<ExportOutcome> {
  const bundle = exportBundle(profileIndex, body)
  if (as === 'file') return { as, bundle: await bundle }
  const text = bundle.then(bundleText)
  const copying = copyText(text)
  return { as, text: await text, copied: await copying }
}

/** How long Copy JSON reads "Copied" before it reverts. */
const COPIED_MS = 1600

/** The export request and what follows it: a download closes the dialog, a
 *  copy leaves it open on "Copied", and a copy the browser refused leaves the
 *  text in `fallback` to be copied by hand. */
function useExporter(profileIndex: number, onClose: () => void) {
  const [copied, setCopied] = useState(false)
  const [fallback, setFallback] = useState<string | null>(null)

  useEffect(() => {
    if (!copied) return
    const timer = window.setTimeout(() => setCopied(false), COPIED_MS)
    return () => window.clearTimeout(timer)
  }, [copied])

  function settle(outcome: ExportOutcome) {
    if (outcome.as === 'file') {
      downloadJSON(outcome.bundle, exportFilename(new Date()))
      onClose()
      return
    }
    setFallback(outcome.copied ? null : outcome.text)
    setCopied(outcome.copied)
  }

  const request = useMutation({
    mutationFn: (req: ExportRequest) => runExport(profileIndex, req),
    onSuccess: settle,
  })

  return { request, copied, fallback, clearFallback: () => setFallback(null) }
}

/** Cancel, Copy JSON and Download. Download is the one primary; the label of
 *  whichever is running says so, and both wait while it does. */
function ExportFooter({
  onClose,
  disabled,
  running,
  copied,
  onRun,
}: {
  onClose: () => void
  disabled: boolean
  running: 'file' | 'text' | null
  copied: boolean
  onRun: (as: 'file' | 'text') => void
}) {
  return (
    <ModalFooter>
      <div className="flex min-w-0 flex-1 flex-wrap items-center justify-end gap-2">
        <button type="button" onClick={onClose} className="btn-ghost">
          Cancel
        </button>
        <button
          type="button"
          disabled={disabled}
          onClick={() => onRun('text')}
          className="btn-secondary min-w-[6.5rem]"
        >
          {running === 'text' ? 'Copying…' : copied ? 'Copied' : 'Copy JSON'}
        </button>
        <button type="button" disabled={disabled} onClick={() => onRun('file')} className="btn-primary">
          {running === 'file' ? 'Exporting…' : 'Download'}
        </button>
        <span role="status" className="sr-only">
          {copied ? 'Copied to clipboard' : ''}
        </span>
      </div>
    </ModalFooter>
  )
}

/** The export text in a read-only box, focused and selected, for a browser
 *  that refused the clipboard. */
function CopyFallback({ text }: { text: string }) {
  const ref = useRef<HTMLTextAreaElement>(null)

  useEffect(() => reveal(ref.current), [text])

  return (
    <div className="flex flex-col gap-2">
      <p role="alert" className="callout-danger type-data">
        Couldn't copy. Copy the JSON below.
      </p>
      <textarea
        ref={ref}
        readOnly
        rows={8}
        value={text}
        aria-label="Export JSON"
        className="field block h-auto w-full resize-none py-2.5 font-mono text-[13px] leading-snug pointer-coarse:text-[16px]"
      />
    </div>
  )
}
