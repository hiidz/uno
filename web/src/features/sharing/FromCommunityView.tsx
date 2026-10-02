import type { ReactNode } from 'react'
import { RefreshCw } from 'lucide-react'
import type { Catalog, Collection, CommunityItem, SubscriptionState } from '@/api'
import { Icon } from '@/components/Icon'
import { EditorShell } from '@/features/builder/EditorShell'
import { itemMeta } from '@/features/community/communityQuery'
import { useCommunityList } from '@/features/community/useCommunity'
import type { GenreLookups } from '@/features/library/useLibrary'
import { UpdateCount } from './Changes'
import { CatalogBody, CollectionBody } from './PublicationBodies'
import { updateWaits, viewStickers } from './sharingState'
import { SharingStickers } from './SharingStickers'

/** What the pane does for a row added from Community: the way out, the row's
 *  own Duplicate and Delete, and Update…, which opens its publication's page
 *  in Community. Beside Update… sits how many changes the update makes, and
 *  above it how many have added the row, both fetched for `profileIndex`. */
export interface FromCommunityActions {
  profileIndex: number
  /** A push would change what Nuvio holds for this row: the sign says To push. */
  waitingForPush: boolean
  onClose: () => void
  onDuplicate?: () => void
  onDelete?: () => void
  onUpdate: () => void
}

/**
 * A catalog added from Community, opened in the pane: no form, since only its
 * publisher changes it. Its recipe is the spec tiles beside its live results;
 * Duplicate to edit makes a copy that is the profile's own.
 */
export function CatalogFromCommunity({
  catalog,
  genres,
  ...actions
}: { catalog: Catalog; genres: GenreLookups } & FromCommunityActions) {
  return (
    <ViewFrame tone="catalog" purpose="View catalog" title={catalog.name} row={catalog} docked="results" {...actions}>
      {(lead) => <CatalogBody catalog={catalog} genres={genres} lead={lead} />}
    </ViewFrame>
  )
}

/**
 * A collection added from Community, opened in the pane: a card for each
 * folder, its catalogs opening in place, beside the Preview panel. A catalog
 * inside it never opens an editor.
 */
export function CollectionFromCommunity({
  collection,
  genres,
  ...actions
}: { collection: Collection; genres: GenreLookups } & FromCommunityActions) {
  return (
    <ViewFrame tone="collection" purpose="View collection" title={collection.title} row={collection} docked="preview" {...actions}>
      {(lead) => <CollectionBody collection={collection} genres={genres} lead={lead} />}
    </ViewFrame>
  )
}

/**
 * The editor shell around a view. The sign carries the row's flags
 * (`viewStickers`) with From Community where Update available would be, since
 * the Update… button, first action in the body while an update waits, says it.
 * Nothing here can be unsaved, so closing never asks. While Update… shows, Duplicate
 * to edit is outlined, so one filled button is on screen.
 */
function ViewFrame({
  tone,
  purpose,
  title,
  row,
  docked,
  children,
  profileIndex,
  waitingForPush,
  onClose,
  onDuplicate,
  onDelete,
  onUpdate,
}: {
  tone: 'catalog' | 'collection'
  purpose: string
  title: string
  row: Pick<Catalog, 'publication' | 'subscription'>
  docked: 'results' | 'preview'
  children: (lead: ReactNode) => ReactNode
} & FromCommunityActions) {
  const updating = updateWaits(row)
  const lead = <ViewLead profileIndex={profileIndex} subscription={row.subscription} onUpdate={onUpdate} />
  return (
    <EditorShell
      purpose={purpose}
      tone={tone}
      badges={<SharingStickers stickers={viewStickers(row, waitingForPush)} />}
      step={undefined}
      title={title}
      onRequestClose={onClose}
      onDuplicate={onDuplicate}
      onDelete={onDelete}
      docked={docked}
      footer={
        <>
          <button type="button" className="btn-secondary" onClick={onClose}>
            Close
          </button>
          {onDuplicate && (
            <button type="button" className={updating ? 'btn-secondary' : 'btn-primary'} onClick={onDuplicate}>
              Duplicate to edit
            </button>
          )}
        </>
      }
    >
      {children(lead)}
    </EditorShell>
  )
}

interface ViewLeadProps {
  profileIndex: number
  subscription: SubscriptionState | null
  onUpdate: () => void
}

/**
 * The body's lead: how many have added the row and when it last changed, in
 * dim words once the Community list has answered, and while an update waits a
 * Community-pink Update… with the number of changes beside it. Nothing when
 * neither shows.
 */
function ViewLead({ profileIndex, subscription, onUpdate }: ViewLeadProps) {
  const item = useFollowed(profileIndex, subscription)
  const updating = updateWaits({ subscription })
  if (!item && !updating) return null
  return (
    <div className="flex flex-col gap-3">
      {item && <p className="type-data text-dim m-0 text-[13px]">{itemMeta(item, new Date())}</p>}
      {updating && <UpdateRow profileIndex={profileIndex} subscription={subscription} onUpdate={onUpdate} />}
    </div>
  )
}

/** The Community list's row for the publication a subscription follows, once
 *  the list has answered and while it still lists it. */
function useFollowed(profileIndex: number, subscription: SubscriptionState | null): CommunityItem | undefined {
  const list = useCommunityList(profileIndex)
  return list.data?.find((listed) => listed.id === subscription?.publication_id)
}

/** Update… in the Community accent, with how many changes it makes beside it. */
function UpdateRow({ profileIndex, subscription, onUpdate }: ViewLeadProps) {
  return (
    <div className="tone-community flex flex-wrap items-center gap-3">
      <button type="button" className="btn-primary" onClick={onUpdate}>
        <Icon icon={RefreshCw} size={16} />
        Update…
      </button>
      <UpdateCount profileIndex={profileIndex} subscription={subscription} />
    </div>
  )
}
