import { refreshWithToken, signInWithPassword, signOut as nuvioSignOut } from './client'
import type { NuvioTokenResponse, NuvioUser } from './client'

const REFRESH_TOKEN_KEY = 'uno:nuvio:refresh_token'
const BROADCAST_CHANNEL_NAME = 'uno:nuvio:auth'

interface Session {
  accessToken: string
  refreshToken: string
  expiresAt: number
  user: NuvioUser
}

export type AuthStatus = 'loading' | 'authenticated' | 'unauthenticated'

export interface AuthState {
  status: AuthStatus
  user: NuvioUser | null
}

type BroadcastMessage =
  | { type: 'session'; accessToken: string; refreshToken: string; expiresAt: number; user: NuvioUser }
  | { type: 'signed-out' }

// The access token lives only here, in memory — never persisted, so a page
// reload can't leak it from disk. The refresh token is mirrored to
// localStorage (see read/writeStoredRefreshToken) since it must survive a
// reload with no backend session to hold it instead.
let session: Session | null = null
let publicState: AuthState = { status: 'loading', user: null }
const listeners = new Set<() => void>()

function emit() {
  for (const listener of listeners) listener()
}

function setPublicState(next: AuthState) {
  publicState = next
  emit()
}

export function subscribeAuth(listener: () => void): () => void {
  listeners.add(listener)
  return () => listeners.delete(listener)
}

export function getAuthState(): AuthState {
  return publicState
}

export function getAccessToken(): string | null {
  return session?.accessToken ?? null
}

function readStoredRefreshToken(): string | null {
  return localStorage.getItem(REFRESH_TOKEN_KEY)
}

function writeStoredRefreshToken(token: string | null) {
  if (token) {
    localStorage.setItem(REFRESH_TOKEN_KEY, token)
  } else {
    localStorage.removeItem(REFRESH_TOKEN_KEY)
  }
}

function toSession(res: NuvioTokenResponse): Session {
  return {
    accessToken: res.access_token,
    refreshToken: res.refresh_token,
    expiresAt: Date.now() + res.expires_in * 1000,
    user: res.user,
  }
}

const channel = typeof BroadcastChannel !== 'undefined' ? new BroadcastChannel(BROADCAST_CHANNEL_NAME) : null

function broadcast(message: BroadcastMessage) {
  channel?.postMessage(message)
}

function applySession(next: Session, opts: { broadcast: boolean }) {
  session = next
  writeStoredRefreshToken(next.refreshToken)
  setPublicState({ status: 'authenticated', user: next.user })
  if (opts.broadcast) {
    broadcast({
      type: 'session',
      accessToken: next.accessToken,
      refreshToken: next.refreshToken,
      expiresAt: next.expiresAt,
      user: next.user,
    })
  }
}

function clearSession(opts: { broadcast: boolean }) {
  session = null
  writeStoredRefreshToken(null)
  setPublicState({ status: 'unauthenticated', user: null })
  if (opts.broadcast) broadcast({ type: 'signed-out' })
}

// A session/sign-out from another tab lands here. The same event can arrive
// via both BroadcastChannel and the `storage` listener below, so this must
// be idempotent: bail out if we already reflect what the message says.
function handleRemoteMessage(message: BroadcastMessage) {
  if (message.type === 'signed-out') {
    if (session === null) return
    session = null
    setPublicState({ status: 'unauthenticated', user: null })
    return
  }
  if (session?.refreshToken === message.refreshToken) return
  session = {
    accessToken: message.accessToken,
    refreshToken: message.refreshToken,
    expiresAt: message.expiresAt,
    user: message.user,
  }
  setPublicState({ status: 'authenticated', user: message.user })
}

channel?.addEventListener('message', (event: MessageEvent<BroadcastMessage>) => {
  handleRemoteMessage(event.data)
})

// Fallback for the rare environment without BroadcastChannel. In the common
// case BroadcastChannel already delivered the full session and the guards
// above make this a no-op; this only does real work when it's the sole
// signal a refresh happened elsewhere.
window.addEventListener('storage', (event) => {
  if (event.key !== REFRESH_TOKEN_KEY) return
  if (event.newValue === null) {
    if (session === null) return
    session = null
    setPublicState({ status: 'unauthenticated', user: null })
    return
  }
  if (session?.refreshToken === event.newValue) return
  void refresh()
})

let refreshInFlight: Promise<Session> | null = null

// Single-flight: concurrent callers (e.g. several 401s in one tab) share
// one in-flight refresh call instead of each firing their own.
export function refresh(): Promise<Session> {
  if (refreshInFlight) return refreshInFlight

  const attemptedToken = session?.refreshToken ?? readStoredRefreshToken()
  if (!attemptedToken) {
    clearSession({ broadcast: false })
    return Promise.reject(new Error('no refresh token available'))
  }

  refreshInFlight = refreshWithToken(attemptedToken)
    .then((res) => {
      const next = toSession(res)
      applySession(next, { broadcast: true })
      return next
    })
    .catch((err: unknown) => {
      // Refresh tokens rotate server-side and burn on use, so two tabs
      // racing to refresh the same token strand one of them with a 4xx.
      // If a newer session already landed via broadcast while we were in
      // flight, another tab won that race — adopt its session instead of
      // signing out a perfectly valid one.
      if (session && session.refreshToken !== attemptedToken) {
        return session
      }
      clearSession({ broadcast: true })
      throw err
    })
    .finally(() => {
      refreshInFlight = null
    })

  return refreshInFlight
}

let bootstrapPromise: Promise<void> | null = null

// Runs once per page load no matter how many times it's called. React
// StrictMode mounts effects twice in dev, and a refresh token can only be
// redeemed once — a naive second call would present an already-burned token
// and sign the user out on every dev reload.
export function bootstrap(): Promise<void> {
  bootstrapPromise ??= doBootstrap()
  return bootstrapPromise
}

async function doBootstrap(): Promise<void> {
  if (!readStoredRefreshToken()) {
    setPublicState({ status: 'unauthenticated', user: null })
    return
  }
  // refresh() already sets publicState on both the success and failure
  // paths; swallow the rejection here so it isn't left unhandled.
  await refresh().catch(() => {})
}

export async function login(email: string, password: string): Promise<void> {
  const res = await signInWithPassword(email, password)
  applySession(toSession(res), { broadcast: true })
}

export async function logout(): Promise<void> {
  const accessToken = session?.accessToken
  clearSession({ broadcast: true })
  if (accessToken) {
    await nuvioSignOut(accessToken).catch(() => {})
  }
}
