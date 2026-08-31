import { getJSON, getList, sendJSON } from './http'
import type {
  Catalog,
  CatalogPreview,
  CatalogType,
  CertificationsByCountry,
  Collection,
  Country,
  Genre,
  Language,
  NuvioProfile,
  PreviewRequest,
  SelectedCatalog,
  SelectedProfile,
} from './types'

/**
 * One function per endpoint. Both halves of each pair are always fetched —
 * `GET /api/catalogs` returns *your own* public catalogs alongside everyone
 * else's (`GetCommunityCatalogs` is `is_public = TRUE` with no owner
 * exclusion), so neither response alone can tell you what you own. The merge
 * and the ownership tagging live in `features/library/useLibrary`.
 */

export function fetchOwnedCatalogs(profileIndex: number): Promise<Catalog[]> {
  return getList<Catalog>(`/api/p/${profileIndex}/catalogs`)
}

export function fetchCommunityCatalogs(): Promise<Catalog[]> {
  return getList<Catalog>('/api/catalogs')
}

export function fetchOwnedCollections(profileIndex: number): Promise<Collection[]> {
  return getList<Collection>(`/api/p/${profileIndex}/collections`)
}

export function fetchCommunityCollections(): Promise<Collection[]> {
  return getList<Collection>('/api/collections')
}

/**
 * The profile's current selection — what is live on the TV right now, and the
 * baseline the Home pane's pending edits are diffed against. Both are ordered
 * by `sort_order`; that order is the value, so preserve it.
 *
 * There is no write function here. Selection is only ever persisted by Push,
 * which sends the whole pending selection in its own request body. Nothing on
 * the Home pane writes on its own.
 */
export function fetchCatalogSelection(profileIndex: number): Promise<SelectedCatalog[]> {
  return getList<SelectedCatalog>(`/api/p/${profileIndex}/catalogs/selection`)
}

export function fetchCollectionSelection(profileIndex: number): Promise<Collection[]> {
  return getList<Collection>(`/api/p/${profileIndex}/collections/selection`)
}

/** Live from TMDB via the Go side. Takes the catalog's own `type`
 *  (`movie`/`series`), same as every other endpoint — the backend derives
 *  TMDB's `movie`/`tv` vocabulary internally. */
export function fetchGenres(type: CatalogType): Promise<Genre[]> {
  return getList<Genre>(`/api/genres/${type}`)
}

/** Live from TMDB via the Go side, same shape as `fetchGenres` — every
 *  country's age-rating scale for this catalog type. */
export function fetchCertifications(type: CatalogType): Promise<CertificationsByCountry> {
  return getJSON<CertificationsByCountry>(`/api/certifications/${type}`)
}

/** Live from TMDB via the Go side. Not split by catalog type — TMDB's
 *  language table backs `with_original_language` on both movies and tv. */
export function fetchLanguages(): Promise<Language[]> {
  return getList<Language>('/api/languages')
}

/** Live from TMDB via the Go side. Names the codes `fetchCertifications`
 *  returns — that response is keyed by code, with no name attached. */
export function fetchCountries(): Promise<Country[]> {
  return getList<Country>('/api/countries')
}

/**
 * Runs a catalog recipe against TMDB and returns tiles, saving nothing.
 *
 * A `POST` that creates no resource, because the recipe is the input and it's
 * too big and too structured to be a query string. Not profile-scoped — there's
 * no vault read behind it, so a 404 here is a genuine routing error rather than
 * `ProfileNotSelectedError`.
 *
 * It takes a raw recipe rather than a saved catalog id so the same call serves a
 * catalog being tuned in the builder, which has no id yet.
 *
 * A `502` means TMDB was unreachable. Callers keep their placeholder tiles on
 * that rather than rendering an error, degrading to a layout-only preview.
 */
export function fetchCatalogPreview(body: PreviewRequest): Promise<CatalogPreview> {
  return sendJSON<CatalogPreview>('POST', '/api/catalogs/preview', body)
}

/**
 * Every profile on the caller's live Nuvio account. Not profile-scoped —
 * this is what the picker chooses *from*, so there is no profile to scope it
 * to yet.
 */
export function fetchProfiles(): Promise<NuvioProfile[]> {
  return getList<NuvioProfile>('/api/profiles')
}

/**
 * Resolves (or creates) Uno's own profile row for one Nuvio profile slot.
 * The picker's only write: everything past this point in the app assumes a
 * profile has been selected.
 */
export function selectProfile(profileIndex: number): Promise<SelectedProfile> {
  return sendJSON<SelectedProfile>('POST', '/api/profiles/select', { profile_index: profileIndex })
}
