// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { RouterProvider, createMemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { Catalog } from '@/api'
import { useHomeEdits, useHomeSelection } from '@/features/home/useHomeSelection'
import { catalog } from '@/test/fixtures'
import { pickProfile, watchActiveProfile } from './activeProfile'
import { Builder } from './Builder'

// What each profile's server holds right now; a test changes it between visits.
const server = vi.hoisted(() => ({ catalogs: new Map<number, unknown[]>(), profilesDown: false }))
// Each test signs in as an account of its own, so no test inherits another's
// picked profile.
const auth = vi.hoisted(() => ({ account: '', count: 0 }))

vi.mock('@/auth', () => ({ useAuth: () => ({ status: 'authenticated', user: { id: auth.account } }) }))

vi.mock('@/api', async () => ({
  ApiError: class extends Error {},
  // Nuvio has profiles 1 and 2 on this account.
  fetchProfiles: async () => {
    if (server.profilesDown) throw new Error('Nuvio is down')
    return [1, 2].map((profile_index) => ({ profile_index, name: `Profile ${profile_index}`, uses_primary_addons: false }))
  },
  selectProfile: async (profileIndex: number) => ({ manifest_url: `https://uno.test/u/${profileIndex}/manifest.json` }),
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
vi.mock('@/features/builder/Configure', () => ({
  Configure: function Configure() {
    const home = useHomeSelection()
    const edits = useHomeEdits()
    return (
      <>
        <p data-testid="rows">{home.ready ? home.rows.map((row) => row.id).join(',') || 'none' : 'loading'}</p>
        <button type="button" onClick={() => edits.removeCatalog(home.rows[0].id)}>
          Remove the first row
        </button>
      </>
    )
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

const routes = [
  { path: '/profiles', element: <p>Who is watching</p> },
  { path: '/login', element: <p>Sign in</p> },
  { element: <Builder />, children: [{ path: '/configure' }, { path: '/configure/catalog/:id' }, { path: '/community' }] },
]

/** The builder at `path`, with `profile` picked first, or none. */
function open(profile: number | null, path = '/configure') {
  const router = createMemoryRouter(routes, { initialEntries: [path] })
  unwatch = watchActiveProfile(queryClient)
  if (profile !== null) pickProfile(auth.account, profile)
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
  server.profilesDown = false
  auth.count += 1
  auth.account = `account-${auth.count}`
})
afterEach(() => {
  unwatch?.()
  unwatch = undefined
})

describe('Builder', () => {
  it.each([
    ['none is picked', null],
    ['the one picked isn’t the account’s', 3],
  ])('sends the address to the picker and keeps it when %s', async (_, profile) => {
    const router = open(profile, '/configure/catalog/c1')
    expect(await screen.findByText('Who is watching')).toBeInTheDocument()
    expect(router.state.location.pathname).toBe('/profiles')
    expect(router.state.location.state).toMatchObject({ from: { pathname: '/configure/catalog/c1' } })
  })

  it('rebuilds Home from fresh rows when it opens on a profile again', async () => {
    server.catalogs.set(1, [onHome('a', 'Alpha')])
    const router = open(1)
    await waitFor(async () => expect(await rows()).toHaveTextContent('a'))

    await act(() => router.navigate('/profiles'))
    server.catalogs.set(1, [onHome('b', 'Bravo')])
    pickProfile(auth.account, 1)
    await act(() => router.navigate('/configure'))

    await waitFor(() => expect(screen.getByTestId('rows')).toHaveTextContent('b'))
    expect(screen.getByTestId('rows')).not.toHaveTextContent('a')
  })

  it('rebuilds the pending and baseline state when the profile changes under the mounted builder', async () => {
    server.catalogs.set(1, [onHome('a', 'Alpha')])
    server.catalogs.set(2, [onHome('x', 'Xray')])
    open(1)
    await waitFor(async () => expect(await rows()).toHaveTextContent('a'))

    act(() => pickProfile(auth.account, 2))

    await waitFor(() => expect(screen.getByTestId('rows')).toHaveTextContent('x'))
    expect(screen.getByTestId('rows')).not.toHaveTextContent('a')
    expect(screen.getAllByText('All pushed').length).toBeGreaterThan(0)
  })

  it('asks before leaving the builder with unpushed changes, and stays on Stay here', async () => {
    server.catalogs.set(1, [onHome('a', 'Alpha')])
    const router = open(1)
    await waitFor(async () => expect(await rows()).toHaveTextContent('a'))
    fireEvent.click(screen.getByRole('button', { name: 'Remove the first row' }))

    await act(() => router.navigate('/profiles'))
    fireEvent.click(await screen.findByRole('button', { name: 'Stay here' }))
    expect(router.state.location.pathname).toBe('/configure')
    expect(screen.getByTestId('rows')).toHaveTextContent('none')

    await act(() => router.navigate('/profiles'))
    fireEvent.click(await screen.findByRole('button', { name: 'Discard and leave' }))
    await waitFor(() => expect(router.state.location.pathname).toBe('/profiles'))
  })

  it('opens the leave question on Stay here, and stays on Escape without stalling the next move', async () => {
    server.catalogs.set(1, [onHome('a', 'Alpha')])
    const router = open(1)
    await waitFor(async () => expect(await rows()).toHaveTextContent('a'))
    fireEvent.click(screen.getByRole('button', { name: 'Remove the first row' }))

    await act(() => router.navigate('/profiles'))
    const question = await screen.findByRole('alertdialog', { name: 'Discard unpushed changes?' })
    await waitFor(() => expect(screen.getByRole('button', { name: 'Stay here' })).toHaveFocus())
    fireEvent.keyDown(question, { key: 'Escape' })
    await waitFor(() => expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument())
    expect(router.state.location.pathname).toBe('/configure')

    await act(() => router.navigate('/profiles'))
    fireEvent.click(await screen.findByRole('button', { name: 'Discard and leave' }))
    await waitFor(() => expect(router.state.location.pathname).toBe('/profiles'))
  })

  it('keeps unpushed changes, without asking, across a move to Community and back', async () => {
    server.catalogs.set(1, [onHome('a', 'Alpha')])
    const router = open(1)
    await waitFor(async () => expect(await rows()).toHaveTextContent('a'))
    fireEvent.click(screen.getByRole('button', { name: 'Remove the first row' }))

    await act(() => router.navigate('/community'))
    await act(() => router.navigate(-1))
    expect(router.state.location.pathname).toBe('/configure')
    expect(screen.queryByText('Discard unpushed changes?')).not.toBeInTheDocument()
    expect(await rows()).toHaveTextContent('none')
  })

  it('stays up, unpushed changes and all, when a refetch of the profiles fails', async () => {
    server.catalogs.set(1, [onHome('a', 'Alpha')])
    open(1)
    await waitFor(async () => expect(await rows()).toHaveTextContent('a'))
    fireEvent.click(screen.getByRole('button', { name: 'Remove the first row' }))

    server.profilesDown = true
    await act(() => queryClient.refetchQueries({ queryKey: ['profiles'] }))
    await waitFor(() => expect(queryClient.getQueryState(['profiles'])?.status).toBe('error'))
    expect(screen.queryByText("Couldn't open this profile")).not.toBeInTheDocument()
    expect(screen.getByTestId('rows')).toHaveTextContent('none')
  })

  it('says when the profile can’t load at all, and offers to try again', async () => {
    server.profilesDown = true
    open(1)
    expect(await screen.findByText("Couldn't open this profile")).toBeInTheDocument()
    server.profilesDown = false
    fireEvent.click(screen.getByRole('button', { name: 'Try again' }))
    expect(await rows()).toBeInTheDocument()
  })

  it('goes to sign-in unasked when the session ends', async () => {
    server.catalogs.set(1, [onHome('a', 'Alpha')])
    const router = open(1)
    await waitFor(async () => expect(await rows()).toHaveTextContent('a'))
    fireEvent.click(screen.getByRole('button', { name: 'Remove the first row' }))

    await act(() => router.navigate('/login', { replace: true }))
    expect(router.state.location.pathname).toBe('/login')
    expect(screen.queryByText('Discard unpushed changes?')).not.toBeInTheDocument()
  })
})
