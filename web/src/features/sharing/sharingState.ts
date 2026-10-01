import type { Catalog, Collection, Folder, PublicationState, SubscriptionState } from '@/api'

/** Where an owner's own row stands with Community: never published (or
 *  published and then unpublished), published as it is, or published with
 *  edits made since. */
export type OwnSharing = 'private' | 'live' | 'changed' | 'unpublished'

export function ownSharing(publication: PublicationState | null): OwnSharing {
  if (publication === null) return 'private'
  if (publication.status === 'unpublished') return 'unpublished'
  return publication.changed_since_publish ? 'changed' : 'live'
}

/** Whether an own row's publication is live: Community lists it now. */
export function isPublished(row: { publication: PublicationState | null }): boolean {
  return row.publication?.status === 'live'
}

/** One sticker a row carries about sharing, in words. `tone` picks its
 *  class: `published` is the pink fill, `from` the pink outline of a row
 *  added from Community, `update` an ink outline and `quiet` the dim one. */
export interface SharingSticker {
  label: string
  tone: 'published' | 'from' | 'update' | 'quiet'
}

/** A row's stickers as words for a screen reader, each after a comma:
 *  ", published, changed". */
export function stickerWords(stickers: SharingSticker[]): string {
  return stickers.map((sticker) => `, ${sticker.label.toLowerCase()}`).join('')
}

export const STICKER_CLASS: Record<SharingSticker['tone'], string> = {
  published: 'stk stk-published',
  from: 'stk stk-from',
  update: 'stk stk-update',
  quiet: 'stk stk-kind',
}

/** The sharing stickers for a library row or an editor's sign. A subscribed
 *  copy says so, and whether an update waits or its publisher unpublished
 *  it; an own row says it is published, and whether it changed since. */
export function rowStickers(row: {
  publication: PublicationState | null
  subscription: SubscriptionState | null
}): SharingSticker[] {
  if (row.subscription) return subscriptionStickers(row.subscription)
  const state = ownSharing(row.publication)
  if (state === 'live') return [{ label: 'Published', tone: 'published' }]
  if (state === 'changed')
    return [
      { label: 'Published', tone: 'published' },
      { label: 'Changed', tone: 'quiet' },
    ]
  return []
}

/** The sticker a row added from Community carries wherever it is listed. */
export const FROM_COMMUNITY: SharingSticker = { label: 'From Community', tone: 'from' }

function subscriptionStickers(subscription: SubscriptionState): SharingSticker[] {
  if (subscription.update_available) return [FROM_COMMUNITY, { label: 'Update', tone: 'update' }]
  if (subscription.unpublished) return [FROM_COMMUNITY, { label: 'Unpublished', tone: 'quiet' }]
  return [FROM_COMMUNITY]
}

/** Every catalog a collection's folders use, once each, in folder order —
 *  what publishing it publishes. `own` are scoped to it; `library` are listed
 *  catalogs it references, which a publish publishes as they are. */
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

/** The line under an own row's Community buttons, if any: that a publish
 *  waits for a save. A row published as it is has nothing to publish until it
 *  is saved, so it needs no line. */
export function sharingNote(state: OwnSharing, dirty: boolean): string | null {
  return dirty && state !== 'live' ? 'Save first: only what’s saved is published.' : null
}

/** A failed call's message, for a dialog's error line. */
export function errorText(error: Error | null): string | null {
  return error?.message ?? null
}
