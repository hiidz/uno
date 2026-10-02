// @vitest-environment jsdom
import type { ComponentProps } from 'react'
import { fireEvent, render, screen, within } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { catalog } from '@/test/fixtures'
import { PublishDialog } from './PublishDialog'

const genres = { movie: new Map([[27, 'Horror']]), tv: new Map() }
const horror = catalog({ id: 'h', name: 'Horror nights', params: '{"with_genres":"27"}' })

function renderDialog(props: Partial<ComponentProps<typeof PublishDialog>> = {}) {
  const handlers = { onConfirm: vi.fn(), onClose: vi.fn(), onUnpublish: vi.fn() }
  render(
    <PublishDialog
      open
      subject={{ kind: 'catalog', name: 'Horror nights', catalog: horror }}
      update={false}
      since={null}
      genres={genres}
      pending={false}
      error={null}
      {...handlers}
      {...props}
    />,
  )
  return handlers
}

describe('PublishDialog', () => {
  it('asks before publishing a catalog, showing its recipe', () => {
    const { onConfirm, onClose } = renderDialog()
    const dialog = screen.getByRole('dialog')
    expect(within(dialog).getByRole('heading', { name: 'Publish “Horror nights”?' })).toBeInTheDocument()
    expect(within(dialog).getByText('Movies')).toBeInTheDocument()
    expect(within(dialog).getByText('Horror')).toBeInTheDocument()
    fireEvent.click(within(dialog).getByRole('button', { name: 'Publish' }))
    expect(onConfirm).toHaveBeenCalled()
    fireEvent.click(within(dialog).getByRole('button', { name: 'Cancel' }))
    expect(onClose).toHaveBeenCalled()
  })

  it('shows what the changes are above what the publication holds, when it publishes changes', () => {
    renderDialog({ update: true, since: <p>Since you last published: a list</p> })
    const since = screen.getByText('Since you last published: a list')
    const recipe = screen.getByText('Horror')
    expect(since.compareDocumentPosition(recipe) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  })

  it('offers Unpublish for a published row', () => {
    const { onUnpublish } = renderDialog({ update: true })
    fireEvent.click(screen.getByRole('button', { name: 'Unpublish' }))
    expect(onUnpublish).toHaveBeenCalled()
  })

  it('has no Unpublish for a row not yet published', () => {
    renderDialog()
    expect(screen.queryByRole('button', { name: 'Unpublish' })).toBeNull()
  })

  it('lists a collection’s own catalogs and its library catalogs apart', () => {
    renderDialog({
      subject: {
        kind: 'collection',
        name: 'Night',
        folderCount: 2,
        own: [catalog({ id: 'o', name: 'Own one' })],
        library: [catalog({ id: 'l', name: 'Library one' })],
      },
    })
    expect(screen.getByText('2 folders, 1 catalog of its own')).toBeInTheDocument()
    expect(screen.getByText('From your library, published as they are now')).toBeInTheDocument()
    expect(screen.getByText('Library one')).toBeInTheDocument()
  })

  it('leaves out the library heading when a collection uses none', () => {
    renderDialog({ subject: { kind: 'collection', name: 'Night', folderCount: 1, own: [], library: [] } })
    expect(screen.queryByText('From your library, published as they are now')).toBeNull()
  })

  it('publishes an update, and says so while it runs', () => {
    renderDialog({ update: true, pending: true, error: 'TMDB is unreachable' })
    expect(screen.getByRole('heading', { name: 'Publish your changes to “Horror nights”?' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Publishing…' })).toBeDisabled()
    expect(screen.getByRole('alert')).toHaveTextContent('TMDB is unreachable')
  })

  it('says Publishing… while a first publish runs, and draws nothing without a subject', () => {
    const { unmount } = render(
      <PublishDialog open subject={null} update={false} since={null} genres={genres} pending={false} error={null} onConfirm={vi.fn()} onClose={vi.fn()} onUnpublish={vi.fn()} />,
    )
    expect(screen.queryByRole('dialog')).toBeNull()
    unmount()
    renderDialog({ pending: true })
    expect(screen.getByRole('button', { name: 'Publishing…' })).toBeDisabled()
  })
})
