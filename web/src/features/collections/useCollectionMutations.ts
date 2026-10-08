import { useMutation, useQueryClient } from '@tanstack/react-query'
import {
  createCollection,
  deleteCollection,
  duplicateCollection,
  invalidateProfileLists,
  updateCollection,
} from '@/api'
import type { CollectionPayload, CollectionSave } from '@/api'

/**
 * Collection writes, with the invalidation they imply.
 *
 * Refreshes the library, which holds the pending push list too, and Community,
 * as `useCatalogMutations` does: a save can change whether a published
 * collection has changed since it was published, and delete unpublishes it.
 * The refetch is safe for the user's pending home-screen edits: they are client
 * state until Push, and the one-shot hydration guard in `HomeSelectionContext`
 * keeps the refetch from clobbering them.
 */
export function useCollectionMutations(profileIndex: number) {
  const queryClient = useQueryClient()

  function invalidate() {
    void invalidateProfileLists(queryClient, profileIndex)
  }

  const create = useMutation({
    mutationFn: (payload: CollectionPayload) => createCollection(profileIndex, payload),
    onSuccess: invalidate,
  })

  const update = useMutation({
    mutationFn: ({ id, payload }: { id: string; payload: CollectionSave }) =>
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
