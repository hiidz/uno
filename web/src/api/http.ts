import { pluralCount } from '@/lib/plural'
import { apiFetch } from './client'

export class ApiError extends Error {
  // Declared rather than a constructor parameter property: the project builds
  // with `erasableSyntaxOnly`, which rejects the shorthand.
  readonly status: number

  /**
   * The error body parsed as JSON, when it *was* JSON. Most Go handlers write
   * errors with `http.Error` (plain text), so this is `undefined` for nearly
   * every failure in the app — `message` is the channel that always works.
   *
   * It exists for the one endpoint that reports a structured failure on a
   * non-2xx status: `POST .../push` answers in JSON whatever happens, so the
   * caller can tell an ordinary failure from one where a compensating undo
   * also failed.
   */
  readonly body?: unknown

  constructor(status: number, message: string, body?: unknown) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.body = body
  }
}

/**
 * The `404` `requireProfile` answers before an `/api/p/{i}/...` handler runs,
 * when the profile slot was never selected. That's a routing problem, not a
 * missing resource, so it's typed separately: callers send the user back to the
 * picker instead of rendering an error. A route's own 404 (a catalog or a
 * publication not found) stays an ordinary `ApiError`.
 */
export class ProfileNotSelectedError extends ApiError {
  constructor(message: string) {
    super(404, message)
    this.name = 'ProfileNotSelectedError'
  }
}

/**
 * A `429`: this account has sent too many requests of this kind lately. The
 * `Retry-After` header gives the seconds to wait, which the message puts in
 * words. Nothing retries it: `apiFetch` retries only a 401, and queries never
 * retry a 4xx.
 */
export class RateLimitedError extends ApiError {
  constructor(retryAfter: string | null) {
    super(429, `Too many requests. Try again in ${waitWords(retryAfter)}.`)
    this.name = 'RateLimitedError'
  }
}

/** A `Retry-After` in seconds as words (1 second, 10 seconds), or as a
 *  moment when the header is missing or not a whole number. */
function waitWords(retryAfter: string | null): string {
  const seconds = Number(retryAfter) // 0 for a missing or empty header
  if (!Number.isInteger(seconds) || seconds < 1) return 'a moment'
  return pluralCount(seconds, 'second')
}

/** Best-effort: an error body that happens to be JSON becomes `ApiError.body`.
 *  Never throws — a plain-text body is the norm, not an exceptional case. */
function parseJSONOrUndefined(text: string): unknown {
  if (!text) return undefined
  try {
    return JSON.parse(text)
  } catch {
    return undefined
  }
}

/** The text of `requireProfile`'s 404 (`internal/api/auth.go`), which tells it
 *  from a route's own. */
const PROFILE_NOT_FOUND = 'profile not found'

/** The error a failed answer is thrown as. */
async function failure(res: Response): Promise<ApiError> {
  if (res.status === 429) return new RateLimitedError(res.headers.get('Retry-After'))
  // The Go handlers write errors with `http.Error`, so the body is plain
  // text and more specific than anything the status alone gives.
  const body = (await res.text().catch(() => '')).trim()
  const message = body || `Request failed (${res.status})`
  if (res.status === 404 && body === PROFILE_NOT_FOUND) {
    return new ProfileNotSelectedError(message)
  }
  return new ApiError(res.status, message, parseJSONOrUndefined(body))
}

async function request(path: string, init?: RequestInit): Promise<unknown> {
  const res = await apiFetch(path, init)
  if (!res.ok) throw await failure(res)
  if (res.status === 204) return null
  return await res.json()
}

export async function getJSON<T>(path: string): Promise<T> {
  return (await request(path)) as T
}

/**
 * POST/PUT/DELETE with a JSON body.
 *
 * A `400` surfaces as an `ApiError` carrying the server's plain-text body. That
 * body is all the Go handlers give — `http.Error`, with no field name in a
 * machine-readable position — so forms mirror `provider.Validate()` client-side
 * and treat a 400 that gets through as an unexpected-case banner.
 */
export async function sendJSON<T>(
  method: 'POST' | 'PUT' | 'DELETE',
  path: string,
  body?: unknown,
): Promise<T> {
  return (await request(path, {
    method,
    headers: body === undefined ? undefined : { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  })) as T
}

/**
 * Coerces a non-array response to `[]` so no call site has to null-check a list.
 * A guard, not a live case: `parseCatalogs`/`parseCollections`/`parseFolders`
 * all initialise their slice, so `json.Marshal` never writes `null` for one.
 * Only the top-level response — nested arrays (`folders`, `refs`) are
 * coerced where they're read.
 */
export async function getList<T>(path: string): Promise<T[]> {
  const data = await request(path)
  return Array.isArray(data) ? (data as T[]) : []
}
