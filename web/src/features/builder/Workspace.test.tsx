// @vitest-environment jsdom
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { useReducer, type ReactNode } from 'react'
import { RouterProvider, createMemoryRouter, useLocation } from 'react-router-dom'
import { beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'
import type { Catalog, Collection } from '@/api'
import { catalog, collection } from '@/test/fixtures'
import { EditorGuardProvider } from './EditorGuard'
import { builderView } from './paths'
import { Workspace } from './Workspace'
import { ToastProvider } from '@/components/Toasts'

// Below `lg`: the layer is what these tests are about. `stacked.ts` reads
// `matchMedia` once and keeps it, so it is stubbed before anything renders.
beforeAll(() => {
  window.matchMedia = ((query: string) => ({
    matches: false,
    media: query,
    addEventListener: () => {},
    removeEventListener: () => {},
  })) as unknown as typeof window.matchMedia
  window.scrollTo = () => {}
  Element.prototype.scrollIntoView = () => {}
})

vi.mock('@/api/client', () => ({ apiFetch: vi.fn(() => new Promise(() => {})) }))

const rows = vi.hoisted(() => ({ catalogs: [] as Catalog[], collections: [] as Collection[] }))

vi.mock('@/features/library/useLibrary', () => ({
  useLibrary: () => ({
    catalogs: rows.catalogs,
    collections: rows.collections,
    genres: { movie: new Map(), tv: new Map() },
    genreLists: { movie: [], tv: [] },
    certifications: { movie: {}, tv: {} },
    languages: [],
    countryNames: new Map(),
    isLoading: false,
    loaded: true,
    error: null,
    refetch: () => {},
  }),
}))

/** A mutation whose `mutate` succeeds at once. */
function mutation() {
  return {
    mutate: (_input: unknown, options?: { onSuccess?: (result: unknown) => void }) => options?.onSuccess?.({}),
    reset: () => {},
    isPending: false,
    error: null,
  }
}

/** Every input an update mutation was called with, in order. */
const saves = vi.hoisted(() => ({ inputs: [] as unknown[] }))

/** An update whose `mutate` records its input in `saves` and succeeds at once. */
function recordedUpdate() {
  return {
    ...mutation(),
    mutate(input: unknown, options?: { onSuccess?: (result: unknown) => void }) {
      saves.inputs.push(input)
      options?.onSuccess?.({})
    },
  }
}

vi.mock('@/features/catalogs/useCatalogMutations', () => ({
  useCatalogMutations: () => ({ create: mutation(), duplicate: mutation(), update: recordedUpdate(), remove: mutation() }),
}))
vi.mock('@/features/collections/useCollectionMutations', () => ({
  useCollectionMutations: () => ({ create: mutation(), update: recordedUpdate(), remove: mutation(), duplicate: mutation() }),
}))
vi.mock('@/features/push/usePushWaiting', () => ({ usePushWaiting: () => new Set() }))
vi.mock('@/features/sharing/useWorkspaceSharing', () => ({
  useWorkspaceSharing: () => ({ catalogSharing: () => ({}), collectionSharing: () => ({}), dialogs: null }),
}))
vi.mock('@/features/bundle/ExportDialog', () => ({ ExportDialog: () => null }))
vi.mock('@/features/bundle/ImportDialog', () => ({
  ImportDialog: ({ open, onImported }: { open: boolean; onImported: (result: unknown) => void }) =>
    open ? (
      <button onClick={() => onImported({ catalogs: [{}, {}], collections: [{}] })}>Finish import</button>
    ) : null,
}))
vi.mock('./NewItemDialog', () => ({ NewItemDialog: () => null }))

vi.mock('@/features/home/HomePane', () => ({
  HomePane: ({ onShowLibrary }: { onShowLibrary: () => void }) => (
    <section aria-label="Home">
      <button onClick={onShowLibrary}>Library</button>
    </section>
  ),
}))

interface RowProps {
  onSelectCatalog: (row: Catalog) => void
  onSelectCollection: (row: Collection) => void
  onImport: () => void
  notice: ReactNode
}

vi.mock('@/features/library/LibrarySection', () => ({
  LibrarySection: ({ onSelectCatalog, onSelectCollection, onImport, notice }: RowProps) => (
    <>
      <button onClick={onImport}>Import</button>
      {notice}
      {rows.catalogs.map((row) => (
        <button key={row.id} onClick={() => onSelectCatalog(row)}>
          Open {row.name}
        </button>
      ))}
      {rows.collections.map((row) => (
        <button key={row.id} onClick={() => onSelectCollection(row)}>
          Open {row.title}
        </button>
      ))}
    </>
  ),
}))

interface EditorProps {
  initial: unknown
  onSave(value: unknown): void
  onRequestClose(): void
  onDelete?: () => void
}

/** An editor whose Save hands back the form it opened with, less its
 *  revision, as a collection editor's payload carries none: what a catalog
 *  editor saves, and as much of a collection payload as Workspace reads. */
function FakeEditor({ initial, onSave, onRequestClose, onDelete }: EditorProps) {
  return (
    <main>
      <h1 tabIndex={-1} data-landing>
        Editor
      </h1>
      <button onClick={() => onSave({ ...(initial as object), revision: undefined })}>Save row</button>
      <button onClick={onRequestClose}>Close editor</button>
      <button onClick={onDelete}>Delete row</button>
    </main>
  )
}

vi.mock('@/features/catalogs/CatalogEditor', () => ({ CatalogEditor: FakeEditor }))
vi.mock('@/features/collections/CollectionEditor', () => ({ CollectionEditor: FakeEditor }))

beforeEach(() => {
  rows.catalogs = [catalog({ id: 'c1', name: 'Noir' })]
  saves.inputs = []
  rows.collections = [collection({ id: 'k1', title: 'Night shift' })]
})

/** The Workspace on its path, as `Builder` puts it there. Rerender draws it
 *  again with the rows as they are now, as a refetch would. */
function OnPath() {
  const view = builderView(useLocation().pathname)
  const [, rerender] = useReducer((n: number) => n + 1, 0)
  return (
    <EditorGuardProvider>
      <button onClick={rerender}>Rerender</button>
      <Workspace profileIndex={1} editor={view?.editor ?? null} onOpenPublication={() => {}} />
    </EditorGuardProvider>
  )
}

function renderWorkspace(path = '/configure') {
  const router = createMemoryRouter([{ path: '/configure/*', element: <OnPath /> }], { initialEntries: [path] })
  render(
    <ToastProvider>
      <RouterProvider router={router} />
    </ToastProvider>,
  )
  return router
}

const layer = () => screen.queryByRole('dialog', { name: /Noir|Night shift/ })

describe('Workspace below lg', () => {
  it('opens a row as a layer over the page and keeps Home under it', () => {
    renderWorkspace()
    fireEvent.click(screen.getByRole('button', { name: 'Open Noir' }))
    expect(layer()).toBeInTheDocument()
    expect(screen.getByLabelText('Home')).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: 'Editor' })).toHaveFocus()

    fireEvent.click(screen.getByRole('button', { name: 'Close editor' }))
    expect(layer()).toBeNull()
  })

  it('opens a row at its own path, which the browser Back steps off', async () => {
    const router = renderWorkspace()
    fireEvent.click(screen.getByRole('button', { name: 'Open Night shift' }))
    expect(router.state.location.pathname).toBe('/configure/collection/k1')
    expect(layer()).toBeInTheDocument()
    await act(() => router.navigate(-1))
    expect(router.state.location.pathname).toBe('/configure')
    expect(layer()).toBeNull()
  })

  it('closes with a step back to Home, not a step forward onto it', () => {
    const router = renderWorkspace()
    fireEvent.click(screen.getByRole('button', { name: 'Open Noir' }))
    fireEvent.click(screen.getByRole('button', { name: 'Close editor' }))
    expect(router.state.historyAction).toBe('POP')
    expect(router.state.location.pathname).toBe('/configure')
  })

  it('opens a typed editor address, and closes it in place', () => {
    const router = renderWorkspace('/configure/catalog/c1')
    expect(layer()).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Close editor' }))
    expect(router.state.historyAction).toBe('REPLACE')
    expect(router.state.location.pathname).toBe('/configure')
  })

  it('sends the address of a row the library doesn’t hold to Home, and says so once', async () => {
    const router = renderWorkspace('/configure/catalog/gone')
    expect(router.state.location.pathname).toBe('/configure')
    expect(layer()).toBeNull()
    expect(await screen.findByText('That catalog isn’t in this profile.')).toBeInTheDocument()
    await waitFor(() => expect(router.state.location.state).toBeNull())
  })

  it.each([
    ['catalog', 'Noir'],
    ['collection', 'Night shift'],
  ])('closes the layer once its %s is deleted', (kind, name) => {
    renderWorkspace()
    fireEvent.click(screen.getByRole('button', { name: `Open ${name}` }))
    fireEvent.click(screen.getByRole('button', { name: 'Delete row' }))
    fireEvent.click(screen.getByRole('button', { name: `Delete ${kind}` }))
    expect(layer()).toBeNull()
  })

  it('scrolls between the rail and Home without an editor', () => {
    renderWorkspace()
    fireEvent.click(screen.getByRole('button', { name: /Your home screen/ }))
    fireEvent.click(screen.getByRole('button', { name: 'Library' }))
    expect(layer()).toBeNull()
  })
})

describe('Workspace delete confirms', () => {
  it('tells a collection what goes with it', () => {
    rows.collections = [collection({ id: 'k1', title: 'Night shift', home_position: 0 })]
    renderWorkspace()
    fireEvent.click(screen.getByRole('button', { name: 'Open Night shift' }))
    fireEvent.click(screen.getByRole('button', { name: 'Delete row' }))
    expect(screen.getByText('Also removes it from: Nuvio (next push)')).toBeInTheDocument()
  })
})

describe('Workspace import', () => {
  it('names what the rail gained', () => {
    renderWorkspace()
    fireEvent.click(screen.getByRole('button', { name: 'Import' }))
    fireEvent.click(screen.getByRole('button', { name: 'Finish import' }))
    expect(screen.getByText('Imported 2 catalogs and 1 collection')).toBeInTheDocument()
  })
})

describe('Workspace saves', () => {
  it('sends the revision each editor opened at, not the one a refetch brings', () => {
    rows.catalogs = [catalog({ id: 'c1', name: 'Noir', revision: 4 })]
    rows.collections = [collection({ id: 'k1', title: 'Night shift', revision: 2 })]
    renderWorkspace()
    fireEvent.click(screen.getByRole('button', { name: 'Open Noir' }))
    rows.catalogs = [catalog({ id: 'c1', name: 'Noir', revision: 5 })]
    fireEvent.click(screen.getByRole('button', { name: 'Rerender' }))
    fireEvent.click(screen.getByRole('button', { name: 'Save row' }))

    fireEvent.click(screen.getByRole('button', { name: 'Open Night shift' }))
    fireEvent.click(screen.getByRole('button', { name: 'Save row' }))

    expect(saves.inputs).toEqual([
      { id: 'c1', payload: expect.objectContaining({ name: 'Noir', revision: 4 }) },
      { id: 'k1', payload: expect.objectContaining({ title: 'Night shift', revision: 2 }) },
    ])
  })
})
