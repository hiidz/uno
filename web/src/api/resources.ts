import { getJSON, getList, sendJSON } from './http'
import type {
  Catalog,
  CatalogPreview,
  CatalogType,
  CertificationsByCountry,
  Collection,
  CommunityCatalog,
  CommunityCollection,
  Company,
  CompanySearchResult,
  Country,
  Genre,
  GenreOptionsRequest,
  Keyword,
  Language,
  NuvioProfile,
  PreviewRequest,
  SelectedCatalog,
  SelectedProfile,
  TMDBCollection,
  WatchProvider,
  WatchRegion,
} from './types'

/** One function per endpoint. The community routes are profile-scoped: the
 *  closed graph means the server excludes the caller's own rows and computes
 *  `taken` server-side, so there is no merge to do on this side. */

export function fetchOwnedCatalogs(profileIndex: number): Promise<Catalog[]> {
  return getList<Catalog>(`/api/p/${profileIndex}/catalogs`)
}

export function fetchCommunityCatalogs(profileIndex: number): Promise<CommunityCatalog[]> {
  return getList<CommunityCatalog>(`/api/p/${profileIndex}/community/catalogs`)
}

export function fetchOwnedCollections(profileIndex: number): Promise<Collection[]> {
  return getList<Collection>(`/api/p/${profileIndex}/collections`)
}

export function fetchCommunityCollections(profileIndex: number): Promise<CommunityCollection[]> {
  return getList<CommunityCollection>(`/api/p/${profileIndex}/community/collections`)
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

/** Live from TMDB via the Go side. Scoped to a region because a service
 *  carries a different id per market — passing none returns everything TMDB
 *  knows about, which is what the picker shows before a region is chosen. */
export function fetchWatchProviders(type: CatalogType, region: string): Promise<WatchProvider[]> {
  const query = region ? `?region=${encodeURIComponent(region)}` : ''
  return getList<WatchProvider>(`/api/watch-providers/${type}${query}`)
}

/** Live from TMDB via the Go side. The countries TMDB has streaming data
 *  for — a strict subset of `fetchCountries`, so `watch_region` reads from
 *  this one. */
export function fetchWatchRegions(): Promise<WatchRegion[]> {
  return getList<WatchRegion>('/api/watch-regions')
}

/** Live from TMDB via the Go side: production companies whose name matches
 *  `query`, counted and filtered for catalogs of `type`. The server rejects a
 *  blank query, so callers only send a trimmed, non-empty one. */
export function searchCompanies(query: string, type: CatalogType): Promise<CompanySearchResult[]> {
  return getList<CompanySearchResult>(
    `/api/companies/search?q=${encodeURIComponent(query)}&type=${type}`,
  )
}

/** Names one company id. A `404` means TMDB has no company with that id. */
export function fetchCompany(id: number): Promise<Company> {
  return getJSON<Company>(`/api/companies/${id}`)
}

/** Live from TMDB via the Go side: keywords matching `query`. The server
 *  rejects a blank query, so callers only send a trimmed, non-empty one. */
export function searchKeywords(query: string): Promise<Keyword[]> {
  return getList<Keyword>(`/api/keywords/search?q=${encodeURIComponent(query)}`)
}

/** Names one keyword id. A `404` means TMDB has no keyword with that id. */
export function fetchKeyword(id: number): Promise<Keyword> {
  return getJSON<Keyword>(`/api/keywords/${id}`)
}

/** Live from TMDB via the Go side: movie collections matching `query`. The
 *  server rejects a blank query, so callers only send a trimmed, non-empty one. */
export function searchCollections(query: string): Promise<TMDBCollection[]> {
  return getList<TMDBCollection>(`/api/collections/search?q=${encodeURIComponent(query)}`)
}

/** Names one collection id. A `404` means TMDB has no collection with that id. */
export function fetchCollection(id: number): Promise<TMDBCollection> {
  return getJSON<TMDBCollection>(`/api/collections/${id}`)
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
 * The genres a pick can narrow this recipe by — `provider.GenreExtraOptions`,
 * the same list the addon manifest advertises for the catalog. Not
 * `fetchGenres`: that is TMDB's whole list, and a genre the recipe already
 * requires, excludes, or (with an "any of" list) leaves out narrows nothing.
 */
export function fetchCatalogGenreOptions(body: GenreOptionsRequest): Promise<Genre[]> {
  return sendJSON<Genre[]>('POST', '/api/catalogs/genre-options', body)
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
