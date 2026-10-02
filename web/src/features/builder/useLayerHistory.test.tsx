// @vitest-environment jsdom
import { act, renderHook } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useLayerHistory } from './useLayerHistory'

let back: ReturnType<typeof vi.spyOn>

beforeEach(() => {
  vi.useFakeTimers()
  window.history.replaceState({ idx: 1 }, '')
  back = vi.spyOn(window.history, 'back').mockImplementation(() => {})
})

afterEach(() => {
  vi.useRealTimers()
  vi.restoreAllMocks()
})

/** What the browser does on Back: the entry under the current one comes
 *  back, and `popstate` reports its state. */
function pressBack() {
  act(() => {
    window.history.replaceState({ idx: 1 }, '')
    window.dispatchEvent(new PopStateEvent('popstate', { state: { idx: 1 } }))
  })
}

describe('useLayerHistory', () => {
  it('pushes one entry while open', () => {
    renderHook(() => useLayerHistory(true, vi.fn()), { reactStrictMode: true })
    act(() => vi.runAllTimers())
    expect(window.history.state).toEqual({ idx: 1, unoEditor: true })
    expect(back).not.toHaveBeenCalled()
  })

  it('pushes nothing while not covering the page', () => {
    renderHook(() => useLayerHistory(false, vi.fn()))
    expect(window.history.state).toEqual({ idx: 1 })
  })

  it('steps back off its entry when closed any other way', () => {
    const { unmount } = renderHook(() => useLayerHistory(true, vi.fn()))
    unmount()
    act(() => vi.runAllTimers())
    expect(back).toHaveBeenCalledTimes(1)
  })

  it('steps back off its entry when it stops covering the page', () => {
    const { rerender } = renderHook(({ open }) => useLayerHistory(open, vi.fn()), {
      initialProps: { open: true },
    })
    rerender({ open: false })
    expect(back).toHaveBeenCalledTimes(1)
  })

  it('asks to close on Back, and pushes again when the close is held', () => {
    const requestClose = vi.fn()
    renderHook(() => useLayerHistory(true, requestClose))
    pressBack()
    expect(requestClose).toHaveBeenCalledTimes(1)
    // Still open — the discard question is up — so the entry is back on.
    expect(window.history.state).toEqual({ idx: 1, unoEditor: true })
  })

  it('does not step back once something else has moved past its entry', () => {
    const { unmount } = renderHook(() => useLayerHistory(true, vi.fn()))
    // Switch profile navigates to /profiles over the layer's entry.
    window.history.pushState({ idx: 2 }, '')
    unmount()
    act(() => vi.runAllTimers())
    expect(back).not.toHaveBeenCalled()
  })
})
