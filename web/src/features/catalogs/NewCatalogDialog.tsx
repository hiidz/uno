import { useEffect, useState } from 'react'
import type { CatalogType } from '@/api'
import { Modal, ModalBody, ModalFooter, ModalHeader } from '@/components/Modal'
import { Field, Segmented, TextInput } from './fields'

/**
 * Names a catalog into existence, and nothing more.
 *
 * **Two fields, because only two of them are hard to change later.** A name is
 * how the row is found in the rail, and `type` is immutable once the catalog
 * exists — every filter in the builder branches on it, and the supported way to
 * change one is to duplicate. Everything else is a filter, and filters are what
 * the editor is for.
 *
 * So this creates a bare catalog with no filters at all, which is a real and
 * useful state: it matches everything TMDB has of that type. Tuning it down is
 * the next step, in the pane, against a preview.
 */
export function NewCatalogDialog({
  open,
  saving,
  serverError,
  onCreate,
  onClose,
}: {
  open: boolean
  saving: boolean
  /** Plain-text body of a server 400. Only `name` can be wrong here and this
   *  form checks it, so it renders as an unexpected-case banner. */
  serverError: string | null
  onCreate: (name: string, type: CatalogType) => void
  onClose: () => void
}) {
  const [name, setName] = useState('')
  const [type, setType] = useState<CatalogType>('movie')
  const [showError, setShowError] = useState(false)

  useEffect(() => {
    if (!open) return
    setName('')
    setType('movie')
    setShowError(false)
  }, [open])

  const invalid = name.trim() === ''

  function submit() {
    setShowError(true)
    if (invalid) return
    onCreate(name.trim(), type)
  }

  return (
    <Modal open={open} onClose={onClose} labelledBy="new-catalog-title" width="440px">
      <ModalHeader>
        <h2 id="new-catalog-title" className="type-display m-0 text-[15px]">
          New catalog
        </h2>
      </ModalHeader>

      <ModalBody>
        <div className="flex flex-col gap-5">
          <Field label="Name" error={showError && invalid ? 'Give this catalog a name.' : undefined}>
            <TextInput
              value={name}
              onChange={setName}
              placeholder="Trending Sci-Fi"
              invalid={showError && invalid}
            />
          </Field>

          <Field label="Source" hint="TMDB is currently the only source available.">
            <Segmented
              ariaLabel="Catalog source"
              value="tmdb"
              onChange={() => {}}
              options={[{ value: 'tmdb', label: 'TMDB' }]}
            />
          </Field>

          <Field
            label="Type"
            hint="Can't be changed later — it determines which filter options are available."
          >
            <Segmented
              ariaLabel="Catalog type"
              value={type}
              onChange={setType}
              options={[
                { value: 'movie', label: 'Movie' },
                { value: 'series', label: 'Series' },
              ]}
            />
          </Field>

          <p className="type-data text-dimmer m-0 text-[10.5px]">
            Created with no filters, so it matches everything. Select it in the library to add
            them.
          </p>

          {serverError && (
            <p className="type-data text-danger border-danger m-0 border-l-2 pl-3 text-[11px]">
              The server rejected this catalog: {serverError}
            </p>
          )}
        </div>
      </ModalBody>

      <ModalFooter>
        <button type="button" onClick={onClose} className="btn-ghost">
          Cancel
        </button>
        <button type="button" onClick={submit} disabled={saving} className="btn-primary">
          {saving ? 'Creating…' : 'Create catalog'}
        </button>
      </ModalFooter>
    </Modal>
  )
}
