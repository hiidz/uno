import { useMemo } from 'react'
import type { CommunityItem, SnapshotChange, SubscriptionState } from '@/api'
import type { GenreLookups } from '@/features/library/useLibrary'
import { pluralCount } from '@/lib/plural'
import { changeCount } from './changeWords'
import { ChangesBlock } from './ChangeList'
import { updateWaits } from './sharingState'
import { UpdateSummary } from './MarkViews'
import { updateMarks, updateSummary, type UpdateMarks } from './updateMarks'
import { useChangesSincePublish, useUpdateChanges } from './useChanges'

/** A Community row as the update shelf and its marks read it. */
type UpdateItem = Pick<CommunityItem, 'id' | 'kind' | 'subscribed' | 'update_available'>

/**
 * What an Update would change, as a shelf on the Community page of a row this
 * profile added, first in the lead above what the page shows of the new
 * version: one line, counted ("1 new folder · 2 folders renamed · …"), each
 * part a link to where the page marks it (`useUpdateMarks`). Only while an
 * update waits; nothing otherwise.
 */
export function UpdateChanges({ profileIndex, item, genres }: { profileIndex: number; item: UpdateItem; genres: GenreLookups }) {
  const waiting = item.subscribed && item.update_available
  const changes = useUpdateChanges(profileIndex, item.id, waiting)
  if (!waiting) return null
  return (
    <ChangesBlock
      shelf
      title="In this update"
      changes={changes}
      genres={genres}
      renderList={(list) => <UpdateSummary items={updateSummary(updateMarks(list), item.kind)} />}
    />
  )
}

/** Where an update's changes fall on the new version a page shows, from the
 *  shelf's own call; null while no update waits or its changes are loading. */
export function useUpdateMarks(profileIndex: number, item: UpdateItem): UpdateMarks | null {
  const waiting = item.subscribed && item.update_available
  const changes = useUpdateChanges(profileIndex, item.id, waiting)
  return useMemo(() => marksOf(waiting, changes.data), [waiting, changes.data])
}

function marksOf(waiting: boolean, changes: SnapshotChange[] | undefined): UpdateMarks | null {
  if (!waiting || !changes) return null
  return updateMarks(changes)
}

/** The publication a subscription follows; none for a row that follows nothing. */
function publicationID(subscription: SubscriptionState | null): string {
  return subscription?.publication_id ?? ''
}

/**
 * How many changes an Update would make, "7 changes", beside the From
 * Community view's Update…: the count the Community page's list shows, from
 * the same call. Nothing while it loads, if it fails, or if the list is empty.
 */
export function UpdateCount({
  profileIndex,
  subscription,
}: {
  profileIndex: number
  subscription: SubscriptionState | null
}) {
  const waiting = updateWaits({ subscription })
  const changes = useUpdateChanges(profileIndex, publicationID(subscription), waiting)
  const count = changeCount(changes.data)
  if (!waiting || count === 0) return null
  return <span className="type-data text-dim text-[13.5px]">{pluralCount(count, 'change')}</span>
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
