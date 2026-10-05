import { ApiError, sendJSON } from './http'

/**
 * Push — the one call that persists the home screen and syncs it to Nuvio.
 *
 * The body carries the whole pending Home as one ordered list of rows, each a
 * catalog or a collection, matching Go's `pushRequest`: a row's place in the
 * list is its place on Home.
 */
export interface PushRequest {
  rows: PushRow[]
}

/** One row of the pending Home: a catalog with whether it gets a home row, or
 *  a collection with whether Nuvio shows it first. */
export type PushRow =
  | { catalog_id: string; show_in_home: boolean }
  | { collection_id: string; pin_to_top: boolean }

/**
 * The server answers in JSON whatever happens (past auth), so this one shape
 * covers success and failure alike.
 *
 * No per-stage flags: the handler attempts Nuvio *before* writing anything
 * locally, and puts back what Nuvio already took when a later step fails, so an
 * ordinary failure means nothing changed anywhere and there is no partial state
 * to report. `undo_failed` marks the one case that guarantee doesn't cover —
 * putting back what Nuvio took failed too. `refused` names why the server
 * turned the push away before contacting Nuvio, when it is one the builder has
 * words for.
 */
export interface PushResult {
  success: boolean
  manifest_url?: string
  error?: string
  undo_failed?: boolean
  refused?: PushRefusal
}

/** A collection on Home has no folders, the Nuvio profile uses profile 1's
 *  addons, the profile's Nuvio slot is empty or holds another Nuvio profile
 *  now, or Nuvio's home-order list for the profile couldn't be read — the
 *  `refused…` values in `internal/api/push.go`. */
export type PushRefusal = 'empty_collection' | 'shares_addons' | 'profile_changed' | 'home_order_unreadable'

function isPushResult(value: unknown): value is PushResult {
  return typeof value === 'object' && value !== null && typeof (value as PushResult).success === 'boolean'
}

/**
 * Structured failures come back on a non-2xx status, which `sendJSON` raises as
 * an `ApiError`, so the failure path is recovered from `ApiError.body` instead
 * of surfacing as a thrown error with a JSON blob for a message.
 *
 * Anything that *isn't* a recognisable `PushResult` still throws: a 404 from
 * `requireProfile` (a `ProfileNotSelectedError`, which sends the user back to
 * the picker), a 401 that survived the refresh-and-retry in `apiFetch`, or a
 * response that never parsed at all — a proxy error page, a dropped
 * connection. Those are genuinely "we don't know what happened", and the
 * caller must not report them as "nothing changed".
 */
export async function pushSelection(
  profileIndex: number,
  body: PushRequest,
): Promise<PushResult> {
  try {
    return await sendJSON<PushResult>('POST', `/api/p/${profileIndex}/push`, body)
  } catch (err) {
    if (err instanceof ApiError && isPushResult(err.body)) return err.body
    throw err
  }
}
