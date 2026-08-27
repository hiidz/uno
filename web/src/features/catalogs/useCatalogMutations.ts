import { useMutation, useQueryClient } from '@tanstack/react-query'
import { createCatalog, deleteCatalog, queryKeys, updateCatalog } from '@/api'
import type { CatalogPayload } from '@/api'

/**
 * Catalog writes, with the invalidation they imply.
 *
 * Both the owned list and the community list have to be invalidated on every
 * write, not just the owned one: `is_public` can change on any save, and a
 * public catalog appears in *both* responses. Invalidating one would leave the
 * rail showing a stale copy of the row it just edited.
 *
 * The collection lists are invalidated too, because of a real cascade:
 * `DELETE FROM catalogs` drops the row's `folder_catalogs` entries
 * (`ON DELETE CASCADE`), so every cached collection that referenced it is now
 * wrong — the Manage overlay would keep listing a folder member that no longer
 * exists, and saving that collection would `400` on a catalog id the user can't
 * see. Create and `is_public` flips take the same path because they change what
 * a folder is *allowed* to reference.
 *
 * The selection queries are **not** invalidated. Selection is client state until
 * Push — refetching it would clobber the user's pending home-screen edits with
 * what the server last saw.
 */
export function useCatalogMutations(profileIndex: number) {
  const queryClient = useQueryClient()

  function invalidate() {
    void queryClient.invalidateQueries({ queryKey: queryKeys.ownedCatalogs(profileIndex) })
    void queryClient.invalidateQueries({ queryKey: queryKeys.communityCatalogs() })
    void queryClient.invalidateQueries({ queryKey: queryKeys.ownedCollections(profileIndex) })
    void queryClient.invalidateQueries({ queryKey: queryKeys.communityCollections() })
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
