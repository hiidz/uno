// @vitest-environment jsdom
import { act, renderHook } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { useEditorForm } from './useEditorForm'

interface Form {
  name: string
}

const isSame = (a: Form, b: Form) => a.name === b.name

function renderForm(baseline: Form) {
  const onDirtyChange = vi.fn()
  const rendered = renderHook(({ baseline }) => useEditorForm(baseline, isSame, onDirtyChange), {
    initialProps: { baseline },
  })
  return { ...rendered, onDirtyChange }
}

describe('useEditorForm', () => {
  it('reports dirtiness as the form moves away from its baseline and back', () => {
    const { result, onDirtyChange } = renderForm({ name: 'Row' })
    expect(onDirtyChange).toHaveBeenLastCalledWith(false)

    act(() => result.current.setState({ name: 'Renamed' }))
    expect(onDirtyChange).toHaveBeenLastCalledWith(true)

    act(() => result.current.setState({ name: 'Row' }))
    expect(onDirtyChange).toHaveBeenLastCalledWith(false)
  })

  it('keeps edits across renders with the same baseline', () => {
    const baseline = { name: 'Row' }
    const { result, rerender } = renderForm(baseline)
    act(() => result.current.setState({ name: 'Renamed' }))
    rerender({ baseline })
    expect(result.current.state).toEqual({ name: 'Renamed' })
  })

  it('reveals errors instead of saving while there are any, and saves the state once there are none', () => {
    const { result } = renderForm({ name: 'Row' })
    const save = vi.fn()

    act(() => result.current.submit(2, save))
    expect(result.current.showErrors).toBe(true)
    expect(save).not.toHaveBeenCalled()

    act(() => result.current.setState({ name: 'Fixed' }))
    act(() => result.current.submit(0, save))
    expect(save).toHaveBeenCalledWith({ name: 'Fixed' })
  })
})
