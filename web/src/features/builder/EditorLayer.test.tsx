// @vitest-environment jsdom
import type { RefObject } from 'react'
import { render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { EditorLayer } from './EditorLayer'

afterEach(() => {
  vi.restoreAllMocks()
  document.body.replaceChildren()
})

/** A page shaped like the builder's: a header beside the workspace, the rail
 *  and the layer inside it. */
function renderLayer(stacked: boolean, openKey = 'catalog/mob-two') {
  const header = document.createElement('header')
  const workspace = document.createElement('div')
  const rail = document.createElement('aside')
  rail.tabIndex = -1
  const slot = document.createElement('div')
  workspace.append(rail, slot)
  document.body.append(header, workspace)
  const fallbackFocus: RefObject<HTMLElement | null> = { current: rail }
  const layer = (key: string) => (
    <EditorLayer stacked={stacked} label="Mob two" openKey={key} fallbackFocus={fallbackFocus}>
      <h1 tabIndex={-1} data-landing>
        Mob two
      </h1>
      <button type="button">Duplicate</button>
    </EditorLayer>
  )
  const view = render(layer(openKey), { container: slot })
  return { ...view, header, rail, reopen: (key: string) => view.rerender(layer(key)) }
}

describe('EditorLayer', () => {
  it('is a modal dialog over the page below lg', () => {
    renderLayer(true)
    const layer = screen.getByRole('dialog', { name: 'Mob two' })
    expect(layer).toHaveAttribute('aria-modal', 'true')
    expect(layer).toHaveAttribute('data-editor-layer')
    expect(document.documentElement.style.overflow).toBe('hidden')
  })

  it('makes the rest of the page inert while it covers it, and lets it go', () => {
    const { header, rail, unmount } = renderLayer(true)
    expect(header).toHaveAttribute('inert')
    expect(rail).toHaveAttribute('inert')

    unmount()
    expect(header).not.toHaveAttribute('inert')
    expect(rail).not.toHaveAttribute('inert')
    expect(document.documentElement.style.overflow).toBe('')
  })

  it('is the plain pane column from lg', () => {
    const { header } = renderLayer(false)
    expect(screen.queryByRole('dialog')).toBeNull()
    expect(header).not.toHaveAttribute('inert')
    expect(document.documentElement.style.overflow).toBe('')
  })

  it('lands focus on the title, and gives it back to what opened it', () => {
    const opener = document.createElement('button')
    document.body.append(opener)
    opener.focus()
    const { unmount } = renderLayer(true)
    expect(screen.getByRole('heading', { name: 'Mob two' })).toHaveFocus()

    unmount()
    expect(opener).toHaveFocus()
  })

  it('gives focus to the rail when what opened it is gone', () => {
    const opener = document.createElement('button')
    document.body.append(opener)
    opener.focus()
    const { unmount, rail } = renderLayer(true)
    opener.remove()

    unmount()
    expect(rail).toHaveFocus()
  })

  it('gives focus back to what opened it from lg too', () => {
    const opener = document.createElement('button')
    document.body.append(opener)
    opener.focus()
    const { unmount } = renderLayer(false)
    opener.blur()

    unmount()
    expect(opener).toHaveFocus()
  })

  it('takes a row opened from outside as the new opener, and keeps the old one for a row opened from inside', () => {
    const first = document.createElement('button')
    const second = document.createElement('button')
    document.body.append(first, second)
    first.focus()
    const { reopen, unmount } = renderLayer(false)
    second.focus()
    reopen('catalog/second')
    screen.getByRole('button', { name: 'Duplicate' }).focus()
    reopen('catalog/copy')

    unmount()
    expect(second).toHaveFocus()
  })

  it('leaves focus alone when something else has taken it', () => {
    const opener = document.createElement('button')
    const other = document.createElement('button')
    document.body.append(opener, other)
    opener.focus()
    const { unmount } = renderLayer(false)
    other.focus()

    unmount()
    expect(other).toHaveFocus()
  })
})
