import { beforeEach, describe, expect, it, vi } from 'vitest'
import { apiFetch } from './client'
import { fetchCommunityPage, fetchWatchProviders } from './resources'

vi.mock('./client', () => ({ apiFetch: vi.fn() }))

const fetchMock = vi.mocked(apiFetch)

beforeEach(() => {
  fetchMock.mockReset()
})

describe('resources', () => {
  it('reads a Community page by kind and sort, with the search and cursor only when set', async () => {
    const page = { items: [], next_cursor: null }
    fetchMock.mockImplementation(() => Promise.resolve(Response.json(page)))
    await expect(fetchCommunityPage(2, { kind: 'catalog', sort: 'name', q: '' }, '')).resolves.toEqual(page)
    await fetchCommunityPage(2, { kind: 'collection', sort: 'newest', q: 'horror night' }, 'abc')
    expect(fetchMock.mock.calls.map(([url]) => url)).toEqual([
      '/api/p/2/community?kind=catalog&sort=name',
      '/api/p/2/community?kind=collection&sort=newest&q=horror+night&cursor=abc',
    ])
  })

  it('asks for watch providers in a region only when one is chosen', async () => {
    fetchMock.mockImplementation(() => Promise.resolve(Response.json([])))
    await fetchWatchProviders('series', '')
    await fetchWatchProviders('movie', 'GB')
    expect(fetchMock.mock.calls.map(([url]) => url)).toEqual(['/api/watch-providers/series', '/api/watch-providers/movie?region=GB'])
  })
})
