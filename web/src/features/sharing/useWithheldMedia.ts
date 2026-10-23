import { useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { fetchWithheldMedia, queryKeys } from '@/api'
import { communitySetting } from '@/lib/pageConfig'
import { asksWithheldMedia } from './sharingState'

/** The pictures Community would leave out of an own collection as saved,
 *  fetched while `enabled`. Never kept, like `useChangesSincePublish`: a save
 *  of the collection changes it. */
export function useWithheldMedia(profileIndex: number, collectionID: string, enabled: boolean) {
  return useQuery({
    queryKey: queryKeys.withheldMedia(profileIndex, collectionID),
    queryFn: () => fetchWithheldMedia(profileIndex, collectionID),
    enabled,
    staleTime: 0,
    gcTime: 0,
  })
}

/** The addresses Community would leave out of the row the editor has open
 *  (`editing`, null for none) when it is a collection, on a server that
 *  shares pictures from trusted sites only; undefined anywhere else, until
 *  the answer arrives, or if it fails. */
export function useWithheldAddresses(
  profileIndex: number,
  editing: { kind: 'catalog' | 'collection'; id: string } | null,
): ReadonlySet<string> | undefined {
  const asks = editing !== null && asksWithheldMedia(editing.kind, communitySetting())
  const media = useWithheldMedia(profileIndex, editing?.id ?? '', asks)
  const pictures = asks ? media.data?.pictures : undefined
  return useMemo(() => (pictures ? new Set(pictures.map((picture) => picture.url)) : undefined), [pictures])
}
