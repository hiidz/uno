import type { Catalog, Collection, Folder, PublicationState, SubscriptionState } from '@/api'

/** Where an owner's own row stands with Community: never shared (or shared
 *  and then stopped), shared as it is, or shared with edits made since. */
export type OwnSharing = 'private' | 'live' | 'changed' | 'withdrawn'

export function ownSharing(publication: PublicationState | null): OwnSharing {
  if (publication === null) return 'private'
  if (publication.status === 'withdrawn') return 'withdrawn'
  return publication.changed_since_publish ? 'changed' : 'live'
}

/** Whether an own row's publication is live: Community lists it now. */
export function isShared(row: { publication: PublicationState | null }): boolean {
  return row.publication?.status === 'live'
}

/** One sticker a row carries about sharing, in words. `tone` picks its
 *  class: `shared` is the pink fill, `from` the pink outline of a copy taken
 *  from Community, `update` an ink outline and `quiet` the dim one. */
export interface SharingSticker {
  label: string
  tone: 'shared' | 'from' | 'update' | 'quiet'
}

/** A row's stickers as words for a screen reader, each after a comma:
 *  ", shared, changed". */
export function stickerWords(stickers: SharingSticker[]): string {
  return stickers.map((sticker) => `, ${sticker.label.toLowerCase()}`).join('')
}

export const STICKER_CLASS: Record<SharingSticker['tone'], string> = {
  shared: 'stk stk-shared',
  from: 'stk stk-from',
  update: 'stk stk-update',
  quiet: 'stk stk-kind',
}

/** The sharing stickers for a library row or an editor's sign. A subscribed
 *  copy says so, and whether an update waits or its owner stopped sharing
 *  it; an own row says it is shared, and whether it changed since. */
export function rowStickers(row: {
  publication: PublicationState | null
  subscription: SubscriptionState | null
}): SharingSticker[] {
  if (row.subscription) return subscriptionStickers(row.subscription)
  const state = ownSharing(row.publication)
  if (state === 'live') return [{ label: 'Shared', tone: 'shared' }]
  if (state === 'changed')
    return [
      { label: 'Shared', tone: 'shared' },
      { label: 'Changed', tone: 'quiet' },
    ]
  return []
}

/** The sticker a row added from Community carries wherever it is listed. */
export const FROM_COMMUNITY: SharingSticker = { label: 'From Community', tone: 'from' }

function subscriptionStickers(subscription: SubscriptionState): SharingSticker[] {
  if (subscription.update_available) return [FROM_COMMUNITY, { label: 'Update', tone: 'update' }]
  if (subscription.withdrawn) return [FROM_COMMUNITY, { label: 'No longer shared', tone: 'quiet' }]
  return [FROM_COMMUNITY]
}

/** Every catalog a collection's folders use, once each, in folder order —
 *  what publishing it shares. `own` are scoped to it; `library` are listed
 *  catalogs it references, which a publish shares as they are. */
export function publishGroups(collection: Pick<Collection, 'id' | 'folders' | 'catalogs'>): {
  own: Catalog[]
  library: Catalog[]
} {
  const used = usedCatalogs(collection)
  return {
    own: used.filter((c) => c.collection_id === collection.id),
    library: used.filter((c) => c.collection_id !== collection.id),
  }
}

/** The catalogs a collection's folders reference, once each, in folder
 *  order; a ref its `catalogs` doesn't hold is left out. */
function usedCatalogs(collection: Pick<Collection, 'folders' | 'catalogs'>): Catalog[] {
  const byID = new Map((collection.catalogs ?? []).map((c) => [c.id, c]))
  const ids = new Set((collection.folders ?? []).flatMap(folderCatalogIDs))
  return [...ids].flatMap((id) => byID.get(id) ?? [])
}

function folderCatalogIDs(folder: Pick<Folder, 'refs'>): string[] {
  return (folder.refs ?? []).map((ref) => ref.catalog_id)
}

/** The line under an own row's Sharing buttons, if any: that a share waits
 *  for a save. A row shared as it is has nothing to share until it is saved,
 *  so it needs no line. */
export function sharingNote(state: OwnSharing, dirty: boolean): string | null {
  return dirty && state !== 'live' ? 'Save first: sharing shares what’s saved.' : null
}

/** A failed call's message, for a dialog's error line. */
export function errorText(error: Error | null): string | null {
  return error?.message ?? null
}

const FROM_WORDS = {
  current: 'Taken from Community. It follows its owner’s updates until you save a change to it.',
  update: 'Taken from Community. Its owner has published an update.',
  withdrawn: 'Taken from Community. Its owner stopped sharing it, so it gets no more updates.',
}

/** Where a copy taken from Community stands, in its From Community row:
 *  following its owner, with an update waiting, or getting no more. */
export function fromWords(subscription: SubscriptionState): string {
  if (subscription.update_available) return FROM_WORDS.update
  if (subscription.withdrawn) return FROM_WORDS.withdrawn
  return FROM_WORDS.current
}
