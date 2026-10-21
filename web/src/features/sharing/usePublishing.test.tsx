// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { Catalog, Collection, SubscriptionState } from '@/api'
import type { ToastInput } from '@/components/toastQueue'
import { failWith, fakeApi, type FakeRoute } from '@/test/fakeApi'
import { catalog, collection, folder } from '@/test/fixtures'
import type { OwnRow } from './sharingState'
import { usePublishing } from './usePublishing'

const api = vi.hoisted(() => ({ current: null as null | ((input: RequestInfo | URL, init?: RequestInit) => Promise<Response>) }))
vi.mock('@/api/client', () => ({ apiFetch: (input: RequestInfo | URL, init?: RequestInit) => api.current!(input, init) }))

const genres = { movie: new Map([[27, 'Horror']]), tv: new Map() }
const subscription: SubscriptionState = { publication_id: 'pub', update_available: true }

/** Renders what Community renders from the hook for one own row: a way to
 *  open each step, and the dialogs. */
function Harness(props: {
  catalog?: Catalog
  collection?: Collection
  onToast: (toast: ToastInput) => void
  onUnpublished?: () => void
}) {
  const publishing = usePublishing({
    profileIndex: 1,
    genres,
    onToast: props.onToast,
    onUnpublished: props.onUnpublished,
  })
  const own: OwnRow = props.collection
    ? { kind: 'collection', row: props.collection }
    : { kind: 'catalog', row: props.catalog ?? catalog() }
  return (
    <>
      <button type="button" onClick={() => publishing.publish(own)}>
        Start publish
      </button>
      <button type="button" onClick={() => publishing.unpublish(own)}>
        Start unpublish
      </button>
      {publishing.dialogs}
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
const question = () => screen.getByRole('alertdialog')

beforeEach(() => {
  api.current = null
})

describe('usePublishing', () => {
  it('publishes a catalog through the publish dialog', async () => {
    const { onToast, calls } = renderHarness(
      { catalog: catalog({ id: 'c1', name: 'Horror', params: '{"with_genres":"27"}' }) },
      { 'POST /api/p/1/catalogs/c1/publish': catalog({ id: 'c1' }) },
    )
    fireEvent.click(screen.getByRole('button', { name: 'Start publish' }))
    expect(within(dialog()).getByRole('heading', { name: 'Publish “Horror”?' })).toBeInTheDocument()
    fireEvent.click(within(dialog()).getByRole('button', { name: 'Publish' }))
    await waitFor(() => expect(onToast).toHaveBeenCalledWith({ text: 'Published “Horror”', tone: 'success' }))
    expect(calls).toContain('POST /api/p/1/catalogs/c1/publish')
    expect(screen.queryByRole('dialog')).toBeNull()
  })

  it('lists what changed since the last publish in the dialog of a changed row, and only there', async () => {
    const changed = { id: 'p', changed_since_publish: true }
    const { calls } = renderHarness(
      { collection: collection({ id: 'col1', title: 'Night', publication: changed }) },
      {
        'GET /api/p/1/collections/col1/changes-since-publish': [
          { op: 'changed', kind: 'catalog', aspect: 'name', name: 'Horror', was: 'Scary' },
        ],
      },
    )
    expect(calls).toEqual([])
    fireEvent.click(screen.getByRole('button', { name: 'Start publish' }))
    expect(await within(dialog()).findByRole('heading', { name: 'Since you last published' })).toBeInTheDocument()
    expect(await within(dialog()).findByText('Name: “Scary” → “Horror”')).toBeInTheDocument()
    expect(calls).toEqual(['GET /api/p/1/collections/col1/changes-since-publish'])
  })

  it('lists nothing in the dialog of a first publish', () => {
    const { calls } = renderHarness({ catalog: catalog({ id: 'c1', name: 'Horror' }) }, {})
    fireEvent.click(screen.getByRole('button', { name: 'Start publish' }))
    expect(screen.queryByRole('heading', { name: 'Since you last published' })).toBeNull()
    expect(calls).toEqual([])
  })

  it('publishes a changed catalog’s update, and shows a refusal in place', async () => {
    const changed = { id: 'p', changed_since_publish: true }
    const { onToast } = renderHarness(
      { catalog: catalog({ id: 'c1', name: 'Horror', publication: changed }) },
      { 'POST /api/p/1/catalogs/c1/publish': () => failWith(502, 'TMDB is unreachable') },
    )
    fireEvent.click(screen.getByRole('button', { name: 'Start publish' }))
    fireEvent.click(within(dialog()).getByRole('button', { name: 'Publish update' }))
    expect(await within(dialog()).findByRole('alert')).toHaveTextContent('TMDB is unreachable')
    expect(onToast).not.toHaveBeenCalled()
    fireEvent.click(within(dialog()).getByRole('button', { name: 'Cancel' }))
    expect(screen.queryByRole('dialog')).toBeNull()
  })

  it('reports a published update', async () => {
    const changed = { id: 'p', changed_since_publish: true }
    const { onToast } = renderHarness(
      { catalog: catalog({ id: 'c1', name: 'Horror', publication: changed }) },
      { 'POST /api/p/1/catalogs/c1/publish': catalog({ id: 'c1' }) },
    )
    fireEvent.click(screen.getByRole('button', { name: 'Start publish' }))
    fireEvent.click(within(dialog()).getByRole('button', { name: 'Publish update' }))
    await waitFor(() => expect(onToast).toHaveBeenCalledWith({ text: 'Published your changes to “Horror”', tone: 'success' }))
  })

  it('unpublishes once asked, then says it is gone', async () => {
    const live = { id: 'p', changed_since_publish: false }
    const onUnpublished = vi.fn()
    const { onToast, calls } = renderHarness(
      { catalog: catalog({ id: 'c1', name: 'Horror', publication: live }), onUnpublished },
      { 'POST /api/p/1/catalogs/c1/unpublish': catalog({ id: 'c1' }) },
    )
    fireEvent.click(screen.getByRole('button', { name: 'Start unpublish' }))
    expect(within(question()).getByText(/Community stops listing it/)).toBeInTheDocument()
    fireEvent.click(within(question()).getByRole('button', { name: 'Unpublish' }))
    await waitFor(() => expect(onToast).toHaveBeenCalledWith({ text: 'Unpublished “Horror”', tone: 'success' }))
    expect(calls).toContain('POST /api/p/1/catalogs/c1/unpublish')
    expect(onUnpublished).toHaveBeenCalled()
  })

  it('keeps a failed stop in the question', async () => {
    const live = { id: 'p', changed_since_publish: false }
    const { onToast } = renderHarness(
      { catalog: catalog({ id: 'c1', name: 'Horror', publication: live }) },
      { 'POST /api/p/1/catalogs/c1/unpublish': () => failWith(500, 'database locked') },
    )
    fireEvent.click(screen.getByRole('button', { name: 'Start unpublish' }))
    fireEvent.click(within(question()).getByRole('button', { name: 'Unpublish' }))
    expect(await within(question()).findByRole('alert')).toHaveTextContent('database locked')
    expect(onToast).not.toHaveBeenCalled()
    fireEvent.click(within(question()).getByRole('button', { name: 'Cancel' }))
    expect(screen.queryByRole('alertdialog')).toBeNull()
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
    expect(screen.getByRole('button', { name: 'Start publish' })).toBeEnabled()
    fireEvent.click(screen.getByRole('button', { name: 'Start publish' }))
    expect(within(dialog()).getByText('From your library, public as part of this collection')).toBeInTheDocument()
    expect(within(dialog()).getByText('Giallo')).toBeInTheDocument()
    expect(within(dialog()).getByText('From Community')).toBeInTheDocument()
    fireEvent.click(within(dialog()).getByRole('button', { name: 'Publish' }))
    await waitFor(() => expect(onToast).toHaveBeenCalledWith({ text: 'Published “Night”', tone: 'success' }))
    expect(calls).toContain('POST /api/p/1/collections/col2/publish')
  })

  it('turns the publish dialog of a changed row into the Unpublish question', async () => {
    const changed = { id: 'p', changed_since_publish: true }
    renderHarness(
      { catalog: catalog({ id: 'c1', name: 'Horror', publication: changed }) },
      { 'GET /api/p/1/catalogs/c1/changes-since-publish': [] },
    )
    fireEvent.click(screen.getByRole('button', { name: 'Start publish' }))
    fireEvent.click(within(dialog()).getByRole('button', { name: 'Unpublish' }))
    expect(within(question()).getByText(/Community stops listing it/)).toBeInTheDocument()
    expect(screen.queryByRole('heading', { name: 'Publish your changes to “Horror”?' })).toBeNull()
  })

  it('lists what a collection publishes in the publish dialog', () => {
    const own = catalog({ id: 's1', name: 'Scoped one', collection_id: 'col1' })
    renderHarness(
      { collection: collection({ title: 'Night', folders: [folder({ refs: [{ catalog_id: 's1', genre: '' }] })], catalogs: [own] }) },
      {},
    )
    fireEvent.click(screen.getByRole('button', { name: 'Start publish' }))
    expect(within(dialog()).getByText('1 folder, 1 catalog of its own')).toBeInTheDocument()
  })
})
