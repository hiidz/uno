// @vitest-environment jsdom
import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { StateView } from './StateView'

describe('StateView', () => {
  it('draws a list’s skeleton while it loads, hidden from screen readers, which hear the loading words', () => {
    render(
      <StateView isLoading loadingLabel="Loading catalogs…" skeleton={<span>bones</span>}>
        <span>rows</span>
      </StateView>,
    )
    expect(screen.getByText('Loading catalogs…')).toHaveClass('sr-only')
    expect(screen.getByText('bones').parentElement).toHaveAttribute('aria-hidden', 'true')
    expect(screen.queryByText('rows')).not.toBeInTheDocument()
  })

  it('says the loading words where a list has no skeleton', () => {
    render(
      <StateView isLoading loadingLabel="Loading…">
        <span>rows</span>
      </StateView>,
    )
    expect(screen.getByText('Loading…')).not.toHaveClass('sr-only')
  })

  it('shows an error’s headline, the server’s words and a Retry', () => {
    const onRetry = vi.fn()
    render(
      <StateView isLoading={false} error={new Error('TMDB timed out')} errorLabel="Couldn't load." onRetry={onRetry}>
        <span>rows</span>
      </StateView>,
    )
    expect(screen.getByText("Couldn't load.")).toHaveClass('text-danger')
    expect(screen.getByText('TMDB timed out')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(onRetry).toHaveBeenCalledOnce()
  })

  it('shows the empty guidance in place of the children', () => {
    render(
      <StateView isLoading={false} isEmpty emptyLabel="Make a catalog to start.">
        <span>rows</span>
      </StateView>,
    )
    expect(screen.getByText('Make a catalog to start.')).toBeInTheDocument()
    expect(screen.queryByText('rows')).not.toBeInTheDocument()
  })

  it('draws the children once loaded with nothing to report', () => {
    render(
      <StateView isLoading={false}>
        <span>rows</span>
      </StateView>,
    )
    expect(screen.getByText('rows')).toBeInTheDocument()
  })

  it('sets each state as one line without padding when compact, an error as its headline alone', () => {
    const { rerender } = render(
      <StateView size="compact" isLoading loadingLabel="Loading services…">
        <span>chips</span>
      </StateView>,
    )
    expect(screen.getByText('Loading services…')).not.toHaveClass('py-2')

    rerender(
      <StateView size="compact" isLoading={false} error={new Error('500')} errorLabel="Couldn't load services.">
        <span>chips</span>
      </StateView>,
    )
    expect(screen.getByText("Couldn't load services.")).toHaveClass('text-danger')
    expect(screen.queryByText('500')).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Retry' })).not.toBeInTheDocument()
  })
})
