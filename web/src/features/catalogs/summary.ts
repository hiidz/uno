import type { CatalogType, Certification, Genre, Language } from '@/api'
import type { CountryLookup } from './countries'
import { countryName } from './countries'
import type { GenreJoin } from './catalogForm'
import { SORT_FIELDS, parseGenreList } from './catalogForm'
import { plural, pluralCount } from '@/lib/plural'

/**
 * Plain-English summaries for the catalog editor's closed collapsible
 * sections — DESIGN.md's "Collapsible sections as credit-row buttons": each
 * head keeps its summary while closed, so the whole query reads down the
 * page without opening anything.
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

function andList(items: string[]): string {
  if (items.length < 2) return items[0] ?? ''
  return `${items.slice(0, -1).join(', ')} and ${items[items.length - 1]}`
}

function orList(items: string[]): string {
  if (items.length < 2) return items[0] ?? ''
  return `${items.slice(0, -1).join(', ')} or ${items[items.length - 1]}`
}

export function sumGenres(
  genres: Genre[],
  withIds: number[],
  withJoin: GenreJoin,
  withoutIds: number[],
): string {
  if (!withIds.length && !withoutIds.length) return 'Any genre'
  const nameOf = new Map(genres.map((genre) => [genre.id, genre.name]))
  const withNames = withIds.map((id) => nameOf.get(id) ?? String(id))
  const withoutNames = withoutIds.map((id) => nameOf.get(id) ?? String(id))
  const parts: string[] = []
  if (withNames.length) parts.push(withJoin === 'and' ? andList(withNames) : orList(withNames))
  if (withoutNames.length) parts.push(`not ${orList(withoutNames)}`)
  return parts.join(' · ')
}

const fmtRating = (n: number) => (n >= 10 ? '10' : n.toFixed(1))
const fmtVotes = (n: number) => (n >= 5000 ? '5,000+' : n.toLocaleString('en-GB'))
const fmtRuntime = (n: number) => (n >= 300 ? '300+ min' : `${n} min`)

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

function cap1(s: string): string {
  return s.charAt(0).toUpperCase() + s.slice(1)
}

export function sumRatings(
  ratingLow: number | undefined,
  ratingHigh: number | undefined,
  votesLow: number | undefined,
  votesHigh: number | undefined,
  runtimeLow: number | undefined,
  runtimeHigh: number | undefined,
): string {
  return cap1(
    [
      ratingPhrase(ratingLow, ratingHigh),
      votesPhrase(votesLow, votesHigh),
      runtimePhrase(runtimeLow, runtimeHigh),
    ].join(' · '),
  )
}

export function sumLanguage(code: string | undefined, languages: Language[]): string {
  if (!code) return 'Any language'
  const name = languages.find((lang) => lang.iso_639_1 === code)?.english_name ?? code
  return `In ${name}`
}

/** The windows people actually ask for, as presets rather than a bare number
 *  of days — shared with the Release date / First aired section body. */
export const DATE_PRESETS: { days: number; label: string }[] = [
  { days: 30, label: '30 days' },
  { days: 90, label: '90 days' },
  { days: 182, label: '6 months' },
  { days: 365, label: '1 year' },
]
export const UPCOMING_DAYS = 1
export const DAYS_PER_YEAR = 365

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
  if (mode === 'any') return 'Any time'
  if (mode === 'fixed') {
    if (gte && lte) return `${verb} ${fmtShortDate(gte)} to ${fmtShortDate(lte)}`
    if (gte) return `${verb} from ${fmtShortDate(gte)}`
    if (lte) return `${verb} up to ${fmtShortDate(lte)}`
    return `${verb} — no dates chosen yet`
  }
  if (!days) return `${verb} recently — no window chosen yet`
  const line =
    days === UPCOMING_DAYS
      ? `${verb.toLowerCase() === 'released' ? 'released' : 'airing'} from today on, updated daily`
      : `${verb.toLowerCase()} since ${formatWindowStart(days)}, updated daily`
  return `${cap1(recentLabel(days))} · ${line}`
}

function fmtShortDate(iso: string): string {
  const date = new Date(iso)
  if (Number.isNaN(date.getTime())) return iso
  return date.toLocaleDateString('en-GB', { day: 'numeric', month: 'short', year: 'numeric' })
}

export function sumAge(
  country: string | undefined,
  countryNames: CountryLookup,
  gte: string | undefined,
  lte: string | undefined,
  scale: Certification[],
): string {
  if (!country) return 'Any age rating'
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
  if (!region) return 'Any service'
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
  const { ids, join } = parseGenreList(raw)
  const withoutCount = parseGenreList(withoutRaw).ids.length
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
  const id = parseGenreList(raw).ids[0]
  if (id === undefined) return 'No collection picked'
  return name ?? `Collection ${id}`
}

/** A collection row always holds the same films, so shuffling it only
 *  changes their order. */
export const sumShuffle = (randomized: boolean, collectionRow = false): string =>
  collectionRow
    ? randomized
      ? 'On, a new order each time the row opens'
      : 'Off, in release order'
    : randomized
      ? 'On, a different set each time the row opens'
      : 'Off, the same set every time'

/** The detail a company search result shows after its name — "US · 176
 *  films" — counted for the catalog's type. */
export function companyDetail(
  company: { origin_country: string; title_count: number },
  type: CatalogType,
): string {
  const n = company.title_count
  const count = type === 'movie' ? pluralCount(n, 'film') : `${n} series`
  return company.origin_country ? `${company.origin_country} · ${count}` : count
}
