import { useMemo } from 'react'
import { useQueries, useQuery } from '@tanstack/react-query'
import {
  fetchCertifications,
  fetchCountries,
  fetchGenres,
  fetchLanguages,
  fetchOwnedCatalogs,
  fetchOwnedCollections,
  ProfileNotSelectedError,
  queryKeys,
} from '@/api'
import type { Catalog, CertificationsByCountry, Collection, Folder, Genre, Language } from '@/api'
import { buildCountryLookup, type CountryLookup } from '@/features/catalogs/countries'
import { buildGenreLookup, type GenreLookup } from './recipe'

/** The library is exactly this profile's own catalogs — the closed-graph model
 *  never offers another owner's rows to reference or edit here. Aliased,
 *  rather than used as `Catalog` directly, so the many call sites naming
 *  "the library's catalog type" don't all silently start meaning something
 *  else if the two types ever diverge again. */
export type LibraryCatalog = Catalog

export interface LibraryCollection extends Omit<Collection, 'folders'> {
  /** Coerced as a guard — `parseFolders` initialises its slice, so the wire
   *  shouldn't carry `null` for a collection with no folders. */
  folders: Folder[]
}

/**
 * Kept per-kind rather than merged into one map. TMDB's movie and tv genre id
 * spaces are separate: `878` is Science Fiction on movies and doesn't exist on
 * tv, which has `10765` Sci-Fi & Fantasy instead. Resolving a series catalog's
 * genre against the movie list mislabels silently rather than failing.
 */
export interface GenreLookups {
  movie: GenreLookup
  tv: GenreLookup
}

/** Same per-kind split as GenreLookups, and for the same reason: a country's
 *  movie ratings and its TV ratings are different scales entirely (US movies
 *  go G/PG/PG-13/R/NC-17, US TV goes TV-Y/TV-PG/TV-14/TV-MA). */
interface CertificationLookups {
  movie: CertificationsByCountry
  tv: CertificationsByCountry
}

export interface Library {
  catalogs: LibraryCatalog[]
  collections: LibraryCollection[]
  /** Resolves TMDB genre ids to names for the recipe line. Empty until the
   *  genre queries land — recipes fall back to raw ids, which is degraded but
   *  not broken, so the list never blocks on it. */
  genres: GenreLookups
  /** The same genres as lists, in TMDB's order, for the catalog editor's
   *  genre chips. Empty until the queries land, like `genres`. */
  genreLists: { movie: Genre[]; tv: Genre[] }
  /** Every country's age-rating scale, for the certification picker. Empty
   *  until the queries land — same degrade-gracefully treatment as genres. */
  certifications: CertificationLookups
  /** TMDB's full language table, for the "Original language" picker. Empty
   *  until the query lands — same degrade-gracefully treatment as genres. */
  languages: Language[]
  /** Names the country codes in `certifications`, keyed by ISO 3166-1. Empty
   *  until the query lands — same degrade-gracefully treatment as genres. */
  countryNames: CountryLookup
  isLoading: boolean
  /** Only set while a list has no rows to show — see `error` below. */
  error: Error | null
  /** Which of the two lists that is, so the one that did load keeps its rows. */
  failed: { catalogs: boolean; collections: boolean }
  /** Both lists have rows to show. The Home pane hydrates from them, so it
   *  waits for both: a Home read from one list alone would push without the
   *  other's rows, and Push replaces what Nuvio holds. */
  listsLoaded: boolean
  refetch: () => void
}

/** Stable fallbacks for a list that hasn't loaded, so memos keyed on it don't
 *  recompute on every render while it's pending or failed. */
const NO_CATALOGS: LibraryCatalog[] = []
const NO_GENRES: Genre[] = []
const NO_LANGUAGES: Language[] = []

/**
 * Both types' TMDB genres, as lists and as id → name lookups. Genre lists are
 * static and account-wide, so they're cached indefinitely and kept out of any
 * loading or error state: a failed genre lookup degrades a recipe line to raw
 * ids, it doesn't fail what shows it. Community reads these alone.
 */
export function useGenreLookups(): Pick<Library, 'genres' | 'genreLists'> {
  const genreResults = useQueries({
    queries: [
      { queryKey: queryKeys.genres('movie'), queryFn: () => fetchGenres('movie'), staleTime: Infinity },
      { queryKey: queryKeys.genres('series'), queryFn: () => fetchGenres('series'), staleTime: Infinity },
    ],
  })

  const movieGenres = genreResults[0].data ?? NO_GENRES
  const tvGenres = genreResults[1].data ?? NO_GENRES
  const genreLists = useMemo(() => ({ movie: movieGenres, tv: tvGenres }), [movieGenres, tvGenres])
  const genres = useMemo<GenreLookups>(
    () => ({ movie: buildGenreLookup(movieGenres), tv: buildGenreLookup(tvGenres) }),
    [movieGenres, tvGenres],
  )
  return { genres, genreLists }
}

export function useLibrary(profileIndex: number): Library {
  const results = useQueries({
    queries: [
      {
        queryKey: queryKeys.ownedCatalogs(profileIndex),
        queryFn: () => fetchOwnedCatalogs(profileIndex),
      },
      {
        queryKey: queryKeys.ownedCollections(profileIndex),
        queryFn: () => fetchOwnedCollections(profileIndex),
      },
    ],
  })

  const [ownedCatalogs, ownedCollections] = results

  const { genres, genreLists } = useGenreLookups()

  const certificationResults = useQueries({
    queries: [
      {
        queryKey: queryKeys.certifications('movie'),
        queryFn: () => fetchCertifications('movie'),
        staleTime: Infinity,
      },
      {
        queryKey: queryKeys.certifications('series'),
        queryFn: () => fetchCertifications('series'),
        staleTime: Infinity,
      },
    ],
  })

  const movieCertifications = certificationResults[0].data
  const tvCertifications = certificationResults[1].data
  const certifications = useMemo<CertificationLookups>(
    () => ({ movie: movieCertifications ?? {}, tv: tvCertifications ?? {} }),
    [movieCertifications, tvCertifications],
  )

  // Same account-wide, cached-indefinitely treatment as genres above — TMDB's
  // language table barely changes and a failed fetch just leaves the picker
  // empty rather than failing the library.
  const languagesResult = useQuery({
    queryKey: queryKeys.languages(),
    queryFn: fetchLanguages,
    staleTime: Infinity,
  })
  const languages = languagesResult.data ?? NO_LANGUAGES

  const countriesResult = useQuery({
    queryKey: queryKeys.countries(),
    queryFn: fetchCountries,
    staleTime: Infinity,
  })
  const countryNames = useMemo(() => buildCountryLookup(countriesResult.data ?? []), [countriesResult.data])

  const catalogs = ownedCatalogs.data ?? NO_CATALOGS

  const collections = useMemo(
    () => (ownedCollections.data ?? []).map((c) => ({ ...c, folders: c.folders ?? [] })),
    [ownedCollections.data],
  )

  return {
    catalogs,
    collections,
    genres,
    genreLists,
    certifications,
    languages,
    countryNames,
    isLoading: results.some((r) => r.isPending),
    // A failed background refetch keeps the rows it already had, so only a
    // query with nothing to show counts as failed. A profile-not-selected 404
    // always counts: it is what sends the builder back to the picker.
    error:
      (results.find(
        (r) => r.error && (r.data === undefined || r.error instanceof ProfileNotSelectedError),
      )?.error as Error | undefined) ?? null,
    failed: {
      catalogs: ownedCatalogs.isError && ownedCatalogs.data === undefined,
      collections: ownedCollections.isError && ownedCollections.data === undefined,
    },
    listsLoaded: ownedCatalogs.data !== undefined && ownedCollections.data !== undefined,
    refetch: () => {
      for (const r of results) void r.refetch()
    },
  }
}
