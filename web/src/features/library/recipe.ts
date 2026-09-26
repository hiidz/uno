import type { Catalog, CatalogType, Genre, TMDBParams } from '@/api'
import {
  DATE_PRESETS,
  DAYS_PER_YEAR,
  UPCOMING_DAYS,
  parseIdList,
  parseParams,
} from '@/features/catalogs/params'
import { capitalize } from '@/lib/capitalize'
import { andList, orList } from '@/lib/list'
import { plural, pluralCount } from '@/lib/plural'

/**
 * Renders a catalog's stored params as plain English — the line the rail, the
 * Home list and a folder's catalog picker show in place of content, and the
 * sentence the catalog editor opens with.
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

export type GenreLookup = ReadonlyMap<number, string>

export function buildGenreLookup(genres: Genre[]): GenreLookup {
  return new Map(genres.map((g) => [g.id, g.name]))
}

/** A catalog's kind as its sticker reads: "Movies" or "Series". */
export function typeLabel(type: CatalogType): string {
  return type === 'movie' ? 'Movies' : 'Series'
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

/** A stored genre list's names, joined the way the list is — "Action and
 *  Comedy", or "Action or Comedy" for a pipe-joined one. An id the lookup
 *  can't name stands in for its name, or with `knownOnly` is left out. */
function genreNames(raw: string | undefined, lookup: GenreLookup, knownOnly = false): string | null {
  const { ids, join } = parseIdList(raw)
  const names = ids.flatMap((id) => {
    const name = lookup.get(id)
    if (name) return [name]
    return knownOnly ? [] : [String(id)]
  })
  if (names.length === 0) return null
  return join === 'or' ? orList(names) : andList(names)
}

function countIDs(raw: string | undefined): number {
  return parseIdList(raw).ids.length
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

/** A rolling window in the units it was chosen in: whole years, or the
 *  builder's own presets, or plain days. */
function describeDays(days: number): string {
  if (days % DAYS_PER_YEAR === 0) {
    const years = days / DAYS_PER_YEAR
    return years === 1 ? 'in the last year' : `in the last ${years} years`
  }
  const preset = DATE_PRESETS.find((option) => option.days === days)
  return `in the last ${preset?.label ?? `${days} days`}`
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
    return withinDays === UPCOMING_DAYS ? 'not out yet' : `${verb} ${describeDays(withinDays)}`
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
export function describeRecipe(catalog: Pick<Catalog, 'type' | 'params'>, lookup: GenreLookup): string[] {
  return describeParams(catalog.type, parseParams(catalog.params), lookup)
}

/** `describeRecipe` over params already parsed, for a caller that reads them
 *  itself too. */
function describeParams(type: Catalog['type'], p: TMDBParams, lookup: GenreLookup): string[] {
  const out: string[] = []

  // A collection row lists one TMDB collection's films and applies no other
  // filter, so the collection is the whole recipe. Unnamed, since naming it
  // needs a TMDB lookup, and called a movie collection so it can't read as
  // one of Uno's own collections; movie params only.
  if (type === 'movie' && countIDs(p.with_collection)) {
    out.push('from a movie collection')
    if (p.randomized) out.push('shuffled')
    return out
  }

  if (p.sort_by) out.push(SORT_PHRASE[p.sort_by] ?? p.sort_by)

  const genres = genreNames(p.with_genres, lookup)
  if (genres) out.push(genres)

  const without = genreNames(p.without_genres, lookup)
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
    type === 'movie'
      ? dateWindow(
          p.primary_release_date_gte,
          p.primary_release_date_lte,
          p.released_within_days,
          'released',
        )
      : dateWindow(p.first_air_date_gte, p.first_air_date_lte, p.aired_within_days, 'aired')
  if (window) out.push(window)

  // Both kinds carry a rating: theatrical ratings for movies, TV content
  // ratings for series, each read against `certification_country`'s scale.
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
  const serviceCount = countIDs(p.with_watch_providers)
  if (serviceCount) {
    const services = `${serviceCount} streaming ${plural(serviceCount, 'service')}`
    out.push(p.watch_region ? `on ${services} in ${countryLabel(p.watch_region)}` : `on ${services}`)
  }

  // Counted for the same reason: naming a company, keyword or network needs a
  // TMDB lookup per id.
  const companies = countIDs(p.with_companies)
  if (companies) out.push(`from ${pluralCount(companies, 'studio')}`)
  const notCompanies = countIDs(p.without_companies)
  if (notCompanies) out.push(`not from ${pluralCount(notCompanies, 'studio')}`)
  const keywords = countIDs(p.with_keywords)
  if (keywords) out.push(`tagged with ${pluralCount(keywords, 'keyword')}`)
  const notKeywords = countIDs(p.without_keywords)
  if (notKeywords) out.push(`not tagged with ${pluralCount(notKeywords, 'keyword')}`)
  // Series params only, like the collection above is movie params only.
  const networks = type === 'series' ? countIDs(p.with_networks) : 0
  if (networks) out.push(`on ${pluralCount(networks, 'network')}`)

  // "shuffled", not "random": the backend picks a random TMDB page in [1,20],
  // not a random sample of the whole result set.
  if (p.randomized) out.push('shuffled')

  return out
}

/** A recipe's segments as one line of plain words, capitalised to start a
 *  line — the form the rail and the home screen's rows show. */
export function recipeLine(catalog: Pick<Catalog, 'type' | 'params'>, lookup: GenreLookup): string {
  return capitalize(describeRecipe(catalog, lookup).join(' · '))
}

/**
 * The recipe as one sentence — "Shows horror movies, highest rated, 30 or more
 * ratings." — for the top of the catalog editor. Built from `describeRecipe`'s
 * own segments, which lead with the sort phrase and then the genres whenever
 * each is set; those two become the sentence's subject and first clause.
 * Empty when the catalog has no filters at all.
 */
export function recipeSentence(
  catalog: Pick<Catalog, 'type' | 'params'>,
  lookup: GenreLookup,
): string {
  const p = parseParams(catalog.params)
  const rest = describeParams(catalog.type, p, lookup)
  if (rest.length === 0) return ''
  if (catalog.type === 'movie' && countIDs(p.with_collection)) {
    return `Shows the films in one movie collection${p.randomized ? ', shuffled' : ''}.`
  }

  const sort = p.sort_by ? rest.shift() : undefined
  if (countIDs(p.with_genres)) rest.shift()
  // Only the genres the lookup can name. An id standing in for a name — the
  // genre list still loading, or an id TMDB has retired — would read as a
  // count of titles as the subject ("27 movies"), so it is left out instead.
  const genres = genreNames(p.with_genres, lookup, true)
  const noun = catalog.type === 'movie' ? 'movies' : 'series'
  const subject = genres ? `${genres.toLowerCase()} ${noun}` : noun
  // "A-Z" and "Z-A" keep their capitals, with or without "by original
  // title"; every other sort phrase reads mid-sentence.
  const order = sort && !/^[A-Z]-[A-Z]\b/.test(sort) ? sort.toLowerCase() : sort
  return `Shows ${[subject, order, ...rest].filter(Boolean).join(', ')}.`
}

/**
 * What a list of catalogs shows and searches on for one catalog, from a single
 * pass over its recipe: the summary line (`recipeLine`'s form) and the search
 * text — its name and its rendered summary, so "horror" finds a catalog
 * filtered to that genre and "japanese" one filtered to that language.
 */
export function catalogListing(
  catalog: Catalog,
  lookup: GenreLookup,
): { line: string; searchText: string } {
  const segments = describeRecipe(catalog, lookup)
  return {
    line: capitalize(segments.join(' · ')),
    searchText: `${catalog.name} ${segments.join(' ')}`.toLowerCase(),
  }
}
