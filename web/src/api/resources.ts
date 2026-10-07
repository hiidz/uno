import { getJSON, getList, sendJSON } from './http'
import type {
  CatalogPreview,
  CatalogType,
  CertificationsByCountry,
  CommunityItem,
  Company,
  CompanySearchResult,
  Country,
  Genre,
  GenreOptionsRequest,
  Keyword,
  Language,
  LibraryData,
  Network,
  NetworkSearchResult,
  NuvioProfile,
  PreviewRequest,
  PublicationDetail,
  ReleasedCopy,
  SelectedProfile,
  ServerConfig,
  SnapshotChange,
  TMDBCollection,
  TMDBKeyStatus,
  WatchProvider,
  WatchRegion,
} from './types'

/** One function per endpoint. The Community routes are profile-scoped: the
 *  server leaves out the caller's own publications and marks the ones it
 *  subscribes to, so there is no merge to do on this side. */

/** Everything the builder shows for a profile, in one call: its listed
 *  catalogs, its collections, and what a push of the Home as the server stores
 *  it would change in Nuvio. The Home pane adds its own unpushed edits to the
 *  pending list. */
export function fetchLibrary(profileIndex: number): Promise<LibraryData> {
  return getJSON<LibraryData>(`/api/p/${profileIndex}/library`)
}

/** Every live publication Community lists, in one call: the SPA searches,
 *  filters and sorts them itself (`features/community/communityQuery.ts`). */
export function fetchCommunity(profileIndex: number): Promise<CommunityItem[]> {
  return getList<CommunityItem>(`/api/p/${profileIndex}/community`)
}

/** One publication with its snapshot. 404s once it is unpublished. */
export function fetchPublication(profileIndex: number, publicationID: string): Promise<PublicationDetail> {
  return getJSON<PublicationDetail>(`/api/p/${profileIndex}/community/${publicationID}`)
}

/** What an Update would change in this profile's added row, row by row: its
 *  copy against the publisher's latest version. Empty for a row in step. */
export function fetchUpdateChanges(profileIndex: number, publicationID: string): Promise<SnapshotChange[]> {
  return getList<SnapshotChange>(`/api/p/${profileIndex}/community/${publicationID}/changes`)
}

/** What publishing an own row again would change in its publication: the saved
 *  row against what it last published. Empty for a row never published. */
export function fetchChangesSincePublish(
  profileIndex: number,
  kind: 'catalog' | 'collection',
  id: string,
): Promise<SnapshotChange[]> {
  return getList<SnapshotChange>(`/api/p/${profileIndex}/${kind}s/${id}/changes-since-publish`)
}

/** This profile's rows released and not yet acknowledged: copies whose
 *  publisher unpublished them, oldest release first. */
export function fetchReleased(profileIndex: number): Promise<ReleasedCopy[]> {
  return getList<ReleasedCopy>(`/api/p/${profileIndex}/released`)
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

/** Live from TMDB via the Go side: TV networks whose name matches `query`,
 *  counted and filtered like `searchCompanies` for series. The server rejects
 *  a blank query, so callers only send a trimmed, non-empty one. */
export function searchNetworks(query: string): Promise<NetworkSearchResult[]> {
  return getList<NetworkSearchResult>(`/api/networks/search?q=${encodeURIComponent(query)}`)
}

/** Names one network id. A `404` means TMDB has no network with that id. */
export function fetchNetwork(id: number): Promise<Network> {
  return getJSON<Network>(`/api/networks/${id}`)
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

/** How this server reaches TMDB. Needs no sign-in. */
export function fetchServerConfig(): Promise<ServerConfig> {
  return getJSON<ServerConfig>('/api/config')
}

/** Whether the signed-in account has saved a TMDB key, on a server where each
 *  account brings one (a 404 anywhere else). */
export function fetchTMDBKey(): Promise<TMDBKeyStatus> {
  return getJSON<TMDBKeyStatus>('/api/account/tmdb-key')
}
