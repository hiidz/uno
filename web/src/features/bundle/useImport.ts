import { useMutation, useQueryClient } from '@tanstack/react-query'
import { importBundle, queryKeys } from '@/api'

/**
 * Writes a bundle, then refreshes the library it added to. Its promise settles
 * only once the library has refetched, so the outcome message arrives with the
 * new rows already in the rail. Community is left alone, since no private row
 * appears there.
 */
export function useImport(profileIndex: number) {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: ({ bundle, reuse, skip }: { bundle: unknown; reuse: Record<string, string>; skip: number[] }) =>
      importBundle(profileIndex, bundle, reuse, skip),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: queryKeys.library(profileIndex) }),
  })
}
