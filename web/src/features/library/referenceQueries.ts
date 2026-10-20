import { queryOptions, type QueryClient } from '@tanstack/react-query'
import { fetchCertifications, fetchCountries, fetchGenres, fetchLanguages, queryKeys } from '@/api'
import type { CatalogType } from '@/api'

/**
 * TMDB's reference lists the builder draws a library with: genres and
 * certifications for both types, languages and countries. They are the same
 * for every account and profile, so they sit outside the `['p', i, …]`
 * prefix and are cached indefinitely.
 */
export const referenceQueries = {
  genres: (type: CatalogType) =>
    queryOptions({ queryKey: queryKeys.genres(type), queryFn: () => fetchGenres(type), staleTime: Infinity }),
  certifications: (type: CatalogType) =>
    queryOptions({
      queryKey: queryKeys.certifications(type),
      queryFn: () => fetchCertifications(type),
      staleTime: Infinity,
    }),
  languages: () => queryOptions({ queryKey: queryKeys.languages(), queryFn: () => fetchLanguages(), staleTime: Infinity }),
  countries: () => queryOptions({ queryKey: queryKeys.countries(), queryFn: () => fetchCountries(), staleTime: Infinity }),
}

/** Starts every reference list that isn't cached yet, so they load while a
 *  profile is being selected rather than after. */
export function prefetchReferenceData(queryClient: QueryClient) {
  for (const type of ['movie', 'series'] as const) {
    void queryClient.prefetchQuery(referenceQueries.genres(type))
    void queryClient.prefetchQuery(referenceQueries.certifications(type))
  }
  void queryClient.prefetchQuery(referenceQueries.languages())
  void queryClient.prefetchQuery(referenceQueries.countries())
}
