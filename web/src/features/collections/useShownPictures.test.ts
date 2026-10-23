// @vitest-environment jsdom
import { act, renderHook } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useShownAddress } from './useShownPictures'

beforeEach(() => {
  vi.useFakeTimers()
})

afterEach(() => {
  vi.useRealTimers()
})

describe('useShownAddress', () => {
  it('shows an https address once typing has held still for half a second', () => {
    const { result, rerender } = renderHook(({ raw }) => useShownAddress(raw), {
      initialProps: { raw: 'https://image.tmdb.org/a.jpg' },
    })
    expect(result.current).toBe('https://image.tmdb.org/a.jpg')

    rerender({ raw: ' https://image.tmdb.org/b.jpg ' })
    act(() => vi.advanceTimersByTime(499))
    expect(result.current).toBe('https://image.tmdb.org/a.jpg')
    act(() => vi.advanceTimersByTime(1))
    expect(result.current).toBe('https://image.tmdb.org/b.jpg')
  })

  it('shows nothing for a half-typed or non-https address', () => {
    for (const raw of ['', 'i.postimg', 'htt', 'http://image.tmdb.org/a.jpg']) {
      const { result } = renderHook(() => useShownAddress(raw))
      expect(result.current).toBe('')
    }
  })
})
