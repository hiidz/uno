import { useMutation, useQueryClient } from '@tanstack/react-query'
import { createCatalog, deleteCatalog, duplicateCatalog, invalidateProfileLists, updateCatalog } from '@/api'
import type { CatalogPayload, CatalogSave } from '@/api'

/**
 * Catalog writes, with the invalidation they imply.
 *
 * Invalidates Community too (`invalidateProfileLists`): a save can change
 * whether a published catalog has changed since it was published, and a
 * delete unpublishes it.
 *
 * The library holds the collections too, so one refetch covers a real cascade:
 * `DELETE FROM catalogs` drops the row's `folder_catalogs` entries
 * (`ON DELETE CASCADE`), so every cached collection that referenced it is now
 * wrong — the collection editor would keep listing a folder member that no
 * longer exists, and saving that collection would `400` on a catalog id the
 * user can't see. Create takes the same path because it adds a row a folder
 * may reference. What waits for a push is in the library as well.
 *
 * Home's selection is client state until Push, and the one-shot hydration
 * guard in `HomeSelectionContext` is what keeps the refetch from clobbering the
 * user's pending home-screen edits; only a row the refetch shows deleted leaves
 * them (`usePrunedHome`).
 */
export function useCatalogMutations(profileIndex: number) {
  const queryClient = useQueryClient()

  // Not awaited: a save settles when the server answers, not when the lists
  // have refetched behind it.
  function invalidate() {
    void invalidateProfileLists(queryClient, profileIndex)
  }

  const create = useMutation({
    mutationFn: (payload: CatalogPayload) => createCatalog(profileIndex, payload),
    onSuccess: invalidate,
  })

  const duplicate = useMutation({
    mutationFn: (id: string) => duplicateCatalog(profileIndex, id),
    onSuccess: invalidate,
  })

  const update = useMutation({
    mutationFn: ({ id, payload }: { id: string; payload: CatalogSave }) =>
      updateCatalog(profileIndex, id, payload),
    onSuccess: invalidate,
  })

  const remove = useMutation({
    mutationFn: (id: string) => deleteCatalog(profileIndex, id),
    onSuccess: invalidate,
  })

  return { create, duplicate, update, remove }
}
