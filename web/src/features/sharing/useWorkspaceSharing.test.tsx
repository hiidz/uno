// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { Catalog, Collection, SubscriptionState } from '@/api'
import type { ToastMessage } from '@/components/useToast'
import { failWith, fakeApi, type FakeRoute } from '@/test/fakeApi'
import { catalog, collection, folder } from '@/test/fixtures'
import { useWorkspaceSharing } from './useWorkspaceSharing'

const api = vi.hoisted(() => ({ current: null as null | ((input: RequestInfo | URL, init?: RequestInit) => Promise<Response>) }))
vi.mock('@/api/client', () => ({ apiFetch: (input: RequestInfo | URL, init?: RequestInit) => api.current!(input, init) }))

const genres = { movie: new Map([[27, 'Horror']]), tv: new Map() }
const subscription: SubscriptionState = { publication_id: 'pub', update_available: true, unpublished: false }

/** Renders what the workspace renders from the hook for an own row: its
 *  sharing setting and stickers, and the dialogs. */
function Harness(props: {
  catalog?: Catalog
  collection?: Collection
  dirty?: boolean
  onToast: (toast: ToastMessage) => void
}) {
  const sharing = useWorkspaceSharing({ profileIndex: 1, genres, dirty: props.dirty ?? false, onToast: props.onToast })
  const own = props.collection ? sharing.collectionSharing(props.collection) : sharing.catalogSharing(props.catalog)
  return (
    <>
      <div data-testid="badges">{own?.sharingBadges}</div>
      {own?.sharingRow}
      {sharing.dialogs}
    </>
  )
}

function renderHarness(props: Omit<Parameters<typeof Harness>[0], 'onToast'>, routes: Record<string, FakeRoute>) {
  const fake = fakeApi(routes)
  api.current = fake.apiFetch
  const onToast = vi.fn()
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <Harness {...props} onToast={onToast} />
    </QueryClientProvider>,
  )
  return { onToast, calls: fake.calls }
}

const dialog = () => screen.getByRole('dialog')

beforeEach(() => {
  api.current = null
})

describe('useWorkspaceSharing', () => {
  it('shows nothing for a row the library doesn’t list yet', () => {
    renderHarness({}, {})
    expect(screen.queryByText('Community')).toBeNull()
    expect(screen.getByTestId('badges')).toBeEmptyDOMElement()
  })

  it('publishes a catalog through the publish dialog', async () => {
    const { onToast, calls } = renderHarness(
      { catalog: catalog({ id: 'c1', name: 'Horror', params: '{"with_genres":"27"}' }) },
      { 'POST /api/p/1/catalogs/c1/publish': catalog({ id: 'c1' }) },
    )
    fireEvent.click(screen.getByRole('button', { name: 'Publish…' }))
    expect(within(dialog()).getByRole('heading', { name: 'Publish “Horror”?' })).toBeInTheDocument()
    fireEvent.click(within(dialog()).getByRole('button', { name: 'Publish' }))
    await waitFor(() => expect(onToast).toHaveBeenCalledWith({ text: 'Published “Horror”', tone: 'success' }))
    expect(calls).toContain('POST /api/p/1/catalogs/c1/publish')
    expect(screen.queryByRole('dialog')).toBeNull()
  })

  it('publishes a changed catalog’s update, and shows a refusal in place', async () => {
    const changed = { id: 'p', status: 'live' as const, changed_since_publish: true }
    const { onToast } = renderHarness(
      { catalog: catalog({ id: 'c1', name: 'Horror', publication: changed }) },
      { 'POST /api/p/1/catalogs/c1/publish': () => failWith(502, 'TMDB is unreachable') },
    )
    expect(screen.getByText('Changed')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Publish update…' }))
    fireEvent.click(within(dialog()).getByRole('button', { name: 'Publish update' }))
    expect(await within(dialog()).findByRole('alert')).toHaveTextContent('TMDB is unreachable')
    expect(onToast).not.toHaveBeenCalled()
    fireEvent.click(within(dialog()).getByRole('button', { name: 'Cancel' }))
    expect(screen.queryByRole('dialog')).toBeNull()
  })

  it('reports a published update', async () => {
    const changed = { id: 'p', status: 'live' as const, changed_since_publish: true }
    const { onToast } = renderHarness(
      { catalog: catalog({ id: 'c1', name: 'Horror', publication: changed }) },
      { 'POST /api/p/1/catalogs/c1/publish': catalog({ id: 'c1' }) },
    )
    fireEvent.click(screen.getByRole('button', { name: 'Publish update…' }))
    fireEvent.click(within(dialog()).getByRole('button', { name: 'Publish update' }))
    await waitFor(() => expect(onToast).toHaveBeenCalledWith({ text: 'Published your changes to “Horror”', tone: 'success' }))
  })

  it('unpublishes once asked', async () => {
    const live = { id: 'p', status: 'live' as const, changed_since_publish: false }
    const { onToast, calls } = renderHarness(
      { catalog: catalog({ id: 'c1', name: 'Horror', publication: live }) },
      { 'POST /api/p/1/catalogs/c1/unpublish': catalog({ id: 'c1' }) },
    )
    fireEvent.click(screen.getByRole('button', { name: 'Unpublish' }))
    expect(within(dialog()).getByText(/Community stops listing it/)).toBeInTheDocument()
    fireEvent.click(within(dialog()).getByRole('button', { name: 'Unpublish' }))
    await waitFor(() => expect(onToast).toHaveBeenCalledWith({ text: 'Unpublished “Horror”', tone: 'success' }))
    expect(calls).toContain('POST /api/p/1/catalogs/c1/unpublish')
  })

  it('keeps a failed stop in the question', async () => {
    const live = { id: 'p', status: 'live' as const, changed_since_publish: false }
    const { onToast } = renderHarness(
      { catalog: catalog({ id: 'c1', name: 'Horror', publication: live }) },
      { 'POST /api/p/1/catalogs/c1/unpublish': () => failWith(500, 'database locked') },
    )
    fireEvent.click(screen.getByRole('button', { name: 'Unpublish' }))
    fireEvent.click(within(dialog()).getByRole('button', { name: 'Unpublish' }))
    expect(await within(dialog()).findByRole('alert')).toHaveTextContent('database locked')
    expect(onToast).not.toHaveBeenCalled()
    fireEvent.click(within(dialog()).getByRole('button', { name: 'Cancel' }))
    expect(screen.queryByRole('dialog')).toBeNull()
  })

  it('publishes a collection using a catalog added from Community, marking that catalog', async () => {
    const added = catalog({ id: 't1', name: 'Giallo', subscription })
    const { onToast, calls } = renderHarness(
      {
        collection: collection({
          id: 'col2',
          title: 'Night',
          folders: [folder({ refs: [{ catalog_id: 't1', genre: '' }] })],
          catalogs: [added],
        }),
      },
      { 'POST /api/p/1/collections/col2/publish': collection({ id: 'col2' }) },
    )
    expect(screen.getByRole('button', { name: 'Publish…' })).toBeEnabled()
    fireEvent.click(screen.getByRole('button', { name: 'Publish…' }))
    expect(within(dialog()).getByText('From your library, published as they are now')).toBeInTheDocument()
    expect(within(dialog()).getByText('Giallo')).toBeInTheDocument()
    expect(within(dialog()).getByText('From Community')).toBeInTheDocument()
    fireEvent.click(within(dialog()).getByRole('button', { name: 'Publish' }))
    await waitFor(() => expect(onToast).toHaveBeenCalledWith({ text: 'Published “Night”', tone: 'success' }))
    expect(calls).toContain('POST /api/p/1/collections/col2/publish')
  })

  it('lists what a collection publishes in the publish dialog', () => {
    const own = catalog({ id: 's1', name: 'Scoped one', collection_id: 'col1' })
    renderHarness(
      { collection: collection({ title: 'Night', folders: [folder({ refs: [{ catalog_id: 's1', genre: '' }] })], catalogs: [own] }) },
      {},
    )
    fireEvent.click(screen.getByRole('button', { name: 'Publish…' }))
    expect(within(dialog()).getByText('1 folder, 1 catalog of its own')).toBeInTheDocument()
  })
})
