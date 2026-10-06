import type { Catalog } from '@/api'
import { publisherUnpublished } from './sharingState'

interface UnpublishedNoticeProps {
  row: Pick<Catalog, 'publication' | 'publisher_unpublished'>
}

/** The line an editor leads with while its row is one its publisher
 *  unpublished: added from Community, and now this profile's own to edit.
 *  Saving the row clears it. Nothing otherwise. */
export function UnpublishedNotice({ row }: UnpublishedNoticeProps) {
  if (!publisherUnpublished(row)) return null
  return (
    <p className="text-dim m-0 mb-5 max-w-[52ch] text-[14.5px] leading-[1.5]">
      Its publisher unpublished this. It’s yours to edit now.
    </p>
  )
}
