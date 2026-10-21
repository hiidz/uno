// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { signInWithPassword, signOut } from './client'

describe('Nuvio auth client', () => {
  // The block the server puts in the page. It is read at call time, so a test
  // (or a page) sets it after the module has loaded.
  beforeEach(() => {
    const block = document.createElement('script')
    block.type = 'application/json'
    block.id = 'uno-config'
    block.textContent = JSON.stringify({
      nuvioBaseURL: 'https://nuvio.example.com',
      nuvioPublishableKey: 'pk',
      tmdb_key_mode: 'shared',
    })
    document.head.append(block)
  })

  afterEach(() => {
    document.getElementById('uno-config')?.remove()
    vi.unstubAllGlobals()
  })

  it('calls the Nuvio base URL with the publishable key from the page’s config block', async () => {
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

  it.each([
    [
      'wrong credentials in Uno\'s words',
      Response.json({ code: 400, error_code: 'invalid_credentials', msg: 'Invalid login credentials' }, { status: 400 }),
      "That email and password don't match a Nuvio account.",
    ],
    [
      'any other error in Nuvio\'s words',
      Response.json({ code: 429, error_code: 'over_request_rate_limit', msg: 'Request rate limit reached' }, { status: 429 }),
      'Request rate limit reached',
    ],
    [
      'an older error body in its description',
      Response.json({ error: 'invalid_grant', error_description: 'Refresh Token Not Found' }, { status: 400 }),
      'Refresh Token Not Found',
    ],
    [
      'a body with no words as the fallback',
      new Response('<html>502 Bad Gateway</html>', { status: 502 }),
      "Nuvio couldn't sign you in. Try again in a moment.",
    ],
    ['a null body as the fallback', Response.json(null, { status: 500 }), "Nuvio couldn't sign you in. Try again in a moment."],
  ])('words %s', async (_, res, message) => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(res))

    await expect(signInWithPassword('a@example.com', 'wrong')).rejects.toMatchObject({
      name: 'NuvioAuthError',
      status: res.status,
      message,
    })
  })

  it('says to start Uno’s server when the page has no config block, and sends nothing', async () => {
    document.getElementById('uno-config')?.remove()
    const fetchMock = vi.fn()
    vi.stubGlobal('fetch', fetchMock)

    await expect(signInWithPassword('a@example.com', 'secret')).rejects.toThrow(
      "The page has no Uno settings block. It comes from Uno's server, so start it and reload.",
    )
    expect(fetchMock).not.toHaveBeenCalled()
  })
})
