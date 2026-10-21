// @vitest-environment jsdom
import { useEffect, useRef, useState } from 'react'
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'
import { ConfirmDialog } from './ConfirmDialog'
import { Modal } from './Modal'
import { MoreMenu, MoreMenuItem } from './MoreMenu'

afterEach(() => {
  document.body.replaceChildren()
})

function openMenu(name: string) {
  fireEvent.pointerDown(screen.getByRole('button', { name }), { button: 0, ctrlKey: false, pointerType: 'mouse' })
}

function ModalFromButton() {
  const [open, setOpen] = useState(false)
  return (
    <>
      <button type="button" onClick={() => setOpen(true)}>
        Import
      </button>
      <Modal open={open} onClose={() => setOpen(false)} labelledBy="title">
        <h2 id="title">Import</h2>
        <button type="button">Choose file</button>
      </Modal>
    </>
  )
}

/** A confirmation raised from a "⋯" entry, mounted only while it is asked,
 *  as the Workspace's prompt is. */
function ConfirmFromMenu({ opensField = false }: { opensField?: boolean }) {
  const [asking, setAsking] = useState(false)
  const [done, setDone] = useState(false)
  return (
    <>
      {done && opensField && <FocusedField />}
      <MoreMenu label="Mob two">
        <MoreMenuItem onSelect={() => setAsking(true)}>Delete</MoreMenuItem>
      </MoreMenu>
      {asking && (
        <ConfirmDialog
          open
          title="Delete this catalog?"
          body="It goes for good."
          confirmLabel="Delete catalog"
          onConfirm={() => {
            setAsking(false)
            setDone(true)
          }}
          onCancel={() => setAsking(false)}
        />
      )}
    </>
  )
}

/** What a confirmed action opens and focuses as it mounts, as a new
 *  editor's name field is. */
function FocusedField() {
  const ref = useRef<HTMLInputElement>(null)
  useEffect(() => ref.current?.focus(), [])
  return <input ref={ref} aria-label="Name" />
}

describe('focus return', () => {
  it('gives focus back to what opened a Modal when it closes', async () => {
    render(<ModalFromButton />)
    const opener = screen.getByRole('button', { name: 'Import' })
    opener.focus()
    fireEvent.click(opener)
    await screen.findByRole('dialog')

    fireEvent.keyDown(document.activeElement ?? document.body, { key: 'Escape' })
    await waitFor(() => expect(opener).toHaveFocus())
  })

  it('gives focus back to the "⋯" a confirmation was raised from, on cancel', async () => {
    render(<ConfirmFromMenu />)
    openMenu('More for Mob two')
    fireEvent.click(await screen.findByRole('menuitem', { name: 'Delete' }))
    await screen.findByRole('alertdialog')

    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))
    await waitFor(() => expect(screen.getByRole('button', { name: 'More for Mob two' })).toHaveFocus())
  })

  it('leaves focus where the confirmed action put it', async () => {
    render(<ConfirmFromMenu opensField />)
    openMenu('More for Mob two')
    fireEvent.click(await screen.findByRole('menuitem', { name: 'Delete' }))
    await screen.findByRole('alertdialog')

    fireEvent.click(screen.getByRole('button', { name: 'Delete catalog' }))
    await act(() => new Promise((resolve) => setTimeout(resolve, 20)))
    expect(screen.getByRole('textbox', { name: 'Name' })).toHaveFocus()
  })
})
