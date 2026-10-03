// @vitest-environment jsdom
import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { BlockablePushButton, PushBlockNote } from './PushBlockControls'
import type { Push } from './usePush'

function push(overrides: Partial<Push> = {}): Push {
  return { push: vi.fn(), ready: true, pushing: false, outcome: null, dismiss: vi.fn(), ...overrides }
}

describe('BlockablePushButton', () => {
  it('is on when nothing blocks the push', () => {
    render(<BlockablePushButton push={push()} block={null} />)
    expect(screen.getByRole('button', { name: /Push/ })).toBeEnabled()
  })

  it('is off while something blocks the push', () => {
    render(<BlockablePushButton push={push()} block={{ kind: 'shares-addons' }} />)
    expect(screen.getByRole('button', { name: /Push/ })).toBeDisabled()
  })
})

describe('PushBlockNote', () => {
  it('says nothing when nothing blocks the push', () => {
    const { container } = render(<PushBlockNote block={null} />)
    expect(container).toBeEmptyDOMElement()
  })

  it('names the empty collection and what to do about it', () => {
    render(<PushBlockNote block={{ kind: 'empty-collection', title: 'Weekend' }} />)
    expect(screen.getByRole('status')).toHaveTextContent('Push is off: “Weekend” has no folders.')
    expect(screen.getByRole('status')).toHaveTextContent('Add a folder to it, or take it off Home.')
  })

  it('says a profile using profile 1’s addons can’t be pushed to, and the way out', () => {
    render(<PushBlockNote block={{ kind: 'shares-addons' }} />)
    expect(screen.getByRole('status')).toHaveTextContent("this profile uses profile 1's addons in Nuvio")
    expect(screen.getByRole('status')).toHaveTextContent('Turn sharing off for this profile in Nuvio')
  })
})
