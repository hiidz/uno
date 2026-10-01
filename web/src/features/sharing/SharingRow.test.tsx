// @vitest-environment jsdom
import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import type { PublicationState } from '@/api'
import { SharingRow } from './SharingRow'

const live: PublicationState = { id: 'p', status: 'live', changed_since_publish: false }

function renderRow(publication: PublicationState | null, { dirty = false } = {}) {
  const handlers = { onPublish: vi.fn(), onUnpublish: vi.fn() }
  render(<SharingRow publication={publication} dirty={dirty} {...handlers} />)
  return handlers
}

const button = (name: string) => screen.queryByRole('button', { name })

describe('SharingRow', () => {
  it('offers Publish… on a private row, under the Community label', () => {
    const { onPublish } = renderRow(null)
    expect(screen.getByText('Community')).toBeInTheDocument()
    expect(screen.getByText('Private')).toBeInTheDocument()
    fireEvent.click(button('Publish…')!)
    expect(onPublish).toHaveBeenCalled()
    expect(button('Unpublish')).toBeNull()
  })

  it('offers only Unpublish on a row published as it is', () => {
    const { onUnpublish } = renderRow(live)
    expect(screen.getByText('Published')).toBeInTheDocument()
    expect(button('Publish…')).toBeNull()
    fireEvent.click(button('Unpublish')!)
    expect(onUnpublish).toHaveBeenCalled()
  })

  it('offers Publish update… once the saved row changed since', () => {
    renderRow({ ...live, changed_since_publish: true })
    expect(screen.getByText('Published')).toBeInTheDocument()
    expect(button('Publish update…')).toBeEnabled()
    expect(button('Unpublish')).toBeInTheDocument()
  })

  it('offers Publish again… once unpublished', () => {
    renderRow({ ...live, status: 'unpublished' })
    expect(screen.getByText('Unpublished. People who added it keep it.')).toBeInTheDocument()
    expect(button('Publish again…')).toBeEnabled()
  })

  it('waits for a save before publishing what is saved', () => {
    renderRow(null, { dirty: true })
    expect(button('Publish…')).toBeDisabled()
    expect(screen.getByText('Save first: only what’s saved is published.')).toBeInTheDocument()
  })

  it('says nothing about saving while a row is published as it is', () => {
    renderRow(live, { dirty: true })
    expect(screen.queryByText('Save first: only what’s saved is published.')).toBeNull()
  })
})
