import { useMutation, useQueryClient } from '@tanstack/react-query'
import { importBundle, queryKeys } from '@/api'

/**
 * Writes a bundle, then refreshes the library it added to. Its promise settles
 * only once both owned lists have refetched, so the outcome message arrives
 * with the new rows already in the rail.
 *
 * The owned-list keys prefix the selection keys, so those are marked stale
 * too. An import doesn't change them — every row it writes is private and off
 * Home — and a refetch returns what they held. Community is left alone, since
 * no private row appears there.
 */
export function useImport(profileIndex: number) {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: ({ bundle, reuse, skip }: { bundle: unknown; reuse: Record<string, string>; skip: number[] }) =>
      importBundle(profileIndex, bundle, reuse, skip),
    onSuccess: () =>
      Promise.all([
        queryClient.invalidateQueries({ queryKey: queryKeys.ownedCatalogs(profileIndex) }),
        queryClient.invalidateQueries({ queryKey: queryKeys.ownedCollections(profileIndex) }),
      ]),
  })
}
