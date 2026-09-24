import { useId, useState } from 'react'
import { useMutation } from '@tanstack/react-query'
import { exportBundle } from '@/api'
import { Checkbox } from '@/components/fields'
import { Modal, ModalBody, ModalFooter, ModalHeader } from '@/components/Modal'
import type { LibraryCatalog, LibraryCollection } from '@/features/library/useLibrary'
import { downloadJSON, exportFilename } from './download'

/**
 * Picks catalogs and collections from the library and downloads them as one
 * bundle file. A collection carries every catalog its folders use, so a
 * listed catalog a picked collection references is exported with it whether
 * or not its own box is ticked.
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
        <h2 id="export-title" className="type-display m-0 text-[15px]">
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

  const exporting = useMutation({
    mutationFn: (body: { catalog_ids: string[]; collection_ids: string[] }) =>
      exportBundle(profileIndex, body),
    onSuccess: (bundle) => {
      downloadJSON(bundle, exportFilename(new Date()))
      onClose()
    },
  })

  const catalogIDs = catalogs.filter((c) => picked.has(c.id)).map((c) => c.id)
  const collectionIDs = collections.filter((c) => picked.has(c.id)).map((c) => c.id)
  const empty = catalogIDs.length === 0 && collectionIDs.length === 0

  function toggle(id: string, on: boolean) {
    setPicked((current) => {
      const next = new Set(current)
      if (on) next.add(id)
      else next.delete(id)
      return next
    })
  }

  function setAll(ids: string[], on: boolean) {
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
            onToggle={toggle}
            onSetAll={setAll}
          />
          <PickGroup
            label="Collections"
            rows={collections.map((c) => ({ id: c.id, name: c.title }))}
            picked={picked}
            onToggle={toggle}
            onSetAll={setAll}
          />
          <p className="type-data text-dimmer m-0 text-[11px]">
            Collections include the catalogs they use.
          </p>
          {exporting.error && (
            <p
              role="alert"
              className="type-data text-danger border-danger m-0 border-l-2 pl-3 text-[11px]"
            >
              Couldn't export: {exporting.error.message}
            </p>
          )}
        </div>
      </ModalBody>
      <ModalFooter>
        <button type="button" onClick={onClose} className="btn-ghost">
          Cancel
        </button>
        <button
          type="button"
          disabled={empty || exporting.isPending}
          onClick={() => exporting.mutate({ catalog_ids: catalogIDs, collection_ids: collectionIDs })}
          className="btn-primary"
        >
          {exporting.isPending ? 'Exporting…' : 'Export'}
        </button>
      </ModalFooter>
    </>
  )
}

function PickGroup({
  label,
  rows,
  picked,
  onToggle,
  onSetAll,
}: {
  label: string
  rows: Array<{ id: string; name: string }>
  picked: ReadonlySet<string>
  onToggle: (id: string, on: boolean) => void
  onSetAll: (ids: string[], on: boolean) => void
}) {
  const labelID = useId()
  const ids = rows.map((row) => row.id)
  const count = ids.filter((id) => picked.has(id)).length

  return (
    <div role="group" aria-labelledby={labelID} className="flex min-w-0 flex-col gap-2">
      <div className="border-line flex items-center gap-2 border-b pb-2">
        <span id={labelID} className="type-eyebrow flex-1">
          {label}
        </span>
        <span className="type-data text-dimmer text-[10px]">
          {count} of {rows.length}
        </span>
        <button type="button" className="btn-ghost" onClick={() => onSetAll(ids, true)}>
          All
        </button>
        <button type="button" className="btn-ghost" onClick={() => onSetAll(ids, false)}>
          None
        </button>
      </div>
      {rows.length === 0 ? (
        <p className="type-data text-dimmer m-0 text-[11px]">None in your library.</p>
      ) : (
        rows.map((row) => (
          <Checkbox
            key={row.id}
            checked={picked.has(row.id)}
            onChange={(on) => onToggle(row.id, on)}
            label={row.name}
          />
        ))
      )}
    </div>
  )
}
