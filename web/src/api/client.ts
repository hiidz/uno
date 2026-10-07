import { getAccessToken, isTokenRefused, logout, refresh } from '@/auth'
import { router } from '@/routes/router'

function withAuthHeader(init: RequestInit, token: string | null): RequestInit {
  if (!token) return init
  const headers = new Headers(init.headers)
  headers.set('Authorization', `Bearer ${token}`)
  return { ...init, headers }
}

// Last resort: refresh couldn't produce a usable session, or the server
// still rejected a token refresh() just handed us. Clearing local state
// flips any RequireAuth-wrapped route to redirect on its own, but the
// explicit navigate() also covers a 401 raised from outside a routed page.
async function redirectToLogin(): Promise<void> {
  await logout().catch(() => {})
  void router.navigate('/login', { replace: true })
}

// Whether refresh produced a new session. A refused refresh token sends the
// user to login; any other failure (offline, Nuvio's auth down) rejects, so the
// request fails and the session, with whatever the user hasn't pushed, stays.
async function refreshed(): Promise<boolean> {
  try {
    await refresh()
    return true
  } catch (err) {
    if (!isTokenRefused(err)) throw err
    await redirectToLogin()
    return false
  }
}

// Fetch wrapper for every /api/* call: attaches the current access token,
// and on a 401 refreshes once (single-flight — see auth/session.ts) and
// retries with the new token before giving up. Only a 401 that survives a
// fresh token counts as a real auth failure; a bare 401 just means the
// access token expired, the ordinary case refresh exists for.
export async function apiFetch(input: RequestInfo | URL, init: RequestInit = {}): Promise<Response> {
  const res = await fetch(input, withAuthHeader(init, getAccessToken()))
  if (res.status !== 401) return res
  if (!(await refreshed())) return res

  const retryRes = await fetch(input, withAuthHeader(init, getAccessToken()))
  if (retryRes.status === 401) {
    await redirectToLogin()
  }
  return retryRes
}
