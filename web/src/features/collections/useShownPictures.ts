import { useDebounce } from '@/lib/useDebounce'
import { isWebAddress, type FolderFormState } from './collectionForm'

/** How long an address holds still before its picture is drawn. */
const SETTLE_MS = 500

/** `raw` once typing in it has held still, when it is an address the server
 *  takes; `''` otherwise, so a half-typed address is never fetched. */
export function useShownAddress(raw: string): string {
  const settled = useDebounce(raw.trim(), SETTLE_MS)
  return isWebAddress(settled) ? settled : ''
}

/** The folder's pictures as the Appearance shelf draws them, each address
 *  settled (`useShownAddress`). */
export interface ShownPictures {
  cover: string
  focusGIF: string
  heroBackdrop: string
  titleLogo: string
}

export function useShownPictures(folder: FolderFormState): ShownPictures {
  return {
    cover: useShownAddress(folder.coverImageURL),
    focusGIF: useShownAddress(folder.focusGIFURL),
    heroBackdrop: useShownAddress(folder.heroBackdropURL),
    titleLogo: useShownAddress(folder.titleLogoURL),
  }
}
