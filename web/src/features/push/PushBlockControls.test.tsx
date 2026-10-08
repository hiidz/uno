// @vitest-environment jsdom
import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { PushBlockNote } from './PushBlockControls'

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
