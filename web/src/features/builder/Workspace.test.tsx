// @vitest-environment jsdom
import { act, fireEvent, render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'
import type { Catalog, Collection } from '@/api'
import { catalog, collection } from '@/test/fixtures'
import { EditorGuardProvider } from './EditorGuard'
import { Workspace } from './Workspace'

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
  window.history.replaceState({ idx: 1 }, '')
})

function renderWorkspace() {
  render(
    <EditorGuardProvider>
      <Workspace profileIndex={1} onOpenPublication={() => {}} />
    </EditorGuardProvider>,
  )
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

  it('closes the layer on the browser Back', () => {
    renderWorkspace()
    fireEvent.click(screen.getByRole('button', { name: 'Open Night shift' }))
    expect(layer()).toBeInTheDocument()
    act(() => {
      window.history.replaceState({ idx: 1 }, '')
      window.dispatchEvent(new PopStateEvent('popstate', { state: { idx: 1 } }))
    })
    expect(layer()).toBeNull()
  })

  it('closes the layer once its catalog is deleted', () => {
    renderWorkspace()
    fireEvent.click(screen.getByRole('button', { name: 'Open Noir' }))
    fireEvent.click(screen.getByRole('button', { name: 'Delete row' }))
    fireEvent.click(screen.getByRole('button', { name: 'Delete catalog' }))
    expect(layer()).toBeNull()
  })

  it('closes the layer once its collection is deleted', () => {
    renderWorkspace()
    fireEvent.click(screen.getByRole('button', { name: 'Open Night shift' }))
    fireEvent.click(screen.getByRole('button', { name: 'Delete row' }))
    fireEvent.click(screen.getByRole('button', { name: 'Delete collection' }))
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
  it('asks a bare catalog only the essentials', () => {
    renderWorkspace()
    fireEvent.click(screen.getByRole('button', { name: 'Open Noir' }))
    fireEvent.click(screen.getByRole('button', { name: 'Delete row' }))
    expect(screen.getByText('Noir')).toBeInTheDocument()
    expect(screen.queryByText(/Also removes it from/)).toBeNull()
    expect(screen.getByText("This can't be undone.")).toBeInTheDocument()
  })

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
    const { rerender } = render(
      <EditorGuardProvider>
        <Workspace profileIndex={1} onOpenPublication={() => {}} />
      </EditorGuardProvider>,
    )
    fireEvent.click(screen.getByRole('button', { name: 'Open Noir' }))
    rows.catalogs = [catalog({ id: 'c1', name: 'Noir', revision: 5 })]
    rerender(
      <EditorGuardProvider>
        <Workspace profileIndex={1} onOpenPublication={() => {}} />
      </EditorGuardProvider>,
    )
    fireEvent.click(screen.getByRole('button', { name: 'Save row' }))

    fireEvent.click(screen.getByRole('button', { name: 'Open Night shift' }))
    fireEvent.click(screen.getByRole('button', { name: 'Save row' }))

    expect(saves.inputs).toEqual([
      { id: 'c1', payload: expect.objectContaining({ name: 'Noir', revision: 4 }) },
      { id: 'k1', payload: expect.objectContaining({ title: 'Night shift', revision: 2 }) },
    ])
  })
})
