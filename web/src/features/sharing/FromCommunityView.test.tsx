// @vitest-environment jsdom
import type { ComponentProps } from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, fireEvent, render, screen, within } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { Catalog, SubscriptionState } from '@/api'
import { fakeApi } from '@/test/fakeApi'
import { catalog, collection, communityItem, folder } from '@/test/fixtures'
import { CatalogFromCommunity, CollectionFromCommunity } from './FromCommunityView'

// The live results and the Preview panel are fetches these tests are not
// about: every request stays pending, but the ones a test routes.
const api = vi.hoisted(() => ({ current: null as null | ((input: RequestInfo | URL, init?: RequestInit) => Promise<Response>) }))
vi.mock('@/api/client', () => ({
  apiFetch: (input: RequestInfo | URL, init?: RequestInit) => api.current?.(input, init) ?? new Promise(() => {}),
}))

const genres = {
  movie: new Map([
    [80, 'Crime'],
    [53, 'Thriller'],
  ]),
  tv: new Map<number, string>(),
}
const following: SubscriptionState = { publication_id: 'pub', update_available: false }

const noir = catalog({
  id: 'n1',
  name: 'Noir after midnight',
  params: JSON.stringify({ with_genres: '80,53', vote_average_gte: 7, sort_by: 'popularity.desc' }),
  subscription: following,
})

afterEach(() => {
  api.current = null
})

function wrap(ui: React.ReactElement) {
  return render(<QueryClientProvider client={new QueryClient()}>{ui}</QueryClientProvider>)
}

function renderCatalog(props: Partial<ComponentProps<typeof CatalogFromCommunity>> = {}) {
  const handlers = { profileIndex: 1, waitingForPush: false, onClose: vi.fn(), onDuplicate: vi.fn(), onDelete: vi.fn(), onUpdate: vi.fn() }
  wrap(<CatalogFromCommunity catalog={noir} genres={genres} {...handlers} {...props} />)
  return handlers
}

const FORM_CONTROLS = ['textbox', 'combobox', 'checkbox', 'spinbutton', 'radio', 'switch', 'slider'] as const

function expectNoForm() {
  for (const role of FORM_CONTROLS) expect(screen.queryAllByRole(role)).toEqual([])
  expect(screen.queryByRole('button', { name: /^Save/ })).toBeNull()
}

describe('CatalogFromCommunity', () => {
  it('shows the recipe as spec tiles, the filters it leaves open as Any', () => {
    renderCatalog()
    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent('Noir after midnight')
    const genresTile = screen.getByText('Genres', { selector: 'dt' }).closest('div')!
    expect(within(genresTile).getByText('Crime and Thriller')).toBeInTheDocument()
    for (const label of ['Type', 'Genres', 'Rating', 'Order']) {
      expect(screen.getByText(label, { selector: 'dt' })).toBeInTheDocument()
    }
    expect(screen.getByText('Movies', { selector: 'dd' })).toBeInTheDocument()
    expect(screen.getByText('7.0 or more', { selector: 'dd' })).toBeInTheDocument()
    expect(screen.getByText('Votes', { selector: 'dt' }).nextElementSibling).toHaveTextContent('Any')
  })

  it('has no form: no inputs, no Save, no sentence or note about what it is', () => {
    renderCatalog()
    expectNoForm()
    expect(screen.queryByText(/follows its|gets its publisher/i)).toBeNull()
  })

  it('says where it came from with the From Community sticker', () => {
    renderCatalog()
    expect(screen.getAllByText('From Community').length).toBeGreaterThan(0)
  })

  it('flags To push while a push would change what Nuvio holds for it', () => {
    renderCatalog()
    expect(screen.queryByText('To push')).toBeNull()
    cleanup()
    renderCatalog({ waitingForPush: true })
    expect(screen.getAllByText('To push').length).toBeGreaterThan(0)
  })

  it('duplicates to edit, in the region’s colour while no update waits', () => {
    const { onDuplicate } = renderCatalog()
    const button = screen.getByRole('button', { name: 'Duplicate to edit' })
    expect(button).toHaveClass('btn-primary')
    fireEvent.click(button)
    expect(onDuplicate).toHaveBeenCalledTimes(1)
    expect(screen.queryByRole('button', { name: 'Update…' })).toBeNull()
  })

  it('offers Update… first while an update waits, and outlines Duplicate to edit', () => {
    const { onUpdate } = renderCatalog({
      catalog: { ...noir, subscription: { ...following, update_available: true } },
    })
    const update = screen.getByRole('button', { name: 'Update…' })
    expect(update).toHaveClass('btn-primary')
    expect(screen.getByRole('button', { name: 'Duplicate to edit' })).toHaveClass('btn-secondary')
    expect(screen.queryByText('Update available', { selector: '.stk' })).toBeNull()
    expect(screen.getAllByText('From Community').length).toBeGreaterThan(0)
    fireEvent.click(update)
    expect(onUpdate).toHaveBeenCalledTimes(1)
  })

  it('counts what the update changes beside Update…, and no list', async () => {
    const fake = fakeApi({
      'GET /api/p/1/community/pub/changes': [
        { op: 'removed', kind: 'folder', name: '80s' },
        { op: 'added', kind: 'catalog', name: 'Heat', folder: 'Classics' },
        { op: 'added', kind: 'catalog', name: 'Ronin', folder: 'Classics' },
      ],
    })
    api.current = fake.apiFetch
    renderCatalog({ catalog: { ...noir, subscription: { ...following, update_available: true } } })
    const count = await screen.findByText('3 changes')
    expect(count.previousElementSibling).toBe(screen.getByRole('button', { name: 'Update…' }))
    expect(screen.queryByText('Folder “80s”')).toBeNull()
    expect(fake.calls).toContain('GET /api/p/1/community/pub/changes')
  })

  it('leaves who added it and when it changed to Community', () => {
    const fake = fakeApi({
      'GET /api/p/1/community': [communityItem({ id: 'pub', subscriber_count: 3, updated_at: '2026-09-22T10:00:00Z' })],
    })
    api.current = fake.apiFetch
    renderCatalog()
    expect(screen.queryByText(/Added by|ublished|pdated/)).toBeNull()
    expect(fake.calls).not.toContain('GET /api/p/1/community')
  })

  it('asks for no summary while no update waits', () => {
    const fake = fakeApi({ 'GET /api/p/1/community/pub/changes': [] })
    api.current = fake.apiFetch
    renderCatalog()
    expect(fake.calls.some((call) => call.endsWith('/changes'))).toBe(false)
  })

  it('closes from the footer and with Escape, without asking', () => {
    const { onClose } = renderCatalog()
    fireEvent.click(screen.getByRole('button', { name: 'Close' }))
    expect(onClose).toHaveBeenCalledTimes(1)
    fireEvent.keyDown(document, { key: 'Escape' })
    expect(onClose).toHaveBeenCalledTimes(2)
  })

  it('carries the row’s own Delete in the phone header, and says why it is held', () => {
    const { onDelete } = renderCatalog()
    fireEvent.click(screen.getByRole('button', { name: 'Delete Noir after midnight' }))
    expect(onDelete).toHaveBeenCalledTimes(1)
  })
})

const night: Catalog[] = [
  catalog({ id: 'a', name: 'Slasher classics', params: JSON.stringify({ with_genres: '80', sort_by: 'vote_average.desc' }), collection_id: 'col9' }),
  catalog({ id: 'b', name: 'Quiet horror', type: 'series', params: JSON.stringify({ with_networks: '1|2' }), collection_id: 'col9' }),
]
const nightCollection = collection({
  id: 'col9',
  title: 'Horror Nights',
  subscription: following,
  catalogs: night,
  folders: [
    folder({ id: 'f1', title: 'Slashers', cover_emoji: '🔪', refs: [{ catalog_id: 'a', genre: 'Slasher' }, { catalog_id: 'b', genre: '' }] }),
    folder({ id: 'f2', title: 'Later', refs: [{ catalog_id: 'a', genre: '' }] }),
  ],
})

function renderCollection(props: Partial<ComponentProps<typeof CollectionFromCommunity>> = {}) {
  const handlers = { profileIndex: 1, waitingForPush: false, onClose: vi.fn(), onDuplicate: vi.fn(), onDelete: vi.fn(), onUpdate: vi.fn() }
  wrap(<CollectionFromCommunity collection={nightCollection} genres={genres} {...handlers} {...props} />)
  return handlers
}

describe('CollectionFromCommunity', () => {
  it('draws a card for each folder, its catalogs collapsed to a name over its recipe line', () => {
    renderCollection()
    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent('Horror Nights')
    const cards = document.querySelectorAll<HTMLElement>('.fold-detail')
    expect(cards).toHaveLength(2)
    expect(cards[0]).toHaveTextContent('🔪Slashers')
    expect(cards[1]).toHaveTextContent('Later')
    const slasher = screen.getAllByRole('button', { name: /Slasher classics/ })[0]
    expect(slasher).toHaveAttribute('aria-expanded', 'false')
    expect(slasher).toHaveTextContent('Highest rated · Crime • Slasher')
    expect(screen.queryByText('Type', { selector: 'dt' })).toBeNull()
  })

  it('opens a catalog in place to its spec tiles, and closes it again', () => {
    renderCollection()
    const quiet = screen.getByRole('button', { name: /Quiet horror/ })
    fireEvent.click(quiet)
    expect(quiet).toHaveAttribute('aria-expanded', 'true')
    expect(screen.getByText('Series', { selector: 'dd' })).toBeInTheDocument()
    expect(screen.getByText('Networks', { selector: 'dt' })).toBeInTheDocument()
    expect(quiet).not.toHaveTextContent('Series · ')
    fireEvent.click(quiet)
    expect(quiet).toHaveAttribute('aria-expanded', 'false')
    expect(screen.queryByText('Series', { selector: 'dd' })).toBeNull()
  })

  it('shows the genre a folder narrows a catalog to among its tiles', () => {
    renderCollection()
    fireEvent.click(screen.getAllByRole('button', { name: /Slasher classics/ })[0])
    expect(screen.getByText('Narrowed to', { selector: 'dt' })).toBeInTheDocument()
    expect(screen.getByText('Slasher', { selector: 'dd' })).toBeInTheDocument()
  })

  it('has no form, and never opens an editor for a catalog inside it', () => {
    renderCollection()
    fireEvent.click(screen.getByRole('button', { name: /Quiet horror/ }))
    expectNoForm()
    expect(screen.queryByRole('dialog')).toBeNull()
  })

  it('duplicates to edit, with Update… first while an update waits', () => {
    const { onDuplicate, onUpdate } = renderCollection({
      collection: { ...nightCollection, subscription: { ...following, update_available: true } },
    })
    expect(screen.getByRole('button', { name: 'Duplicate to edit' })).toHaveClass('btn-secondary')
    fireEvent.click(screen.getByRole('button', { name: 'Update…' }))
    expect(onUpdate).toHaveBeenCalledTimes(1)
    fireEvent.click(screen.getByRole('button', { name: 'Duplicate to edit' }))
    expect(onDuplicate).toHaveBeenCalledTimes(1)
  })

  it('counts what the update changes beside Update… for a collection too', async () => {
    const fake = fakeApi({ 'GET /api/p/1/community/pub/changes': [{ op: 'changed', kind: 'collection', aspect: 'order' }] })
    api.current = fake.apiFetch
    renderCollection({ collection: { ...nightCollection, subscription: { ...following, update_available: true } } })
    expect(await screen.findByText('1 change')).toBeInTheDocument()
  })

  it('counts each folder’s catalogs beside its name', () => {
    renderCollection()
    const cards = document.querySelectorAll<HTMLElement>('.fold-detail')
    expect(cards[0]).toHaveTextContent('2 catalogs')
    expect(cards[1]).toHaveTextContent('1 catalog')
  })
})
