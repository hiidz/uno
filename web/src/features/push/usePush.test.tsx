// @vitest-environment jsdom
import type { ReactNode } from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, renderHook } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ProfileNotSelectedError, queryKeys, type PushResult } from '@/api'
import { toPushPayload, type HomeState } from '@/features/home/pending'
import { usePush } from './usePush'

const api = vi.hoisted(() => ({ pushSelection: vi.fn<(i: number, body: unknown) => Promise<PushResult>>() }))

// Only `pushSelection` is faked. The error class and query keys are the real
// ones, taken from their own modules so the barrel's auth session and router
// stay unloaded.
vi.mock('@/api', async () => ({
  ProfileNotSelectedError: (await import('@/api/http')).ProfileNotSelectedError,
  queryKeys: (await import('@/api/keys')).queryKeys,
  pushSelection: api.pushSelection,
}))
vi.mock('@/api/client', () => ({ apiFetch: vi.fn() }))

const navigate = vi.fn()
vi.mock('react-router-dom', () => ({ useNavigate: () => navigate }))

const home = vi.hoisted(() => ({
  ready: true,
  state: null as unknown as HomeState,
  snapshot: vi.fn(),
  markPushed: vi.fn(),
}))
vi.mock('@/features/home/useHomeSelection', () => ({ useHomeSelection: () => home }))

const PUSHED: HomeState = { catalogs: [{ id: 'c1', showInHome: true }], collections: ['col1'] }

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

  it('sends the selection as it was when pressed, and acknowledges only that', async () => {
    const answer = deferred<PushResult>()
    api.pushSelection.mockReturnValue(answer.promise)
    const invalidate = vi.spyOn(queryClient, 'invalidateQueries')
    const { result } = renderPush()

    act(() => result.current.push())
    expect(api.pushSelection).toHaveBeenCalledWith(4, toPushPayload(PUSHED))
    expect(result.current.pushing).toBe(true)

    // An edit made while the push is in flight.
    home.state = { ...PUSHED, collections: [] }

    await act(async () => answer.resolve({ success: true, manifest_url: 'https://uno/manifest.json' }))
    expect(home.markPushed).toHaveBeenCalledWith(PUSHED)
    expect(result.current.pushing).toBe(false)
    expect(result.current.outcome).toEqual({ kind: 'success', manifestURL: 'https://uno/manifest.json' })
    for (const queryKey of [
      queryKeys.catalogSelection(4),
      queryKeys.collectionSelection(4),
      queryKeys.ownedCollections(4),
    ]) {
      expect(invalidate).toHaveBeenCalledWith({ queryKey })
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

    await act(async () => answer.resolve({ success: true }))
    api.pushSelection.mockResolvedValueOnce({ success: true })
    await act(async () => result.current.push())
    expect(api.pushSelection).toHaveBeenCalledTimes(2)
  })

  it('reports a refused push as failed, acknowledging nothing', async () => {
    api.pushSelection.mockResolvedValue({ success: false, error: 'nuvio refused' })
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

  it('reports no answer as unknown, never as failed', async () => {
    api.pushSelection.mockRejectedValue(new TypeError('Failed to fetch'))
    const { result } = renderPush()
    await act(async () => result.current.push())
    expect(result.current.outcome).toEqual({ kind: 'unknown' })
    expect(result.current.pushing).toBe(false)
    expect(home.markPushed).not.toHaveBeenCalled()
  })

  it('sends the user to pick a profile when none is selected', async () => {
    api.pushSelection.mockRejectedValue(new ProfileNotSelectedError('profile not selected'))
    const { result } = renderPush()
    await act(async () => result.current.push())
    expect(navigate).toHaveBeenCalledWith('/profiles', { replace: true })
    expect(result.current.outcome).toBeNull()
  })

  it('clears the outcome on dismiss', async () => {
    api.pushSelection.mockResolvedValue({ success: false })
    const { result } = renderPush()
    await act(async () => result.current.push())
    act(() => result.current.dismiss())
    expect(result.current.outcome).toBeNull()
  })
})
