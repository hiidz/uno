// @vitest-environment jsdom
import type { ComponentProps } from 'react'
import { fireEvent, render, screen, within } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { catalog } from '@/test/fixtures'
import { PublishDialog } from './PublishDialog'

const genres = { movie: new Map([[27, 'Horror']]), tv: new Map() }
const horror = catalog({ id: 'h', name: 'Horror nights', params: '{"with_genres":"27"}' })

function renderDialog(props: Partial<ComponentProps<typeof PublishDialog>> = {}) {
  const handlers = { onConfirm: vi.fn(), onClose: vi.fn() }
  render(
    <PublishDialog
      open
      subject={{ kind: 'catalog', name: 'Horror nights', catalog: horror }}
      update={false}
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
  it('asks before sharing a catalog, showing its recipe', () => {
    const { onConfirm, onClose } = renderDialog()
    const dialog = screen.getByRole('dialog')
    expect(within(dialog).getByRole('heading', { name: 'Share “Horror nights”?' })).toBeInTheDocument()
    expect(within(dialog).getByText('Movies')).toBeInTheDocument()
    expect(within(dialog).getByText('Horror')).toBeInTheDocument()
    fireEvent.click(within(dialog).getByRole('button', { name: 'Share' }))
    expect(onConfirm).toHaveBeenCalled()
    fireEvent.click(within(dialog).getByRole('button', { name: 'Cancel' }))
    expect(onClose).toHaveBeenCalled()
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
    expect(screen.getByText('From your library, shared as they are now')).toBeInTheDocument()
    expect(screen.getByText('Library one')).toBeInTheDocument()
  })

  it('leaves out the library heading when a collection uses none', () => {
    renderDialog({ subject: { kind: 'collection', name: 'Night', folderCount: 1, own: [], library: [] } })
    expect(screen.queryByText('From your library, shared as they are now')).toBeNull()
  })

  it('publishes an update, and says so while it runs', () => {
    renderDialog({ update: true, pending: true, error: 'TMDB is unreachable' })
    expect(screen.getByRole('heading', { name: 'Publish your changes to “Horror nights”?' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Publishing…' })).toBeDisabled()
    expect(screen.getByRole('alert')).toHaveTextContent('TMDB is unreachable')
  })

  it('says Sharing… while a first share runs, and draws nothing without a subject', () => {
    const { unmount } = render(
      <PublishDialog open subject={null} update={false} genres={genres} pending={false} error={null} onConfirm={vi.fn()} onClose={vi.fn()} />,
    )
    expect(screen.queryByRole('dialog')).toBeNull()
    unmount()
    renderDialog({ pending: true })
    expect(screen.getByRole('button', { name: 'Sharing…' })).toBeDisabled()
  })
})
