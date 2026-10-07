import { keepPreviousData, useInfiniteQuery, useQuery } from '@tanstack/react-query'
import { fetchCommunityPage, fetchPublication, queryKeys, type CommunityPage, type CommunityQuery } from '@/api'

/** Community's pages for `query` while `enabled`, the first at once and each
 *  next one on `fetchNextPage`. A new query keeps the rows on show until its
 *  first page arrives, so typing a search never blanks the list. */
export function useCommunityList(profileIndex: number, query: CommunityQuery, enabled: boolean) {
  return useInfiniteQuery({
    queryKey: queryKeys.communityList(profileIndex, query),
    queryFn: ({ pageParam }) => fetchCommunityPage(profileIndex, query, pageParam),
    initialPageParam: '',
    getNextPageParam: nextCursor,
    placeholderData: keepPreviousData,
    enabled,
  })
}

function nextCursor(page: CommunityPage): string | undefined {
  return page.next_cursor ?? undefined
}

/** One publication with its snapshot, for its page. */
export function usePublication(profileIndex: number, publicationID: string) {
  return useQuery({
    queryKey: queryKeys.publication(profileIndex, publicationID),
    queryFn: () => fetchPublication(profileIndex, publicationID),
  })
}

/** The publication whose page is open, through the same query its page reads;
 *  nothing while none is. */
export function useOpenPublication(profileIndex: number, openID: string | null) {
  const id = openID ?? ''
  return useQuery({
    queryKey: queryKeys.publication(profileIndex, id),
    queryFn: () => fetchPublication(profileIndex, id),
    enabled: openID !== null,
  })
}
