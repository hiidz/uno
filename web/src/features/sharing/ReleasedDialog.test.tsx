// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { ReleasedCopy } from '@/api'
import { failWith, fakeApi, type FakeRoute } from '@/test/fakeApi'
import { ReleasedDialog } from './ReleasedDialog'

const api = vi.hoisted(() => ({ current: null as null | ((input: RequestInfo | URL, init?: RequestInit) => Promise<Response>) }))
vi.mock('@/api/client', () => ({ apiFetch: (input: RequestInfo | URL, init?: RequestInit) => api.current!(input, init) }))

const horror: ReleasedCopy = { kind: 'catalog', id: 'c1', name: 'Horror' }
const night: ReleasedCopy = { kind: 'collection', id: 'k1', name: 'Night shift' }

function renderDialog(routes: Record<string, FakeRoute>) {
  const fake = fakeApi(routes)
  api.current = fake.apiFetch
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <ReleasedDialog profileIndex={1} />
    </QueryClientProvider>,
  )
  return fake.calls
}

const told = (name: string) => `Its publisher removed “${name}” from Community. It’s now yours to edit.`

beforeEach(() => {
  api.current = null
})

describe('ReleasedDialog', () => {
  it('shows nothing while no row is released', async () => {
    const calls = renderDialog({ 'GET /api/p/1/released': [] })
    await waitFor(() => expect(calls).toEqual(['GET /api/p/1/released']))
    expect(screen.queryByRole('dialog')).toBeNull()
  })

  it('tells of each released row in turn, acknowledging each as it is dismissed', async () => {
    const calls = renderDialog({
      'GET /api/p/1/released': [horror, night],
      'POST /api/p/1/catalogs/c1/acknowledge-release': [night],
      'POST /api/p/1/collections/k1/acknowledge-release': [],
    })
    expect(await screen.findByText(told('Horror'))).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Got it' }))
    expect(await screen.findByText(told('Night shift'))).toBeInTheDocument()

    fireEvent.keyDown(screen.getByRole('dialog'), { key: 'Escape' })
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    expect(calls).toEqual([
      'GET /api/p/1/released',
      'POST /api/p/1/catalogs/c1/acknowledge-release',
      'POST /api/p/1/collections/k1/acknowledge-release',
    ])
  })

  it('stays open and says why when the acknowledgement fails', async () => {
    renderDialog({
      'GET /api/p/1/released': [horror],
      'POST /api/p/1/catalogs/c1/acknowledge-release': () => failWith(500, 'failed to acknowledge'),
    })
    fireEvent.click(await screen.findByRole('button', { name: 'Got it' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('failed to acknowledge')
    expect(screen.getByText(told('Horror'))).toBeInTheDocument()
  })

  it('closes when the row was deleted elsewhere, rereading the list', async () => {
    let listed: ReleasedCopy[] = [horror]
    const calls = renderDialog({
      'GET /api/p/1/released': () => listed,
      'POST /api/p/1/catalogs/c1/acknowledge-release': () => {
        listed = []
        return failWith(404, 'catalog not found')
      },
    })
    fireEvent.click(await screen.findByRole('button', { name: 'Got it' }))
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    expect(calls).toEqual(['GET /api/p/1/released', 'POST /api/p/1/catalogs/c1/acknowledge-release', 'GET /api/p/1/released'])
  })
})
