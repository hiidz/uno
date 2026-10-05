import { useMutation, useQueryClient } from '@tanstack/react-query'
import { createCollection, deleteCollection, duplicateCollection, queryKeys, updateCollection } from '@/api'
import type { CollectionPayload } from '@/api'

/**
 * Collection writes, with the invalidation they imply.
 *
 * Invalidates Community too, as `useCatalogMutations` does: a save can change
 * whether a published collection has changed since it was published, and delete
 * unpublishes it. That also refetches the collection selection
 * query, since
 * `['p', i, 'collections']` prefix-matches its `…, 'selection'` child;
 * selection is client state until Push, and the one-shot hydration guard in
 * `HomeSelectionContext` is what keeps that refetch from clobbering the user's
 * pending home-screen edits.
 *
 * The catalog list is never invalidated here: edits to catalogs inside a
 * collection change scoped rows, which the catalog list never holds. The
 * dependency runs the other way, which is why `useCatalogMutations` is the
 * hook that has to invalidate *collections*.
 */
export function useCollectionMutations(profileIndex: number) {
  const queryClient = useQueryClient()

  function invalidate() {
    void queryClient.invalidateQueries({ queryKey: queryKeys.ownedCollections(profileIndex) })
    void queryClient.invalidateQueries({ queryKey: queryKeys.pendingPush(profileIndex) })
    void queryClient.invalidateQueries({ queryKey: queryKeys.community(profileIndex) })
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

  const duplicate = useMutation({
    mutationFn: (id: string) => duplicateCollection(profileIndex, id),
    onSuccess: invalidate,
  })

  return { create, update, remove, duplicate }
}
