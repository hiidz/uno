// Talks directly to Nuvio's auth endpoints — never through Uno's own API.
// The base URL and publishable key come from the server's environment, in the
// JSON block the server puts in the page it serves (`internal/static`).
interface NuvioConfig {
  nuvioBaseURL: string
  nuvioPublishableKey: string
}

/** The page's `uno-config` block. Read at call time, so a test (or a page)
 *  can set it after this module has loaded. */
function nuvioConfig(): NuvioConfig {
  const block = document.getElementById('uno-config')
  if (!block?.textContent) {
    throw new Error("The page has no Uno settings block. It comes from Uno's server, so start it and reload.")
  }
  return JSON.parse(block.textContent) as NuvioConfig
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

/** GoTrue's error body, `{code, error_code, msg}`; older endpoints answer
 *  `{error, error_description}`. */
interface GoTrueError {
  error_code?: unknown
  msg?: unknown
  error_description?: unknown
}

// Wrong credentials get Uno's own words; anything else is said in Nuvio's, or
// in a short fallback when the body holds none.
async function errorMessageFor(res: Response): Promise<string> {
  const body = (await res.json().catch(() => ({}))) as GoTrueError | null
  if (body?.error_code === 'invalid_credentials') {
    return "That email and password don't match a Nuvio account."
  }
  const words = body?.msg ?? body?.error_description
  return typeof words === 'string' && words ? words : "Nuvio couldn't sign you in. Try again in a moment."
}

async function tokenRequest(
  grantType: 'password' | 'refresh_token',
  body: Record<string, string>,
): Promise<NuvioTokenResponse> {
  const { nuvioBaseURL, nuvioPublishableKey } = nuvioConfig()
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
  const { nuvioBaseURL, nuvioPublishableKey } = nuvioConfig()
  await fetch(`${nuvioBaseURL}/auth/v1/logout?scope=local`, {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${accessToken}`,
      apikey: nuvioPublishableKey,
    },
  })
}
