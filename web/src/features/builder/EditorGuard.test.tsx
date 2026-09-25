// @vitest-environment jsdom
import { act, renderHook } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { EditorGuardProvider, useEditorGuard } from './EditorGuard'

function renderGuard() {
  return renderHook(() => useEditorGuard(), {
    wrapper: EditorGuardProvider,
    // StrictMode double-invokes state updaters, which is how a held action
    // would run twice if `proceed` ran it from one.
    reactStrictMode: true,
  })
}

describe('EditorGuard', () => {
  it('runs an action at once when there is nothing to lose', () => {
    const { result } = renderGuard()
    const run = vi.fn()
    act(() => result.current.guard(run))
    expect(run).toHaveBeenCalledTimes(1)
    expect(result.current.blocked).toBe(false)
  })

  it('holds an action over unsaved changes, and drops it on cancel', () => {
    const { result } = renderGuard()
    const run = vi.fn()
    act(() => result.current.setDirty(true))
    act(() => result.current.guard(run))
    expect(run).not.toHaveBeenCalled()
    expect(result.current.blocked).toBe(true)

    act(() => result.current.cancel())
    expect(result.current.blocked).toBe(false)
    act(() => result.current.proceed())
    expect(run).not.toHaveBeenCalled()
  })

  it('runs the held action exactly once on proceed', () => {
    const { result } = renderGuard()
    const run = vi.fn()
    act(() => result.current.setDirty(true))
    act(() => result.current.guard(run))

    act(() => result.current.proceed())
    expect(run).toHaveBeenCalledTimes(1)
    expect(result.current.blocked).toBe(false)
  })
})
