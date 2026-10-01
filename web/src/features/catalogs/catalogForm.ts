import { CATALOG_PROVIDER } from '@/api'
import type { Catalog, CatalogPayload, CatalogType, TMDBParams } from '@/api'
import { parseIdList, parseParams } from './params'

/**
 * The catalog builder's form model, and the client-side mirror of
 * `provider.Validate()`.
 *
 * The mirror exists because the server can't give us per-field errors: every
 * `400` from the catalog handlers is `http.Error(w, err.Error(), …)` — plain
 * text, no field name in a machine-readable position. So the rules live here
 * too, and a server 400 that gets through means this mirror has drifted.
 *
 * Where possible the rules are encoded *structurally* rather than checked
 * after the fact — the date window is one mode toggle, so "both fixed and
 * rolling" is unrepresentable rather than merely invalid.
 */

/** Fixed range and rolling window are mutually exclusive server-side. Modelled
 *  as a mode so the form can't express both at once. */
export type DateMode = 'any' | 'fixed' | 'rolling'

/** What a movie catalog lists: the result of its discover filters, or one TMDB
 *  collection's films. Form state only — the stored shape is the params, and a
 *  saved `with_collection` reads back as `'collection'`. Series are always
 *  `'filters'`: TMDB has no collections for series. */
export type SourceMode = 'filters' | 'collection'

/** The most ids each of `with_companies`, `with_keywords`,
 *  `without_companies`, `without_keywords` and `with_networks` may hold.
 *  Mirrors the server's cap in `provider.Validate()`. */
export const MAX_ENTITY_IDS = 20

export interface CatalogFormState {
  name: string
  type: CatalogType
  dateMode: DateMode
  sourceMode: SourceMode
  params: TMDBParams
  /** The collection this catalog is scoped to; `null` means listed. Never
   *  sent: a catalog's scope is not part of a `PUT`, and a scoped catalog
   *  moves to the library through its collection's save (`withCatalogEdit`).
   *  `isSameCatalog` still compares it, so a staged Move to library counts as
   *  an edit. */
  collectionID: string | null
}

/** TMDB's complete `sort_by` enum per discover endpoint. Mirrors
 *  `validMovieSortValues` / `validTVSortValues` in internal/provider. Note the
 *  lists genuinely differ — tv has no `revenue` or `title`, and uses `name`. */
const SORT_OPTIONS: Record<CatalogType, string[]> = {
  movie: [
    'popularity.desc', 'popularity.asc',
    'vote_average.desc', 'vote_average.asc',
    'vote_count.desc', 'vote_count.asc',
    'primary_release_date.desc', 'primary_release_date.asc',
    'title.asc', 'title.desc',
    'original_title.asc', 'original_title.desc',
    'revenue.desc', 'revenue.asc',
  ],
  series: [
    'popularity.desc', 'popularity.asc',
    'vote_average.desc', 'vote_average.asc',
    'vote_count.desc', 'vote_count.asc',
    'first_air_date.desc', 'first_air_date.asc',
    'name.asc', 'name.desc',
    'original_name.asc', 'original_name.desc',
  ],
}

/** `sort_by` as a field name plus direction, split apart because "asc/desc"
 *  as text inside a dropdown option is easy to misread — a direction toggle
 *  next to a plain field name reads at a glance instead. */
export const SORT_FIELDS: Record<CatalogType, { value: string; label: string }[]> = {
  movie: [
    { value: 'popularity', label: 'Popularity' },
    { value: 'vote_average', label: 'Rating' },
    { value: 'vote_count', label: 'Number of ratings' },
    { value: 'primary_release_date', label: 'Release date' },
    { value: 'title', label: 'Title' },
    { value: 'original_title', label: 'Original title' },
    { value: 'revenue', label: 'Revenue' },
  ],
  series: [
    { value: 'popularity', label: 'Popularity' },
    { value: 'vote_average', label: 'Rating' },
    { value: 'vote_count', label: 'Number of ratings' },
    { value: 'first_air_date', label: 'First aired' },
    { value: 'name', label: 'Name' },
    { value: 'original_name', label: 'Original name' },
  ],
}

export function parseSortBy(sortBy: string | undefined): { field: string; direction: 'asc' | 'desc' } {
  if (!sortBy) return { field: '', direction: 'desc' }
  const dot = sortBy.lastIndexOf('.')
  if (dot === -1) return { field: sortBy, direction: 'desc' }
  const direction = sortBy.slice(dot + 1)
  return { field: sortBy.slice(0, dot), direction: direction === 'asc' ? 'asc' : 'desc' }
}

export function serializeSortBy(field: string, direction: 'asc' | 'desc'): string | undefined {
  return field ? `${field}.${direction}` : undefined
}

export function emptyForm(type: CatalogType = 'movie', collectionID: string | null = null): CatalogFormState {
  return { name: '', type, dateMode: 'any', sourceMode: 'filters', params: {}, collectionID }
}

function dateModeOf(params: TMDBParams, type: CatalogType): DateMode {
  if (type === 'movie') {
    if (params.released_within_days) return 'rolling'
    if (params.primary_release_date_gte || params.primary_release_date_lte) return 'fixed'
  } else {
    if (params.aired_within_days) return 'rolling'
    if (params.first_air_date_gte || params.first_air_date_lte) return 'fixed'
  }
  return 'any'
}

export function formFromCatalog(catalog: Catalog): CatalogFormState {
  const params = parseParams(catalog.params)
  return {
    name: catalog.name,
    type: catalog.type,
    dateMode: dateModeOf(params, catalog.type),
    sourceMode: catalog.type === 'movie' && params.with_collection ? 'collection' : 'filters',
    params,
    collectionID: catalog.collection_id,
  }
}

/**
 * The create payload for an atomic catalog duplicate — `Workspace.tsx`'s
 * `confirmDuplicateCatalog`, which posts this directly rather than opening an
 * editor first. A duplicate gets a distinguishable name and is never born
 * shared or scoped — sharing and scoping are deliberate acts, not
 * something inherited from whoever it was duplicated from; only reachable from
 * the library, which is listed catalogs only. `type`/`params` are copied
 * verbatim: a duplicate is a straight copy.
 */
export function duplicatePayload(catalog: Catalog): CatalogPayload {
  return {
    type: catalog.type,
    name: `${catalog.name} (copy)`,
    provider: CATALOG_PROVIDER,
    params: catalog.params,
    collection_id: null,
  }
}

/** Drops empty strings, zeros and NaN so the payload carries only fields the
 *  user actually set — matching Go's `omitempty`, where an absent field means
 *  "not filtered on" rather than zero. */
function compact(params: TMDBParams): TMDBParams {
  const out: Record<string, unknown> = {}
  for (const [key, value] of Object.entries(params)) {
    if (value === undefined || value === null || value === '') continue
    if (typeof value === 'number' && !Number.isFinite(value)) continue
    if (value === false) continue
    out[key] = value
  }
  return out as TMDBParams
}

/** Strips the date fields the current mode doesn't use, so switching from
 *  fixed to rolling can't leave a stale bound behind that the server would
 *  reject as "both set". */
function applyDateMode(state: CatalogFormState): TMDBParams {
  const p = { ...state.params }
  const fixedKeys =
    state.type === 'movie'
      ? (['primary_release_date_gte', 'primary_release_date_lte'] as const)
      : (['first_air_date_gte', 'first_air_date_lte'] as const)
  const rollingKey = state.type === 'movie' ? 'released_within_days' : 'aired_within_days'

  if (state.dateMode !== 'fixed') for (const key of fixedKeys) delete p[key]
  if (state.dateMode !== 'rolling') delete p[rollingKey]

  // The other branch's date fields must never be sent — a series catalog
  // carrying primary_release_date is silently ignored at best. Certification
  // is shared: movies use theatrical ratings, series use TV content ratings,
  // both scoped by the same certification_country. Networks are series only,
  // and the server rejects them on a movie catalog.
  if (state.type === 'movie') {
    delete p.first_air_date_gte
    delete p.first_air_date_lte
    delete p.aired_within_days
    delete p.with_networks
  } else {
    delete p.primary_release_date_gte
    delete p.primary_release_date_lte
    delete p.released_within_days
  }
  return p
}

/** The only fields a collection row sends. The server reads a collection's
 *  films straight from TMDB and rejects any other filter beside it, so every
 *  field not listed here — including any added later — is dropped. */
const COLLECTION_KEYS = ['with_collection', 'randomized'] as const satisfies readonly (keyof TMDBParams)[]

/** A movie catalog in collection mode lists one TMDB collection's films
 *  rather than running a discover query. */
export function isCollectionRow(state: Pick<CatalogFormState, 'type' | 'sourceMode'>): boolean {
  return state.type === 'movie' && state.sourceMode === 'collection'
}

/** The params a save or preview sends: the date mode applied, then only the
 *  current source mode's fields — `COLLECTION_KEYS` on a collection row, and
 *  everything but `with_collection` otherwise. Form state keeps both sides'
 *  values, so switching mode back brings them back. */
function recipeParams(state: CatalogFormState): TMDBParams {
  const p = applyDateMode(state)
  if (!isCollectionRow(state)) {
    delete p.with_collection
    return p
  }
  const kept: TMDBParams = {}
  for (const key of COLLECTION_KEYS) {
    if (p[key] !== undefined) Object.assign(kept, { [key]: p[key] })
  }
  return kept
}

export type FieldErrors = Partial<Record<string, string>>

/**
 * Mirrors every rule in `TMDBMovieParams.Validate` / `TMDBTVParams.Validate`,
 * plus `CatalogForm.Validate`'s required fields. Keyed by field so the form can
 * render errors in place instead of as a banner.
 */
export function validateForm(state: CatalogFormState): FieldErrors {
  const errors: FieldErrors = {}
  // Checked against what is sent, so a field the current mode drops can't
  // raise an error.
  const p = recipeParams(state)
  const collectionRow = isCollectionRow(state)

  if (!state.name.trim()) errors.name = 'Give this catalog a name.'

  if (collectionRow && parseIdList(p.with_collection).ids.length === 0) {
    errors.with_collection = 'Pick a collection.'
  }

  if (p.sort_by && !SORT_OPTIONS[state.type].includes(p.sort_by)) {
    errors.sort_by = `${state.type === 'movie' ? 'Movies' : 'Series'} can't be sorted this way.`
  }

  // "required together" pairs. Both are grouped controls in the form, so these
  // should be unreachable — they're the backstop, not the mechanism.
  const hasCert = Boolean(p.certification ?? p.certification_gte ?? p.certification_lte)
  if (hasCert && !p.certification_country) {
    errors.certification_country = 'Pick a country — age ratings differ by country.'
  }
  if (p.with_watch_providers && !p.watch_region) {
    errors.watch_region = 'Pick a country — streaming services differ by country.'
  }

  if (state.dateMode === 'rolling' && !collectionRow) {
    const days = state.type === 'movie' ? p.released_within_days : p.aired_within_days
    if (!days || days < 1) errors.within_days = 'Pick how far back to look.'
  }

  const ratingLow = p.vote_average_gte
  const ratingHigh = p.vote_average_lte
  if (ratingLow != null && ratingHigh != null && ratingLow > ratingHigh) {
    errors.vote_average = 'The first number is higher than the second.'
  }
  const runtimeLow = p.with_runtime_gte
  const runtimeHigh = p.with_runtime_lte
  if (runtimeLow != null && runtimeHigh != null && runtimeLow > runtimeHigh) {
    errors.with_runtime = 'The first number is higher than the second.'
  }
  const votesLow = p.vote_count_gte
  const votesHigh = p.vote_count_lte
  if (votesLow != null && votesHigh != null && votesLow > votesHigh) {
    errors.vote_count = 'The first number is higher than the second.'
  }

  if (parseIdList(p.with_companies).ids.length > MAX_ENTITY_IDS) {
    errors.with_companies = `Pick at most ${MAX_ENTITY_IDS} production companies.`
  }
  if (parseIdList(p.with_keywords).ids.length > MAX_ENTITY_IDS) {
    errors.with_keywords = `Pick at most ${MAX_ENTITY_IDS} keywords.`
  }
  if (parseIdList(p.without_companies).ids.length > MAX_ENTITY_IDS) {
    errors.without_companies = `Leave out at most ${MAX_ENTITY_IDS} production companies.`
  }
  if (parseIdList(p.without_keywords).ids.length > MAX_ENTITY_IDS) {
    errors.without_keywords = `Leave out at most ${MAX_ENTITY_IDS} keywords.`
  }
  if (parseIdList(p.with_networks).ids.length > MAX_ENTITY_IDS) {
    errors.with_networks = `Pick at most ${MAX_ENTITY_IDS} networks.`
  }

  return errors
}

/**
 * The form's filters as the wire string — `catalogs.params`, and the `params`
 * field of a preview request.
 *
 * Split out of `toPayload` because previewing a recipe needs exactly this and
 * none of the rest: the preview endpoint takes `{type, params}` and no identity
 * at all, so a catalog that doesn't exist yet can still be run.
 */
export function paramsString(state: CatalogFormState): string {
  return JSON.stringify(compact(recipeParams(state)))
}

export function toPayload(state: CatalogFormState): CatalogPayload {
  return {
    type: state.type,
    name: state.name.trim(),
    // Derived, never rendered — see CatalogPayload.
    provider: CATALOG_PROVIDER,
    params: paramsString(state),
  }
}

/**
 * Structural equality over everything that reaches the wire, plus the scope
 * (`collectionID`), so the editor can tell an untouched form from an edited
 * one without diffing by hand.
 *
 * The dismissal surface is the whole Library rail: selecting another row
 * replaces the editor, and selecting rows is the main thing that rail is for.
 * So this short form needs the same guard as the collection builder's folder
 * tree.
 */
export function isSameCatalog(a: CatalogFormState, b: CatalogFormState): boolean {
  return comparable(a) === comparable(b)
}

/** What `isSameCatalog` compares: the payload and the scope, which the
 *  payload leaves out. */
function comparable(state: CatalogFormState): string {
  return JSON.stringify([toPayload(state), state.collectionID])
}
