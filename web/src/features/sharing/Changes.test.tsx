// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { SnapshotChange, SubscriptionState } from '@/api'
import { failWith, fakeApi, type FakeRoute } from '@/test/fakeApi'
import { communityItem } from '@/test/fixtures'
import { SinceLastPublished, UpdateChanges, UpdateCount } from './Changes'

const api = vi.hoisted(() => ({ current: null as null | ((input: RequestInfo | URL, init?: RequestInit) => Promise<Response>) }))
vi.mock('@/api/client', () => ({ apiFetch: (input: RequestInfo | URL, init?: RequestInit) => api.current!(input, init) }))

const genres = { movie: new Map<number, string>(), tv: new Map<number, string>() }

const changes: SnapshotChange[] = [
  { op: 'removed', kind: 'folder', key: 'f80', name: '80s' },
  { op: 'added', kind: 'catalog', key: 'heat', name: 'Heat', folder_key: 'fc', folder: 'Classics' },
  { op: 'added', kind: 'catalog', key: 'ronin', name: 'Ronin', folder_key: 'fc', folder: 'Classics' },
]

function renderWith(routes: Record<string, FakeRoute>, ui: React.ReactElement) {
  const fake = fakeApi(routes)
  api.current = fake.apiFetch
  render(<QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>{ui}</QueryClientProvider>)
  return fake.calls
}

beforeEach(() => {
  api.current = null
})

describe('UpdateChanges', () => {
  const waiting = communityItem({ id: 'pub', kind: 'collection', catalog: null, subscribed: true, update_available: true })

  it('says what the update does in one counted line, fetched while an update waits', async () => {
    const calls = renderWith(
      { 'GET /api/p/1/community/pub/changes': changes },
      <UpdateChanges profileIndex={1} item={waiting} genres={genres} />,
    )
    expect(screen.getByRole('heading', { name: 'In this update' })).toBeInTheDocument()
    expect(await screen.findByRole('button', { name: '1 folder removed' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '2 catalogs added' })).toBeInTheDocument()
    expect(screen.getByText('3 changes')).toBeInTheDocument()
    expect(calls).toEqual(['GET /api/p/1/community/pub/changes'])
  })

  it('jumps from a part of the line to where the page marks it', async () => {
    renderWith({ 'GET /api/p/1/community/pub/changes': changes }, <UpdateChanges profileIndex={1} item={waiting} genres={genres} />)
    const card = document.createElement('div')
    card.id = 'update-folder-fc'
    card.tabIndex = -1
    card.scrollIntoView = vi.fn()
    document.body.append(card)
    fireEvent.click(await screen.findByRole('button', { name: '2 catalogs added' }))
    expect(card.scrollIntoView).toHaveBeenCalled()
    expect(document.activeElement).toBe(card)
    card.remove()
  })

  it('fetches nothing and shows nothing for a row that is in step, or not added', () => {
    const calls = renderWith(
      { 'GET /api/p/1/community/pub/changes': changes },
      <>
        <UpdateChanges profileIndex={1} item={{ ...waiting, update_available: false }} genres={genres} />
        <UpdateChanges profileIndex={1} item={{ ...waiting, subscribed: false }} genres={genres} />
      </>,
    )
    expect(screen.queryByRole('heading')).toBeNull()
    expect(calls).toEqual([])
  })

  it('says so when the list can’t load, and stays on the page', async () => {
    renderWith(
      { 'GET /api/p/1/community/pub/changes': () => failWith(404, 'not in Community any more') },
      <UpdateChanges profileIndex={1} item={waiting} genres={genres} />,
    )
    expect(await screen.findByText('Couldn’t load what changed.')).toBeInTheDocument()
  })
})

describe('UpdateCount', () => {
  const following: SubscriptionState = { publication_id: 'pub', update_available: true }

  it('counts the list’s lines while an update waits', async () => {
    renderWith({ 'GET /api/p/1/community/pub/changes': changes }, <UpdateCount profileIndex={1} subscription={following} />)
    expect(await screen.findByText('3 changes')).toBeInTheDocument()
  })

  it('shows nothing, and fetches nothing, without an update waiting', () => {
    const calls = renderWith(
      { 'GET /api/p/1/community/pub/changes': changes },
      <>
        <UpdateCount profileIndex={1} subscription={{ ...following, update_available: false }} />
        <UpdateCount profileIndex={1} subscription={null} />
      </>,
    )
    expect(document.body).toHaveTextContent('')
    expect(calls).toEqual([])
  })

  it('shows nothing for an empty list or one that can’t load', async () => {
    const calls = renderWith({ 'GET /api/p/1/community/pub/changes': [] }, <UpdateCount profileIndex={1} subscription={following} />)
    await waitFor(() => expect(calls).toHaveLength(1))
    expect(document.body).toHaveTextContent('')
  })
})

describe('SinceLastPublished', () => {
  it('lists what publishing again would change, for the row it names', async () => {
    const calls = renderWith(
      { 'GET /api/p/1/collections/col9/changes-since-publish': changes },
      <SinceLastPublished profileIndex={1} kind="collection" id="col9" enabled genres={genres} />,
    )
    expect(screen.getByRole('heading', { name: 'Since you last published' })).toBeInTheDocument()
    expect(await screen.findByText('“Heat” to “Classics”')).toBeInTheDocument()
    expect(calls).toEqual(['GET /api/p/1/collections/col9/changes-since-publish'])
  })

  it('asks a catalog’s route for a catalog', async () => {
    const calls = renderWith(
      { 'GET /api/p/1/catalogs/c1/changes-since-publish': [{ op: 'changed', kind: 'catalog', aspect: 'name', name: 'B', was: 'A' }] },
      <SinceLastPublished profileIndex={1} kind="catalog" id="c1" enabled genres={genres} />,
    )
    expect(await screen.findByText('Name: “A” → “B”')).toBeInTheDocument()
    expect(calls).toEqual(['GET /api/p/1/catalogs/c1/changes-since-publish'])
  })

  it('does nothing unless the row publishes changes', () => {
    const calls = renderWith(
      { 'GET /api/p/1/catalogs/c1/changes-since-publish': changes },
      <SinceLastPublished profileIndex={1} kind="catalog" id="c1" enabled={false} genres={genres} />,
    )
    expect(document.body).toHaveTextContent('')
    expect(calls).toEqual([])
  })
})
