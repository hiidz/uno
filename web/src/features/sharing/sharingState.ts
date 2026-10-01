import type { Catalog, Collection, Folder, PendingChange, PublicationState, SubscriptionState } from '@/api'

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

/** One sticker a row carries about where it stands, in words. `tone` picks its
 *  class: `community` is the pink outline (Published, Publish changes, From
 *  Community), `update` the pink fill, the one sticker that asks for an act,
 *  `push` the Nuvio-yellow outline and `quiet` the dim one. */
export interface SharingSticker {
  label: string
  tone: 'community' | 'update' | 'push' | 'quiet'
}

/** A row's stickers as words for a screen reader, each after a comma:
 *  ", published, push to nuvio". */
export function stickerWords(stickers: SharingSticker[]): string {
  return stickers.map((sticker) => `, ${sticker.label.toLowerCase()}`).join('')
}

export const STICKER_CLASS: Record<SharingSticker['tone'], string> = {
  community: 'stk stk-community',
  update: 'stk stk-update',
  push: 'stk stk-push',
  quiet: 'stk stk-kind',
}

const PUBLISHED: SharingSticker = { label: 'Published', tone: 'community' }
const PUBLISH_CHANGES: SharingSticker = { label: 'Publish changes', tone: 'community' }
/** The sticker a row added from Community carries wherever it is listed. */
export const FROM_COMMUNITY: SharingSticker = { label: 'From Community', tone: 'community' }
/** The only sticker that says "update": a publisher's newer version waits. */
export const UPDATE_AVAILABLE: SharingSticker = { label: 'Update available', tone: 'update' }
const UNPUBLISHED: SharingSticker = { label: 'Unpublished', tone: 'quiet' }
const PUSH_TO_NUVIO: SharingSticker = { label: 'Push to Nuvio', tone: 'push' }

type StickerRow = { publication: PublicationState | null; subscription: SubscriptionState | null }

/** The one Community sticker a row carries, changing with its state: an own
 *  row reads Published, then Publish changes once edited since; a row added
 *  from Community reads From Community, then Update available. */
function communitySticker(row: StickerRow): SharingSticker | null {
  if (row.subscription) return row.subscription.update_available ? UPDATE_AVAILABLE : FROM_COMMUNITY
  const state = ownSharing(row.publication)
  if (state === 'live') return PUBLISHED
  return state === 'changed' ? PUBLISH_CHANGES : null
}

/** What the library rail shows besides the kind: the row's Community sticker. */
export function railStickers(row: StickerRow): SharingSticker[] {
  const sticker = communitySticker(row)
  return sticker ? [sticker] : []
}

/** Every flag a row carries, for an editor's sign and the Home pane: its
 *  Community sticker, Unpublished once its publisher unpublished a row added
 *  from Community, and Push to Nuvio while `waitingForPush` says Nuvio holds
 *  it differently from how a push would send it now. */
export function rowStickers(row: StickerRow, waitingForPush: boolean): SharingSticker[] {
  const stickers = railStickers(row)
  if (row.subscription?.unpublished) stickers.push(UNPUBLISHED)
  if (waitingForPush) stickers.push(PUSH_TO_NUVIO)
  return stickers
}

/** `rowStickers` for the view a row added from Community opens as, whose
 *  sign says From Community even while an update waits: its Update… button
 *  says the update. */
export function viewStickers(row: StickerRow, waitingForPush: boolean): SharingSticker[] {
  const stickers = rowStickers(row, waitingForPush)
  const update = stickers.indexOf(UPDATE_AVAILABLE)
  if (update >= 0) stickers[update] = FROM_COMMUNITY
  return stickers
}

/** Whether a row added from Community has a newer version of its publisher's
 *  waiting. */
export function updateWaits(row: Pick<StickerRow, 'subscription'>): boolean {
  return row.subscription?.update_available === true
}

/** The ids of the rows a push would change in Nuvio, from the waiting list:
 *  the added and the changed. A removed row is gone from Uno and has no row
 *  to flag; a catalog used only inside a collection never appears, as its
 *  edit flags the collection. Empty until the list loads. */
export function waitingIDs(pending: readonly PendingChange[] | undefined): ReadonlySet<string> {
  const ids = new Set<string>()
  for (const row of pending ?? []) if (row.change !== 'removed') ids.add(row.id)
  return ids
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
