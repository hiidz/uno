// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { PublicationDetail } from '@/api'
import { failWith, fakeApi, type FakeRoute } from '@/test/fakeApi'
import { communityItem } from '@/test/fixtures'
import type { OpenPublication } from './communityQuery'
import { CommunityView } from './CommunityView'

const api = vi.hoisted(() => ({ current: null as null | ((input: RequestInfo | URL, init?: RequestInit) => Promise<Response>) }))
vi.mock('@/api/client', () => ({ apiFetch: (input: RequestInfo | URL, init?: RequestInit) => api.current!(input, init) }))

const night = communityItem({
  id: 'night',
  kind: 'collection',
  title: 'Horror Nights',
  catalog: null,
  catalog_names: ['Slasher classics'],
  folder_count: 1,
  catalog_count: 1,
  subscriber_count: 4,
  subscribed: true,
  update_available: true,
})
const a24 = communityItem({ id: 'a24', title: 'A24 Horror', catalog_names: ['A24 Horror'], published_at: '2026-09-01T10:00:00Z' })
const zombies = communityItem({ id: 'zombies', title: 'Zombies', catalog_names: ['Zombies'], published_at: '2026-09-25T10:00:00Z' })

const nightDetail: PublicationDetail = {
  ...night,
  unpublished: false,
  snapshot: {
    format: 'uno-publication',
    version: 1,
    catalogs: [{ key: 'k1', name: 'Slasher classics', type: 'movie', provider: 'tmdb', params: {} }],
    collection: {
      title: 'Horror Nights',
      view_mode: 'ROWS',
      show_all_tab: false,
      backdrop_image_url: '',
      focus_glow_enabled: false,
      folders: [
        {
          key: 'f1',
          title: 'Slashers',
          tile_shape: 'POSTER',
          hide_title: false,
          cover_emoji: '',
          cover_image_url: '',
          focus_gif_url: '',
          focus_gif_enabled: false,
          hero_backdrop_url: '',
          hero_video_url: '',
          title_logo_url: '',
          refs: [{ catalog: 'k1', genre: 'Slasher' }],
        },
      ],
    },
  },
}

/** The library's two lists answer empty: every action settles only once
 *  they have refetched. */
function renderView(routes: Record<string, FakeRoute>, initialOpen: OpenPublication | null = null) {
  const fake = fakeApi({ 'GET /api/p/1/catalogs': [], 'GET /api/p/1/collections': [], ...routes })
  api.current = fake.apiFetch
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <MemoryRouter initialEntries={['/configure']}>
        <Routes>
          <Route path="/configure" element={<CommunityView profileIndex={1} initialOpen={initialOpen} />} />
          <Route path="/profiles" element={<p>Pick a profile</p>} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  )
  return fake.calls
}

const rowButton = (id: string) => document.getElementById(`community-row-${id}`)
const titles = () => [...document.querySelectorAll('[id^="community-row-"] .font-bold')].map((el) => el.textContent)

beforeEach(() => {
  api.current = null
})

describe('CommunityView', () => {
  it('lists one kind at a time in one call, with each row’s summary and meta', async () => {
    const calls = renderView({ 'GET /api/p/1/community': [night, a24, zombies] })
    expect(await screen.findByText('A24 Horror')).toBeInTheDocument()
    expect(titles()).toEqual(['A24 Horror', 'Zombies'])
    expect(screen.queryByText('Horror Nights')).toBeNull()

    fireEvent.click(screen.getByRole('button', { name: 'Collections' }))
    expect(screen.getByText('Horror Nights')).toBeInTheDocument()
    expect(screen.getByText('1 folder · 1 catalog')).toBeInTheDocument()
    expect(screen.getByText(/Added by 4/)).toBeInTheDocument()
    expect(screen.getByText('Update available')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Update…' })).toBeEnabled()
    expect(calls.filter((call) => call.startsWith('GET /api/p/1/community'))).toEqual(['GET /api/p/1/community'])
  })

  it('searches titles and catalog names, and sorts by name or newest', async () => {
    renderView({ 'GET /api/p/1/community': [night, a24, zombies] })
    await screen.findByText('A24 Horror')
    fireEvent.click(screen.getByRole('button', { name: 'Newest' }))
    expect(titles()).toEqual(['Zombies', 'A24 Horror'])

    fireEvent.change(screen.getByLabelText('Search Community'), { target: { value: 'ZOMB' } })
    expect(titles()).toEqual(['Zombies'])
    expect(screen.getByText('1 of 2 catalogs')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Collections' }))
    fireEvent.change(screen.getByLabelText('Search Community'), { target: { value: 'slasher' } })
    expect(titles()).toEqual(['Horror Nights'])
  })

  it('says when nothing is published, and when nothing matches', async () => {
    renderView({ 'GET /api/p/1/community': [a24] })
    await screen.findByText('A24 Horror')
    fireEvent.click(screen.getByRole('button', { name: 'Collections' }))
    expect(screen.getByText(/Nobody has published any collections yet/)).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Catalogs' }))
    fireEvent.change(screen.getByLabelText('Search Community'), { target: { value: 'nothing like it' } })
    expect(screen.getByText('No catalogs match this search.')).toBeInTheDocument()
  })

  it('adds it, and the row shows it added once the list refetches', async () => {
    let added = false
    renderView({
      'GET /api/p/1/community': () => [{ ...a24, subscribed: added }],
      'POST /api/p/1/community/a24/subscribe': () => {
        added = true
        return { kind: 'catalog' }
      },
    })
    fireEvent.click(await screen.findByRole('button', { name: 'Add' }))
    expect(await screen.findByText('Added to your catalogs')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Added' })).toBeDisabled()
  })

  it('says what a stale or failed Add means', async () => {
    let answer = failWith(409, 'already subscribed')
    renderView({
      'GET /api/p/1/community': [a24],
      'POST /api/p/1/community/a24/subscribe': () => answer,
    })
    fireEvent.click(await screen.findByRole('button', { name: 'Add' }))
    expect(await screen.findByText('Already added')).toBeInTheDocument()

    answer = failWith(404, 'publication not found')
    fireEvent.click(await screen.findByRole('button', { name: 'Add' }))
    expect(await screen.findByText('Its publisher unpublished it.')).toBeInTheDocument()

    answer = failWith(500, 'database locked')
    fireEvent.click(await screen.findByRole('button', { name: 'Add' }))
    expect(await screen.findByText('Couldn’t add it: database locked')).toBeInTheDocument()
  })

  it('duplicates from the ⋯ menu', async () => {
    const calls = renderView({
      'GET /api/p/1/community': [a24],
      'POST /api/p/1/community/a24/duplicate': { kind: 'catalog' },
    })
    const more = await screen.findByRole('button', { name: 'More for A24 Horror' })
    fireEvent.pointerDown(more, { button: 0, ctrlKey: false, pointerType: 'mouse' })
    fireEvent.click(await screen.findByRole('menuitem', { name: 'Duplicate' }))
    expect(await screen.findByText('Duplicated to your catalogs')).toBeInTheDocument()
    expect(calls).toContain('POST /api/p/1/community/a24/duplicate')
  })

  it('opens a publication’s page, and comes back to its row', async () => {
    renderView({
      'GET /api/p/1/community': [night, a24],
      'GET /api/p/1/community/night': nightDetail,
    })
    await screen.findByText('A24 Horror')
    fireEvent.click(screen.getByRole('button', { name: 'Collections' }))
    fireEvent.click(screen.getByText('Horror Nights'))
    expect((await screen.findAllByText('Slashers')).length).toBeGreaterThan(0)
    expect(screen.getByText(/Slasher classics/)).toBeInTheDocument()
    expect(screen.queryByLabelText('Search Community')).toBeNull()
    expect(screen.getByRole('heading', { name: 'Horror Nights' })).toHaveFocus()

    fireEvent.click(screen.getByRole('button', { name: 'Back to Community' }))
    expect(await screen.findByLabelText('Search Community')).toBeInTheDocument()
    await waitFor(() => expect(rowButton('night')).toHaveFocus())

    fireEvent.click(screen.getByRole('button', { name: 'Catalogs' }))
    fireEvent.click(screen.getByText('A24 Horror'))
    await screen.findByRole('button', { name: 'Back to Community' })
    act(() => {
      fireEvent.keyDown(document, { key: 'Escape' })
    })
    expect(await screen.findByLabelText('Search Community')).toBeInTheDocument()
  })

  it('says when a publication’s page can’t load', async () => {
    renderView({
      'GET /api/p/1/community': [a24],
      'GET /api/p/1/community/a24': () => failWith(500, 'boom'),
    })
    fireEvent.click(await screen.findByText('A24 Horror'))
    expect(await screen.findByText('Couldn’t load this. Its publisher may have unpublished it.')).toBeInTheDocument()
  })

  it('shows a catalog publication’s recipe on its page', async () => {
    renderView({
      'GET /api/p/1/community': [a24],
      'GET /api/p/1/community/a24': { ...a24, unpublished: false, snapshot: { format: 'uno-publication', version: 1, catalogs: [a24.catalog!] } },
    })
    fireEvent.click(await screen.findByText('A24 Horror'))
    expect(await screen.findByText('One page of results')).toBeInTheDocument()
    expect(screen.getByText('Type', { selector: 'dt' })).toBeInTheDocument()
    expect(screen.getByText('Movies', { selector: 'dd' })).toBeInTheDocument()
  })

  it('opens a collection publication’s catalogs in place, as the view of an added row does', async () => {
    renderView({
      'GET /api/p/1/community': [night],
      'GET /api/p/1/community/night': nightDetail,
    })
    fireEvent.click(await screen.findByRole('button', { name: 'Collections' }))
    fireEvent.click(screen.getByText('Horror Nights'))
    const catalog = await screen.findByRole('button', { name: /Slasher classics/ })
    expect(catalog).toHaveAttribute('aria-expanded', 'false')
    fireEvent.click(catalog)
    expect(catalog).toHaveAttribute('aria-expanded', 'true')
    expect(screen.getByText('Narrowed to', { selector: 'dt' })).toBeInTheDocument()
  })

  it('explains Add and Duplicate in the row’s tip', async () => {
    renderView({ 'GET /api/p/1/community': [a24] })
    await screen.findByText('A24 Horror')
    fireEvent.click(screen.getAllByRole('button', { name: 'About add' })[0])
    expect(
      await screen.findByText(
        'Add puts it in your library, read-only, and it gets its publisher’s updates. Duplicate (⋯) makes a copy that’s yours to edit.',
      ),
    ).toBeInTheDocument()
  })

  it('draws nothing for a publication whose snapshot holds no catalog', async () => {
    renderView({
      'GET /api/p/1/community': [a24],
      'GET /api/p/1/community/a24': { ...a24, unpublished: false, snapshot: { format: 'uno-publication', version: 1, catalogs: null } },
    })
    fireEvent.click(await screen.findByText('A24 Horror'))
    await screen.findByRole('button', { name: 'Back to Community' })
    expect(screen.queryByText('One page of results')).toBeNull()
  })

  it('opens the new version from Update…, and applies it from its page', async () => {
    let updated = false
    const calls = renderView({
      'GET /api/p/1/community': () => [{ ...night, update_available: !updated }],
      'GET /api/p/1/community/night': nightDetail,
      'POST /api/p/1/community/night/update': () => {
        updated = true
        return { kind: 'collection' }
      },
    })
    await screen.findByRole('heading', { name: 'Community' })
    fireEvent.click(await screen.findByRole('button', { name: 'Collections' }))
    fireEvent.click(screen.getByRole('button', { name: 'Update…' }))
    expect((await screen.findAllByText('Slashers')).length).toBeGreaterThan(0)
    expect(calls).not.toContain('POST /api/p/1/community/night/update')

    fireEvent.click(screen.getByRole('button', { name: 'Update' }))
    expect(await screen.findByText('Updated “Horror Nights”')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Added' })).toBeDisabled()
    expect(screen.getByRole('heading', { name: 'Horror Nights' })).toBeInTheDocument()
  })

  it('starts on a publication’s page, and comes back to its kind’s list', async () => {
    renderView(
      {
        'GET /api/p/1/community': [night, a24],
        'GET /api/p/1/community/night': nightDetail,
      },
      { id: 'night', kind: 'collection' },
    )
    expect(await screen.findByRole('heading', { name: 'Horror Nights' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Update' })).toBeEnabled()
    fireEvent.click(screen.getByRole('button', { name: 'Back to Community' }))
    expect(await screen.findByText('Horror Nights')).toBeInTheDocument()
    expect(screen.queryByText('A24 Horror')).toBeNull()
  })

  it('sends an unselected profile back to the picker', async () => {
    renderView({ 'GET /api/p/1/community': () => failWith(404, 'profile not selected') })
    expect(await screen.findByText('Pick a profile')).toBeInTheDocument()
  })
})
