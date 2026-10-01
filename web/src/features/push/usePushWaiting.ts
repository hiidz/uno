import { useQuery } from '@tanstack/react-query'
import { fetchPendingPush, queryKeys } from '@/api'
import { waitingIDs } from '@/features/sharing/sharingState'

const NONE: ReadonlySet<string> = new Set()

/**
 * The ids of the rows a push would change in Nuvio (`waitingIDs`), from the
 * same query the Home pane's list of changes reads, so the flags and that
 * list agree. Empty until the list loads.
 */
export function usePushWaiting(profileIndex: number): ReadonlySet<string> {
  const { data } = useQuery({
    queryKey: queryKeys.pendingPush(profileIndex),
    queryFn: () => fetchPendingPush(profileIndex),
    select: waitingIDs,
  })
  return data ?? NONE
}
