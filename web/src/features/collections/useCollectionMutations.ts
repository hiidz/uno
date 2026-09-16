import { useMutation, useQueryClient } from '@tanstack/react-query'
import { createCollection, deleteCollection, queryKeys, updateCollection } from '@/api'
import type { CollectionPayload } from '@/api'

/**
 * Collection writes, with the invalidation they imply.
 *
 * Same rule as `useCatalogMutations`, for the same reason: both the owned list
 * and the community list have to be invalidated on every write, because
 * `is_public` can change on any save and a public collection appears in *both*
 * responses. That also refetches the collection selection query, since
 * `['p', i, 'collections']` prefix-matches its `…, 'selection'` child;
 * selection is client state until Push, and the one-shot hydration guard in
 * `HomeSelectionContext` is what keeps that refetch from clobbering the user's
 * pending home-screen edits.
 *
 * Nothing here touches the catalog queries. A collection references catalogs
 * but never modifies them; the dependency runs the other way, which is why
 * `useCatalogMutations` is the hook that has to invalidate *collections*.
 */
export function useCollectionMutations(profileIndex: number) {
  const queryClient = useQueryClient()

  function invalidate() {
    void queryClient.invalidateQueries({ queryKey: queryKeys.ownedCollections(profileIndex) })
    void queryClient.invalidateQueries({ queryKey: queryKeys.communityCollections(profileIndex) })
  }

  const create = useMutation({
    mutationFn: (payload: CollectionPayload) => createCollection(profileIndex, payload),
    onSuccess: invalidate,
  })

  const update = useMutation({
    mutationFn: ({ id, payload }: { id: string; payload: CollectionPayload }) =>
      updateCollection(profileIndex, id, payload),
    onSuccess: invalidate,
  })

  const remove = useMutation({
    mutationFn: (id: string) => deleteCollection(profileIndex, id),
    onSuccess: invalidate,
  })

  return { create, update, remove }
}
