import type { TMDBParams } from '@/api'

/**
 * The formats a catalog's stored params are written in: the JSON string
 * itself, TMDB's joined id lists, and the rolling date windows the builder
 * offers. Imports nothing from the features, so the form model
 * (`catalogForm.ts`), its summaries (`summary.ts`) and the library's
 * plain-English recipe (`features/library/recipe.ts`) all read one copy.
 */

/** Never `JSON.parse` a catalog's params at a call site — a single malformed
 *  row would take down the whole list. */
export function parseParams(raw: string): TMDBParams {
  if (!raw) return {}
  try {
    const parsed: unknown = JSON.parse(raw)
    return parsed && typeof parsed === 'object' ? (parsed as TMDBParams) : {}
  } catch {
    return {}
  }
}

/** Genre, company, keyword, network and provider ids are each stored as one
 *  string, comma-joined for "all of them" and pipe-joined for "any of them".
 *  A single id has no separator, so it reads as `and`. */
export type IdJoin = 'and' | 'or'

/** The ids in a stored list, in order, and how they're joined. Anything that
 *  isn't a positive whole number is dropped rather than carried through as
 *  `NaN`, which would render as a chip nobody can remove. */
export function parseIdList(raw: string | undefined): { ids: number[]; join: IdJoin } {
  if (!raw) return { ids: [], join: 'and' }
  const ids = raw
    .split(/[,|]/)
    .map((part) => Number(part.trim()))
    .filter((id) => Number.isInteger(id) && id > 0)
  return { ids, join: raw.includes('|') ? 'or' : 'and' }
}

export function serializeIdList(ids: number[], join: IdJoin): string {
  return ids.join(join === 'or' ? '|' : ',')
}

/** The rolling windows people actually ask for, as presets rather than a bare
 *  number of days. Each is still just `_within_days` on the wire. */
export const DATE_PRESETS: { days: number; label: string }[] = [
  { days: 30, label: '30 days' },
  { days: 90, label: '90 days' },
  { days: 182, label: '6 months' },
  { days: 365, label: '1 year' },
]

/** "Upcoming": a one-day window, which over a discover page sorted by
 *  popularity is the unreleased slate. */
export const UPCOMING_DAYS = 1

export const DAYS_PER_YEAR = 365
