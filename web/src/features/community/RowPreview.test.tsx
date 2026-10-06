// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, fireEvent, render, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { fakeApi } from '@/test/fakeApi'
import { communityFolder, communityItem } from '@/test/fixtures'
import { RowFolders, RowPosters } from './RowPreview'
import { firstPosters, shownFolders } from './rowPreviewModel'

const api = vi.hoisted(() => ({ current: null as null | ((input: RequestInfo | URL, init?: RequestInit) => Promise<Response>) }))
vi.mock('@/api/client', () => ({ apiFetch: (input: RequestInfo | URL, init?: RequestInit) => api.current!(input, init) }))

/** An IntersectionObserver whose callback a test fires by hand. */
function stubObserver() {
  const seen: { fire: () => void } = { fire: () => {} }
  class Observer {
    constructor(callback: IntersectionObserverCallback) {
      seen.fire = () => callback([{ isIntersecting: true } as IntersectionObserverEntry], this as unknown as IntersectionObserver)
    }
    observe() {}
    disconnect() {}
  }
  vi.stubGlobal('IntersectionObserver', Observer)
  return seen
}

function renderWithQuery(ui: React.ReactElement) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(<QueryClientProvider client={client}>{ui}</QueryClientProvider>)
}

afterEach(() => vi.unstubAllGlobals())

describe('row preview model', () => {
  it('fills five faces from the first results, blank where no poster is', () => {
    expect(firstPosters(undefined)).toEqual(['', '', '', '', ''])
    const item = (poster?: string) => ({ tmdb_id: 1, title: 't', year: '2001', poster })
    const preview = { randomized: false, total_results: 3, items: [item('a.jpg'), item(), item('c.jpg')] }
    expect(firstPosters(preview)).toEqual(['a.jpg', '', 'c.jpg', '', ''])
  })

  it('shows six folders and counts the rest', () => {
    const folders = ['1', '2', '3', '4', '5', '6', '7', '8'].map((t) => communityFolder(t))
    expect(shownFolders(folders).shown.map((f) => f.title)).toEqual(['1', '2', '3', '4', '5', '6'])
    expect(shownFolders(folders).more).toBe(2)
    expect(shownFolders(folders.slice(0, 2)).more).toBe(0)
  })
})

describe('RowPosters', () => {
  it('fetches a catalog’s posters once its row is seen, and draws them in order', async () => {
    const observer = stubObserver()
    const fake = fakeApi({
      'POST /api/catalogs/preview': { randomized: false, total_results: 2, items: [{ tmdb_id: 1, title: 'A', year: '2001', poster: 'https://image.tmdb.org/a.jpg' }] },
    })
    api.current = fake.apiFetch
    const { container } = renderWithQuery(<RowPosters item={communityItem({ catalog: { key: 'k', name: 'n', type: 'movie', provider: 'tmdb', params: { sort_by: 'popularity.desc' } } })} />)
    expect(container.querySelectorAll('.row-face')).toHaveLength(5)
    expect(fake.calls).toEqual([])
    act(() => observer.fire())
    await waitFor(() => expect(container.querySelector('img')?.getAttribute('src')).toBe('https://image.tmdb.org/a.jpg'))
    expect(fake.calls).toEqual(['POST /api/catalogs/preview'])
    expect(container.querySelectorAll('img')).toHaveLength(1)
  })

  it('draws nothing for a collection, and fetches nothing where nothing can observe the row', () => {
    const fake = fakeApi({})
    api.current = fake.apiFetch
    const { container } = renderWithQuery(<RowPosters item={communityItem({ kind: 'collection', catalog: null })} />)
    expect(container.innerHTML).toBe('')
    renderWithQuery(<RowPosters item={communityItem()} />)
    expect(fake.calls).toEqual([])
  })
})

describe('RowFolders', () => {
  it('draws each folder as its tile: the cover image over the emoji over the name, and opens the page on a tap', () => {
    const onOpen = vi.fn()
    const { container } = render(
      <RowFolders
        item={communityItem({
          kind: 'collection',
          catalog: null,
          folders: [
            communityFolder('Ghosts', { cover_emoji: '👻', tile_shape: 'LANDSCAPE' }),
            communityFolder('Slashers', { cover_image_url: 'https://image.tmdb.org/s.jpg' }),
            communityFolder('  '),
          ],
        })}
        onOpen={onOpen}
      />,
    )
    const faces = container.querySelectorAll<HTMLElement>('.row-face')
    expect(faces).toHaveLength(3)
    expect(faces[0]!.textContent).toBe('👻')
    expect(faces[0]!.style.getPropertyValue('--ratio')).toBe(String(16 / 9))
    expect(faces[1]!.querySelector('img')?.getAttribute('src')).toBe('https://image.tmdb.org/s.jpg')
    expect(faces[2]!.textContent).toBe('Untitled folder')
    expect(container.textContent).not.toContain('more')
    fireEvent.click(faces[1]!)
    expect(onOpen).toHaveBeenCalledOnce()
  })

  it('says how many more folders there are past six, and draws nothing for a catalog', () => {
    const folders = ['1', '2', '3', '4', '5', '6', '7'].map((t) => communityFolder(t))
    const { container } = render(<RowFolders item={communityItem({ kind: 'collection', catalog: null, folders })} onOpen={() => {}} />)
    expect(container.textContent).toContain('+1 more')
    const catalog = render(<RowFolders item={communityItem()} onOpen={() => {}} />)
    expect(catalog.container.innerHTML).toBe('')
  })
})
