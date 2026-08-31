import { useEffect, useState } from 'react'
import { Field, TextInput } from '@/components/fields'
import { Modal, ModalBody, ModalFooter, ModalHeader } from '@/components/Modal'

/**
 * Titles a collection into existence, and nothing more.
 *
 * **One field, by the same rule as the catalog dialog: ask only for what can't
 * be changed later, plus what the row is found by.** For a collection that is
 * just the title — nothing about a collection is immutable, unlike a catalog's
 * `type`. View mode, pinning, the All tab and the backdrop are all editable
 * afterwards, and folders are the substance of the editor rather than something
 * to guess at up front.
 *
 * So this creates an empty collection, which renders as an empty row until it
 * has folders. That's a real intermediate state, and saving opens the editor on
 * it, which is where it stops being one.
 */
export function NewCollectionDialog({
  open,
  saving,
  serverError,
  onCreate,
  onClose,
}: {
  open: boolean
  saving: boolean
  /** Plain-text body of a server 400. Only `title` can be wrong here and this
   *  form checks it, so it renders as an unexpected-case banner. */
  serverError: string | null
  onCreate: (title: string) => void
  onClose: () => void
}) {
  const [title, setTitle] = useState('')
  const [showError, setShowError] = useState(false)

  useEffect(() => {
    if (!open) return
    setTitle('')
    setShowError(false)
  }, [open])

  const invalid = title.trim() === ''

  function submit() {
    setShowError(true)
    if (invalid) return
    onCreate(title.trim())
  }

  return (
    <Modal open={open} onClose={onClose} labelledBy="new-collection-title" width="440px">
      <ModalHeader>
        <h2 id="new-collection-title" className="type-display m-0 text-[15px]">
          New collection
        </h2>
      </ModalHeader>

      <ModalBody>
        <div className="flex flex-col gap-5">
          <Field
            label="Title"
            error={showError && invalid ? 'Give this collection a title.' : undefined}
          >
            <TextInput
              value={title}
              onChange={setTitle}
              placeholder="Saturday night"
              invalid={showError && invalid}
            />
          </Field>

          {serverError && (
            <p className="type-data text-danger border-danger m-0 border-l-2 pl-3 text-[11px]">
              Couldn't create this collection: {serverError}
            </p>
          )}
        </div>
      </ModalBody>

      <ModalFooter>
        <button type="button" onClick={onClose} className="btn-ghost">
          Cancel
        </button>
        <button type="button" onClick={submit} disabled={saving} className="btn-primary">
          {saving ? 'Creating…' : 'Create collection'}
        </button>
      </ModalFooter>
    </Modal>
  )
}
