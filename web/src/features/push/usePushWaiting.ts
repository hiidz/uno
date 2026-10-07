import { useQuery } from '@tanstack/react-query'
import { libraryQuery } from '@/features/library/useLibrary'
import { waitingIDs } from '@/features/sharing/sharingState'

const NONE: ReadonlySet<string> = new Set()

/**
 * The ids of the rows a push would change in Nuvio (`waitingIDs`), from the
 * same read the Home pane's list of changes reads, so the flags and that
 * list agree. Empty until the list loads.
 */
export function usePushWaiting(profileIndex: number): ReadonlySet<string> {
  const { data } = useQuery({
    ...libraryQuery(profileIndex),
    select: (library) => waitingIDs(library.pending),
  })
  return data ?? NONE
}
