// @vitest-environment jsdom
import { render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'
import { StartupShell } from './LoadingShells'

afterEach(() => window.history.replaceState(null, '', '/'))

describe('StartupShell', () => {
  it('draws the builder’s header and an empty main on a builder address', () => {
    window.history.replaceState(null, '', '/p/2/community')
    render(<StartupShell />)
    expect(screen.getByRole('banner')).toBeInTheDocument()
    expect(screen.getByRole('main')).toHaveAttribute('aria-busy', 'true')
  })

  it('draws only the entry pages’ fascia anywhere else', () => {
    window.history.replaceState(null, '', '/profiles')
    render(<StartupShell />)
    expect(screen.queryByRole('banner')).not.toBeInTheDocument()
    expect(screen.queryByRole('main')).not.toBeInTheDocument()
  })
})
