import { beforeEach, describe, expect, it, vi } from 'vitest'
import { apiFetch } from './client'

const auth = vi.hoisted(() => ({
  getAccessToken: vi.fn<() => string | null>(),
  refresh: vi.fn(),
  logout: vi.fn(),
}))
vi.mock('@/auth', () => auth)

const router = vi.hoisted(() => ({ navigate: vi.fn() }))
vi.mock('@/routes/router', () => ({ router }))

const fetchMock = vi.fn<typeof fetch>()

function authorization(call: number): string | null {
  return new Headers(fetchMock.mock.calls[call][1]?.headers).get('Authorization')
}

const status = (code: number) => new Response(null, { status: code })

beforeEach(() => {
  vi.stubGlobal('fetch', fetchMock)
  fetchMock.mockReset()
  auth.getAccessToken.mockReset().mockReturnValue('token-1')
  auth.refresh.mockReset().mockResolvedValue(undefined)
  auth.logout.mockReset().mockResolvedValue(undefined)
  router.navigate.mockReset()
})

describe('apiFetch', () => {
  it('sends the current token, keeping the caller’s own headers', async () => {
    fetchMock.mockResolvedValueOnce(status(200))
    await apiFetch('/api/profiles', { method: 'POST', headers: { 'Content-Type': 'application/json' } })
    expect(authorization(0)).toBe('Bearer token-1')
    expect(new Headers(fetchMock.mock.calls[0][1]?.headers).get('Content-Type')).toBe('application/json')
    expect(fetchMock.mock.calls[0][1]?.method).toBe('POST')
  })

  it('sends no Authorization header without a token', async () => {
    auth.getAccessToken.mockReturnValue(null)
    fetchMock.mockResolvedValueOnce(status(200))
    await apiFetch('/api/profiles')
    expect(authorization(0)).toBeNull()
  })

  it('hands back any answer but a 401 without refreshing', async () => {
    fetchMock.mockResolvedValueOnce(status(500))
    await expect(apiFetch('/api/profiles')).resolves.toMatchObject({ status: 500 })
    expect(auth.refresh).not.toHaveBeenCalled()
  })

  it('refreshes once on a 401 and retries with the new token', async () => {
    auth.getAccessToken.mockReturnValueOnce('expired').mockReturnValueOnce('token-2')
    fetchMock.mockResolvedValueOnce(status(401)).mockResolvedValueOnce(status(200))

    await expect(apiFetch('/api/profiles')).resolves.toMatchObject({ status: 200 })
    expect(auth.refresh).toHaveBeenCalledTimes(1)
    expect(authorization(0)).toBe('Bearer expired')
    expect(authorization(1)).toBe('Bearer token-2')
    expect(auth.logout).not.toHaveBeenCalled()
  })

  it('signs out to the login page when the refresh fails, without retrying', async () => {
    auth.refresh.mockRejectedValue(new Error('invalid grant'))
    fetchMock.mockResolvedValueOnce(status(401))

    await expect(apiFetch('/api/profiles')).resolves.toMatchObject({ status: 401 })
    expect(fetchMock).toHaveBeenCalledTimes(1)
    expect(auth.logout).toHaveBeenCalled()
    expect(router.navigate).toHaveBeenCalledWith('/login', { replace: true })
  })

  it('signs out to the login page when a fresh token is refused too', async () => {
    fetchMock.mockResolvedValueOnce(status(401)).mockResolvedValueOnce(status(401))

    await expect(apiFetch('/api/profiles')).resolves.toMatchObject({ status: 401 })
    expect(fetchMock).toHaveBeenCalledTimes(2)
    expect(auth.logout).toHaveBeenCalled()
    expect(router.navigate).toHaveBeenCalledWith('/login', { replace: true })
  })
})
