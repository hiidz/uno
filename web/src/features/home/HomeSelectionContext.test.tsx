// @vitest-environment jsdom
import type { ReactNode } from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, renderHook, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { Collection, SelectedCatalog } from '@/api'
import type { Library } from '@/features/library/useLibrary'
import { catalog, selectedCatalog } from '@/test/fixtures'
import { HomeSelectionProvider } from './HomeSelectionContext'
import { useHomeSelection } from './useHomeSelection'

const api = vi.hoisted(() => ({
  fetchCatalogSelection: vi.fn<(i: number) => Promise<SelectedCatalog[]>>(),
  fetchCollectionSelection: vi.fn<(i: number) => Promise<Collection[]>>(),
}))
vi.mock('@/api', async () => ({ ...api, queryKeys: (await import('@/api/keys')).queryKeys }))

const library = vi.hoisted(() => ({ current: null as unknown as Library }))
vi.mock('@/features/library/useLibrary', () => ({ useLibrary: () => library.current }))

library.current = {
  catalogs: [
    catalog({ id: 'a', name: 'Alpha' }),
    catalog({ id: 'b', name: 'Bravo' }),
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
  refetch: () => {},
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
  api.fetchCatalogSelection.mockReset().mockResolvedValue([
    selectedCatalog({ id: 'a', name: 'Alpha' }),
    selectedCatalog({ id: 'b', name: 'Bravo' }),
  ])
  api.fetchCollectionSelection.mockReset().mockResolvedValue([])
})

describe('HomeSelectionProvider', () => {
  it('ignores edits until the selection loads, then starts with nothing pending', async () => {
    const { result } = renderSelection()
    expect(result.current.ready).toBe(false)
    act(() => result.current.addCatalog('c'))

    await waitFor(() => expect(result.current.ready).toBe(true))
    expect(result.current.catalogs.map((c) => c.id)).toEqual(['a', 'b'])
    expect(result.current.pendingCount).toBe(0)
    expect(result.current.isDirty).toBe(false)
  })

  it('keeps pending edits through a refetch that brings different data', async () => {
    const { result } = await renderLoaded()
    act(() => result.current.removeCatalog('a'))
    expect(result.current.changes.map((c) => c.text)).toEqual(['Removed “Alpha” from home screen'])

    api.fetchCatalogSelection.mockResolvedValue([selectedCatalog({ id: 'c', name: 'Charlie' })])
    await act(() => queryClient.refetchQueries())
    expect(api.fetchCatalogSelection).toHaveBeenCalledTimes(2)
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

  it('reports a selection that never loaded, but not a refetch that fails later', async () => {
    api.fetchCatalogSelection.mockRejectedValueOnce(new Error('selection unavailable'))
    const failed = renderSelection()
    await waitFor(() => expect(failed.result.current.error?.message).toBe('selection unavailable'))
    expect(failed.result.current.ready).toBe(false)
    failed.unmount()

    queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const { result } = await renderLoaded()
    api.fetchCatalogSelection.mockRejectedValue(new Error('selection unavailable'))
    await act(() => queryClient.refetchQueries())
    expect(result.current.error).toBeNull()
    expect(result.current.ready).toBe(true)
  })

  it('keeps a Discover row out of the way when reordering home rows', async () => {
    const { result } = await renderLoaded()
    act(() => result.current.addCatalog('c'))
    act(() => result.current.toggleShowInHome('a'))
    act(() => result.current.reorderCatalogs(['c', 'b']))
    expect(result.current.catalogs).toEqual([
      { id: 'c', showInHome: true },
      { id: 'b', showInHome: true },
      { id: 'a', showInHome: false },
    ])
  })
})
