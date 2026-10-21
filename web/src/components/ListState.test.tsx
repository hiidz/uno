// @vitest-environment jsdom
import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { ListState } from './ListState'

describe('ListState', () => {
  it('draws a list’s skeleton while it loads, hidden from screen readers, which hear the loading words', () => {
    render(
      <ListState isLoading loadingLabel="Loading catalogs…" skeleton={<span>bones</span>}>
        <span>rows</span>
      </ListState>,
    )
    expect(screen.getByText('Loading catalogs…')).toHaveClass('sr-only')
    expect(screen.getByText('bones').parentElement).toHaveAttribute('aria-hidden', 'true')
    expect(screen.queryByText('rows')).not.toBeInTheDocument()
  })

  it('says the loading words where a list has no skeleton', () => {
    render(
      <ListState isLoading loadingLabel="Loading…">
        <span>rows</span>
      </ListState>,
    )
    expect(screen.getByText('Loading…')).not.toHaveClass('sr-only')
  })
})
