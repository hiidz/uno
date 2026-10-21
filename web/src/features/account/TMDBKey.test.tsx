// @vitest-environment jsdom
import type { ReactNode } from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, within } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ApiError } from '@/api/http'
import type { TMDBKeyStatus } from '@/api'
import { TMDBKeyGate, TMDBKeyShelf } from './TMDBKey'
import { holdsProfiles, keyStep } from './useTMDBKey'
import type { KeyStep } from './useTMDBKey'
import { noteKeyProblem, useKeyProblem } from './keyProblem'

const api = vi.hoisted(() => ({
  fetchTMDBKey: vi.fn(),
  saveTMDBKey: vi.fn<(key: string) => Promise<TMDBKeyStatus>>(),
  removeTMDBKey: vi.fn<() => Promise<null>>(),
}))
vi.mock('@/api', async () => ({ queryKeys: (await import('@/api/keys')).queryKeys, ...api }))
vi.mock('@/api/client', () => ({ apiFetch: vi.fn() }))

function renderWith(ui: ReactNode) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const view = render(<QueryClientProvider client={queryClient}>{ui}</QueryClientProvider>)
  return { queryClient, ...view }
}

const KEY = '0123456789abcdef0123456789ABCDEF'

function ProblemProbe() {
  return <span>{useKeyProblem() ? 'problem' : 'fine'}</span>
}

beforeEach(() => {
  for (const fn of Object.values(api)) fn.mockReset()
})

describe('keyStep', () => {
  it('checks first, then asks for nothing without a status, a key without one saved, and shows the last four once saved', () => {
    expect(keyStep(true, undefined)).toEqual({ kind: 'checking' })
    expect(keyStep(false, undefined)).toEqual({ kind: 'none' })
    expect(keyStep(false, { set: false })).toEqual({ kind: 'needed' })
    expect(keyStep(false, { set: true, last4: 'cdef' })).toEqual({ kind: 'set', last4: 'cdef' })
  })

  it('holds the profiles while checking and while a key is needed', () => {
    expect(holdsProfiles({ kind: 'checking' })).toBe(true)
    expect(holdsProfiles({ kind: 'needed' })).toBe(true)
    expect(holdsProfiles({ kind: 'none' })).toBe(false)
    expect(holdsProfiles({ kind: 'set', last4: 'cdef' })).toBe(false)
  })
})

describe('TMDBKeyGate', () => {
  it('renders only while a key is needed', () => {
    renderWith(
      <>
        <TMDBKeyGate step={{ kind: 'none' }} />
        <TMDBKeyGate step={{ kind: 'set', last4: 'cdef' }} />
      </>,
    )
    expect(screen.queryByRole('region')).not.toBeInTheDocument()
  })

  it('enables Save only for the shape of an API key, trimmed', () => {
    renderWith(<TMDBKeyGate step={{ kind: 'needed' }} />)
    const field = screen.getByLabelText('TMDB API Key')
    const save = screen.getByRole('button', { name: 'Save key' })
    for (const value of ['abc', 'eyJhbGciOiJIUzI1NiJ9', `${KEY}0`, KEY.replace('0', 'g')]) {
      fireEvent.change(field, { target: { value } })
      expect(save).toBeDisabled()
    }
    fireEvent.change(field, { target: { value: ` ${KEY}\n` } })
    expect(save).toBeEnabled()
    expect(field).toHaveAccessibleDescription(/^32 letters and numbers/)
  })

  it('shows the server’s refusal under the field, and clears it on the next edit', async () => {
    api.saveTMDBKey.mockRejectedValue(new ApiError(400, 'TMDB didn’t accept this key.'))
    renderWith(<TMDBKeyGate step={{ kind: 'needed' }} />)
    const field = screen.getByLabelText('TMDB API Key')
    fireEvent.change(field, { target: { value: KEY } })
    fireEvent.click(screen.getByRole('button', { name: 'Save key' }))

    expect(await screen.findByRole('alert')).toHaveTextContent('TMDB didn’t accept this key.')
    expect(field).toHaveAttribute('aria-invalid', 'true')
    expect(field).toHaveAccessibleDescription(/^TMDB didn’t accept this key\. 32 letters and numbers/)

    fireEvent.change(field, { target: { value: KEY.toLowerCase() } })
    await vi.waitFor(() => expect(screen.queryByRole('alert')).not.toBeInTheDocument())
  })

  it('clears the builder’s key problem once a key is saved', async () => {
    noteKeyProblem(new ApiError(422, 'TMDB didn’t accept your key.'))
    api.saveTMDBKey.mockResolvedValue({ set: true, last4: 'cdef' })
    const { queryClient } = renderWith(
      <>
        <TMDBKeyGate step={{ kind: 'needed' }} />
        <ProblemProbe />
      </>,
    )
    expect(screen.getByText('problem')).toBeInTheDocument()
    fireEvent.change(screen.getByLabelText('TMDB API Key'), { target: { value: KEY } })
    fireEvent.click(screen.getByRole('button', { name: 'Save key' }))
    expect(await screen.findByText('fine')).toBeInTheDocument()
    expect(queryClient.getQueryData(['account', 'tmdb-key'])).toEqual({ set: true, last4: 'cdef' })
  })
})

describe('TMDBKeyShelf', () => {
  const set: KeyStep = { kind: 'set', last4: 'cdef' }

  it('shows only the saved key’s last four, and opens a replacement in place', async () => {
    api.saveTMDBKey.mockResolvedValue({ set: true, last4: '9999' })
    renderWith(<TMDBKeyShelf step={set} />)
    const shelf = screen.getByRole('region', { name: 'Your TMDB key' })
    expect(within(shelf).getByText('ends in cdef')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Replace' }))
    expect(screen.getByLabelText('TMDB API Key')).toHaveFocus()
    expect(screen.getByRole('link', { name: 'themoviedb.org' })).toHaveAttribute(
      'href',
      'https://www.themoviedb.org/settings/api',
    )
    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))
    expect(screen.queryByLabelText('TMDB API Key')).not.toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Replace' }))
    fireEvent.keyDown(screen.getByLabelText('TMDB API Key'), { key: 'Escape' })
    expect(screen.queryByLabelText('TMDB API Key')).not.toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Replace' }))
    fireEvent.change(screen.getByLabelText('TMDB API Key'), { target: { value: KEY } })
    fireEvent.click(screen.getByRole('button', { name: 'Save key' }))
    await vi.waitFor(() => expect(screen.queryByLabelText('TMDB API Key')).not.toBeInTheDocument())
    expect(api.saveTMDBKey.mock.calls[0][0]).toBe(KEY)
  })

  it('asks before removing the key, saying what it costs', async () => {
    api.removeTMDBKey.mockResolvedValue(null)
    const { queryClient } = renderWith(<TMDBKeyShelf step={set} />)
    fireEvent.click(screen.getByRole('button', { name: 'Remove' }))
    const dialog = await screen.findByRole('dialog')
    expect(dialog).toHaveTextContent('your home screen can’t load Uno’s rows')

    fireEvent.click(within(dialog).getByRole('button', { name: 'Keep it' }))
    expect(api.removeTMDBKey).not.toHaveBeenCalled()

    fireEvent.click(screen.getByRole('button', { name: 'Remove' }))
    fireEvent.click(within(await screen.findByRole('dialog')).getByRole('button', { name: 'Remove key' }))
    await vi.waitFor(() => expect(queryClient.getQueryData(['account', 'tmdb-key'])).toEqual({ set: false }))
  })

  it('renders nothing unless a key is set', () => {
    renderWith(<TMDBKeyShelf step={{ kind: 'needed' }} />)
    expect(screen.queryByRole('region')).not.toBeInTheDocument()
  })
})
