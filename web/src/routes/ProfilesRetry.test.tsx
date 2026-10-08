// @vitest-environment jsdom
import { describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { ProfilesRetry } from './ProfilesRetry'

describe('ProfilesRetry', () => {
  it('fetches the profiles again when the list failed', () => {
    const onRetry = vi.fn()
    render(<ProfilesRetry failed refused={false} onRetry={onRetry} />)
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(onRetry).toHaveBeenCalledOnce()
  })

  it('offers nothing when the list loaded or the account is refused', () => {
    const { container, rerender } = render(<ProfilesRetry failed={false} refused={false} onRetry={vi.fn()} />)
    expect(container).toBeEmptyDOMElement()
    rerender(<ProfilesRetry failed refused onRetry={vi.fn()} />)
    expect(container).toBeEmptyDOMElement()
  })
})
