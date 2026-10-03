// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, within } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ApiError } from '@/api/http'
import type { NuvioProfile, ServerConfig, TMDBKeyStatus } from '@/api'
import { ProfilePicker } from './ProfilePicker'

const api = vi.hoisted(() => ({
  fetchProfiles: vi.fn<() => Promise<NuvioProfile[]>>(),
  selectProfile: vi.fn(),
  fetchServerConfig: vi.fn<() => Promise<ServerConfig>>(),
  fetchTMDBKey: vi.fn<() => Promise<TMDBKeyStatus>>(),
  saveTMDBKey: vi.fn<(key: string) => Promise<TMDBKeyStatus>>(),
  removeTMDBKey: vi.fn<() => Promise<null>>(),
}))

// Only the profile and key calls are faked; the error class and query keys
// are the real ones, from their own modules, so the barrel's auth session and
// router stay unloaded.
vi.mock('@/api', async () => ({
  ApiError: (await import('@/api/http')).ApiError,
  queryKeys: (await import('@/api/keys')).queryKeys,
  ...api,
}))
vi.mock('@/api/client', () => ({ apiFetch: vi.fn() }))
vi.mock('@/auth', () => ({ logout: vi.fn() }))
vi.mock('./Builder', () => ({}))

function renderPicker() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>
        <ProfilePicker />
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

const main: NuvioProfile = {
  id: 'p1',
  user_id: 'u',
  profile_index: 1,
  name: 'Main',
  uses_primary_addons: false,
  avatar_color_hex: '#1E88E5',
  avatar_image_url: '',
  pin_enabled: false,
}

beforeEach(() => {
  for (const fn of Object.values(api)) fn.mockReset()
  api.fetchServerConfig.mockResolvedValue({ tmdb_key_mode: 'shared' })
})

describe('ProfilePicker', () => {
  it('lists the account’s profiles', async () => {
    api.fetchProfiles.mockResolvedValue([main])
    renderPicker()
    expect(await screen.findByRole('button', { name: /Profile 1, Main/ })).toBeInTheDocument()
  })

  it('marks a profile that uses profile 1’s addons in Nuvio, and still opens it', async () => {
    const kids = { ...main, id: 'p2', profile_index: 2, name: 'Kids', uses_primary_addons: true }
    api.fetchProfiles.mockResolvedValue([main, kids])
    renderPicker()
    const card = await screen.findByRole('button', { name: /Profile 2, Kids, uses profile 1’s addons in Nuvio/ })
    expect(card).toBeEnabled()
    expect(within(card).getByText(/Uses profile 1’s addons in Nuvio/)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Profile 1, Main' })).toBeEnabled()
  })

  it('draws each profile as Nuvio does: its picture, else its colour and initial, and a PIN badge', async () => {
    const pictured = { ...main, id: 'p2', profile_index: 2, name: 'Kids', avatar_image_url: 'https://img.example/k.png' }
    const locked = { ...main, id: 'p3', profile_index: 3, name: 'émile', avatar_color_hex: '', pin_enabled: true }
    api.fetchProfiles.mockResolvedValue([main, pictured, locked])
    renderPicker()

    const mainCard = await screen.findByRole('button', { name: 'Profile 1, Main' })
    expect(within(mainCard).getByText('M')).toHaveStyle({ backgroundColor: '#1E88E5' })
    expect(mainCard.querySelector('img')).toBeNull()

    const kidsCard = screen.getByRole('button', { name: 'Profile 2, Kids' })
    const img = kidsCard.querySelector('img')
    expect(img).toHaveAttribute('src', 'https://img.example/k.png')
    fireEvent.error(img!)
    expect(img).not.toBeVisible()

    const lockedCard = screen.getByRole('button', { name: 'Profile 3, émile, PIN in Nuvio' })
    expect(within(lockedCard).getByText('PIN in Nuvio')).toBeInTheDocument()
    expect(within(lockedCard).getByText('É')).toHaveStyle({ backgroundColor: '#1E88E5' })
  })

  it('says plainly when this server doesn’t admit the account, with the way to another one', async () => {
    api.fetchServerConfig.mockResolvedValue({ tmdb_key_mode: 'per-account' })
    api.fetchProfiles.mockRejectedValue(new ApiError(403, "this Nuvio account can't use this Uno server"))
    renderPicker()
    expect(await screen.findByText('This Nuvio account can’t use this Uno.')).toBeInTheDocument()
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Sign in with a different account' })).toBeEnabled()
    expect(screen.queryByText(/TMDB/)).not.toBeInTheDocument()
    expect(api.fetchTMDBKey).not.toHaveBeenCalled()
  })

  it('shows any other failure as an alert with the server’s words', async () => {
    api.fetchProfiles.mockRejectedValue(new ApiError(502, 'nuvio unavailable'))
    renderPicker()
    expect(await screen.findByRole('alert')).toHaveTextContent('nuvio unavailable')
    expect(screen.queryByText('This Nuvio account can’t use this Uno.')).not.toBeInTheDocument()
  })

  it('asks for no TMDB key on a server with a shared one', async () => {
    api.fetchProfiles.mockResolvedValue([main])
    renderPicker()
    expect(await screen.findByRole('button', { name: /Profile 1, Main/ })).toBeEnabled()
    expect(screen.queryByText(/TMDB/)).not.toBeInTheDocument()
    expect(api.fetchTMDBKey).not.toHaveBeenCalled()
  })

  it('holds the profiles while it is still finding out whether to ask for a key', async () => {
    api.fetchServerConfig.mockResolvedValue({ tmdb_key_mode: 'per-account' })
    api.fetchTMDBKey.mockReturnValue(new Promise(() => {}))
    api.fetchProfiles.mockResolvedValue([main])
    renderPicker()
    const card = await screen.findByRole('button', { name: /Profile 1, Main/ })
    await vi.waitFor(() => expect(api.fetchTMDBKey).toHaveBeenCalled())
    expect(card).toBeDisabled()
    expect(card).not.toHaveAttribute('aria-describedby')
    expect(screen.queryByRole('region')).not.toBeInTheDocument()
  })

  it('holds the profiles until the account saves a TMDB key, then shows its last four', async () => {
    api.fetchServerConfig.mockResolvedValue({ tmdb_key_mode: 'per-account' })
    api.fetchTMDBKey.mockResolvedValue({ set: false })
    api.fetchProfiles.mockResolvedValue([main])
    api.saveTMDBKey.mockResolvedValue({ set: true, last4: 'cdef' })
    renderPicker()

    const gate = await screen.findByRole('region', { name: 'Add your TMDB key to start' })
    const card = await screen.findByRole('button', { name: /Profile 1, Main/ })
    expect(card).toBeDisabled()
    expect(card).toHaveAccessibleDescription('Add your TMDB key to start')
    expect(within(gate).getByRole('link', { name: 'themoviedb.org' })).toHaveAttribute(
      'href',
      'https://www.themoviedb.org/settings/api',
    )

    const save = within(gate).getByRole('button', { name: 'Save key' })
    expect(save).toBeDisabled()
    fireEvent.change(within(gate).getByLabelText('TMDB API Key'), { target: { value: '0123456789abcdef0123456789abcdef' } })
    fireEvent.click(save)

    expect(await screen.findByText('ends in cdef')).toBeInTheDocument()
    expect(api.saveTMDBKey.mock.calls[0][0]).toBe('0123456789abcdef0123456789abcdef')
    expect(screen.queryByRole('region', { name: 'Add your TMDB key to start' })).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: /Profile 1, Main/ })).toBeEnabled()
  })
})
