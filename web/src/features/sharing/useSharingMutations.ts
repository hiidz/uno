import { useMutation, useQueryClient } from '@tanstack/react-query'
import {
  type Catalog,
  type Collection,
  invalidateProfileLists,
  publishCatalog,
  publishCollection,
  withdrawCatalog,
  withdrawCollection,
} from '@/api'

/** Which of the owner's rows a sharing call acts on. */
export interface SharingTarget {
  kind: 'catalog' | 'collection'
  id: string
}

const CALLS = {
  publish: { catalog: publishCatalog, collection: publishCollection },
  withdraw: { catalog: withdrawCatalog, collection: withdrawCollection },
} as const

/**
 * The owner's own sharing calls: publish (or publish an update) and stop
 * sharing. Each changes a library row's sharing state and what Community
 * lists, so each refreshes the library and Community together, and settles
 * only once they have refetched: the editor that asked shows its new state as
 * soon as the call resolves.
 */
export function useSharingMutations(profileIndex: number) {
  const queryClient = useQueryClient()
  const refresh = () => invalidateProfileLists(queryClient, profileIndex)

  function useCall(call: keyof typeof CALLS) {
    return useMutation({
      mutationFn: ({ kind, id }: SharingTarget): Promise<Catalog | Collection> => CALLS[call][kind](profileIndex, id),
      onSuccess: refresh,
    })
  }

  return {
    publish: useCall('publish'),
    withdraw: useCall('withdraw'),
  }
}
