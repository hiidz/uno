// @vitest-environment jsdom
import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { LibraryItem } from './LibraryItem'

function mount() {
  const props = {
    kind: 'movie' as const,
    name: 'Horror',
    summary: 'Horror movies',
    selected: false,
    onHome: false,
    onSelect: vi.fn(),
    onToggle: vi.fn(),
    onDuplicate: vi.fn(),
    onDelete: vi.fn(),
  }
  render(<LibraryItem {...props} />)
  return props
}

describe('LibraryItem', () => {
  it('keeps the home sticker beside the row’s button, not inside it', () => {
    const props = mount()
    const row = screen.getByRole('button', { name: /^Horror/ })
    const sticker = screen.getByRole('button', { name: 'Put Horror on your home screen' })
    expect(row).not.toContainElement(sticker)

    fireEvent.click(sticker)
    expect(props.onToggle).toHaveBeenCalledOnce()
    expect(props.onSelect).not.toHaveBeenCalled()

    fireEvent.click(row)
    expect(props.onSelect).toHaveBeenCalledOnce()
  })
})
