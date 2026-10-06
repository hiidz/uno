// @vitest-environment jsdom
import type { ReactNode } from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, renderHook, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { PendingChange } from '@/api'
import type { Library, LibraryCollection } from '@/features/library/useLibrary'
import { catalog, collection } from '@/test/fixtures'
import { HomeSelectionProvider } from './HomeSelectionContext'
import { useHomeSelection } from './useHomeSelection'

const api = vi.hoisted(() => ({
  fetchPendingPush: vi.fn<(i: number) => Promise<PendingChange[]>>(),
}))
vi.mock('@/api', async () => ({ ...api, queryKeys: (await import('@/api/keys')).queryKeys }))

const library = vi.hoisted(() => ({ current: null as unknown as Library }))
vi.mock('@/features/library/useLibrary', () => ({ useLibrary: () => library.current }))

/** The owned lists as the server reads them: Alpha and Bravo on Home, Charlie
 *  off it. */
const LOADED: Library = {
  catalogs: [
    catalog({ id: 'a', name: 'Alpha', home_position: 0, show_in_home: true }),
    catalog({ id: 'b', name: 'Bravo', home_position: 1, show_in_home: true }),
    catalog({ id: 'c', name: 'Charlie' }),
  ],
  collections: [],
  genres: { movie: new Map(), tv: new Map() },
  genreLists: { movie: [], tv: [] },
  certifications: { movie: {}, tv: {} },
  languages: [],
  countryNames: new Map(),
  isLoading: false,
  error: null,
  failed: { catalogs: false, collections: false },
  listsLoaded: true,
  refetch: () => {},
}

function libraryCollection(overrides: Parameters<typeof collection>[0]): LibraryCollection {
  return { ...collection(overrides), folders: [] }
}

let queryClient: QueryClient

function renderSelection() {
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>
      <HomeSelectionProvider profileIndex={0}>{children}</HomeSelectionProvider>
    </QueryClientProvider>
  )
  return renderHook(() => useHomeSelection(), { wrapper })
}

async function renderLoaded() {
  const rendered = renderSelection()
  await waitFor(() => expect(rendered.result.current.ready).toBe(true))
  return rendered
}

beforeEach(() => {
  queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  library.current = LOADED
  api.fetchPendingPush.mockReset().mockResolvedValue([])
})

describe('HomeSelectionProvider', () => {
  it('ignores edits until both owned lists load, then starts with nothing pending', async () => {
    library.current = { ...LOADED, catalogs: [], isLoading: true, listsLoaded: false }
    const { result, rerender } = renderSelection()
    expect(result.current.ready).toBe(false)
    act(() => result.current.addCatalog('c'))

    library.current = LOADED
    rerender()
    await waitFor(() => expect(result.current.ready).toBe(true))
    expect(result.current.catalogs.map((c) => c.id)).toEqual(['a', 'b'])
    expect(result.current.pendingCount).toBe(0)
    expect(result.current.isDirty).toBe(false)
  })

  it('waits for the collections too while the catalogs have loaded', () => {
    library.current = { ...LOADED, listsLoaded: false }
    const { result } = renderSelection()
    expect(result.current.ready).toBe(false)
    expect(result.current.rows).toEqual([])
  })

  it('keeps pending edits through a refetch that brings different data', async () => {
    const { result, rerender } = await renderLoaded()
    act(() => result.current.removeCatalog('a'))
    expect(result.current.changes.map((c) => c.text)).toEqual(['Removed “Alpha” from home screen'])

    library.current = {
      ...LOADED,
      catalogs: [
        catalog({ id: 'a', name: 'Alpha' }),
        catalog({ id: 'b', name: 'Bravo' }),
        catalog({ id: 'c', name: 'Charlie', home_position: 0, show_in_home: true }),
      ],
    }
    rerender()
    expect(result.current.catalogs.map((c) => c.id)).toEqual(['b'])
    expect(result.current.changes.map((c) => c.text)).toEqual(['Removed “Alpha” from home screen'])
  })

  it('keeps an edit made while a push was in flight pending once it lands', async () => {
    const { result } = await renderLoaded()
    act(() => result.current.removeCatalog('a'))
    const sent = result.current.snapshot()
    act(() => result.current.addCatalog('c'))

    act(() => result.current.markPushed(sent))
    expect(result.current.changes.map((c) => c.text)).toEqual(['Added “Charlie”, 2nd on your home screen'])

    act(() => result.current.markPushed(result.current.snapshot()))
    expect(result.current.isDirty).toBe(false)
  })

  it('counts a saved but unpushed change as pending, but not as dirty', async () => {
    library.current = { ...LOADED, collections: [libraryCollection({ id: 'stale', title: 'Stale', home_position: 2 })] }
    api.fetchPendingPush.mockResolvedValue([{ kind: 'collection', id: 'stale', name: 'Stale', change: 'changed' }])
    const { result } = await renderLoaded()
    await waitFor(() => expect(result.current.pendingCount).toBe(1))
    expect(result.current.changes.map((c) => c.text)).toEqual(['“Stale” changed since it was last pushed'])
    expect(result.current.unsavedCount).toBe(0)
    expect(result.current.isDirty).toBe(false)

    act(() => result.current.removeCatalog('a'))
    expect(result.current.pendingCount).toBe(2)
    expect(result.current.unsavedCount).toBe(1)
    expect(result.current.isDirty).toBe(true)
  })

  it('counts an edited catalog on the home screen as waiting for a push', async () => {
    api.fetchPendingPush.mockResolvedValue([{ kind: 'catalog', id: 'a', name: 'Alpha', change: 'changed' }])
    const { result } = await renderLoaded()
    await waitFor(() => expect(result.current.pendingCount).toBe(1))
    expect(result.current.changes.map((c) => c.text)).toEqual(['“Alpha” changed since it was last pushed'])
    expect([...result.current.waitingForPush]).toEqual(['a'])
  })

  it('flags no row for a removal, which has no row left', async () => {
    api.fetchPendingPush.mockResolvedValue([{ kind: 'catalog', id: 'gone', name: 'Gone', change: 'removed' }])
    const { result } = await renderLoaded()
    await waitFor(() => expect(result.current.pendingCount).toBe(1))
    expect(result.current.waitingForPush.size).toBe(0)
  })

  it('drops a row deleted elsewhere from the pending selection, so Push can still go, and lists its removal', async () => {
    const { result, rerender } = await renderLoaded()
    act(() => result.current.addCatalog('c'))

    api.fetchPendingPush.mockResolvedValue([{ kind: 'catalog', id: 'a', name: 'Alpha', change: 'removed' }])
    library.current = { ...LOADED, catalogs: LOADED.catalogs.filter((c) => c.id !== 'a') }
    rerender()
    await act(() => queryClient.refetchQueries())

    await waitFor(() => expect(result.current.catalogs.map((c) => c.id)).toEqual(['b', 'c']))
    expect(result.current.snapshot().rows.map((row) => row.id)).toEqual(['b', 'c'])
    expect(result.current.changes.map((c) => c.text)).toEqual([
      'Added “Charlie”, 2nd on your home screen',
      'Removed “Alpha” from home screen',
    ])
  })

  it('reports an owned list that never loaded, but not a refetch that fails later', async () => {
    const unavailable = new Error('collections unavailable')
    library.current = {
      ...LOADED,
      collections: [],
      error: unavailable,
      failed: { catalogs: false, collections: true },
      listsLoaded: false,
    }
    const failed = renderSelection()
    expect(failed.result.current.error).toBe(unavailable)
    expect(failed.result.current.ready).toBe(false)
    failed.unmount()

    library.current = LOADED
    const { result, rerender } = await renderLoaded()
    library.current = { ...LOADED, error: unavailable, failed: { catalogs: false, collections: true } }
    rerender()
    expect(result.current.error).toBeNull()
    expect(result.current.ready).toBe(true)
  })

  it('keeps a Discover row out of the way when reordering home rows', async () => {
    const { result } = await renderLoaded()
    act(() => result.current.addCatalog('c'))
    act(() => result.current.toggleShowInHome('a'))
    act(() => result.current.reorderBand('home', ['c', 'b']))
    expect(result.current.catalogs).toEqual([
      { kind: 'catalog', id: 'c', showInHome: true },
      { kind: 'catalog', id: 'b', showInHome: true },
      { kind: 'catalog', id: 'a', showInHome: false },
    ])
  })

  it('starts each collection from the pin it was pushed with, and holds its pin as a pending edit', async () => {
    library.current = {
      ...LOADED,
      collections: [
        libraryCollection({ id: 'x', title: 'X-ray', pin_to_top: true, home_position: 2 }),
        libraryCollection({ id: 'y', title: 'Yankee', home_position: 3 }),
        libraryCollection({ id: 'z', title: 'Zulu', pin_to_top: true }),
      ],
    }
    const { result } = await renderLoaded()
    expect(result.current.collections).toEqual([
      { kind: 'collection', id: 'x', pinToTop: true },
      { kind: 'collection', id: 'y', pinToTop: false },
    ])

    act(() => result.current.togglePinToTop('y'))
    act(() => result.current.addCollection('z'))
    expect(result.current.collections).toEqual([
      { kind: 'collection', id: 'x', pinToTop: true },
      { kind: 'collection', id: 'y', pinToTop: true },
      { kind: 'collection', id: 'z', pinToTop: true },
    ])
    expect(result.current.changes.map((c) => c.text)).toEqual([
      'Added “Zulu”, 3rd on your home screen',
      'Pinned “Yankee”',
    ])

    act(() => result.current.moveRow('z', -1))
    expect(result.current.collections.map((c) => c.id)).toEqual(['x', 'z', 'y'])
    act(() => result.current.reorderBand('pinned', ['y', 'x', 'z']))
    expect(result.current.collections.map((c) => c.id)).toEqual(['y', 'x', 'z'])
  })

  it('hydrates Home by each row’s position and mixes catalogs and collections in one order', async () => {
    library.current = {
      ...LOADED,
      catalogs: [
        catalog({ id: 'a', name: 'Alpha', home_position: 1, show_in_home: true }),
        catalog({ id: 'b', name: 'Bravo', home_position: 3, show_in_home: true }),
      ],
      collections: [
        libraryCollection({ id: 'x', title: 'X-ray', home_position: 0 }),
        libraryCollection({ id: 'y', title: 'Yankee', home_position: 2 }),
      ],
    }
    const { result } = await renderLoaded()
    expect(result.current.rows.map((row) => row.id)).toEqual(['x', 'a', 'y', 'b'])

    act(() => result.current.moveRow('b', -1))
    expect(result.current.rows.map((row) => row.id)).toEqual(['x', 'a', 'b', 'y'])
    expect(result.current.changes.map((c) => c.text)).toEqual(['Moved “Bravo” from 4th to 3rd'])
  })
})
