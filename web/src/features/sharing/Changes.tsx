import type { CommunityItem, SubscriptionState } from '@/api'
import type { GenreLookups } from '@/features/library/useLibrary'
import { changeSummary } from './changeWords'
import { ChangesBlock } from './ChangeList'
import { updateWaits } from './sharingState'
import { useChangesSincePublish, useUpdateChanges } from './useChanges'

/**
 * The full list of what an Update would change, for the Community page of a
 * row this profile added, above what the page shows of the new version. Only
 * while an update waits; nothing otherwise.
 */
export function UpdateChanges({
  profileIndex,
  item,
  genres,
}: {
  profileIndex: number
  item: Pick<CommunityItem, 'id' | 'subscribed' | 'update_available'>
  genres: GenreLookups
}) {
  const waiting = item.subscribed && item.update_available
  const changes = useUpdateChanges(profileIndex, item.id, waiting)
  if (!waiting) return null
  return <ChangesBlock title="What this update changes" changes={changes} genres={genres} />
}

/**
 * One line of what an Update would change, "1 folder removed · 2 catalogs
 * added", under the From Community view's Update…. It is the same list the
 * Community page shows in full, from the same call. Nothing while it loads,
 * if it fails, or if the list is empty.
 */
export function UpdateSummary({
  profileIndex,
  subscription,
}: {
  profileIndex: number
  subscription: SubscriptionState | null
}) {
  const waiting = updateWaits({ subscription })
  const changes = useUpdateChanges(profileIndex, subscription?.publication_id ?? '', waiting)
  const summary = changeSummary(changes.data ?? [])
  if (!waiting || !summary) return null
  return <p className="text-dim m-0 text-[13.5px]">{summary}</p>
}

/**
 * "Since you last published", the Publish dialog's full list of what
 * publishing changes will offer everyone who added the row. Only while
 * `enabled` says the row is published and has changed since; the editor shows
 * no list.
 */
export function SinceLastPublished({
  profileIndex,
  kind,
  id,
  enabled,
  genres,
}: {
  profileIndex: number
  kind: 'catalog' | 'collection'
  id: string
  enabled: boolean
  genres: GenreLookups
}) {
  const changes = useChangesSincePublish(profileIndex, kind, id, enabled)
  if (!enabled) return null
  return <ChangesBlock title="Since you last published" changes={changes} genres={genres} />
}
