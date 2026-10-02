// @vitest-environment jsdom
import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { EditorShell } from './EditorShell'

function renderShell() {
  const onRequestClose = vi.fn()
  render(
    <EditorShell
      purpose="Edit catalog"
      tone="catalog"
      step={undefined}
      title="Mob two"
      onRequestClose={onRequestClose}
      footer={null}
      docked="results"
    >
      form
    </EditorShell>,
  )
  return onRequestClose
}

function pressEscape() {
  const event = new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true })
  document.dispatchEvent(event)
  return event
}

describe('EditorShell', () => {
  it('closes from × at every width, with no Library button', () => {
    const onRequestClose = renderShell()
    fireEvent.click(screen.getByRole('button', { name: 'Close editor' }))
    expect(onRequestClose).toHaveBeenCalledTimes(1)
    expect(screen.queryByRole('button', { name: /library/i })).toBeNull()
  })

  it('closes on Escape and marks it handled', () => {
    const onRequestClose = renderShell()
    expect(pressEscape().defaultPrevented).toBe(true)
    expect(onRequestClose).toHaveBeenCalledTimes(1)
  })

  it('leaves Escape to a dialog in front of it', () => {
    const onRequestClose = renderShell()
    const dialog = document.createElement('div')
    dialog.setAttribute('role', 'dialog')
    document.body.append(dialog)
    pressEscape()
    expect(onRequestClose).not.toHaveBeenCalled()
    dialog.remove()
  })

  it('still closes inside its own layer, which is a dialog too', () => {
    const onRequestClose = renderShell()
    const layer = document.createElement('div')
    layer.setAttribute('role', 'dialog')
    layer.setAttribute('data-editor-layer', '')
    document.body.append(layer)
    pressEscape()
    expect(onRequestClose).toHaveBeenCalledTimes(1)
    layer.remove()
  })
})
