import { useQuery } from '@tanstack/react-query'
import {
  fetchCommunityCatalogs,
  fetchCommunityCollections,
  queryKeys,
} from '@/api'
import type { CommunityCatalog, CommunityCollection } from '@/api'

/**
 * The two community lists — public rows owned by someone else, each already
 * excluding this profile's own and carrying `taken` server-side. No merge
 * with the library: under the closed-graph model these are a different set
 * entirely, browsed here and copied via Take, never referenced live.
 */
export function useCommunityCatalogs(profileIndex: number) {
  return useQuery<CommunityCatalog[]>({
    queryKey: queryKeys.communityCatalogs(profileIndex),
    queryFn: () => fetchCommunityCatalogs(profileIndex),
  })
}

export function useCommunityCollections(profileIndex: number) {
  return useQuery<CommunityCollection[]>({
    queryKey: queryKeys.communityCollections(profileIndex),
    queryFn: () => fetchCommunityCollections(profileIndex),
  })
}
