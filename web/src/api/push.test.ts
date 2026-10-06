import { beforeEach, describe, expect, it, vi } from 'vitest'
import { apiFetch } from './client'
import { ApiError, ProfileNotSelectedError } from './http'
import { pushSelection, type PushRequest } from './push'

vi.mock('./client', () => ({ apiFetch: vi.fn() }))

const fetchMock = vi.mocked(apiFetch)

const body: PushRequest = {
  rows: [
    { collection_id: 'col1', pin_to_top: true },
    { catalog_id: 'c1', show_in_home: true },
  ],
}

beforeEach(() => {
  fetchMock.mockReset()
})

describe('pushSelection', () => {
  it('posts the whole selection to the profile push route', async () => {
    fetchMock.mockResolvedValueOnce(Response.json({ success: true }))
    await expect(pushSelection(2, body)).resolves.toEqual({ success: true })
    expect(fetchMock).toHaveBeenCalledWith('/api/p/2/push', expect.objectContaining({
      method: 'POST',
      body: JSON.stringify(body),
    }))
  })

  it('returns a structured failure rather than throwing it', async () => {
    fetchMock.mockResolvedValueOnce(Response.json({ success: false }, { status: 502 }))
    await expect(pushSelection(0, body)).resolves.toEqual({ success: false })
  })

  it('returns a failed undo as a result', async () => {
    fetchMock.mockResolvedValueOnce(Response.json({ success: false, undo_failed: true }, { status: 500 }))
    await expect(pushSelection(0, body)).resolves.toMatchObject({ undo_failed: true })
  })

  // Each of these leaves the outcome unknown, so none may come back as a
  // result the UI would report as "nothing changed".
  it('throws on an error page that is not a push result', async () => {
    fetchMock.mockResolvedValueOnce(new Response('<html>Bad gateway</html>', { status: 502 }))
    await expect(pushSelection(0, body)).rejects.toBeInstanceOf(ApiError)
  })

  it('throws on JSON that is not a push result', async () => {
    fetchMock.mockResolvedValueOnce(Response.json({ message: 'upstream timeout' }, { status: 504 }))
    await expect(pushSelection(0, body)).rejects.toBeInstanceOf(ApiError)
  })

  it('throws on a success status whose body never parses', async () => {
    fetchMock.mockResolvedValueOnce(new Response('not json', { status: 200 }))
    await expect(pushSelection(0, body)).rejects.toBeInstanceOf(SyntaxError)
  })

  it('throws on a dropped connection', async () => {
    fetchMock.mockRejectedValueOnce(new TypeError('Failed to fetch'))
    await expect(pushSelection(0, body)).rejects.toBeInstanceOf(TypeError)
  })

  it('throws when the profile was never selected', async () => {
    fetchMock.mockResolvedValueOnce(new Response('profile not selected', { status: 404 }))
    await expect(pushSelection(0, body)).rejects.toBeInstanceOf(ProfileNotSelectedError)
  })
})
