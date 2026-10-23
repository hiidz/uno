// @vitest-environment jsdom
import type { ComponentProps } from 'react'
import { fireEvent, render, screen, within } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { catalog } from '@/test/fixtures'
import { PublishDialog } from './PublishDialog'
import { asksWithheldMedia } from './sharingState'
import { WithheldCard } from './WithheldPictures'

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
      pictures={null}
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
    expect(screen.getByText('From your library, public as part of this collection')).toBeInTheDocument()
    expect(screen.getByText('Library one')).toBeInTheDocument()
  })

  it('says a collection’s library catalogs are readable with it but not listed on their own', () => {
    renderDialog({
      subject: {
        kind: 'collection',
        name: 'Night',
        folderCount: 1,
        own: [],
        library: [catalog({ id: 'l', name: 'Library one' })],
      },
    })
    expect(screen.getByText(/Anyone who opens this collection can read these catalogs/)).toHaveTextContent(
      'They aren’t listed in Community on their own.',
    )
  })

  it('leaves out the library heading when a collection uses none', () => {
    renderDialog({ subject: { kind: 'collection', name: 'Night', folderCount: 1, own: [], library: [] } })
    expect(screen.queryByText('From your library, public as part of this collection')).toBeNull()
    expect(screen.queryByText(/aren’t listed in Community on their own/)).toBeNull()
  })

  it('publishes an update, and says so while it runs', () => {
    renderDialog({ update: true, pending: true, error: 'TMDB is unreachable' })
    expect(screen.getByRole('heading', { name: 'Publish your changes to “Horror nights”?' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Publishing…' })).toBeDisabled()
    expect(screen.getByRole('alert')).toHaveTextContent('TMDB is unreachable')
  })

  it('says Publishing… while a first publish runs, and draws nothing without a subject', () => {
    const { unmount } = render(
      <PublishDialog open subject={null} update={false} since={null} pictures={null} genres={genres} pending={false} error={null} onConfirm={vi.fn()} onClose={vi.fn()} onUnpublish={vi.fn()} />,
    )
    expect(screen.queryByRole('dialog')).toBeNull()
    unmount()
    renderDialog({ pending: true })
    expect(screen.getByRole('button', { name: 'Publishing…' })).toBeDisabled()
  })

  const media = {
    pictures: [
      { title: 'Studios', field: 'backdrop_image_url', url: 'https://i.postimg.cc/b.jpg', site: 'i.postimg.cc' },
      { title: 'Marvel', field: 'hero_video_url', url: 'https://i.imgur.com/v.mp4', site: 'i.imgur.com' },
    ],
    sites: ['flagcdn.com', 'image.tmdb.org'],
  } as const satisfies Parameters<typeof WithheldCard>[0]['media']

  it('lists the pictures Community leaves out under the opening line, each with its field and site', () => {
    renderDialog({ pictures: <WithheldCard media={{ ...media, pictures: [...media.pictures] }} /> })
    const card = screen.getByRole('region', { name: 'Pictures Community leaves out' })
    expect(within(card).getByRole('heading', { name: '2 pictures won’t show in Community' })).toBeInTheDocument()
    const rows = within(card).getAllByRole('listitem')
    expect(rows.map((row) => row.textContent)).toEqual([
      'StudiosBackground image · i.postimg.cc',
      'MarvelHero video · i.imgur.com',
    ])
    expect(rows[0].querySelector('img')).toHaveAttribute('src', 'https://i.postimg.cc/b.jpg')
    expect(rows[1].querySelector('img')).toBeNull()
    const opening = screen.getByText(/Anyone on Uno can find it/)
    expect(opening.compareDocumentPosition(card) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  })

  it('opens the server’s sites from Which sites?', () => {
    render(<WithheldCard media={{ ...media, pictures: [...media.pictures], sites: [...media.sites] }} />)
    const which = screen.getByRole('button', { name: 'Which sites?' })
    expect(screen.queryByText('flagcdn.com · image.tmdb.org')).toBeNull()
    fireEvent.click(which)
    expect(which).toHaveAttribute('aria-expanded', 'true')
    expect(screen.getByText('flagcdn.com · image.tmdb.org')).toBeInTheDocument()
  })

  it('shows no card when nothing is left out', () => {
    render(<WithheldCard media={{ pictures: [], sites: ['image.tmdb.org'] }} />)
    expect(screen.queryByRole('region')).toBeNull()
  })

  it('asks which pictures are left out only for a collection on a trusted-images server', () => {
    expect(asksWithheldMedia('collection', 'trusted-images')).toBe(true)
    expect(asksWithheldMedia('catalog', 'trusted-images')).toBe(false)
    for (const setting of ['on', 'no-images', 'off'] as const) expect(asksWithheldMedia('collection', setting)).toBe(false)
  })
})
