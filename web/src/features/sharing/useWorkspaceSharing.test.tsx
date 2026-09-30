// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { Catalog, Collection, SubscriptionState } from '@/api'
import type { EditorTarget } from '@/features/builder/target'
import type { OpenPublication } from '@/features/community/communityQuery'
import type { ToastMessage } from '@/components/useToast'
import { failWith, fakeApi, type FakeRoute } from '@/test/fakeApi'
import { catalog, collection, folder } from '@/test/fixtures'
import { useWorkspaceSharing } from './useWorkspaceSharing'

const api = vi.hoisted(() => ({ current: null as null | ((input: RequestInfo | URL, init?: RequestInit) => Promise<Response>) }))
vi.mock('@/api/client', () => ({ apiFetch: (input: RequestInfo | URL, init?: RequestInit) => api.current!(input, init) }))

const genres = { movie: new Map([[27, 'Horror']]), tv: new Map() }
const subscription: SubscriptionState = { publication_id: 'pub', update_available: true, withdrawn: false }
const takenCatalog = catalog({ id: 'c9', name: 'Taken one', subscription })
const takenCollection = collection({ id: 'c9', title: 'Taken one', subscription })

/** Renders what the workspace renders from the hook: an editor's sharing
 *  setting and stickers, its Save, which goes through `confirmCopySave`, and
 *  the dialogs. */
function Harness(props: {
  catalog?: Catalog
  collection?: Collection
  dirty?: boolean
  onToast: (toast: ToastMessage) => void
  onReopen: (target: EditorTarget) => void
  onOpenPublication: (publication: OpenPublication) => void
  onSave: (form: string) => void
  onDuplicate: () => void
}) {
  const sharing = useWorkspaceSharing({
    profileIndex: 1,
    genres,
    dirty: props.dirty ?? false,
    onToast: props.onToast,
    onReopen: props.onReopen,
    onOpenPublication: props.onOpenPublication,
  })
  const own = props.collection
    ? sharing.collectionSharing(props.collection, props.onDuplicate)
    : sharing.catalogSharing(props.catalog, props.onDuplicate)
  return (
    <>
      <div data-testid="badges">{own?.sharingBadges}</div>
      {own?.sharingRow}
      <button type="button" onClick={() => sharing.confirmCopySave(props.collection ?? props.catalog, props.onSave, 'the form')}>
        Save
      </button>
      {sharing.dialogs}
    </>
  )
}

function renderHarness(
  props: Omit<Parameters<typeof Harness>[0], 'onToast' | 'onReopen' | 'onOpenPublication' | 'onSave' | 'onDuplicate'>,
  routes: Record<string, FakeRoute>,
) {
  const fake = fakeApi(routes)
  api.current = fake.apiFetch
  const handlers = { onToast: vi.fn(), onReopen: vi.fn(), onOpenPublication: vi.fn(), onSave: vi.fn(), onDuplicate: vi.fn() }
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <Harness {...props} {...handlers} />
    </QueryClientProvider>,
  )
  return { ...handlers, calls: fake.calls }
}

const dialog = () => screen.getByRole('dialog')

beforeEach(() => {
  api.current = null
})

describe('useWorkspaceSharing', () => {
  it('shows nothing for a row the library doesn’t list yet', () => {
    renderHarness({}, {})
    expect(screen.queryByText('Sharing')).toBeNull()
    expect(screen.getByTestId('badges')).toBeEmptyDOMElement()
  })

  it('shares a catalog through the publish dialog', async () => {
    const { onToast, calls } = renderHarness(
      { catalog: catalog({ id: 'c1', name: 'Horror', params: '{"with_genres":"27"}' }) },
      { 'POST /api/p/1/catalogs/c1/publish': catalog({ id: 'c1' }) },
    )
    fireEvent.click(screen.getByRole('button', { name: 'Share…' }))
    expect(within(dialog()).getByRole('heading', { name: 'Share “Horror”?' })).toBeInTheDocument()
    fireEvent.click(within(dialog()).getByRole('button', { name: 'Share' }))
    await waitFor(() => expect(onToast).toHaveBeenCalledWith({ text: 'Shared “Horror”', tone: 'success' }))
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

  it('stops sharing once asked', async () => {
    const live = { id: 'p', status: 'live' as const, changed_since_publish: false }
    const { onToast, calls } = renderHarness(
      { catalog: catalog({ id: 'c1', name: 'Horror', publication: live }) },
      { 'POST /api/p/1/catalogs/c1/withdraw': catalog({ id: 'c1' }) },
    )
    fireEvent.click(screen.getByRole('button', { name: 'Stop sharing' }))
    expect(within(dialog()).getByText(/Community stops listing it/)).toBeInTheDocument()
    fireEvent.click(within(dialog()).getByRole('button', { name: 'Stop sharing' }))
    await waitFor(() => expect(onToast).toHaveBeenCalledWith({ text: 'Stopped sharing “Horror”', tone: 'success' }))
    expect(calls).toContain('POST /api/p/1/catalogs/c1/withdraw')
  })

  it('shows a copy’s From Community row in place of the Sharing row', () => {
    const { onDuplicate } = renderHarness({ catalog: takenCatalog }, {})
    expect(screen.getByText('Taken from Community. Its owner has published an update.')).toBeInTheDocument()
    expect(screen.queryByText('Sharing')).toBeNull()
    expect(screen.queryByRole('button', { name: 'Share…' })).toBeNull()
    expect(screen.getByTestId('badges')).toHaveTextContent('Update')
    fireEvent.click(screen.getByRole('button', { name: 'Duplicate' }))
    expect(onDuplicate).toHaveBeenCalled()
  })

  it('holds Detach while the copy has unsaved changes, which saving makes its own anyway', () => {
    renderHarness({ catalog: takenCatalog, dirty: true }, {})
    expect(screen.getByRole('button', { name: 'Detach' })).toBeDisabled()
    expect(screen.getByText('Saving your changes makes this copy yours.')).toBeInTheDocument()
  })

  it('saves an own row at once', () => {
    const { onSave } = renderHarness({ catalog: catalog({ id: 'c1', name: 'Horror' }) }, {})
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    expect(onSave).toHaveBeenCalledExactlyOnceWith('the form')
    expect(screen.queryByRole('dialog')).toBeNull()
  })

  it('asks before a copy’s save, and saves only once confirmed', () => {
    const { onSave } = renderHarness({ collection: takenCollection }, {})
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    expect(within(dialog()).getByRole('heading', { name: 'Save and make it yours?' })).toBeInTheDocument()
    expect(within(dialog()).getByText('Taken one')).toBeInTheDocument()
    fireEvent.click(within(dialog()).getByRole('button', { name: 'Keep editing' }))
    expect(screen.queryByRole('dialog')).toBeNull()
    expect(onSave).not.toHaveBeenCalled()

    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    fireEvent.click(within(dialog()).getByRole('button', { name: 'Save' }))
    expect(onSave).toHaveBeenCalledExactlyOnceWith('the form')
    expect(screen.queryByRole('dialog')).toBeNull()
  })

  it('detaches a copy once asked, and keeps a failure in the question', async () => {
    renderHarness({ catalog: takenCatalog }, { 'POST /api/p/1/catalogs/c9/detach': () => failWith(500, 'database locked') })
    fireEvent.click(screen.getByRole('button', { name: 'Detach' }))
    expect(within(dialog()).getByRole('heading', { name: 'Detach “Taken one”?' })).toBeInTheDocument()
    fireEvent.click(within(dialog()).getByRole('button', { name: 'Detach' }))
    expect(await within(dialog()).findByRole('alert')).toHaveTextContent('database locked')
    fireEvent.click(within(dialog()).getByRole('button', { name: 'Cancel' }))
    expect(screen.queryByRole('dialog')).toBeNull()
  })

  it('reports a detach, and reopens the copy from the row the detach left', async () => {
    const detached = catalog({ id: 'c9', name: 'Updated since it opened' })
    const { onToast, onReopen } = renderHarness({ catalog: takenCatalog }, { 'POST /api/p/1/catalogs/c9/detach': detached })
    fireEvent.click(screen.getByRole('button', { name: 'Detach' }))
    fireEvent.click(within(dialog()).getByRole('button', { name: 'Detach' }))
    await waitFor(() => expect(onToast).toHaveBeenCalledWith({ text: 'Detached “Taken one”', tone: 'success' }))
    expect(onReopen).toHaveBeenCalledWith(
      expect.objectContaining({ kind: 'catalog', id: 'c9', initial: expect.objectContaining({ name: 'Updated since it opened' }) }),
    )
  })

  it('reopens a detached collection from the row the detach left', async () => {
    const detached = collection({ id: 'c9', title: 'Night v3', folders: [folder({ title: 'Added by the update' })] })
    const { onReopen } = renderHarness({ collection: takenCollection }, { 'POST /api/p/1/collections/c9/detach': detached })
    fireEvent.click(screen.getByRole('button', { name: 'Detach' }))
    fireEvent.click(within(dialog()).getByRole('button', { name: 'Detach' }))
    await waitFor(() =>
      expect(onReopen).toHaveBeenCalledWith(
        expect.objectContaining({
          kind: 'collection',
          id: 'c9',
          initial: expect.objectContaining({ title: 'Night v3', folders: [expect.objectContaining({ title: 'Added by the update' })] }),
        }),
      ),
    )
  })

  it('reopens nothing after stopping sharing', async () => {
    const live = { id: 'p', status: 'live' as const, changed_since_publish: false }
    const { onToast, onReopen } = renderHarness(
      { catalog: catalog({ id: 'c1', name: 'Horror', publication: live }) },
      { 'POST /api/p/1/catalogs/c1/withdraw': catalog({ id: 'c1' }) },
    )
    fireEvent.click(screen.getByRole('button', { name: 'Stop sharing' }))
    fireEvent.click(within(dialog()).getByRole('button', { name: 'Stop sharing' }))
    await waitFor(() => expect(onToast).toHaveBeenCalled())
    expect(onReopen).not.toHaveBeenCalled()
  })

  it('refuses to share a collection using a catalog taken from Community', () => {
    const taken = catalog({ id: 't1', name: 'Giallo', subscription })
    renderHarness(
      { collection: collection({ folders: [folder({ refs: [{ catalog_id: 't1', genre: '' }] })], catalogs: [taken] }) },
      {},
    )
    expect(screen.getByRole('button', { name: 'Share…' })).toBeDisabled()
    expect(screen.getByText(/Uses 1 catalog taken from Community: “Giallo”/)).toBeInTheDocument()
  })

  it('lists what a collection shares in the publish dialog', () => {
    const own = catalog({ id: 's1', name: 'Scoped one', collection_id: 'col1' })
    renderHarness(
      { collection: collection({ title: 'Night', folders: [folder({ refs: [{ catalog_id: 's1', genre: '' }] })], catalogs: [own] }) },
      {},
    )
    fireEvent.click(screen.getByRole('button', { name: 'Share…' }))
    expect(within(dialog()).getByText('1 folder, 1 catalog of its own')).toBeInTheDocument()
  })

  it('opens a copy’s publication in Community for its Update', () => {
    const { onOpenPublication, calls } = renderHarness({ collection: takenCollection }, {})
    fireEvent.click(screen.getByRole('button', { name: 'Update…' }))
    expect(onOpenPublication).toHaveBeenCalledWith({ id: 'pub', kind: 'collection' })
    expect(calls).toEqual([])
  })
})
