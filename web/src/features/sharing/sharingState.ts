import type { Catalog, CatalogType, Collection, Folder, PendingChange, PublicationState } from '@/api'
import { typeLabel } from '@/features/library/recipe'

/** Where an owner's own row stands with Community: not published (never, or
 *  unpublished since), published as it is, or published with edits made
 *  since. */
export type OwnSharing = 'private' | 'live' | 'changed'

export function ownSharing(publication: PublicationState | null): OwnSharing {
  if (publication === null) return 'private'
  return publication.changed_since_publish ? 'changed' : 'live'
}

/** Whether an own row is published: Community lists it now. */
export function isPublished(row: { publication: PublicationState | null }): boolean {
  return row.publication !== null
}

/** A sticker's look: `hue` names where the fact points (a row's kind its
 *  region, `catalog` tangerine or `collection` green; `community` pink,
 *  `nuvio` Nuvio yellow), and `fill` says it waits on you until
 *  one action clears it; an outline only states. */
export interface StickerTone {
  hue: 'catalog' | 'collection' | 'community' | 'nuvio'
  fill: boolean
}

/** One sticker a row carries about where it stands, in words that name a
 *  state, never an act. */
export interface SharingSticker {
  label: string
  tone: StickerTone
}

/** A row's stickers as words for a screen reader, each after a comma:
 *  ", published, to push". */
export function stickerWords(stickers: SharingSticker[]): string {
  return stickers.map((sticker) => `, ${sticker.label.toLowerCase()}`).join('')
}

/** The classes that draw a sticker of `tone`: `.stk`, its hue, and
 *  `.stk-fill` while it waits on you. */
export function stickerClass(tone: StickerTone): string {
  return `stk stk-${tone.hue}${tone.fill ? ' stk-fill' : ''}`
}

const COMMUNITY: StickerTone = { hue: 'community', fill: false }
const COMMUNITY_FILL: StickerTone = { hue: 'community', fill: true }

const PUBLISHED: SharingSticker = { label: 'Published', tone: COMMUNITY }
const TO_PUBLISH: SharingSticker = { label: 'To publish', tone: COMMUNITY_FILL }
/** The sticker a row added from Community carries wherever it is listed. */
export const FROM_COMMUNITY: SharingSticker = { label: 'From Community', tone: COMMUNITY }
/** The only sticker that says "update": a publisher's newer version waits. */
const UPDATE_AVAILABLE: SharingSticker = { label: 'Update available', tone: COMMUNITY_FILL }
const TO_PUSH: SharingSticker = { label: 'To push', tone: { hue: 'nuvio', fill: true } }

const CATALOG_KIND: StickerTone = { hue: 'catalog', fill: false }

/** A collection's kind, outlined in the collections' green. */
export const COLLECTION_KIND: SharingSticker = { label: 'Collection', tone: { hue: 'collection', fill: false } }

/** A catalog's kind (Movies, Series), outlined in the catalogs' tangerine. */
export function kindSticker(type: CatalogType): SharingSticker {
  return { label: typeLabel(type), tone: CATALOG_KIND }
}

/** A catalog's kind sticker; none while nothing describes the catalog. */
export function kindStickers(catalog: Pick<Catalog, 'type'> | undefined): SharingSticker[] {
  return catalog ? [kindSticker(catalog.type)] : []
}

type StickerRow = Pick<Catalog, 'publication' | 'subscription'>

/** The one Community sticker a row carries, changing with its state: an own
 *  row reads Published, then To publish once edited since; a row added
 *  from Community reads From Community, then Update available. A row its
 *  publisher unpublished is an own row like any other. */
function communitySticker(row: StickerRow): SharingSticker | null {
  if (row.subscription) return row.subscription.update_available ? UPDATE_AVAILABLE : FROM_COMMUNITY
  const state = ownSharing(row.publication)
  if (state === 'live') return PUBLISHED
  return state === 'changed' ? TO_PUBLISH : null
}

/** What the library rail shows besides the kind: the row's Community sticker. */
export function railStickers(row: StickerRow): SharingSticker[] {
  const sticker = communitySticker(row)
  return sticker ? [sticker] : []
}

/** Every flag a row carries, for an editor's sign: its Community sticker,
 *  and To push while `waitingForPush` says Nuvio holds it differently from
 *  how a push would send it now. */
export function rowStickers(row: StickerRow, waitingForPush: boolean): SharingSticker[] {
  const stickers = railStickers(row)
  if (waitingForPush) stickers.push(TO_PUSH)
  return stickers
}

/** What a Home row carries besides its kind: To push while `waitingForPush`
 *  says Nuvio holds it differently from how a push would send it now. Where
 *  it stands with Community is the rail's and the editor's to say. */
export function homeStickers(waitingForPush: boolean): SharingSticker[] {
  return waitingForPush ? [TO_PUSH] : []
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

const STEP_LABEL: Record<OwnSharing, string> = {
  private: 'Publish…',
  changed: 'Publish update…',
  live: 'Unpublish…',
}

/** An own row's next step with Community, as its editor's sign button names
 *  it, and why it waits: Community holds the saved row, so every step waits
 *  while the form has unsaved changes. */
export function sharingStep(state: OwnSharing, dirty: boolean): { label: string; waiting: string | null } {
  return { label: STEP_LABEL[state], waiting: dirty ? 'Save first.' : null }
}

/** A failed call's message, for a dialog's error line. */
export function errorText(error: Error | null): string | null {
  return error?.message ?? null
}
