// @vitest-environment jsdom
import { act, render, renderHook } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { focusRow, rowButtonID, scrollPosition, scrollTo, useScrollMemory } from './scroll'

afterEach(() => vi.restoreAllMocks())

describe('scroll', () => {
  it('reads and sets the window and a column', () => {
    const column = document.createElement('div')
    column.scrollTop = 40
    const windowScroll = vi.spyOn(window, 'scrollTo').mockImplementation(() => {})
    expect(scrollPosition(column)).toEqual({ window: 0, column: 40 })
    expect(scrollPosition(null)).toEqual({ window: 0, column: 0 })
    scrollTo(column, { window: 120, column: 7 })
    expect(windowScroll).toHaveBeenCalledWith(0, 120)
    expect(column.scrollTop).toBe(7)
    scrollTo(null, { window: 5, column: 9 })
    expect(windowScroll).toHaveBeenLastCalledWith(0, 5)
  })

  it('focuses a row’s open button, and ignores one that is gone', () => {
    render(<button id={rowButtonID('p1')} type="button">Row</button>)
    focusRow('p1')
    expect(document.activeElement).toHaveProperty('id', 'community-row-p1')
    expect(() => focusRow('gone')).not.toThrow()
  })

  it('puts the list back where it was, then runs what comes after', () => {
    const windowScroll = vi.spyOn(window, 'scrollTo').mockImplementation(() => {})
    const { result } = renderHook(() => useScrollMemory())
    const after = vi.fn()

    act(() => result.current.restore(after))
    expect(after).not.toHaveBeenCalled()

    act(() => result.current.remember())
    expect(windowScroll).toHaveBeenLastCalledWith(0, 0)
    act(() => result.current.restore(after))
    expect(after).toHaveBeenCalledTimes(1)
  })
})
