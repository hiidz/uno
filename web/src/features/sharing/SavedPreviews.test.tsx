// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { failWith, fakeApi } from '@/test/fakeApi'
import { SavedCatalogPreview } from './SavedPreviews'

const api = vi.hoisted(() => ({ current: null as null | ((input: RequestInfo | URL, init?: RequestInit) => Promise<Response>) }))
vi.mock('@/api/client', () => ({ apiFetch: (input: RequestInfo | URL, init?: RequestInit) => api.current!(input, init) }))

describe('SavedCatalogPreview', () => {
  it('says so when the results can’t load', async () => {
    api.current = fakeApi({ 'POST /api/catalogs/preview': () => failWith(500, 'boom') }).apiFetch
    render(
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
        <SavedCatalogPreview type="movie" params="{}" />
      </QueryClientProvider>,
    )
    expect(await screen.findByText(/Couldn’t load the preview/)).toBeInTheDocument()
  })

  it('says only that nothing matches for a saved recipe with no results', async () => {
    api.current = fakeApi({
      'POST /api/catalogs/preview': { randomized: false, items: [], total_results: 0 },
    }).apiFetch
    render(
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
        <SavedCatalogPreview type="movie" params="{}" />
      </QueryClientProvider>,
    )
    expect(await screen.findByText('Nothing matches these filters.')).toBeInTheDocument()
    expect(screen.queryByText(/loosen a filter/)).toBeNull()
  })
})
