import { useMutation, useQueryClient } from '@tanstack/react-query'
import { createCollection, deleteCollection, duplicateCollection, queryKeys, updateCollection } from '@/api'
import type { CollectionPayload } from '@/api'

/**
 * Collection writes, with the invalidation they imply.
 *
 * Same rule as `useCatalogMutations`, for the same reason: invalidates the
 * community query key too, since the Community tab (`features/community/`)
 * reads it and `is_public` can change on any save. That also refetches the
 * collection selection query, since
 * `['p', i, 'collections']` prefix-matches its `…, 'selection'` child;
 * selection is client state until Push, and the one-shot hydration guard in
 * `HomeSelectionContext` is what keeps that refetch from clobbering the user's
 * pending home-screen edits.
 *
 * The catalog list is invalidated only by a save that moves a catalog to the
 * library (a `catalog_edits` entry with `move_to_library`): that is the one
 * collection write that adds a row to it. Other edits to catalogs inside a
 * collection change scoped rows, which the catalog list never holds.
 * Otherwise the dependency runs the other way, which is why
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
    onSuccess: (_collection, { payload }) => {
      invalidate()
      if (payload.catalog_edits.some((edit) => edit.move_to_library)) {
        void queryClient.invalidateQueries({ queryKey: queryKeys.ownedCatalogs(profileIndex) })
      }
    },
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
