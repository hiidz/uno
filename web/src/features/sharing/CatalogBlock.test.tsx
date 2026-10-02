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
  it('lay flat on the ground, the lists on a row of their own', () => {
    renderBlock({})
    expect(screen.getByText('Studios', { selector: 'dt' }).parentElement).toHaveClass('bg-raised', 'col-span-full')
    const type = screen.getByText('Type', { selector: 'dt' }).parentElement
    expect(type).toHaveClass('bg-raised')
    expect(type).not.toHaveClass('col-span-full')
  })

  it('step up to raised-hi in a folded block once it opens', () => {
    renderBlock({}, { foldable: true })
    fireEvent.click(screen.getByRole('button', { name: /Ghibli and friends/ }))
    expect(screen.getByText('Type', { selector: 'dt' }).parentElement).toHaveClass('bg-raised-hi')
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

  it('shows "…" for studios and keywords while their names load, then names them', async () => {
    const calls = renderBlock(routes)
    expect(value('Studios')).toHaveTextContent('…')
    expect(value('Left-out keyword')).toHaveTextContent('…')
    expect(await screen.findByText('Studio Ghibli or Pixar')).toBeInTheDocument()
    expect(value('Left-out keyword')).toHaveTextContent('gore')
    expect(await screen.findByText(/^Netflix or Disney Plus in /)).toBeInTheDocument()
    expect(calls).toContain('GET /api/watch-providers/movie?region=US')
  })

  it('settles on the count for a list a lookup can’t name', async () => {
    renderBlock({ ...routes, 'GET /api/companies/3': () => failWith(404, 'not found') })
    expect(await screen.findByText('gore')).toBeInTheDocument()
    expect(value('Studios')).toHaveTextContent('2')
  })

  it('loads no names for a folded block until it opens', async () => {
    const calls = renderBlock(routes, { foldable: true })
    expect(calls).toEqual([])
    fireEvent.click(screen.getByRole('button', { name: /Ghibli and friends/ }))
    expect(await screen.findByText('Studio Ghibli or Pixar')).toBeInTheDocument()
  })
})
