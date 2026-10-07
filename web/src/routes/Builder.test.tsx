// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, render, screen, waitFor } from '@testing-library/react'
import { RouterProvider, createMemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { Catalog } from '@/api'
import { useHomeSelection } from '@/features/home/useHomeSelection'
import { catalog } from '@/test/fixtures'
import { watchActiveProfile } from './activeProfile'
import { Builder } from './Builder'

// What each profile's server holds right now; a test changes it between visits.
const server = vi.hoisted(() => ({ catalogs: new Map<number, unknown[]>() }))

vi.mock('@/api', async () => ({
  fetchLibrary: async (profileIndex: number) => ({
    catalogs: server.catalogs.get(profileIndex) ?? [],
    collections: [],
    pending: [],
  }),
  fetchGenres: async () => [],
  fetchCertifications: async () => ({}),
  fetchLanguages: async () => [],
  fetchCountries: async () => [],
  ProfileNotSelectedError: class extends Error {},
  queryKeys: (await import('@/api/keys')).queryKeys,
}))

// Everything the builder draws around Home's state is beside the point here.
vi.mock('@/features/builder/Workspace', () => ({
  Workspace: function Workspace() {
    const home = useHomeSelection()
    return <p data-testid="rows">{home.ready ? home.rows.map((row) => row.id).join(',') || 'none' : 'loading'}</p>
  },
}))
vi.mock('@/features/community/CommunityView', () => ({ CommunityView: () => null }))
vi.mock('@/features/builder/ProfileMenu', () => ({ ProfileMenu: () => null }))
vi.mock('@/features/builder/stacked', () => ({ usePublishedHeaderHeight: () => {} }))
vi.mock('@/features/account/KeyProblemBanner', () => ({ KeyProblemBanner: () => null }))
vi.mock('@/features/push/PushBlockControls', () => ({ BlockablePushButton: () => null, PushBlockNote: () => null }))
vi.mock('@/features/push/PushControls', () => ({ AddonURLButton: () => null, ChangesStrip: () => null, PushBanner: () => null }))
vi.mock('@/features/push/usePush', () => ({ usePush: () => ({ outcome: null, dismiss: () => {} }) }))
vi.mock('@/features/push/usePushBlock', () => ({ usePushBlock: () => null }))

/** A catalog showing on Home, which is what puts its id in the pending rows. */
function onHome(id: string, name: string): Catalog {
  return catalog({ id, name, home_position: 0, show_in_home: true })
}

let queryClient: QueryClient
let unwatch: (() => void) | undefined

function profileEntry(profileIndex: number) {
  return { pathname: '/configure', state: { profileIndex, profileName: `Profile ${profileIndex}` } }
}

function open(profileIndex: number) {
  const router = createMemoryRouter(
    [
      { path: '/profiles', element: null },
      { path: '/configure', element: <Builder /> },
    ],
    { initialEntries: [profileEntry(profileIndex)] },
  )
  unwatch = watchActiveProfile(router, queryClient)
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
  return router
}

const rows = () => screen.findByTestId('rows')

beforeEach(() => {
  // The app's defaults, which keep a visited profile's rows fresh for 30s.
  queryClient = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: 30_000 } } })
  server.catalogs.clear()
})
afterEach(() => {
  unwatch?.()
  unwatch = undefined
})

describe('Builder', () => {
  it('sends a direct visit with no profile back to the picker', async () => {
    const router = createMemoryRouter(
      [
        { path: '/profiles', element: <p>Who is watching</p> },
        { path: '/configure', element: <Builder /> },
      ],
      { initialEntries: ['/configure'] },
    )
    render(
      <QueryClientProvider client={queryClient}>
        <RouterProvider router={router} />
      </QueryClientProvider>,
    )
    expect(await screen.findByText('Who is watching')).toBeInTheDocument()
    expect(router.state.location.pathname).toBe('/profiles')
  })

  it('rebuilds Home from fresh rows when it opens on a profile again', async () => {
    server.catalogs.set(1, [onHome('a', 'Alpha')])
    const router = open(1)
    await waitFor(async () => expect(await rows()).toHaveTextContent('a'))

    await act(() => router.navigate('/profiles'))
    server.catalogs.set(1, [onHome('b', 'Bravo')])
    await act(() => router.navigate(profileEntry(1).pathname, { state: profileEntry(1).state }))

    await waitFor(() => expect(screen.getByTestId('rows')).toHaveTextContent('b'))
    expect(screen.getByTestId('rows')).not.toHaveTextContent('a')
  })

  it('rebuilds the pending and baseline state when the profile changes under the mounted builder', async () => {
    server.catalogs.set(1, [onHome('a', 'Alpha')])
    server.catalogs.set(2, [onHome('x', 'Xray')])
    const router = open(1)
    await waitFor(async () => expect(await rows()).toHaveTextContent('a'))

    await act(() => router.navigate('/configure', { state: profileEntry(2).state }))

    await waitFor(() => expect(screen.getByTestId('rows')).toHaveTextContent('x'))
    expect(screen.getByTestId('rows')).not.toHaveTextContent('a')
    expect(screen.getAllByText('All pushed').length).toBeGreaterThan(0)
  })
})
