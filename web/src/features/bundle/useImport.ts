import { useMutation, useQueryClient } from '@tanstack/react-query'
import { importBundle, queryKeys } from '@/api'

/**
 * Writes a bundle, then refreshes the library it added to. Its promise settles
 * only once both owned lists have refetched, so the outcome message arrives
 * with the new rows already in the rail.
 *
 * The owned-list keys prefix the selection and Community keys, so those are
 * marked stale too. An import changes neither — every row it writes is
 * private and off Home — and a refetch of either returns what they held.
 */
export function useImport(profileIndex: number) {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: ({ bundle, reuse }: { bundle: unknown; reuse: Record<string, string> }) =>
      importBundle(profileIndex, bundle, reuse),
    onSuccess: () =>
      Promise.all([
        queryClient.invalidateQueries({ queryKey: queryKeys.ownedCatalogs(profileIndex) }),
        queryClient.invalidateQueries({ queryKey: queryKeys.ownedCollections(profileIndex) }),
      ]),
  })
}
