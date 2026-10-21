// @vitest-environment jsdom
import { createRef } from 'react'
import { fireEvent, render, screen } from '@testing-library/react'
import { Copy } from 'lucide-react'
import { describe, expect, it, vi } from 'vitest'
import { IconButton } from './IconButton'

describe('IconButton', () => {
  it('is named by its label, and the labeled shape prints the label’s first word', () => {
    render(<IconButton label="Duplicate Noir" icon={Copy} onClick={() => {}} variant="labeled" />)
    const button = screen.getByRole('button', { name: 'Duplicate Noir' })
    expect(button).toHaveTextContent(/^Duplicate$/)
    expect(button).toHaveAttribute('title', 'Duplicate Noir')
    expect(button).toHaveAttribute('type', 'button')
  })

  it('keeps its click from reaching the row it sits in', () => {
    const onRow = vi.fn()
    const onClick = vi.fn()
    render(
      <div onClick={onRow}>
        <IconButton label="Delete Noir" icon={Copy} onClick={onClick} destructive variant="icon" />
      </div>,
    )
    fireEvent.click(screen.getByRole('button'))
    expect(onClick).toHaveBeenCalledOnce()
    expect(onRow).not.toHaveBeenCalled()
  })

  it('draws its circle at the size asked for, 36px by default', () => {
    render(
      <>
        <IconButton label="Default" icon={Copy} variant="icon" />
        <IconButton label="Small" icon={Copy} variant="icon" size="sm" />
        <IconButton label="Tiny" icon={Copy} variant="icon" size="xs" />
      </>,
    )
    expect(screen.getByRole('button', { name: 'Default' })).toHaveClass('h-9', 'w-9')
    expect(screen.getByRole('button', { name: 'Small' })).toHaveClass('h-8', 'w-8')
    expect(screen.getByRole('button', { name: 'Tiny' })).toHaveClass('h-6', 'w-6')
  })

  it('passes its ref and other props through to the button', () => {
    const ref = createRef<HTMLButtonElement>()
    render(<IconButton ref={ref} label="Open" icon={Copy} variant="icon" disabled aria-haspopup="menu" />)
    const button = screen.getByRole('button', { name: 'Open' })
    expect(ref.current).toBe(button)
    expect(button).toBeDisabled()
    expect(button).toHaveAttribute('aria-haspopup', 'menu')
    expect(button).toHaveAttribute('data-slot', 'icon-button')
  })
})
