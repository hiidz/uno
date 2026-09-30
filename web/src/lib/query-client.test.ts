import { describe, expect, it, vi } from 'vitest'
import { ApiError, RateLimitedError } from '@/api/http'
import { queryClient } from './query-client'

vi.mock('@/api/client', () => ({ apiFetch: vi.fn() }))
vi.mock('@/api', async () => ({ ApiError: (await import('@/api/http')).ApiError }))
vi.mock('@/auth', () => ({ getAuthState: () => ({ user: null }), subscribeAuth: () => () => {} }))

const keyProblem = vi.hoisted(() => ({ noteKeyProblem: vi.fn(), clearKeyProblem: vi.fn() }))
vi.mock('@/features/account/keyProblem', () => keyProblem)

const retry = queryClient.getDefaultOptions().queries?.retry as (count: number, error: Error) => boolean

describe('query retries', () => {
  it('never retries a 4xx, a 429 included', () => {
    expect(retry(0, new RateLimitedError('5'))).toBe(false)
    expect(retry(0, new ApiError(403, "this Nuvio account can't use this Uno server"))).toBe(false)
    expect(retry(0, new ApiError(400, 'bad'))).toBe(false)
  })

  it('retries a 5xx or a dropped connection twice', () => {
    expect(retry(0, new ApiError(502, 'nuvio unavailable'))).toBe(true)
    expect(retry(1, new TypeError('Failed to fetch'))).toBe(true)
    expect(retry(2, new ApiError(502, 'nuvio unavailable'))).toBe(false)
  })
})

describe('key problems', () => {
  it('offers every failed query and mutation to the key-problem store', async () => {
    const error = new ApiError(422, 'no key')
    await queryClient
      .fetchQuery({ queryKey: ['key-problem-test'], queryFn: () => Promise.reject(error), retry: false })
      .catch(() => {})
    await queryClient
      .getMutationCache()
      .build(queryClient, { mutationFn: () => Promise.reject(error) })
      .execute(undefined)
      .catch(() => {})
    expect(keyProblem.noteKeyProblem.mock.calls.map(([e]) => e)).toEqual([error, error])
  })
})
