import { useMutation, useQueryClient } from '@tanstack/react-query'
import {
  ApiError,
  duplicateCommunityCatalog,
  duplicateCommunityCollection,
  invalidateProfileLists,
  takeCatalog,
  takeCollection,
  updateTakenCatalog,
  updateTakenCollection,
} from '@/api'

export type CommunityAction = 'take' | 'update' | 'duplicate'

/** A 404 or a 409 from a community POST: the lists the button was pressed on
 *  were behind the server — the original went private or was deleted, a
 *  linked copy already exists, or the copy was unlinked by a save. */
function isStale(error: Error): boolean {
  return error instanceof ApiError && (error.status === 404 || error.status === 409)
}

/**
 * Take, Update and Duplicate, for both kinds. Each changes a library list and
 * the Community list together — a copy appears or changes in "Mine", and the
 * original's `taken`/`update_available` flip — so each refreshes both, and its
 * promise settles only once they have refetched: a row never offers Take
 * again for a copy that already exists. The owned-list keys prefix the
 * selection keys, so the selections refresh too, which an Update to a
 * collection needs because it bumps `version`.
 *
 * A stale failure (`isStale`) refreshes the same lists before it rejects, so
 * the caller's message lands on rows that already show the server's state.
 */
export function useCommunityMutations(profileIndex: number) {
  const queryClient = useQueryClient()

  function refresh() {
    return invalidateProfileLists(queryClient, profileIndex)
  }

  return {
    catalogs: {
      take: useCommunityMutation(profileIndex, takeCatalog, refresh),
      update: useCommunityMutation(profileIndex, updateTakenCatalog, refresh),
      duplicate: useCommunityMutation(profileIndex, duplicateCommunityCatalog, refresh),
    },
    collections: {
      take: useCommunityMutation(profileIndex, takeCollection, refresh),
      update: useCommunityMutation(profileIndex, updateTakenCollection, refresh),
      duplicate: useCommunityMutation(profileIndex, duplicateCommunityCollection, refresh),
    },
  }
}

function useCommunityMutation(
  profileIndex: number,
  act: (profileIndex: number, id: string) => Promise<unknown>,
  refresh: () => Promise<unknown>,
) {
  return useMutation({
    mutationFn: (id: string) => act(profileIndex, id),
    onSuccess: refresh,
    onError: (error) => (isStale(error) ? refresh() : undefined),
  })
}
