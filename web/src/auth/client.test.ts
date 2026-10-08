import { afterEach, describe, expect, it, vi } from 'vitest'
import { signInWithPassword, signOut } from './client'

describe('Nuvio auth client', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  // The config is read at call time, so a test (or a page) sets it after
  // the module has loaded.
  it('calls the Nuvio base URL with the publishable key from window.__UNO_CONFIG__', async () => {
    vi.stubGlobal('window', {
      __UNO_CONFIG__: { nuvioBaseURL: 'https://nuvio.example.com', nuvioPublishableKey: 'pk' },
    })
    const fetchMock = vi.fn().mockResolvedValue(new Response('{}', { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)

    await signInWithPassword('a@example.com', 'secret')
    await signOut('access')

    expect(fetchMock.mock.calls.map(([url]) => url)).toEqual([
      'https://nuvio.example.com/auth/v1/token?grant_type=password',
      'https://nuvio.example.com/auth/v1/logout?scope=local',
    ])
    for (const [, init] of fetchMock.mock.calls) {
      expect((init as RequestInit).headers).toMatchObject({ apikey: 'pk' })
    }
  })
})
