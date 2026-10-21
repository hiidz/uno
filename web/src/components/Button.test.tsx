// @vitest-environment jsdom
import { createRef } from 'react'
import { render, screen } from '@testing-library/react'
import { Popover } from 'radix-ui'
import { describe, expect, it } from 'vitest'
import { Button } from './Button'

describe('Button', () => {
  it('draws each kind with the classes index.css defines for it', () => {
    render(
      <>
        <Button kind="primary">Save</Button>
        <Button kind="accent-outline">Update…</Button>
        <Button kind="sign">New</Button>
      </>,
    )
    expect(screen.getByRole('button', { name: 'Save' })).toHaveClass('btn-primary')
    expect(screen.getByRole('button', { name: 'Update…' })).toHaveClass('btn-secondary', 'btn-accent-outline')
    expect(screen.getByRole('button', { name: 'New' })).toHaveClass('sign-btn')
  })

  it('adds the small size to a text button, never to a sign’s', () => {
    render(
      <>
        <Button kind="secondary" size="sm">Retry</Button>
        <Button kind="sign" size="sm">New</Button>
      </>,
    )
    expect(screen.getByRole('button', { name: 'Retry' })).toHaveClass('btn-secondary', 'btn-sm')
    expect(screen.getByRole('button', { name: 'New' })).not.toHaveClass('btn-sm')
  })

  it('is a plain button unless told otherwise, and keeps a caller’s class', () => {
    render(
      <>
        <Button kind="ghost" className="ml-auto">Cancel</Button>
        <Button kind="primary" type="submit">Sign in</Button>
      </>,
    )
    expect(screen.getByRole('button', { name: 'Cancel' })).toHaveAttribute('type', 'button')
    expect(screen.getByRole('button', { name: 'Cancel' })).toHaveClass('btn-ghost', 'ml-auto')
    expect(screen.getByRole('button', { name: 'Sign in' })).toHaveAttribute('type', 'submit')
  })

  it('passes its ref and aria-disabled through, staying pressable while greyed', () => {
    const ref = createRef<HTMLButtonElement>()
    render(
      <Button ref={ref} kind="primary" aria-disabled="true">
        Save
      </Button>,
    )
    const button = screen.getByRole('button', { name: 'Save' })
    expect(ref.current).toBe(button)
    expect(button).toHaveAttribute('aria-disabled', 'true')
    expect(button).not.toBeDisabled()
    expect(button).toHaveAttribute('data-slot', 'button')
  })

  it('renders as a Radix trigger through asChild', () => {
    render(
      <Popover.Root>
        <Popover.Trigger asChild>
          <Button kind="secondary">Pick</Button>
        </Popover.Trigger>
      </Popover.Root>,
    )
    const button = screen.getByRole('button', { name: 'Pick' })
    expect(button).toHaveClass('btn-secondary')
    expect(button).toHaveAttribute('aria-expanded', 'false')
  })
})
