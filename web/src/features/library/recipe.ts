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
 *  can't name stands in for its name. */
function genreNames(raw: string | undefined, lookup: GenreLookup): string | null {
  const { ids, join } = parseIdList(raw)
  const names = ids.map((id) => lookup.get(id) ?? String(id))
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

  // A TMDB collection row lists that collection's films and applies no other
  // filter, so the TMDB collection is the whole recipe. Unnamed, since naming
  // it needs a TMDB lookup, and always "TMDB collection" so it can't read as
  // one of Uno's own collections; movie params only.
  if (type === 'movie' && countIDs(p.with_collection)) {
    out.push('from a TMDB collection')
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
  if (companies) out.push(`from ${pluralCount(companies, 'production company')}`)
  const notCompanies = countIDs(p.without_companies)
  if (notCompanies) out.push(`not from ${pluralCount(notCompanies, 'production company')}`)
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

/** One fact a recipe sets: a short label and its value, as a spec tile reads. */
export interface RecipeFact {
  label: string
  value: string
  /** A list's own items, once each is named, so a change to it can be read
   *  item by item; none for a single value or a list still counted. */
  list?: FactList
}

/** A list fact's items, how the stored list joins them, and the region a
 *  streaming list is read in ('' for none). */
export interface FactList {
  items: string[]
  join: 'and' | 'or'
  region: string
}

/** One fact, or several or none, a recipe sets. */
type FactBuilder = (source: FactSource) => RecipeFact[]

interface FactSource {
  type: Catalog['type']
  p: TMDBParams
  lookup: GenreLookup
  names: RecipeNames
}

/** The names of TMDB entities by id. */
type NameMap = ReadonlyMap<number, string>

/**
 * The names a recipe's production companies, keywords, networks and streaming
 * services have, as far as they are known: only the ids a lookup has answered
 * for. `recipeFacts` names a list once every id in it is here, and counts it
 * until then.
 */
export interface RecipeNames {
  company?: NameMap
  keyword?: NameMap
  network?: NameMap
  provider?: NameMap
}

const NO_NAMES: RecipeNames = {}

/** What a list of ids is called, for one and for many. */
type Noun = readonly [one: string, many: string]

const COMPANY: Noun = ['Production company', 'Production companies']
const LEFT_OUT_COMPANY: Noun = ['Left-out production company', 'Left-out production companies']
const KEYWORD: Noun = ['Keyword', 'Keywords']
const LEFT_OUT_KEYWORD: Noun = ['Left-out keyword', 'Left-out keywords']
const NETWORK: Noun = ['Network', 'Networks']
const STREAMING_SERVICE: Noun = ['Streaming service', 'Streaming services']

/** An id list as its names, joined the way the list is — "A and B", or "A or
 *  B" for a pipe-joined one — once every id has one; as the number of ids until
 *  then; null for an empty list. */
function namesOrCount(raw: string | undefined, known: NameMap | undefined): string | null {
  const { ids, join } = parseIdList(raw)
  if (ids.length === 0) return null
  const names = ids.flatMap((id) => known?.get(id) ?? [])
  if (names.length < ids.length) return String(ids.length)
  return (join === 'or' ? orList : andList)(names)
}

/** A fact for `label` when `value` says something, else none. */
function fact(label: string, value: string | null | undefined): RecipeFact[] {
  return value ? [{ label, value }] : []
}

function releaseFact({ type, p }: FactSource): RecipeFact[] {
  const movie = type === 'movie'
  const span = movie
    ? dateWindow(p.primary_release_date_gte, p.primary_release_date_lte, p.released_within_days, '')
    : dateWindow(p.first_air_date_gte, p.first_air_date_lte, p.aired_within_days, '')
  return fact(movie ? 'Released' : 'Aired', span && capitalize(span.trim()))
}

function certificationFact({ p }: FactSource): RecipeFact[] {
  const cert = p.certification ?? p.certification_gte ?? p.certification_lte
  if (!cert) return []
  return fact('Age rating', p.certification_country ? `${cert} in ${countryLabel(p.certification_country)}` : cert)
}

function streamingFact({ p, names }: FactSource): RecipeFact[] {
  const region = p.watch_region ? countryLabel(p.watch_region) : ''
  return namedIdsFact(STREAMING_SERVICE, p.with_watch_providers, names.provider, region)
}

function runtimeFact({ p }: FactSource): RecipeFact[] {
  return fact('Runtime', range(p.with_runtime_gte, p.with_runtime_lte, (n) => `${n} min`))
}

function languageFact({ p }: FactSource): RecipeFact[] {
  return fact('Language', p.with_original_language && displayName('language', p.with_original_language))
}

function sortFact({ p }: FactSource): RecipeFact[] {
  return fact('Order', p.sort_by && (SORT_PHRASE[p.sort_by] ?? p.sort_by))
}

function shuffledFact({ p }: FactSource): RecipeFact[] {
  return fact('Shuffled', p.randomized ? 'Yes' : null)
}

/** Every fact a recipe sets, in the order a spec tile grid reads them. */
const FACTS: FactBuilder[] = [
  ({ type }) => fact('Type', typeLabel(type)),
  ({ p, lookup }) => genreFact('Genres', p.with_genres, lookup),
  ({ p, lookup }) => genreFact('Without genres', p.without_genres, lookup),
  releaseFact,
  ({ p }) => fact('Rating', range(p.vote_average_gte, p.vote_average_lte, (n) => n.toFixed(1))),
  ({ p }) => fact('Votes', range(p.vote_count_gte, p.vote_count_lte, (n) => n.toLocaleString())),
  runtimeFact,
  languageFact,
  certificationFact,
  streamingFact,
  // Named, once the lookups have answered; `describeParams` counts them.
  ({ p, names }) => namedIdsFact(COMPANY, p.with_companies, names.company, ''),
  ({ p, names }) => namedIdsFact(LEFT_OUT_COMPANY, p.without_companies, names.company, ''),
  ({ p, names }) => namedIdsFact(KEYWORD, p.with_keywords, names.keyword, ''),
  ({ p, names }) => namedIdsFact(LEFT_OUT_KEYWORD, p.without_keywords, names.keyword, ''),
  ({ type, p, names }) => namedIdsFact(NETWORK, type === 'series' ? p.with_networks : undefined, names.network, ''),
  sortFact,
  shuffledFact,
]

/** A collection row lists one TMDB collection's films and applies no other
 *  filter, so the collection is the whole recipe. */
const COLLECTION_FACTS: FactBuilder[] = [
  ({ type }) => fact('Type', typeLabel(type)),
  () => fact('From', 'A TMDB collection'),
  shuffledFact,
]

/**
 * The recipe as spec tiles: one label and value for each thing it sets, in
 * the vocabulary of `describeRecipe` (`lookup` is the genre map for this
 * catalog's kind). The type is always first; a recipe that sets nothing else
 * has that one fact. Production companies, keywords, networks and streaming
 * services are named from `names`, and counted while a list's names are not
 * all known.
 */
export function recipeFacts(
  catalog: Pick<Catalog, 'type' | 'params'>,
  lookup: GenreLookup,
  names: RecipeNames = NO_NAMES,
): RecipeFact[] {
  const source = { type: catalog.type, p: parseParams(catalog.params), lookup, names }
  const collection = source.type === 'movie' && countIDs(source.p.with_collection) > 0
  return (collection ? COLLECTION_FACTS : FACTS).flatMap((build) => build(source))
}

/** A filter a recipe can leave open: the labels its fact takes when set, and
 *  the tile it reads as when not. */
interface OpenFilter {
  set: readonly string[]
  open: RecipeFact
}

const ANY = 'Any'

function openFilters(type: Catalog['type']): OpenFilter[] {
  const released = type === 'movie' ? 'Released' : 'Aired'
  const filters: OpenFilter[] = [
    { set: ['Genres'], open: { label: 'Genres', value: ANY } },
    { set: [released], open: { label: released, value: 'Any time' } },
    { set: ['Rating'], open: { label: 'Rating', value: ANY } },
    { set: ['Votes'], open: { label: 'Votes', value: ANY } },
    { set: ['Runtime'], open: { label: 'Runtime', value: 'Any length' } },
    { set: ['Language'], open: { label: 'Language', value: ANY } },
    { set: ['Age rating'], open: { label: 'Age rating', value: ANY } },
    { set: STREAMING_SERVICE, open: { label: STREAMING_SERVICE[0], value: ANY } },
    { set: COMPANY, open: { label: COMPANY[0], value: ANY } },
    { set: KEYWORD, open: { label: KEYWORD[1], value: ANY } },
    { set: NETWORK, open: { label: NETWORK[0], value: ANY } },
    { set: ['Order'], open: { label: 'Order', value: SORT_PHRASE['popularity.desc'] } },
  ]
  return type === 'series' ? filters : filters.filter((filter) => filter.set !== NETWORK)
}

/**
 * The filters `facts` (the recipe's own, from `recipeFacts`) leave open, as
 * tiles: "Genres: Any", "Released: Any time", and Order as TMDB's own most
 * popular first, so a reader sees every filter a catalog could set beside the
 * ones it does. A TMDB collection row has none: the collection is its whole
 * recipe.
 */
export function openFacts(catalog: Pick<Catalog, 'type' | 'params'>, facts: readonly RecipeFact[]): RecipeFact[] {
  if (catalog.type === 'movie' && countIDs(parseParams(catalog.params).with_collection) > 0) return []
  const labels = new Set(facts.map((fact) => fact.label))
  return openFilters(catalog.type)
    .filter((filter) => !filter.set.some((label) => labels.has(label)))
    .map((filter) => filter.open)
}

/** How many of a recipe's phrases a folded catalog's line shows. */
const FOLDED_PHRASES = 3

/** A genre list longer than this reads as a count on a folded line. */
const NAMED_GENRES = 2

/** A folded catalog's line: its first phrases, and how many more it has. */
export interface FoldedLine {
  text: string
  more: number
}

/**
 * The line under a folded catalog's name: `describeRecipe`'s first three
 * phrases, its order first, a genre list of more than two read as a count
 * ("14 genres", "no 3 genres"), and how many phrases it leaves out, which
 * the line shows as "+10". The whole recipe is in the tiles the block opens to.
 */
export function foldedLine(catalog: Pick<Catalog, 'type' | 'params'>, lookup: GenreLookup): FoldedLine {
  const p = parseParams(catalog.params)
  const phrases = describeRecipe(catalog, lookup)
  const named = genreNames(p.with_genres, lookup)
  const without = genreNames(p.without_genres, lookup)
  const shown: string[] = []
  for (const phrase of phrases.slice(0, FOLDED_PHRASES)) {
    if (phrase === named) shown.push(genreCount(p.with_genres, phrase))
    else if (phrase === `no ${without}`) shown.push(`no ${genreCount(p.without_genres, without ?? '')}`)
    else shown.push(phrase)
  }
  return { text: capitalize(shown.join(' · ')), more: Math.max(0, phrases.length - FOLDED_PHRASES) }
}

/** A genre list as its names while it is short, else as how many it holds. */
function genreCount(raw: string | undefined, names: string): string {
  const count = countIDs(raw)
  if (count > NAMED_GENRES) return `${count} genres`
  return names
}

/** A genre list's fact: its names joined the way the list is, and the names
 *  one by one. An id the lookup can't name stands in for its name. */
function genreFact(label: string, raw: string | undefined, lookup: GenreLookup): RecipeFact[] {
  const value = genreNames(raw, lookup)
  if (!value) return []
  const { ids, join } = parseIdList(raw)
  const items: string[] = []
  for (const id of ids) items.push(lookup.get(id) ?? String(id))
  return [{ label, value, list: { items, join, region: '' } }]
}

/** A fact naming an id list: `noun` in the form the list's size takes, its
 *  names (or count) and the `region` it is read in after them; its items
 *  once every id is named. */
function namedIdsFact(noun: Noun, raw: string | undefined, known: NameMap | undefined, region: string): RecipeFact[] {
  const value = namesOrCount(raw, known)
  if (!value) return []
  const label = countIDs(raw) === 1 ? noun[0] : noun[1]
  const full = region ? `${value} in ${region}` : value
  const items = knownNames(raw, known)
  if (!items) return [{ label, value: full }]
  return [{ label, value: full, list: { items, join: parseIdList(raw).join, region } }]
}

/** An id list's names in order, or null while any id has none. */
function knownNames(raw: string | undefined, known: NameMap | undefined): string[] | null {
  const names: string[] = []
  for (const id of parseIdList(raw).ids) {
    const name = known?.get(id)
    if (name === undefined) return null
    names.push(name)
  }
  return names
}
