import { useMutation, useQueryClient } from '@tanstack/react-query'
import { queryKeys, takeCatalog, takeCollection } from '@/api'

/**
 * Take, for both kinds. A copy is a fresh, private row this profile fully
 * owns — nothing about the source or the copy changes together afterwards,
 * so there is nothing to reconcile beyond
 * refetching the two lists that just became stale: the library gained a row,
 * and the community list's `taken` flag on this source flipped. Taking again
 * is allowed and creates another copy, so nothing here disables a taken row.
 */
export function useCommunityMutations(profileIndex: number) {
  const queryClient = useQueryClient()

  function invalidate() {
    void queryClient.invalidateQueries({ queryKey: queryKeys.ownedCatalogs(profileIndex) })
    void queryClient.invalidateQueries({ queryKey: queryKeys.ownedCollections(profileIndex) })
    void queryClient.invalidateQueries({ queryKey: queryKeys.communityCatalogs(profileIndex) })
    void queryClient.invalidateQueries({ queryKey: queryKeys.communityCollections(profileIndex) })
  }

  const takeCatalogMutation = useMutation({
    mutationFn: (catalogID: string) => takeCatalog(profileIndex, catalogID),
    onSuccess: invalidate,
  })

  const takeCollectionMutation = useMutation({
    mutationFn: (collectionID: string) => takeCollection(profileIndex, collectionID),
    onSuccess: invalidate,
  })

  return { takeCatalog: takeCatalogMutation, takeCollection: takeCollectionMutation }
}
