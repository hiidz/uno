import type { Catalog, Genre, TMDBParams } from '@/api'

/**
 * Renders a catalog's stored params as a plain-English summary — what the
 * Home list and the folder pickers show in place of content.
 *
 * **Written for someone who has never heard of TMDB.** The params are that
 * API's vocabulary — `popularity.desc`, `with_original_language: "ja"`,
 * `with_watch_providers: "8,337"` — and printing them raw asked the reader to
 * know the query language before they could read their own row. Every segment
 * here is the phrase a person would use for the same filter.
 *
 * Language, country and region codes are named through `Intl.DisplayNames`
 * rather than a shipped table: the browser already has every one of them, and
 * a table would drift.
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

/** How a `sort_by` reads out loud. Keyed on the whole stored value, direction
 *  included, because "newest first" and "oldest first" are different phrases
 *  rather than one phrase plus a suffix. An unrecognised value falls through
 *  to itself — better an unfamiliar word than a wrong one. */
const SORT_PHRASE: Record<string, string> = {
  'popularity.desc': 'Most popular',
  'popularity.asc': 'Least popular',
  'vote_average.desc': 'Highest rated',
  'vote_average.asc': 'Lowest rated',
  'vote_count.desc': 'Most rated',
  'vote_count.asc': 'Least rated',
  'primary_release_date.desc': 'Newest first',
  'primary_release_date.asc': 'Oldest first',
  'first_air_date.desc': 'Newest first',
  'first_air_date.asc': 'Oldest first',
  'revenue.desc': 'Highest grossing',
  'revenue.asc': 'Lowest grossing',
  'title.asc': 'A-Z',
  'title.desc': 'Z-A',
  'name.asc': 'A-Z',
  'name.desc': 'Z-A',
  'original_title.asc': 'A-Z by original title',
  'original_title.desc': 'Z-A by original title',
  'original_name.asc': 'A-Z by original name',
  'original_name.desc': 'Z-A by original name',
}

/** Built once. `Intl.DisplayNames` construction is not free, and this runs per
 *  catalog per render pass. */
const DISPLAY_NAMES = {
  language: safeDisplayNames('language'),
  region: safeDisplayNames('region'),
}

function safeDisplayNames(type: 'language' | 'region'): Intl.DisplayNames | null {
  try {
    return new Intl.DisplayNames(undefined, { type })
  } catch {
    return null
  }
}

/** The code itself is the fallback at every step: `of` throws on a
 *  structurally invalid code and returns undefined for one it simply doesn't
 *  know, and neither should cost the reader the rest of the line. */
function displayName(type: 'language' | 'region', code: string): string {
  try {
    return DISPLAY_NAMES[type]?.of(code) ?? code
  } catch {
    return code
  }
}

/** `CA-QC` is a certification key, not a country — TMDB scopes a few rating
 *  boards to a subdivision. The parent country is the recognisable half. */
function countryLabel(code: string): string {
  const [country, subdivision] = code.split('-')
  const name = displayName('region', country)
  return subdivision ? `${name} (${subdivision})` : name
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
  if (names.length === 1) return names[0]
  const last = names[names.length - 1]
  return `${names.slice(0, -1).join(', ')} ${isOr ? 'or' : 'and'} ${last}`
}

function range(
  gte: number | undefined,
  lte: number | undefined,
  format: (n: number) => string,
): string | null {
  if (gte != null && lte != null) return `${format(gte)}–${format(lte)}`
  if (gte != null) return `${format(gte)} or more`
  if (lte != null) return `${format(lte)} or less`
  return null
}

/** Dates arrive as `YYYY-MM-DD`; the year alone is enough at list width. */
function year(date: string): string {
  return date.slice(0, 4)
}

/** A rolling window in the units it was chosen in. The builder offers 30, 90,
 *  182 and 365 days plus whole years, so those are the ones worth naming. */
function describeDays(days: number): string {
  if (days === 182) return 'in the last 6 months'
  if (days % 365 === 0) {
    const years = days / 365
    return years === 1 ? 'in the last year' : `in the last ${years} years`
  }
  return `in the last ${days} days`
}

function dateWindow(
  gte: string | undefined,
  lte: string | undefined,
  withinDays: number | undefined,
  verb: string,
): string | null {
  // Mutually exclusive server-side; the ordering here just picks one if a
  // stored row somehow carries both.
  if (withinDays) {
    // The builder's "Upcoming" chip is a one-day window, which reads as a
    // date rather than a duration.
    return withinDays === 1 ? 'not out yet' : `${verb} ${describeDays(withinDays)}`
  }
  if (gte && lte) return `${verb} ${year(gte)}–${year(lte)}`
  if (gte) return `${verb} ${year(gte)} or later`
  if (lte) return `${verb} ${year(lte)} or earlier`
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

  if (p.sort_by) out.push(SORT_PHRASE[p.sort_by] ?? p.sort_by)

  const genres = joinGenres(p.with_genres, lookup)
  if (genres) out.push(genres)

  const without = joinGenres(p.without_genres, lookup)
  if (without) out.push(`no ${without}`)

  const rating = range(p.vote_average_gte, p.vote_average_lte, (n) => n.toFixed(1))
  if (rating) out.push(`rated ${rating}`)

  const votes = range(p.vote_count_gte, p.vote_count_lte, (n) => n.toLocaleString())
  if (votes) out.push(`${votes} ratings`)

  const runtime = range(p.with_runtime_gte, p.with_runtime_lte, (n) => String(n))
  if (runtime) out.push(`${runtime} min long`)

  if (p.with_original_language) {
    out.push(`in ${displayName('language', p.with_original_language)}`)
  }

  const window =
    catalog.type === 'movie'
      ? dateWindow(
          p.primary_release_date_gte,
          p.primary_release_date_lte,
          p.released_within_days,
          'released',
        )
      : dateWindow(p.first_air_date_gte, p.first_air_date_lte, p.aired_within_days, 'aired')
  if (window) out.push(window)

  // Certification exists only on movie params - `TMDBTVParams` has no such
  // fields.
  const cert = p.certification ?? p.certification_gte ?? p.certification_lte
  if (cert) {
    out.push(
      p.certification_country
        ? `rated ${cert} in ${countryLabel(p.certification_country)}`
        : `rated ${cert}`,
    )
  }

  // Counted, not named. Provider ids are region-scoped and naming them here
  // would need a fetch per region, while "on 8, 337" told a reader who doesn't
  // hold TMDB's id table in their head nothing at all. The builder names them
  // where the choice is actually made.
  if (p.with_watch_providers) {
    const count = p.with_watch_providers.split(/[,|]/).filter(Boolean).length
    const services = `${count} streaming ${count === 1 ? 'service' : 'services'}`
    out.push(p.watch_region ? `on ${services} in ${countryLabel(p.watch_region)}` : `on ${services}`)
  }

  // "shuffled", not "random": the backend picks a random TMDB page in [1,20],
  // not a random sample of the whole result set.
  if (p.randomized) out.push('shuffled')

  return out
}

/** Everything the rail's search should match on for a catalog: its name and
 *  its rendered summary, so "horror" finds a catalog filtered to that genre
 *  and "japanese" finds one filtered to that language. */
export function catalogSearchText(catalog: Catalog, lookup: GenreLookup): string {
  return `${catalog.name} ${describeRecipe(catalog, lookup).join(' ')}`.toLowerCase()
}
