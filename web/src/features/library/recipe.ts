import type { Catalog, Genre, TMDBParams } from '@/api'

/**
 * Renders a catalog's stored params as a human-readable recipe — what the rail
 * and the Manage overlays show in place of content.
 *
 * Literal about TMDB's own vocabulary: `popularity.desc`, not "Most popular".
 * The audience is the person who wrote the query and may want to check it
 * against TMDB's docs, which prettifying would cost them.
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

export type GenreLookup = ReadonlyMap<number, string>

export function buildGenreLookup(genres: Genre[]): GenreLookup {
  return new Map(genres.map((g) => [g.id, g.name]))
}

function joinGenres(raw: string | undefined, lookup: GenreLookup): string | null {
  if (!raw) return null
  // TMDB's own convention: comma means AND, pipe means OR.
  const isOr = raw.includes('|')
  const ids = raw
    .split(/[,|]/)
    .map((s) => s.trim())
    .filter(Boolean)
  if (ids.length === 0) return null
  const names = ids.map((id) => lookup.get(Number(id)) ?? id)
  return names.join(isOr ? ' / ' : ' + ')
}

function range(
  gte: number | undefined,
  lte: number | undefined,
  format: (n: number) => string,
): string | null {
  if (gte != null && lte != null) return `${format(gte)}–${format(lte)}`
  if (gte != null) return `${format(gte)}+`
  if (lte != null) return `≤${format(lte)}`
  return null
}

/** Dates arrive as `YYYY-MM-DD`; the year alone is enough at rail width. */
function year(date: string): string {
  return date.slice(0, 4)
}

function dateWindow(
  gte: string | undefined,
  lte: string | undefined,
  withinDays: number | undefined,
  rollingLabel: string,
): string | null {
  // Mutually exclusive server-side; the ordering here just picks one if a
  // stored row somehow carries both.
  if (withinDays) return `${rollingLabel} ${withinDays}d`
  if (gte && lte) return `${year(gte)}–${year(lte)}`
  if (gte) return `${year(gte)}+`
  if (lte) return `≤${year(lte)}`
  return null
}

/**
 * Returns the recipe as ordered segments, most-defining first, so callers can
 * join them or truncate at whatever width they have. Empty when a catalog has
 * no filters at all — the caller decides what to show instead.
 *
 * `lookup` must be the genre map for *this catalog's* kind: movie and tv have
 * separate id spaces on TMDB, so passing the wrong one silently mislabels
 * genres rather than failing.
 */
export function describeRecipe(catalog: Catalog, lookup: GenreLookup): string[] {
  const p = parseParams(catalog.params)
  const out: string[] = []

  if (p.sort_by) out.push(p.sort_by)

  const genres = joinGenres(p.with_genres, lookup)
  if (genres) out.push(genres)

  const without = joinGenres(p.without_genres, lookup)
  if (without) out.push(`−${without}`)

  const rating = range(p.vote_average_gte, p.vote_average_lte, (n) => n.toFixed(1))
  if (rating) out.push(`★${rating}`)

  const votes = range(p.vote_count_gte, p.vote_count_lte, (n) => n.toLocaleString())
  if (votes) out.push(`${votes} votes`)

  const runtime = range(p.with_runtime_gte, p.with_runtime_lte, (n) => String(n))
  if (runtime) out.push(`${runtime}m`)

  if (p.with_original_language) out.push(`lang ${p.with_original_language}`)

  const window =
    catalog.type === 'movie'
      ? dateWindow(
          p.primary_release_date_gte,
          p.primary_release_date_lte,
          p.released_within_days,
          'released last',
        )
      : dateWindow(p.first_air_date_gte, p.first_air_date_lte, p.aired_within_days, 'aired last')
  if (window) out.push(window)

  // Certification exists only on movie params — `TMDBTVParams` has no such
  // fields.
  const cert = p.certification ?? p.certification_gte ?? p.certification_lte
  if (cert) out.push(p.certification_country ? `${cert} · ${p.certification_country}` : cert)

  if (p.with_watch_providers) {
    const ids = p.with_watch_providers.split(/[,|]/).filter(Boolean).join(', ')
    out.push(p.watch_region ? `on ${ids} · ${p.watch_region}` : `on ${ids}`)
  }

  // "shuffle", not "random": the backend picks a random TMDB page in [1,20],
  // not a random sample of the whole result set.
  if (p.randomized) out.push('shuffle')

  return out
}

/** Everything the rail's search should match on for a catalog: its name and
 *  its rendered recipe, so "horror" finds a catalog filtered to that genre. */
export function catalogSearchText(catalog: Catalog, lookup: GenreLookup): string {
  return `${catalog.name} ${describeRecipe(catalog, lookup).join(' ')}`.toLowerCase()
}
