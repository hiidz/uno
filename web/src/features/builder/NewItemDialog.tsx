import { useId, useState } from 'react'
import type { ReactNode } from 'react'
import { Field, TextInput } from '@/components/fields'
import { Modal, ModalBody, ModalFooter, ModalHeader } from '@/components/Modal'

/**
 * Names a library item into existence, and nothing more.
 *
 * **It asks for what can't be changed later, plus what the row is found by**,
 * and leaves everything else to the editor it opens on. What it creates is
 * therefore bare — a catalog with no filters, a collection with no folders —
 * which is a real and useful state in both cases rather than a half-made one.
 *
 * That rule is what decides how many fields there are, so `extra` is where a
 * kind with a second immutable field puts it. A kind whose name is the only
 * thing worth settling up front leaves the slot out. `Workspace`'s two call
 * sites say which is which; `CollectionEditor`'s "new inside this collection"
 * asks the same two things a library catalog does.
 */
export function NewItemDialog({
  open,
  noun,
  label,
  placeholder,
  saving,
  serverError,
  extra,
  onCreate,
  onClose,
}: {
  open: boolean
  /** What is being created, carried through every line of copy here — "New
   *  catalog", "Give this catalog a name.", "Create catalog" — and the region
   *  colour of its Create button. */
  noun: 'catalog' | 'collection'
  /** What the one required field is called — "Name", "Title". */
  label: string
  placeholder: string
  saving: boolean
  /** Plain-text body of a server 400. Only the named field can be wrong here
   *  and this form checks it, so it renders as an unexpected-case banner. */
  serverError: string | null
  /** A second field, for a kind that has another thing it can't ask later. */
  extra?: ReactNode
  onCreate: (value: string) => void
  onClose: () => void
}) {
  return (
    <Modal open={open} onClose={onClose} labelledBy={`new-${noun}-title`} width="440px">
      <ModalHeader>
        <h2 id={`new-${noun}-title`} className="type-display m-0 text-[18px]">
          New {noun}
        </h2>
      </ModalHeader>

      {open && (
        <NewItemForm
          noun={noun}
          label={label}
          placeholder={placeholder}
          saving={saving}
          serverError={serverError}
          extra={extra}
          onCreate={onCreate}
          onClose={onClose}
        />
      )}
    </Modal>
  )
}

/**
 * The fields and the two buttons, holding the draft for one opening of the
 * dialog. It is mounted only while the dialog is open, so every opening starts
 * on an empty field with the error unrevealed, however the last one ended.
 */
function NewItemForm({
  noun,
  label,
  placeholder,
  saving,
  serverError,
  extra,
  onCreate,
  onClose,
}: Omit<Parameters<typeof NewItemDialog>[0], 'open'>) {
  const [value, setValue] = useState('')
  const [showError, setShowError] = useState(false)
  const inputId = useId()

  const invalid = value.trim() === ''

  function submit() {
    setShowError(true)
    if (invalid) return
    onCreate(value.trim())
  }

  return (
    <>
      <ModalBody>
        <div className="flex flex-col gap-5">
          <Field
            label={label}
            htmlFor={inputId}
            error={showError && invalid ? `Give this ${noun} a ${label.toLowerCase()}.` : undefined}
          >
            <TextInput
              id={inputId}
              value={value}
              onChange={setValue}
              placeholder={placeholder}
              invalid={showError && invalid}
            />
          </Field>

          {extra}

          {serverError && (
            <p className="callout-danger type-data">
              Couldn't create this {noun}: {serverError}
            </p>
          )}
        </div>
      </ModalBody>

      <ModalFooter tone={noun}>
        <button type="button" onClick={onClose} className="btn-ghost">
          Cancel
        </button>
        <button type="button" onClick={submit} disabled={saving} className="btn-primary">
          {saving ? 'Creating…' : `Create ${noun}`}
        </button>
      </ModalFooter>
    </>
  )
}
