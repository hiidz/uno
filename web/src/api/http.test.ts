import { beforeEach, describe, expect, it, vi } from 'vitest'
import { apiFetch } from './client'
import { ApiError, ProfileNotSelectedError, getJSON, getList, sendJSON } from './http'

// The real `apiFetch` brings in the auth session and the router.
vi.mock('./client', () => ({ apiFetch: vi.fn() }))

const fetchMock = vi.mocked(apiFetch)

function answer(res: Response) {
  fetchMock.mockResolvedValueOnce(res)
}

async function rejection(promise: Promise<unknown>): Promise<unknown> {
  return promise.then(
    () => {
      throw new Error('expected a rejection')
    },
    (err: unknown) => err,
  )
}

beforeEach(() => {
  fetchMock.mockReset()
})

describe('successful responses', () => {
  it('parses a JSON body', async () => {
    answer(Response.json({ id: 'c1' }))
    await expect(getJSON('/api/p/0/catalogs/c1')).resolves.toEqual({ id: 'c1' })
  })

  it('reads a 204 as null', async () => {
    answer(new Response(null, { status: 204 }))
    await expect(sendJSON('DELETE', '/api/p/0/catalogs/c1')).resolves.toBeNull()
  })

  it('sends a body as JSON, and no body or content type without one', async () => {
    answer(Response.json({}))
    await sendJSON('POST', '/api/p/0/catalogs', { name: 'Row' })
    expect(fetchMock).toHaveBeenLastCalledWith('/api/p/0/catalogs', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: '{"name":"Row"}',
    })

    answer(new Response(null, { status: 204 }))
    await sendJSON('DELETE', '/api/p/0/catalogs/c1')
    expect(fetchMock).toHaveBeenLastCalledWith('/api/p/0/catalogs/c1', {
      method: 'DELETE',
      headers: undefined,
      body: undefined,
    })
  })
})

describe('failed responses', () => {
  it('carries the plain-text body as the message', async () => {
    answer(new Response('name is required\n', { status: 400 }))
    const err = await rejection(sendJSON('POST', '/api/p/0/catalogs', {}))
    expect(err).toBeInstanceOf(ApiError)
    expect(err).toMatchObject({ status: 400, message: 'name is required', body: undefined })
  })

  it('falls back to the status when the body is empty', async () => {
    answer(new Response('', { status: 502 }))
    await expect(getJSON('/api/me')).rejects.toMatchObject({ status: 502, message: 'Request failed (502)' })
  })

  it('keeps a JSON error body parsed', async () => {
    answer(Response.json({ success: false }, { status: 502 }))
    await expect(getJSON('/api/p/0/push')).rejects.toMatchObject({ status: 502, body: { success: false } })
  })

  it('reads a 404 under a profile as the profile never having been selected', async () => {
    answer(new Response('profile not selected', { status: 404 }))
    const err = await rejection(getJSON('/api/p/3/catalogs'))
    expect(err).toBeInstanceOf(ProfileNotSelectedError)
    expect(err).toBeInstanceOf(ApiError)
    expect(err).toMatchObject({ status: 404 })
  })

  it('reads a 404 outside a profile as an ordinary failure', async () => {
    answer(new Response('not found', { status: 404 }))
    const err = await rejection(getJSON('/api/profiles'))
    expect(err).toBeInstanceOf(ApiError)
    expect(err).not.toBeInstanceOf(ProfileNotSelectedError)
  })
})

describe('getList', () => {
  it('passes an array through', async () => {
    answer(Response.json([{ id: 'c1' }]))
    await expect(getList('/api/p/0/catalogs')).resolves.toEqual([{ id: 'c1' }])
  })

  it('reads anything that is not an array as an empty list', async () => {
    answer(Response.json(null))
    await expect(getList('/api/p/0/catalogs')).resolves.toEqual([])
  })
})
