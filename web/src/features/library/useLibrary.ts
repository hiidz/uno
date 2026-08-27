import { useMemo } from 'react'
import { useQueries } from '@tanstack/react-query'
import {
  fetchCertifications,
  fetchCommunityCatalogs,
  fetchCommunityCollections,
  fetchGenres,
  fetchOwnedCatalogs,
  fetchOwnedCollections,
  queryKeys,
} from '@/api'
import type { Catalog, CertificationsByCountry, Collection, Folder } from '@/api'
import { mergeOwned } from './merge'
import { buildGenreLookup, type GenreLookup } from './recipe'

export interface LibraryCatalog extends Catalog {
  owned: boolean
}

export interface LibraryCollection extends Omit<Collection, 'folders'> {
  owned: boolean
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
export interface CertificationLookups {
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
  /** Every country's age-rating scale, for the certification picker. Empty
   *  until the queries land — same degrade-gracefully treatment as genres. */
  certifications: CertificationLookups
  isLoading: boolean
  error: Error | null
  refetch: () => void
}

export function useLibrary(profileIndex: number): Library {
  const results = useQueries({
    queries: [
      {
        queryKey: queryKeys.ownedCatalogs(profileIndex),
        queryFn: () => fetchOwnedCatalogs(profileIndex),
      },
      { queryKey: queryKeys.communityCatalogs(), queryFn: fetchCommunityCatalogs },
      {
        queryKey: queryKeys.ownedCollections(profileIndex),
        queryFn: () => fetchOwnedCollections(profileIndex),
      },
      { queryKey: queryKeys.communityCollections(), queryFn: fetchCommunityCollections },
    ],
  })

  const [ownedCatalogs, communityCatalogs, ownedCollections, communityCollections] = results

  // Genre lists are static and account-wide, so they're cached indefinitely and
  // excluded from the loading/error state below: a failed genre lookup degrades
  // the recipe line to raw ids, it doesn't fail a list.
  const genreResults = useQueries({
    queries: [
      { queryKey: queryKeys.genres('movie'), queryFn: () => fetchGenres('movie'), staleTime: Infinity },
      { queryKey: queryKeys.genres('series'), queryFn: () => fetchGenres('series'), staleTime: Infinity },
    ],
  })

  const movieGenres = genreResults[0].data
  const tvGenres = genreResults[1].data
  const genres = useMemo<GenreLookups>(
    () => ({ movie: buildGenreLookup(movieGenres ?? []), tv: buildGenreLookup(tvGenres ?? []) }),
    [movieGenres, tvGenres],
  )

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

  const catalogs = useMemo(
    () => mergeOwned(ownedCatalogs.data ?? [], communityCatalogs.data ?? []),
    [ownedCatalogs.data, communityCatalogs.data],
  )

  const collections = useMemo(
    () =>
      mergeOwned(ownedCollections.data ?? [], communityCollections.data ?? []).map((c) => ({
        ...c,
        folders: c.folders ?? [],
      })),
    [ownedCollections.data, communityCollections.data],
  )

  return {
    catalogs,
    collections,
    genres,
    certifications,
    isLoading: results.some((r) => r.isPending),
    error: (results.find((r) => r.error)?.error as Error | undefined) ?? null,
    refetch: () => {
      for (const r of results) void r.refetch()
    },
  }
}
