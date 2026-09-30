// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ApiError } from '@/api/http'
import type { TMDBKeyStatus } from '@/api'
import { KeyProblemBanner } from './KeyProblemBanner'
import { clearKeyProblem, noteKeyProblem } from './keyProblem'

const api = vi.hoisted(() => ({ fetchTMDBKey: vi.fn<() => Promise<TMDBKeyStatus>>() }))
vi.mock('@/api', async () => ({ queryKeys: (await import('@/api/keys')).queryKeys, ...api }))
vi.mock('@/api/client', () => ({ apiFetch: vi.fn() }))

function renderBanner() {
  const onFix = vi.fn()
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <KeyProblemBanner onFix={onFix} />
    </QueryClientProvider>,
  )
  return onFix
}

beforeEach(() => {
  clearKeyProblem()
  api.fetchTMDBKey.mockReset()
})

describe('KeyProblemBanner', () => {
  it('stays out of the way until a call finds a key problem, and asks nothing before', () => {
    renderBanner()
    expect(screen.queryByRole('status')).not.toBeInTheDocument()
    expect(api.fetchTMDBKey).not.toHaveBeenCalled()
  })

  it('says a saved key was refused, with Replace going to the picker', async () => {
    api.fetchTMDBKey.mockResolvedValue({ set: true, last4: 'cdef' })
    const onFix = renderBanner()
    act(() => noteKeyProblem(new ApiError(422, 'TMDB didn’t accept your key.')))
    expect(await screen.findByRole('status')).toHaveTextContent(
      'TMDB didn’t accept your key. Your home screen can’t load Uno’s rows until you replace it.',
    )
    fireEvent.click(screen.getByRole('button', { name: 'Replace key' }))
    expect(onFix).toHaveBeenCalledOnce()
  })

  it('still shows, in words that fit either, when reading the key fails', async () => {
    api.fetchTMDBKey.mockRejectedValue(new ApiError(502, 'down'))
    renderBanner()
    act(() => noteKeyProblem(new ApiError(422, 'TMDB didn’t accept your key.')))
    expect(await screen.findByRole('status')).toHaveTextContent(
      'Uno couldn’t use your TMDB key. Your home screen can’t load Uno’s rows until you fix it.',
    )
    expect(screen.getByRole('button', { name: 'Check key' })).toBeInTheDocument()
  })

  it('says the account has no key, with Add, and goes once a key is saved', async () => {
    api.fetchTMDBKey.mockResolvedValue({ set: false })
    renderBanner()
    act(() => noteKeyProblem(new ApiError(422, 'This account has no TMDB key yet.')))
    expect(await screen.findByRole('status')).toHaveTextContent('This account has no TMDB key.')
    expect(screen.getByRole('button', { name: 'Add key' })).toBeInTheDocument()
    act(() => clearKeyProblem())
    expect(screen.queryByRole('status')).not.toBeInTheDocument()
  })
})
