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
  fetchServerConfig: vi.fn(),
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

  it('shows the server’s refusal under the field, and clears it on the next edit', async () => {
    api.saveTMDBKey.mockRejectedValue(new ApiError(400, 'That’s the Read Access Token.'))
    renderWith(<TMDBKeyGate step={{ kind: 'needed' }} />)
    const field = screen.getByLabelText('TMDB API Key')
    fireEvent.change(field, { target: { value: 'eyJhbGci' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save key' }))

    expect(await screen.findByRole('alert')).toHaveTextContent('That’s the Read Access Token.')
    expect(field).toHaveAttribute('aria-invalid', 'true')
    expect(field).toHaveAccessibleDescription('That’s the Read Access Token.')

    fireEvent.change(field, { target: { value: 'eyJhbGciX' } })
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
    fireEvent.change(screen.getByLabelText('TMDB API Key'), { target: { value: 'k' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save key' }))
    expect(await screen.findByText('fine')).toBeInTheDocument()
    expect(queryClient.getQueryData(['account', 'tmdb-key'])).toEqual({ set: true, last4: 'cdef' })
  })
})

describe('TMDBKeyShelf', () => {
  const set: KeyStep = { kind: 'set', last4: 'cdef' }

  it('shows only that a key is set and its last four, and opens a replacement in place', async () => {
    api.saveTMDBKey.mockResolvedValue({ set: true, last4: '9999' })
    renderWith(<TMDBKeyShelf step={set} />)
    const shelf = screen.getByRole('region', { name: 'Your TMDB key' })
    expect(within(shelf).getByText('Set')).toBeInTheDocument()
    expect(within(shelf).getByText('ends in cdef')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Replace' }))
    expect(screen.getByLabelText('TMDB API Key')).toHaveFocus()
    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))
    expect(screen.queryByLabelText('TMDB API Key')).not.toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Replace' }))
    fireEvent.change(screen.getByLabelText('TMDB API Key'), { target: { value: 'new' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save key' }))
    await vi.waitFor(() => expect(screen.queryByLabelText('TMDB API Key')).not.toBeInTheDocument())
    expect(api.saveTMDBKey.mock.calls[0][0]).toBe('new')
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
