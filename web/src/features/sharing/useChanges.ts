import { useQuery } from '@tanstack/react-query'
import { fetchChangesSincePublish, fetchUpdateChanges, queryKeys } from '@/api'

/** What an Update would change in an added row, fetched while `enabled` says
 *  one waits. Never kept: it is read just before the Update that applies it,
 *  and a copy from before the publisher's last publish would be wrong. */
export function useUpdateChanges(profileIndex: number, publicationID: string, enabled: boolean) {
  return useQuery({
    queryKey: queryKeys.updateChanges(profileIndex, publicationID),
    queryFn: () => fetchUpdateChanges(profileIndex, publicationID),
    enabled,
    staleTime: 0,
    gcTime: 0,
  })
}

/** What publishing an own row again would change in its publication, fetched
 *  while `enabled` says it has been published and edited since. Never kept,
 *  like `useUpdateChanges`: a save of the row, or of a catalog it uses, changes
 *  it. */
export function useChangesSincePublish(
  profileIndex: number,
  kind: 'catalog' | 'collection',
  id: string,
  enabled: boolean,
) {
  return useQuery({
    queryKey: queryKeys.changesSincePublish(profileIndex, kind, id),
    queryFn: () => fetchChangesSincePublish(profileIndex, kind, id),
    enabled,
    staleTime: 0,
    gcTime: 0,
  })
}
