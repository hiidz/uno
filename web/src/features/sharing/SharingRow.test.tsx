// @vitest-environment jsdom
import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import type { PublicationState } from '@/api'
import { SharingRow } from './SharingRow'

const live: PublicationState = { id: 'p', status: 'live', changed_since_publish: false }

function renderRow(publication: PublicationState | null, { dirty = false, blocked = null as string | null } = {}) {
  const handlers = { onShare: vi.fn(), onStop: vi.fn() }
  render(<SharingRow publication={publication} dirty={dirty} blocked={blocked} {...handlers} />)
  return handlers
}

const button = (name: string) => screen.queryByRole('button', { name })

describe('SharingRow', () => {
  it('offers Share… on a private row', () => {
    const { onShare } = renderRow(null)
    expect(screen.getByText('Private')).toBeInTheDocument()
    fireEvent.click(button('Share…')!)
    expect(onShare).toHaveBeenCalled()
    expect(button('Stop sharing')).toBeNull()
  })

  it('offers only Stop sharing on a row shared as it is', () => {
    const { onStop } = renderRow(live)
    expect(screen.getByText('Shared')).toBeInTheDocument()
    expect(button('Share…')).toBeNull()
    fireEvent.click(button('Stop sharing')!)
    expect(onStop).toHaveBeenCalled()
  })

  it('offers Publish update… once the saved row changed since', () => {
    renderRow({ ...live, changed_since_publish: true })
    expect(screen.getByText('Shared. Your changes since aren’t shared yet.')).toBeInTheDocument()
    expect(button('Publish update…')).toBeEnabled()
    expect(button('Stop sharing')).toBeInTheDocument()
  })

  it('offers Share again… once sharing stopped', () => {
    renderRow({ ...live, status: 'withdrawn' })
    expect(screen.getByText('Not shared any more. Copies people took stay theirs.')).toBeInTheDocument()
    expect(button('Share again…')).toBeEnabled()
  })

  it('waits for a save before sharing what is saved', () => {
    renderRow(null, { dirty: true })
    expect(button('Share…')).toBeDisabled()
    expect(screen.getByText('Save first: sharing shares what’s saved.')).toBeInTheDocument()
  })

  it('says nothing about saving while a row is shared as it is', () => {
    renderRow(live, { dirty: true })
    expect(screen.queryByText('Save first: sharing shares what’s saved.')).toBeNull()
  })

  it('says why a row can’t be shared at all', () => {
    renderRow(null, { dirty: true, blocked: 'Uses 1 catalog taken from Community.' })
    expect(button('Share…')).toBeDisabled()
    expect(screen.getByText('Uses 1 catalog taken from Community.')).toBeInTheDocument()
    expect(screen.queryByText('Save first: sharing shares what’s saved.')).toBeNull()
  })
})
