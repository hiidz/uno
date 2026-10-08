// Talks directly to Nuvio's auth endpoints — never through Uno's own API.
// Both values come from web/.env; vite.config.ts refuses to start or build
// without them.
const NUVIO_BASE_URL: string = import.meta.env.VITE_NUVIO_BASE_URL
const NUVIO_PUBLISHABLE_KEY: string = import.meta.env.VITE_NUVIO_PUBLISHABLE_KEY

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
  const res = await fetch(`${NUVIO_BASE_URL}/auth/v1/token?grant_type=${grantType}`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      apikey: NUVIO_PUBLISHABLE_KEY,
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
  await fetch(`${NUVIO_BASE_URL}/auth/v1/logout?scope=local`, {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${accessToken}`,
      apikey: NUVIO_PUBLISHABLE_KEY,
    },
  })
}
