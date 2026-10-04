import type { CatalogType, Certification, Genre, Language } from '@/api'
import type { CountryLookup } from './countries'
import { countryName } from './countries'
import { SORT_FIELDS } from './catalogForm'
import { DATE_PRESETS, DAYS_PER_YEAR, UPCOMING_DAYS, parseIdList, type IdJoin } from './params'
import { capitalize } from '@/lib/capitalize'
import { andList, orList } from '@/lib/list'
import { plural, pluralCount } from '@/lib/plural'

/**
 * Plain-English summaries for the catalog editor's closed folding sections:
 * each head keeps its summary while closed, so the whole query reads down
 * the page without opening anything.
 *
 * A close cousin of `features/library/recipe.ts`'s `describeRecipe`, not a
 * duplicate of it: that one renders a *saved* catalog's stored params for the
 * library rail, working from ids and codes alone. This one renders the
 * *editor's* live form state, which already carries the resolved objects
 * (the active `Genre[]`, the certification scale, the picked service count)
 * a section body is built from — resolving them a second time from raw
 * params would be the same information fetched twice. Where the phrasing
 * itself can be shared (`SORT_FIELDS`) it is imported, not re-typed.
 */

// What each section's head says while nothing in it is set.
const ANY_GENRE = 'Any genre'
const ANY_LANGUAGE = 'Any language'
const ANY_TIME = 'Any time'
const ANY_AGE = 'Any age rating'
const ANY_SERVICE = 'Any service'
const NO_COLLECTION = 'No TMDB collection picked'
export const ANY_COMPANY = 'Any production company'
export const ANY_KEYWORDS = 'Any keywords'
export const ANY_NETWORK = 'Any network'

const TITLE_LIKE = new Set(['title', 'name', 'original_title', 'original_name'])
const DATE_LIKE = new Set(['primary_release_date', 'first_air_date'])

function directionLabel(field: string, direction: 'asc' | 'desc'): string {
  if (TITLE_LIKE.has(field)) return direction === 'asc' ? 'A to Z' : 'Z to A'
  const [hi, lo] = DATE_LIKE.has(field) ? ['Newest first', 'Oldest first'] : ['Highest first', 'Lowest first']
  return direction === 'desc' ? hi : lo
}

export function sumOrder(type: CatalogType, field: string, direction: 'asc' | 'desc'): string {
  const options = SORT_FIELDS[type]
  const label = options.find((option) => option.value === field)?.label ?? options[0].label
  return `${label}, ${directionLabel(field || 'popularity', direction).toLowerCase()}`
}

export function sumGenres(
  genres: Genre[],
  withIds: number[],
  withJoin: IdJoin,
  withoutIds: number[],
): string {
  if (!withIds.length && !withoutIds.length) return ANY_GENRE
  const nameOf = new Map(genres.map((genre) => [genre.id, genre.name]))
  const withNames = withIds.map((id) => nameOf.get(id) ?? String(id))
  const withoutNames = withoutIds.map((id) => nameOf.get(id) ?? String(id))
  const parts: string[] = []
  if (withNames.length) parts.push(withJoin === 'and' ? andList(withNames) : orList(withNames))
  if (withoutNames.length) parts.push(`not ${orList(withoutNames)}`)
  return parts.join(' · ')
}

// Each bound reads as the number set. A slider parked at its end sets no bound
// at all, and one typed past the end in the number box is a real bound.
const fmtRating = (n: number) => (n >= 10 ? '10' : n.toFixed(1))
const fmtVotes = (n: number) => n.toLocaleString('en-GB')
const fmtRuntime = (n: number) => `${n} min`

function ratingPhrase(low: number | undefined, high: number | undefined): string {
  if (low == null && high == null) return 'any rating'
  if (low != null && high != null) return `rated ${fmtRating(low)} to ${fmtRating(high)}`
  if (low != null) return `rated ${fmtRating(low)}+`
  return `rated up to ${fmtRating(high!)}`
}

function votesPhrase(low: number | undefined, high: number | undefined): string {
  if (low == null && high == null) return 'any number of ratings'
  if (low != null && high != null) return `${fmtVotes(low)} to ${fmtVotes(high)} ratings`
  if (low != null) return `${fmtVotes(low)} or more ratings`
  return `up to ${fmtVotes(high!)} ratings`
}

function runtimePhrase(low: number | undefined, high: number | undefined): string {
  if (low == null && high == null) return 'any length'
  if (low != null && high != null) return `${low} to ${fmtRuntime(high)}`
  if (low != null) return `${fmtRuntime(low)} or longer`
  return `up to ${fmtRuntime(high!)}`
}

export function sumRatings(
  ratingLow: number | undefined,
  ratingHigh: number | undefined,
  votesLow: number | undefined,
  votesHigh: number | undefined,
  runtimeLow: number | undefined,
  runtimeHigh: number | undefined,
): string {
  return capitalize(
    [
      ratingPhrase(ratingLow, ratingHigh),
      votesPhrase(votesLow, votesHigh),
      runtimePhrase(runtimeLow, runtimeHigh),
    ].join(' · '),
  )
}

export function sumLanguage(code: string | undefined, languages: Language[]): string {
  if (!code) return ANY_LANGUAGE
  const name = languages.find((lang) => lang.iso_639_1 === code)?.english_name ?? code
  return `In ${name}`
}

/** The date the server's rolling window would produce for `days` as of today. */
export function formatWindowStart(days: number): string {
  const start = new Date()
  start.setDate(start.getDate() - days)
  return start.toLocaleDateString(undefined, { day: 'numeric', month: 'short', year: 'numeric' })
}

function recentLabel(days: number): string {
  const preset = DATE_PRESETS.find((option) => option.days === days)
  if (preset) return `last ${preset.label}`
  if (days === UPCOMING_DAYS) return 'upcoming'
  if (days % DAYS_PER_YEAR === 0) return `last ${pluralCount(days / DAYS_PER_YEAR, 'year')}`
  return `last ${pluralCount(days, 'day')}`
}

export function sumDate(
  type: CatalogType,
  mode: 'any' | 'fixed' | 'rolling',
  gte: string | undefined,
  lte: string | undefined,
  days: number | undefined,
): string {
  const verb = type === 'movie' ? 'Released' : 'First aired'
  if (mode === 'any') return ANY_TIME
  if (mode === 'fixed') {
    if (gte && lte) return `${verb} ${fmtShortDate(gte)} to ${fmtShortDate(lte)}`
    if (gte) return `${verb} from ${fmtShortDate(gte)}`
    if (lte) return `${verb} up to ${fmtShortDate(lte)}`
    return `${verb} — no dates chosen yet`
  }
  if (!days) return `${verb} recently — no window chosen yet`
  const line =
    days === UPCOMING_DAYS
      ? `${type === 'movie' ? 'released' : 'airing'} from today on, updated daily`
      : `${verb.toLowerCase()} since ${formatWindowStart(days)}, updated daily`
  return `${capitalize(recentLabel(days))} · ${line}`
}

/** A picked date is a calendar day with no zone, which `new Date` reads as UTC
 *  midnight; formatting it in UTC keeps the same day west of Greenwich. */
function fmtShortDate(iso: string): string {
  const date = new Date(iso)
  if (Number.isNaN(date.getTime())) return iso
  return date.toLocaleDateString('en-GB', { day: 'numeric', month: 'short', year: 'numeric', timeZone: 'UTC' })
}

export function sumAge(
  country: string | undefined,
  countryNames: CountryLookup,
  gte: string | undefined,
  lte: string | undefined,
  scale: Certification[],
): string {
  if (!country) return ANY_AGE
  const name = countryName(country, countryNames)
  if (!gte && !lte) return `${name} · any age rating`
  const sorted = [...scale].sort((a, b) => a.order - b.order)
  const lowCode = gte ?? sorted[0]?.certification
  const highCode = lte ?? sorted[sorted.length - 1]?.certification
  return `${name} · ${lowCode} to ${highCode}`
}

export function sumWatch(
  region: string | undefined,
  countryNames: CountryLookup,
  selectedCount: number,
): string {
  if (!region) return ANY_SERVICE
  const name = countryName(region, countryNames)
  if (selectedCount === 0) return `${name} · any service`
  return `${name} · ${selectedCount} streaming ${plural(selectedCount, 'service')}`
}

/** Counted, not named: the names live in the picker's by-id lookups, and a
 *  closed head is read at a glance. `withoutRaw` is the list left out. */
export function sumEntities(
  raw: string | undefined,
  withoutRaw: string | undefined,
  noun: string,
  anyLabel: string,
): string {
  const { ids, join } = parseIdList(raw)
  const withoutCount = parseIdList(withoutRaw).ids.length
  if (ids.length === 0 && withoutCount === 0) return anyLabel
  const parts: string[] = []
  if (ids.length === 1) parts.push(pluralCount(1, noun))
  if (ids.length > 1) {
    parts.push(`${pluralCount(ids.length, noun)}, ${join === 'or' ? 'any' : 'all'} of them`)
  }
  if (withoutCount) parts.push(`not ${pluralCount(withoutCount, noun)}`)
  return parts.join(' · ')
}

/** Named, unlike `sumEntities`: there is only ever one, and the editor reads
 *  its name from the same by-id lookup the picker's chip uses. Until that
 *  lookup answers, the id stands in. */
export function sumCollection(raw: string | undefined, name: string | undefined): string {
  const id = parseIdList(raw).ids[0]
  if (id === undefined) return NO_COLLECTION
  return name ?? `TMDB collection ${id}`
}

/** What shuffle does once it's on. A TMDB collection always holds the same
 *  films, so shuffling it only changes their order. */
export const sumShuffle = (collectionRow = false): string =>
  collectionRow ? 'New order each time' : 'New set each time'

/** A section's summary, ending "· shuffled" while Shuffle is on. */
export function withShuffle(summary: string, randomized: boolean | undefined): string {
  return randomized ? `${summary} · shuffled` : summary
}

const UNSET_SUMMARIES = new Set([
  ANY_GENRE,
  sumRatings(undefined, undefined, undefined, undefined, undefined, undefined),
  ANY_LANGUAGE,
  ANY_TIME,
  ANY_AGE,
  ANY_SERVICE,
  ANY_COMPANY,
  ANY_KEYWORDS,
  ANY_NETWORK,
  NO_COLLECTION,
])

/** A closed section's summary classes: `is-set` once the section narrows the
 *  row, `is-unset` while its head still reads as nothing set — Order's being
 *  TMDB's own popularity order — so what a catalog filters on stands out down
 *  the stack of heads. */
export function summaryClass(summary: string, type: CatalogType): string {
  const unset = UNSET_SUMMARIES.has(summary) || summary === sumOrder(type, '', 'desc')
  return unset ? 'sec-sum is-unset' : 'sec-sum is-set'
}

/** The detail a company or network search result shows after its name —
 *  "US · 176 films" — counted for the catalog's type. */
export function companyDetail(
  company: { origin_country: string; title_count: number },
  type: CatalogType,
): string {
  const n = company.title_count
  const count = type === 'movie' ? pluralCount(n, 'film') : `${n} series`
  return company.origin_country ? `${company.origin_country} · ${count}` : count
}
