import { ApiError, sendJSON } from './http'

/**
 * Push — the one call that persists the home screen and syncs it to Nuvio.
 *
 * The body carries the whole pending selection, so this app never calls the two
 * standalone `PUT .../selection` endpoints. The nested shapes match Go's
 * `vault.CatalogSelectionForm` / `vault.CollectionSelectionForm` exactly.
 */
export interface PushRequest {
  catalogs: { catalogs: Array<{ catalog_id: string; show_in_home: boolean }> }
  collections: { collection_ids: string[] }
}

/**
 * The server answers in JSON whatever happens (past auth), so this one shape
 * covers success and failure alike.
 *
 * No per-stage flags: the handler attempts Nuvio *before* writing anything
 * locally, so an ordinary failure means nothing changed anywhere and there is
 * no partial state to report. `undo_failed` marks the one case that guarantee
 * doesn't cover — the local write failed after Nuvio had accepted the push, and
 * the compensating revert failed too.
 */
export interface PushResult {
  success: boolean
  manifest_url?: string
  error?: string
  undo_failed?: boolean
}

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
