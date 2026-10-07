// @vitest-environment jsdom
import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { DateWindow } from './DateWindow'

function renderWindow(mode: 'fixed' | 'rolling', error?: string) {
  render(
    <DateWindow
      isMovie
      mode={mode}
      gte="2021-01-01"
      lte="2020-01-01"
      days={365}
      error={error}
      onMode={vi.fn()}
      onParams={vi.fn()}
    />,
  )
}

describe('DateWindow', () => {
  it('shows a fixed range error under the two dates', () => {
    renderWindow('fixed', 'The start date is after the end date.')
    expect(screen.getByText('The start date is after the end date.')).toBeTruthy()
  })

  it('shows no error under the dates when there is none', () => {
    renderWindow('fixed')
    expect(screen.queryByText(/start date/)).toBeNull()
  })

  it('shows a window error once, beside the recent chips', () => {
    renderWindow('rolling', 'Pick how far back to look.')
    expect(screen.getAllByText('Pick how far back to look.')).toHaveLength(1)
  })
})
