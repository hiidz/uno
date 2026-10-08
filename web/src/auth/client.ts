// Talks directly to Nuvio's auth endpoints — never through Uno's own API.
// The server's /config.js, which index.html loads before the app, sets the
// base URL and publishable key from its environment.
declare global {
  interface Window {
    __UNO_CONFIG__: { nuvioBaseURL: string; nuvioPublishableKey: string }
  }
}

export interface NuvioUser {
  id: string
  email: string
  created_at: string
}

export interface NuvioTokenResponse {
  access_token: string
  token_type: string
  expires_in: number
  refresh_token: string
  user: NuvioUser
}

export class NuvioAuthError extends Error {
  status: number

  constructor(message: string, status: number) {
    super(message)
    this.name = 'NuvioAuthError'
    this.status = status
  }
}

async function errorMessageFor(res: Response): Promise<string> {
  try {
    const body = (await res.json()) as { message?: string }
    return body.message ?? `Nuvio auth request failed (${res.status})`
  } catch {
    return `Nuvio auth request failed (${res.status})`
  }
}

async function tokenRequest(
  grantType: 'password' | 'refresh_token',
  body: Record<string, string>,
): Promise<NuvioTokenResponse> {
  const { nuvioBaseURL, nuvioPublishableKey } = window.__UNO_CONFIG__
  const res = await fetch(`${nuvioBaseURL}/auth/v1/token?grant_type=${grantType}`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      apikey: nuvioPublishableKey,
    },
    body: JSON.stringify(body),
  })
  if (!res.ok) {
    throw new NuvioAuthError(await errorMessageFor(res), res.status)
  }
  return res.json() as Promise<NuvioTokenResponse>
}

export function signInWithPassword(email: string, password: string): Promise<NuvioTokenResponse> {
  return tokenRequest('password', { email, password })
}

export function refreshWithToken(refreshToken: string): Promise<NuvioTokenResponse> {
  return tokenRequest('refresh_token', { refresh_token: refreshToken })
}

// Fire-and-forget from the caller's side (session.ts clears local state
// regardless of whether this succeeds) — errors are left for the caller to
// decide whether they matter. `scope=local` ends this session alone: GoTrue's
// default revokes every session the account holds, which would sign it out of
// Nuvio on every device.
export async function signOut(accessToken: string): Promise<void> {
  const { nuvioBaseURL, nuvioPublishableKey } = window.__UNO_CONFIG__
  await fetch(`${nuvioBaseURL}/auth/v1/logout?scope=local`, {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${accessToken}`,
      apikey: nuvioPublishableKey,
    },
  })
}
