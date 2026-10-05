// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { Catalog, Collection, SubscriptionState } from '@/api'
import { SignStepButton } from '@/components/PaneSign'
import type { ToastMessage } from '@/components/useToast'
import { failWith, fakeApi, type FakeRoute } from '@/test/fakeApi'
import { catalog, collection, folder } from '@/test/fixtures'
import { useWorkspaceSharing } from './useWorkspaceSharing'

const api = vi.hoisted(() => ({ current: null as null | ((input: RequestInfo | URL, init?: RequestInit) => Promise<Response>) }))
vi.mock('@/api/client', () => ({ apiFetch: (input: RequestInfo | URL, init?: RequestInit) => api.current!(input, init) }))

const genres = { movie: new Map([[27, 'Horror']]), tv: new Map() }
const subscription: SubscriptionState = { publication_id: 'pub', update_available: true, unpublished: false }

/** Renders what the workspace renders from the hook for an own row: its
 *  sign button and stickers, and the dialogs. */
function Harness(props: {
  catalog?: Catalog
  collection?: Collection
  dirty?: boolean
  /** The ids a push would change in Nuvio. */
  waiting?: string[]
  onToast: (toast: ToastMessage) => void
}) {
  const sharing = useWorkspaceSharing({
    profileIndex: 1,
    genres,
    dirty: props.dirty ?? false,
    waitingForPush: new Set(props.waiting),
    onToast: props.onToast,
  })
  const own = props.collection ? sharing.collectionSharing(props.collection) : sharing.catalogSharing(props.catalog)
  return (
    <>
      <div data-testid="badges">{own?.sharingBadges}</div>
      <SignStepButton step={own?.sharingStep} />
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

  it('flags To push beside the Community sticker while a push would change the row', () => {
    const live = { id: 'p', status: 'live' as const, changed_since_publish: false }
    const row = catalog({ id: 'c1', name: 'Horror', publication: live })
    renderHarness({ catalog: row, waiting: ['c1'] }, {})
    expect(within(screen.getByTestId('badges')).getAllByText(/./).map((s) => s.textContent)).toEqual([
      'Published',
      'To push',
    ])
    cleanup()
    renderHarness({ catalog: row, waiting: ['other'] }, {})
    expect(screen.queryByText('To push')).toBeNull()
  })

  it('flags a collection To push when a catalog scoped to it changed', () => {
    renderHarness({ collection: collection({ id: 'col1', title: 'Night' }), waiting: ['col1'] }, {})
    expect(screen.getByText('To push')).toBeInTheDocument()
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

  it('lists what changed since the last publish in the dialog of a changed row, and only there', async () => {
    const changed = { id: 'p', status: 'live' as const, changed_since_publish: true }
    const { calls } = renderHarness(
      { collection: collection({ id: 'col1', title: 'Night', publication: changed }) },
      {
        'GET /api/p/1/collections/col1/changes-since-publish': [
          { op: 'changed', kind: 'catalog', aspect: 'name', name: 'Horror', was: 'Scary' },
        ],
      },
    )
    expect(calls).toEqual([])
    fireEvent.click(screen.getByRole('button', { name: 'Publish update…' }))
    expect(await within(dialog()).findByRole('heading', { name: 'Since you last published' })).toBeInTheDocument()
    expect(await within(dialog()).findByText('Name: “Scary” → “Horror”')).toBeInTheDocument()
    expect(calls).toEqual(['GET /api/p/1/collections/col1/changes-since-publish'])
  })

  it('lists nothing in the dialog of a first publish', () => {
    const { calls } = renderHarness({ catalog: catalog({ id: 'c1', name: 'Horror' }) }, {})
    fireEvent.click(screen.getByRole('button', { name: 'Publish…' }))
    expect(screen.queryByRole('heading', { name: 'Since you last published' })).toBeNull()
    expect(calls).toEqual([])
  })

  it('publishes a changed catalog’s update, and shows a refusal in place', async () => {
    const changed = { id: 'p', status: 'live' as const, changed_since_publish: true }
    const { onToast } = renderHarness(
      { catalog: catalog({ id: 'c1', name: 'Horror', publication: changed }) },
      { 'POST /api/p/1/catalogs/c1/publish': () => failWith(502, 'TMDB is unreachable') },
    )
    expect(screen.getByText('To publish')).toBeInTheDocument()
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
    fireEvent.click(screen.getByRole('button', { name: 'Unpublish…' }))
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
    fireEvent.click(screen.getByRole('button', { name: 'Unpublish…' }))
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

  it('waits for a save: the step is greyed and says so when pressed', async () => {
    renderHarness({ catalog: catalog({ id: 'c1', name: 'Horror' }), dirty: true }, {})
    const step = screen.getByRole('button', { name: 'Publish…' })
    expect(step).toHaveAttribute('aria-disabled', 'true')
    fireEvent.click(step)
    expect(await screen.findByRole('status')).toHaveTextContent('Save first.')
    expect(screen.queryByRole('dialog')).toBeNull()
  })

  it('names the step Publish again… after Unpublish', () => {
    const unpublished = { id: 'p', status: 'unpublished' as const, changed_since_publish: false }
    renderHarness({ catalog: catalog({ id: 'c1', name: 'Horror', publication: unpublished }) }, {})
    expect(screen.getByRole('button', { name: 'Publish again…' })).toBeInTheDocument()
  })

  it('turns the publish dialog of a changed row into the Unpublish question', async () => {
    const changed = { id: 'p', status: 'live' as const, changed_since_publish: true }
    renderHarness(
      { catalog: catalog({ id: 'c1', name: 'Horror', publication: changed }) },
      { 'GET /api/p/1/catalogs/c1/changes-since-publish': [] },
    )
    fireEvent.click(screen.getByRole('button', { name: 'Publish update…' }))
    fireEvent.click(within(dialog()).getByRole('button', { name: 'Unpublish' }))
    expect(within(dialog()).getByText(/Community stops listing it/)).toBeInTheDocument()
    expect(screen.queryByRole('heading', { name: 'Publish your changes to “Horror”?' })).toBeNull()
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
