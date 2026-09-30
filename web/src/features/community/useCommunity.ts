import { useQuery } from '@tanstack/react-query'
import { fetchCommunity, fetchPublication, queryKeys } from '@/api'

/** Every live publication Community lists, in one call. */
export function useCommunityList(profileIndex: number) {
  return useQuery({
    queryKey: queryKeys.communityList(profileIndex),
    queryFn: () => fetchCommunity(profileIndex),
  })
}

/** One publication with its snapshot, for its page. */
export function usePublication(profileIndex: number, publicationID: string) {
  return useQuery({
    queryKey: queryKeys.publication(profileIndex, publicationID),
    queryFn: () => fetchPublication(profileIndex, publicationID),
  })
}
