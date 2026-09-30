import { beforeEach, describe, expect, it, vi } from 'vitest'
import { apiFetch } from './client'
import { fetchCommunity, fetchWatchProviders } from './resources'

vi.mock('./client', () => ({ apiFetch: vi.fn() }))

const fetchMock = vi.mocked(apiFetch)

beforeEach(() => {
  fetchMock.mockReset()
})

describe('resources', () => {
  it('reads all of Community in one call, a null list as empty', async () => {
    fetchMock.mockResolvedValueOnce(Response.json(null))
    await expect(fetchCommunity(2)).resolves.toEqual([])
    expect(fetchMock.mock.calls.map(([url]) => url)).toEqual(['/api/p/2/community'])
  })

  it('asks for watch providers in a region only when one is chosen', async () => {
    fetchMock.mockImplementation(() => Promise.resolve(Response.json([])))
    await fetchWatchProviders('series', '')
    await fetchWatchProviders('movie', 'GB')
    expect(fetchMock.mock.calls.map(([url]) => url)).toEqual(['/api/watch-providers/series', '/api/watch-providers/movie?region=GB'])
  })
})
