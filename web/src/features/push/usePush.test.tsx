// @vitest-environment jsdom
import type { ReactNode } from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, renderHook } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ProfileNotSelectedError, RateLimitedError, queryKeys, type PushRefusal, type PushResult } from '@/api'
import { toPushPayload, type HomeState } from '@/features/home/pending'
import { usePush } from './usePush'

const api = vi.hoisted(() => ({ pushSelection: vi.fn<(i: number, body: unknown) => Promise<PushResult>>() }))

// Only `pushSelection` is faked. The error class and query keys are the real
// ones, taken from their own modules so the barrel's auth session and router
// stay unloaded.
vi.mock('@/api', async () => ({
  ProfileNotSelectedError: (await import('@/api/http')).ProfileNotSelectedError,
  RateLimitedError: (await import('@/api/http')).RateLimitedError,
  queryKeys: (await import('@/api/keys')).queryKeys,
  pushSelection: api.pushSelection,
}))
vi.mock('@/api/client', () => ({ apiFetch: vi.fn() }))

const navigate = vi.fn()
vi.mock('react-router-dom', () => ({ useNavigate: () => navigate }))

const home = vi.hoisted(() => ({
  ready: true,
  state: null as unknown as HomeState,
  homeRevision: 5,
  snapshot: vi.fn(),
  markPushed: vi.fn(),
}))
vi.mock('@/features/home/useHomeSelection', () => ({ useHomeSelection: () => home }))

const PUSHED: HomeState = {
  rows: [
    { kind: 'catalog', id: 'c1', showInHome: true },
    { kind: 'collection', id: 'col1', pinToTop: true },
  ],
}

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason: unknown) => void
  const promise = new Promise<T>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}

let queryClient: QueryClient

function renderPush() {
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  )
  return renderHook(() => usePush(4), { wrapper })
}

beforeEach(() => {
  queryClient = new QueryClient()
  api.pushSelection.mockReset()
  navigate.mockReset()
  home.ready = true
  home.state = PUSHED
  home.snapshot.mockReset().mockImplementation(() => home.state)
  home.markPushed.mockReset()
})

describe('usePush', () => {
  it('does nothing before the home selection has loaded', () => {
    home.ready = false
    const { result } = renderPush()
    expect(result.current.ready).toBe(false)
    act(() => result.current.push())
    expect(api.pushSelection).not.toHaveBeenCalled()
  })

  it('sends the selection as it was when pressed with its home revision, and acknowledges only that with the revision the push answered', async () => {
    const answer = deferred<PushResult>()
    api.pushSelection.mockReturnValue(answer.promise)
    const cached = [queryKeys.library(4)]
    for (const queryKey of cached) queryClient.setQueryData(queryKey, [])
    const { result } = renderPush()

    act(() => result.current.push())
    expect(api.pushSelection).toHaveBeenCalledWith(4, toPushPayload(PUSHED, 5))
    expect(result.current.pushing).toBe(true)

    // An edit made while the push is in flight.
    home.state = { rows: PUSHED.rows.slice(0, 1) }

    await act(async () => answer.resolve({ success: true, home_revision: 6 }))
    expect(home.markPushed).toHaveBeenCalledWith(PUSHED, 6)
    expect(result.current.pushing).toBe(false)
    expect(result.current.outcome).toEqual({ kind: 'success' })
    for (const queryKey of cached) {
      expect(queryClient.getQueryState(queryKey)?.isInvalidated).toBe(true)
    }
  })

  it('ignores a second press while a push is in flight, and takes the next one after', async () => {
    const answer = deferred<PushResult>()
    api.pushSelection.mockReturnValueOnce(answer.promise)
    const { result } = renderPush()

    act(() => {
      result.current.push()
      result.current.push()
    })
    expect(api.pushSelection).toHaveBeenCalledTimes(1)

    await act(async () => answer.resolve({ success: true, home_revision: 6 }))
    api.pushSelection.mockResolvedValueOnce({ success: true, home_revision: 7 })
    await act(async () => result.current.push())
    expect(api.pushSelection).toHaveBeenCalledTimes(2)
  })

  it('reports a refused push as failed, acknowledging nothing', async () => {
    api.pushSelection.mockResolvedValue({ success: false })
    const { result } = renderPush()
    await act(async () => result.current.push())
    expect(result.current.outcome).toEqual({ kind: 'failed' })
    expect(home.markPushed).not.toHaveBeenCalled()
  })

  it('reports a failed undo apart from an ordinary failure', async () => {
    api.pushSelection.mockResolvedValue({ success: false, undo_failed: true })
    const { result } = renderPush()
    await act(async () => result.current.push())
    expect(result.current.outcome).toEqual({ kind: 'undo-failed' })
  })

  it.each([
    ['empty_collection', 'empty-collection'],
    ['shares_addons', 'shares-addons'],
    ['profile_changed', 'profile-changed'],
    ['home_order_unreadable', 'home-order-unreadable'],
    ['too_many_catalogs', 'too-many-catalogs'],
  ] as const)('reports a push the server refused as %s in its own words', async (refused, kind) => {
    api.pushSelection.mockResolvedValue({ success: false, refused })
    const { result } = renderPush()
    await act(async () => result.current.push())
    expect(result.current.outcome).toEqual({ kind })
    expect(home.markPushed).not.toHaveBeenCalled()
  })

  it('reports a refusal it has no words for as an ordinary failure', async () => {
    api.pushSelection.mockResolvedValue({ success: false, refused: 'from_a_newer_server' as PushRefusal })
    const { result } = renderPush()
    await act(async () => result.current.push())
    expect(result.current.outcome).toEqual({ kind: 'failed' })
  })

  it('reports no answer as unknown, never as failed', async () => {
    api.pushSelection.mockRejectedValue(new TypeError('Failed to fetch'))
    const { result } = renderPush()
    await act(async () => result.current.push())
    expect(result.current.outcome).toEqual({ kind: 'unknown' })
    expect(result.current.pushing).toBe(false)
    expect(home.markPushed).not.toHaveBeenCalled()
  })

  it('reports a push the server turned away as rate limited, not unknown', async () => {
    api.pushSelection.mockRejectedValue(new RateLimitedError('2'))
    const { result } = renderPush()
    await act(async () => result.current.push())
    expect(result.current.outcome).toEqual({ kind: 'rate-limited' })
    expect(home.markPushed).not.toHaveBeenCalled()
  })

  it('sends the user to pick a profile when none is selected', async () => {
    api.pushSelection.mockRejectedValue(new ProfileNotSelectedError('profile not selected'))
    const { result } = renderPush()
    await act(async () => result.current.push())
    expect(navigate).toHaveBeenCalledWith('/profiles', { replace: true })
    expect(result.current.outcome).toBeNull()
  })
})
