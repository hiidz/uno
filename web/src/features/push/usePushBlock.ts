import { useHomeSelection } from '@/features/home/useHomeSelection'
import { pushBlock, type PushBlock } from './pushBlock'

/** What blocks a push of the Home selection as it stands now (`pushBlock`).
 *  `sharesAddons` is the picked profile's `uses_primary_addons`, which the
 *  picker hands down; a profile picked without it is checked by the server
 *  alone. */
export function usePushBlock(sharesAddons: boolean | undefined): PushBlock | null {
  const home = useHomeSelection()
  return pushBlock(sharesAddons === true, home.collections, home.collectionById)
}
