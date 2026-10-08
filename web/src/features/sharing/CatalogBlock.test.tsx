// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { failWith, fakeApi, type FakeRoute } from '@/test/fakeApi'
import { catalog } from '@/test/fixtures'
import { CatalogBlock } from './CatalogBlock'

const api = vi.hoisted(() => ({ current: null as null | ((input: RequestInfo | URL, init?: RequestInit) => Promise<Response>) }))
vi.mock('@/api/client', () => ({ apiFetch: (input: RequestInfo | URL, init?: RequestInit) => api.current!(input, init) }))

const genres = { movie: new Map<number, string>(), tv: new Map<number, string>() }

const studios = catalog({
  id: 'c',
  name: 'Ghibli and friends',
  params: JSON.stringify({ with_companies: '10342|3', without_keywords: '849', with_watch_providers: '8|337', watch_region: 'US' }),
})

function renderBlock(routes: Record<string, FakeRoute>, props: Partial<React.ComponentProps<typeof CatalogBlock>> = {}) {
  const fake = fakeApi(routes)
  api.current = fake.apiFetch
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <CatalogBlock catalog={studios} genres={genres} {...props} />
    </QueryClientProvider>,
  )
  return fake.calls
}

const value = (label: string) => screen.getByText(label, { selector: 'dt' }).nextElementSibling

beforeEach(() => {
  api.current = null
})

describe('CatalogBlock tiles', () => {
  it('put the one-value facts ahead of the lists', () => {
    const sorted = catalog({ id: 's', params: JSON.stringify({ with_companies: '10342|3', sort_by: 'vote_average.desc' }) })
    renderBlock({}, { catalog: sorted })
    const labels = screen.getAllByText(/./, { selector: 'dt' }).map((dt) => dt.textContent)
    expect(labels.slice(0, 3)).toEqual(['Type', 'Order', 'Production companies'])
  })

  it('end with the filters the recipe leaves open', () => {
    renderBlock({})
    const labels = screen.getAllByText(/./, { selector: 'dt' }).map((dt) => dt.textContent)
    expect(labels.indexOf('Genres')).toBeGreaterThan(labels.indexOf('Production companies'))
    expect(value('Genres')).toHaveTextContent('Any')
    expect(labels).not.toContain('Production company')
  })

  it('show the open filters inside a folder too', () => {
    renderBlock({}, { foldable: true })
    fireEvent.click(screen.getByRole('button', { name: /Ghibli and friends/ }))
    expect(value('Order')).toHaveTextContent('Most popular')
  })
})

describe('CatalogBlock names', () => {
  const routes = {
    'GET /api/companies/10342': { id: 10342, name: 'Studio Ghibli' },
    'GET /api/companies/3': { id: 3, name: 'Pixar' },
    'GET /api/keywords/849': { id: 849, name: 'gore' },
    'GET /api/watch-providers/movie': [
      { provider_id: 8, provider_name: 'Netflix', display_priority: 1, logo_path: '' },
      { provider_id: 337, provider_name: 'Disney Plus', display_priority: 2, logo_path: '' },
    ],
  }

  it('shows "…" for production companies and keywords while their names load, then names them', async () => {
    const calls = renderBlock(routes)
    expect(value('Production companies')).toHaveTextContent('…')
    expect(value('Left-out keyword')).toHaveTextContent('…')
    expect(await screen.findByText('Studio Ghibli or Pixar')).toBeInTheDocument()
    expect(value('Left-out keyword')).toHaveTextContent('gore')
    expect(await screen.findByText(/^Netflix or Disney Plus in /)).toBeInTheDocument()
    expect(calls).toContain('GET /api/watch-providers/movie?watch_region=US')
  })

  it('settles on the count for a list a lookup can’t name', async () => {
    renderBlock({ ...routes, 'GET /api/companies/3': () => failWith(404, 'not found') })
    expect(await screen.findByText('gore')).toBeInTheDocument()
    expect(value('Production companies')).toHaveTextContent('2')
  })

  it('loads no names for a folded block until it opens', async () => {
    const calls = renderBlock(routes, { foldable: true })
    expect(calls).toEqual([])
    fireEvent.click(screen.getByRole('button', { name: /Ghibli and friends/ }))
    expect(await screen.findByText('Studio Ghibli or Pixar')).toBeInTheDocument()
  })
})

describe('CatalogBlock marks', () => {
  const now = catalog({
    id: 'm',
    name: 'Marked',
    params: JSON.stringify({ with_genres: '28|12', vote_average_gte: 7, sort_by: 'vote_average.desc' }),
  })
  const recipe = {
    op: 'changed' as const,
    kind: 'catalog' as const,
    aspect: 'recipe' as const,
    key: 'm',
    catalog: { key: 'm', name: 'Marked', type: 'movie' as const, provider: 'tmdb', params: { with_genres: '28|12', vote_average_gte: 7, sort_by: 'vote_average.desc' } },
    was_catalog: { key: 'm', name: 'Marked', type: 'movie' as const, provider: 'tmdb', params: { with_genres: '28', vote_average_gte: 6, without_keywords: '849' } },
  }
  const lookup = { movie: new Map([[28, 'Action'], [12, 'Adventure']]), tv: new Map<number, string>() }
  const routes = { 'GET /api/keywords/849': { id: 849, name: 'gore' } }

  it('edges each tile an update changes in the accent, with what it was under the value', async () => {
    renderBlock(routes, { catalog: now, genres: lookup, mark: { words: '', recipe, id: 'update-catalog-m', startOpen: true } })
    const rating = screen.getByText('Rating', { selector: 'dt' }).parentElement!
    expect(rating.className).toContain('var(--accent)')
    expect(rating).toHaveTextContent('was 6.0 or more')
    expect(screen.getByText('Genres', { selector: 'dt' }).parentElement).toHaveTextContent('added Adventure')
    expect(screen.getByText('Order', { selector: 'dt' }).parentElement).toHaveTextContent('was Most popular')
    expect(screen.getByText('Type', { selector: 'dt' }).parentElement!.className).not.toContain('var(--accent)')
    expect(await screen.findByText('was gore')).toBeInTheDocument()
    expect(document.getElementById('update-catalog-m')).toHaveAttribute('tabindex', '-1')
  })

  it('opens a folded block on its marked tiles, with the mark’s words under its name', () => {
    renderBlock(routes, { catalog: now, genres: lookup, foldable: true, mark: { words: 'filters changed', recipe, id: 'update-catalog-m', startOpen: true } })
    expect(screen.getByRole('button', { name: /Marked/ })).toHaveAttribute('aria-expanded', 'true')
    expect(screen.getByText('filters changed')).toBeInTheDocument()
    expect(screen.getByText('Rating', { selector: 'dt' }).parentElement).toHaveTextContent('was 6.0 or more')
  })

  it('says a mark in words alone where the block holds no changed recipe', () => {
    renderBlock(routes, { catalog: now, genres: lookup, foldable: true, mark: { words: 'added', recipe: null, id: undefined, startOpen: false } })
    expect(screen.getByRole('button', { name: /Marked/ })).toHaveAttribute('aria-expanded', 'false')
    expect(screen.getByText('added')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /Marked/ }))
    expect(screen.getByText('Rating', { selector: 'dt' }).parentElement!.className).not.toContain('var(--accent)')
  })
})
