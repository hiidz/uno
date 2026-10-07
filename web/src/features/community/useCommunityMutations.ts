import { useMutation, useQueryClient } from '@tanstack/react-query'
import {
  ApiError,
  duplicatePublication,
  invalidateProfileLists,
  subscribe,
  updateSubscription,
  type CommunityCopy,
} from '@/api'

/** What a Community row or publication page can do in one call. */
export type CommunityAction = 'subscribe' | 'update' | 'duplicate'

/** A 404 or a 409 from a Community call: the row was behind the server — its
 *  publisher unpublished it, or this profile already added it. */
function isStale(error: Error): boolean {
  return error instanceof ApiError && (error.status === 404 || error.status === 409)
}

/**
 * Subscribe (the UI's Add), Update and Duplicate, by publication id. Each
 * writes to the library and changes what Community shows for the
 * publication, so each refreshes both, and settles only once they have
 * refetched: a row never offers Add again for a copy that already exists. A
 * stale failure refreshes the same lists before it rejects, so its message
 * lands on rows that already show the server's state.
 */
export function useCommunityMutations(profileIndex: number) {
  const queryClient = useQueryClient()
  const refresh = () => invalidateProfileLists(queryClient, profileIndex)

  function useAction(act: (profileIndex: number, publicationID: string) => Promise<CommunityCopy>) {
    return useMutation({
      mutationFn: (publicationID: string) => act(profileIndex, publicationID),
      onSuccess: refresh,
      onError: (error) => (isStale(error) ? refresh() : undefined),
    })
  }

  return {
    subscribe: useAction(subscribe),
    update: useAction(updateSubscription),
    duplicate: useAction(duplicatePublication),
  }
}
