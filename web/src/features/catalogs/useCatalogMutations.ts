import { useMutation, useQueryClient } from '@tanstack/react-query'
import { createCatalog, deleteCatalog, invalidateProfileLists, updateCatalog } from '@/api'
import type { CatalogPayload } from '@/api'

/**
 * Catalog writes, with the invalidation they imply.
 *
 * Invalidates the community query keys too: `is_public` can change on any
 * save, and the Community tab (`features/community/`) reads exactly those
 * keys, so a save here has to keep its list current.
 *
 * The collection lists are invalidated too, because of a real cascade:
 * `DELETE FROM catalogs` drops the row's `folder_catalogs` entries
 * (`ON DELETE CASCADE`), so every cached collection that referenced it is now
 * wrong — the collection editor would keep listing a folder member that no
 * longer exists, and saving that collection would `400` on a catalog id the
 * user can't see. Create takes the same path because it adds a row a folder
 * may reference.
 *
 * The selection queries are refetched as well: their keys sit under the owned
 * list keys and invalidation prefix-matches, so `['p', i, 'catalogs']` and
 * `['p', i, 'collections']` each take their `…, 'selection'` child with them.
 * Selection is client state until Push, and the one-shot hydration guard in
 * `HomeSelectionContext` is what keeps those refetches from clobbering the
 * user's pending home-screen edits.
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

  const update = useMutation({
    mutationFn: ({ id, payload }: { id: string; payload: CatalogPayload }) =>
      updateCatalog(profileIndex, id, payload),
    onSuccess: invalidate,
  })

  const remove = useMutation({
    mutationFn: (id: string) => deleteCatalog(profileIndex, id),
    onSuccess: invalidate,
  })

  return { create, update, remove }
}
