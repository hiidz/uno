// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { SnapshotChange } from '@/api'
import { fakeApi, type FakeRoute } from '@/test/fakeApi'
import { UpdateList } from './UpdateList'

const api = vi.hoisted(() => ({ current: null as null | ((input: RequestInfo | URL, init?: RequestInit) => Promise<Response>) }))
vi.mock('@/api/client', () => ({ apiFetch: (input: RequestInfo | URL, init?: RequestInit) => api.current!(input, init) }))

const genres = { movie: new Map<number, string>(), tv: new Map<number, string>() }

function renderList(changes: SnapshotChange[], routes: Record<string, FakeRoute> = {}) {
  api.current = fakeApi(routes).apiFetch
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <UpdateList changes={changes} folders={[]} genres={genres} />
    </QueryClientProvider>,
  )
}

const slashers = (params: object) => ({ key: 'rs', name: 'Retro slashers', type: 'movie' as const, provider: 'tmdb', params })

beforeEach(() => {
  api.current = null
})

describe('UpdateList', () => {
  it('lists a changed recipe’s filters, the new value with the old after “was”, named once the lookups answer', async () => {
    renderList(
      [
        {
          op: 'changed',
          kind: 'catalog',
          aspect: 'recipe',
          name: 'Retro slashers',
          was_catalog: slashers({ primary_release_date_gte: '1980-01-01', primary_release_date_lte: '1989-12-31' }),
          catalog: slashers({
            primary_release_date_gte: '1975-01-01',
            primary_release_date_lte: '1989-12-31',
            with_companies: '10342|3',
            sort_by: 'vote_average.desc',
          }),
        },
      ],
      { 'GET /api/companies/10342': { id: 10342, name: 'Studio Ghibli' }, 'GET /api/companies/3': { id: 3, name: 'Pixar' } },
    )
    expect(screen.getByText('Retro slashers')).toBeInTheDocument()
    expect(screen.getByText('changed')).toBeInTheDocument()
    expect(await screen.findByText('Studio Ghibli or Pixar')).toBeInTheDocument()
    expect(screen.getByText('Highest rated')).toBeInTheDocument()
    expect(screen.getByText('was Most popular')).toBeInTheDocument()
    expect(screen.getByText('was Any')).toBeInTheDocument()
  })

  it('says the filters changed when nothing a tile shows differs', () => {
    renderList([{ op: 'changed', kind: 'catalog', aspect: 'recipe', name: 'Same', was_catalog: slashers({}), catalog: slashers({}) }])
    expect(screen.getByText('Filters changed')).toBeInTheDocument()
  })

  it('shows six lines, then opens the rest in place', () => {
    const many: SnapshotChange[] = Array.from({ length: 8 }, (_, i) => ({ op: 'added', kind: 'catalog', name: `C${i}`, folder: 'F' }))
    renderList(many)
    expect(screen.queryByText('C6')).toBeNull()
    fireEvent.click(screen.getByRole('button', { name: 'and 2 more' }))
    expect(screen.getByText('C7')).toBeInTheDocument()
  })
})
