// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { watchKeyboardInset } from './keyboardInset'

/** A visual viewport that the test resizes, as a keyboard or a pinch does. */
function fakeViewport() {
  const viewport = Object.assign(new EventTarget(), { height: window.innerHeight, offsetTop: 0, scale: 1 })
  vi.stubGlobal('visualViewport', viewport)
  return (change: Partial<typeof viewport>) => {
    Object.assign(viewport, change)
    viewport.dispatchEvent(new Event('resize'))
  }
}

const inset = () => document.documentElement.style.getPropertyValue('--keyboard-h')

afterEach(() => vi.unstubAllGlobals())

describe('watchKeyboardInset', () => {
  it('follows how much of the window the keyboard covers, and counts a pinch zoom as none', () => {
    const change = fakeViewport()
    watchKeyboardInset()
    expect(inset()).toBe('0px')

    change({ height: window.innerHeight - 300 })
    expect(inset()).toBe('300px')

    change({ height: window.innerHeight - 300, offsetTop: 100 })
    expect(inset()).toBe('200px')

    change({ scale: 2, height: window.innerHeight / 2, offsetTop: 0 })
    expect(inset()).toBe('0px')
  })
})
