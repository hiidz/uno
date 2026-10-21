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

/** A row whose Delete asks first, then goes a moment after the answer, as a
 *  library row goes once the library refetches; the rail around it is where
 *  focus lands then. */
function RowGoingLater() {
  const [asking, setAsking] = useState(false)
  const [kept, setKept] = useState(true)
  return (
    <aside aria-label="Library" tabIndex={-1} data-landing>
      {kept && (
        <button type="button" onClick={() => setAsking(true)}>
          Delete Mob two
        </button>
      )}
      {asking && (
        <ConfirmDialog
          open
          title="Delete this catalog?"
          body="It goes for good."
          confirmLabel="Delete catalog"
          onConfirm={() => {
            setAsking(false)
            setTimeout(() => setKept(false), 50)
          }}
          onCancel={() => setAsking(false)}
        />
      )}
    </aside>
  )
}

/** A list of rows, each with a "⋯" whose Remove takes the row away and whose
 *  Move sends it to the other list. */
function TwoLists() {
  const [home, setHome] = useState(['Mob one', 'Mob two', 'Mob three'])
  const [discover, setDiscover] = useState<string[]>([])
  const list = (names: string[], label: string) => (
    <ul aria-label={label}>
      {names.map((name) => (
        <li key={name}>
          <MoreMenu label={name}>
            <MoreMenuItem onSelect={() => setHome((rows) => rows.filter((row) => row !== name))}>Remove</MoreMenuItem>
            <MoreMenuItem
              onSelect={() => {
                setHome((rows) => rows.filter((row) => row !== name))
                setDiscover((rows) => [...rows, name])
              }}
            >
              Move
            </MoreMenuItem>
          </MoreMenu>
        </li>
      ))}
    </ul>
  )
  return (
    <>
      {list(home, 'Home')}
      {list(discover, 'Discover')}
    </>
  )
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

  it('moves focus on to the region a dialog’s opener sat in when the opener goes after the dialog closes', async () => {
    render(<RowGoingLater />)
    const remove = screen.getByRole('button', { name: 'Delete Mob two' })
    remove.focus()
    fireEvent.click(remove)
    await screen.findByRole('alertdialog')

    fireEvent.click(screen.getByRole('button', { name: 'Delete catalog' }))
    await waitFor(() => expect(remove).toHaveFocus())
    await waitFor(() => expect(screen.getByRole('complementary', { name: 'Library' })).toHaveFocus())
  })

  it('moves focus to the next row’s "⋯" when a menu entry removes its own row', async () => {
    render(<TwoLists />)
    openMenu('More for Mob two')
    fireEvent.click(await screen.findByRole('menuitem', { name: 'Remove' }))
    await waitFor(() => expect(screen.getByRole('button', { name: 'More for Mob three' })).toHaveFocus())
  })

  it('moves focus to the row above when the last row is removed', async () => {
    render(<TwoLists />)
    openMenu('More for Mob three')
    fireEvent.click(await screen.findByRole('menuitem', { name: 'Remove' }))
    await waitFor(() => expect(screen.getByRole('button', { name: 'More for Mob two' })).toHaveFocus())
  })

  it('follows a row that moves to another list', async () => {
    render(<TwoLists />)
    openMenu('More for Mob one')
    fireEvent.click(await screen.findByRole('menuitem', { name: 'Move' }))
    await waitFor(() => expect(screen.getByRole('button', { name: 'More for Mob one' })).toHaveFocus())
    expect(screen.getByRole('list', { name: 'Discover' })).toContainElement(document.activeElement as HTMLElement)
  })
})
