import { sendJSON } from './http'
import type { ImportCheck, ImportResult } from './types'

/**
 * Bundle export and import — the portable, ID-free file format defined by the
 * Go types in `internal/vault/bundle.go`.
 *
 * The bundle itself stays `unknown` here: export hands the response to the
 * clipboard untouched, and import sends pasted JSON on unchanged. The format
 * is defined in Go alone, and the server checks every bundle it is sent.
 */

/** `POST .../export` — the selected listed catalogs and collections as a
 *  bundle. Every id must be one of this profile's own. */
export function exportBundle(
  profileIndex: number,
  body: { catalog_ids: string[]; collection_ids: string[] },
): Promise<unknown> {
  return sendJSON<unknown>('POST', `/api/p/${profileIndex}/export`, body)
}

/** `POST .../import/check` — checks a bundle and reports which of its catalogs
 *  match one of this profile's listed catalogs. Writes nothing. */
export function checkImport(profileIndex: number, bundle: unknown): Promise<ImportCheck> {
  return sendJSON<ImportCheck>('POST', `/api/p/${profileIndex}/import/check`, { bundle })
}

/** `POST .../import` — writes the bundle as new, private rows. `reuse` maps a
 *  bundle catalog key to the id of one of this profile's listed catalogs,
 *  which that key's folder refs then point at instead of a copy. The server
 *  checks the bundle again rather than trusting an earlier check. */
export function importBundle(
  profileIndex: number,
  bundle: unknown,
  reuse: Record<string, string>,
): Promise<ImportResult> {
  return sendJSON<ImportResult>('POST', `/api/p/${profileIndex}/import`, { bundle, reuse })
}
