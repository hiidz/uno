// @vitest-environment jsdom
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { NuvioTokenResponse } from './client'

const nuvio = vi.hoisted(() => ({
  refreshWithToken: vi.fn(),
  signInWithPassword: vi.fn(),
  signOut: vi.fn(),
}))
vi.mock('./client', () => nuvio)

/** Stands in for the cross-tab channel: records what this tab posts, and
 *  delivers what another tab would. */
class FakeChannel {
  static current: FakeChannel
  posted: unknown[] = []
  private listener: ((event: MessageEvent) => void) | null = null

  constructor() {
    FakeChannel.current = this
  }

  postMessage(message: unknown) {
    this.posted.push(message)
  }

  addEventListener(_type: 'message', listener: (event: MessageEvent) => void) {
    this.listener = listener
  }

  receive(data: unknown) {
    this.listener?.({ data } as MessageEvent)
  }
}

const REFRESH_TOKEN_KEY = 'uno:nuvio:refresh_token'
const SIGNED_OUT = { type: 'signed-out' }

function tokens(n: number): NuvioTokenResponse {
  return {
    access_token: `access-${n}`,
    token_type: 'bearer',
    expires_in: 3600,
    refresh_token: `refresh-${n}`,
    user: { id: 'u1', email: 'viewer@example.com', created_at: '' },
  }
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

// The session is module state, set up at import, so every test starts from a
// fresh copy of the module.
async function load() {
  vi.resetModules()
  return import('./session')
}

beforeEach(() => {
  vi.stubGlobal('BroadcastChannel', FakeChannel)
  localStorage.clear()
  nuvio.refreshWithToken.mockReset()
  nuvio.signInWithPassword.mockReset()
  nuvio.signOut.mockReset().mockResolvedValue(undefined)
})

describe('refresh', () => {
  it('shares one request between concurrent callers, then allows the next', async () => {
    localStorage.setItem(REFRESH_TOKEN_KEY, 'refresh-0')
    const session = await load()
    const answer = deferred<NuvioTokenResponse>()
    nuvio.refreshWithToken.mockReturnValue(answer.promise)

    const first = session.refresh()
    expect(session.refresh()).toBe(first)
    expect(nuvio.refreshWithToken).toHaveBeenCalledTimes(1)
    expect(nuvio.refreshWithToken).toHaveBeenCalledWith('refresh-0')

    answer.resolve(tokens(1))
    await first
    expect(session.getAccessToken()).toBe('access-1')
    expect(session.getAuthState().status).toBe('authenticated')
    expect(localStorage.getItem(REFRESH_TOKEN_KEY)).toBe('refresh-1')
    expect(FakeChannel.current.posted).toContainEqual(expect.objectContaining({ type: 'session', refreshToken: 'refresh-1' }))

    nuvio.refreshWithToken.mockResolvedValue(tokens(2))
    await session.refresh()
    expect(nuvio.refreshWithToken).toHaveBeenLastCalledWith('refresh-1')
  })

  it('signs out without a request when there is no refresh token', async () => {
    const session = await load()
    await expect(session.refresh()).rejects.toThrow()
    expect(nuvio.refreshWithToken).not.toHaveBeenCalled()
    expect(session.getAuthState().status).toBe('unauthenticated')
  })

  it('signs out every tab when the token is refused', async () => {
    localStorage.setItem(REFRESH_TOKEN_KEY, 'refresh-0')
    const session = await load()
    nuvio.refreshWithToken.mockRejectedValue(new Error('invalid grant'))

    await expect(session.refresh()).rejects.toThrow('invalid grant')
    expect(session.getAuthState().status).toBe('unauthenticated')
    expect(localStorage.getItem(REFRESH_TOKEN_KEY)).toBeNull()
    expect(FakeChannel.current.posted).toContainEqual(SIGNED_OUT)
  })

  it('adopts the session another tab refreshed to, instead of signing out', async () => {
    const session = await load()
    nuvio.signInWithPassword.mockResolvedValue(tokens(1))
    await session.login('viewer@example.com', 'secret')

    const answer = deferred<NuvioTokenResponse>()
    nuvio.refreshWithToken.mockReturnValue(answer.promise)
    const pending = session.refresh()

    // The other tab redeemed refresh-1 first, so this tab's attempt is refused.
    const other = tokens(2)
    FakeChannel.current.receive({
      type: 'session',
      accessToken: other.access_token,
      refreshToken: other.refresh_token,
      expiresAt: Date.now() + 3_600_000,
      user: other.user,
    })
    answer.reject(new Error('refresh token already used'))

    await expect(pending).resolves.toMatchObject({ refreshToken: 'refresh-2' })
    expect(session.getAccessToken()).toBe('access-2')
    expect(session.getAuthState().status).toBe('authenticated')
    expect(FakeChannel.current.posted).not.toContainEqual(SIGNED_OUT)
  })
})

describe('bootstrap', () => {
  it('redeems the stored token once however often it is called', async () => {
    localStorage.setItem(REFRESH_TOKEN_KEY, 'refresh-0')
    const session = await load()
    nuvio.refreshWithToken.mockResolvedValue(tokens(1))

    await Promise.all([session.bootstrap(), session.bootstrap()])
    await session.bootstrap()
    expect(nuvio.refreshWithToken).toHaveBeenCalledTimes(1)
    expect(session.getAuthState().status).toBe('authenticated')
  })

  it('settles signed out without a request when nothing is stored', async () => {
    const session = await load()
    await session.bootstrap()
    expect(nuvio.refreshWithToken).not.toHaveBeenCalled()
    expect(session.getAuthState()).toEqual({ status: 'unauthenticated', user: null })
  })

  it('settles signed out, without rejecting, when the stored token is refused', async () => {
    localStorage.setItem(REFRESH_TOKEN_KEY, 'refresh-0')
    const session = await load()
    nuvio.refreshWithToken.mockRejectedValue(new Error('invalid grant'))
    await expect(session.bootstrap()).resolves.toBeUndefined()
    expect(session.getAuthState().status).toBe('unauthenticated')
  })
})

describe('other tabs', () => {
  it('follows a sign-out from another tab, once', async () => {
    const session = await load()
    nuvio.signInWithPassword.mockResolvedValue(tokens(1))
    await session.login('viewer@example.com', 'secret')
    const listener = vi.fn()
    session.subscribeAuth(listener)

    FakeChannel.current.receive(SIGNED_OUT)
    FakeChannel.current.receive(SIGNED_OUT)
    expect(session.getAccessToken()).toBeNull()
    expect(session.getAuthState().status).toBe('unauthenticated')
    expect(listener).toHaveBeenCalledTimes(1)
  })

  it('ignores a session it already holds', async () => {
    const session = await load()
    nuvio.signInWithPassword.mockResolvedValue(tokens(1))
    await session.login('viewer@example.com', 'secret')
    const listener = vi.fn()
    session.subscribeAuth(listener)

    const same = tokens(1)
    FakeChannel.current.receive({
      type: 'session',
      accessToken: same.access_token,
      refreshToken: same.refresh_token,
      expiresAt: 0,
      user: same.user,
    })
    expect(listener).not.toHaveBeenCalled()
  })

  it('follows a sign-out seen only as the stored token being cleared', async () => {
    const session = await load()
    nuvio.signInWithPassword.mockResolvedValue(tokens(1))
    await session.login('viewer@example.com', 'secret')

    window.dispatchEvent(new StorageEvent('storage', { key: REFRESH_TOKEN_KEY, newValue: null }))
    expect(session.getAuthState().status).toBe('unauthenticated')
  })
})

describe('logout', () => {
  it('clears the session everywhere and revokes it upstream', async () => {
    const session = await load()
    nuvio.signInWithPassword.mockResolvedValue(tokens(1))
    await session.login('viewer@example.com', 'secret')

    await session.logout()
    expect(nuvio.signOut).toHaveBeenCalledWith('access-1')
    expect(session.getAccessToken()).toBeNull()
    expect(localStorage.getItem(REFRESH_TOKEN_KEY)).toBeNull()
    expect(FakeChannel.current.posted).toContainEqual(SIGNED_OUT)
  })

  it('still signs out locally when revoking fails', async () => {
    const session = await load()
    nuvio.signInWithPassword.mockResolvedValue(tokens(1))
    await session.login('viewer@example.com', 'secret')
    nuvio.signOut.mockRejectedValue(new Error('offline'))

    await expect(session.logout()).resolves.toBeUndefined()
    expect(session.getAuthState().status).toBe('unauthenticated')
  })
})

describe('loginWithBypassToken', () => {
  it('authenticates with the token, persisting and broadcasting nothing', async () => {
    const session = await load()
    session.loginWithBypassToken('dev-token')
    expect(session.getAccessToken()).toBe('dev-token')
    expect(session.getAuthState().status).toBe('authenticated')
    expect(localStorage.getItem(REFRESH_TOKEN_KEY)).toBeNull()
    expect(FakeChannel.current.posted).toEqual([])
  })
})
