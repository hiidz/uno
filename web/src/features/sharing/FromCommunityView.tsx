import type { ReactNode } from 'react'
import { RefreshCw } from 'lucide-react'
import type { Catalog, Collection } from '@/api'
import { Icon } from '@/components/Icon'
import { EditorShell } from '@/features/builder/EditorShell'
import type { GenreLookups } from '@/features/library/useLibrary'
import { CatalogBody, CollectionBody } from './PublicationBodies'
import { rowStickers } from './sharingState'
import { SharingStickers } from './SharingStickers'

/** What the pane does for a row added from Community: the way out, the row's
 *  own Duplicate and Delete, and Update…, which opens its publication's page
 *  in Community. */
export interface FromCommunityActions {
  onClose: () => void
  onDuplicate?: () => void
  onDelete?: () => void
  /** Why Delete is disabled — Nuvio may still hold the row — or `null`. */
  deleteBlocked?: string | null
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
 * The editor shell around a view. The sign carries the row's stickers except
 * Update, which the Update… button says; below `sm` the sign hides stickers,
 * so they head the body, followed by Update… while an update waits. Nothing
 * here can be unsaved, so closing never asks. While Update… shows, Duplicate
 * to edit is outlined, so one filled button is on screen.
 */
function ViewFrame({
  tone,
  purpose,
  title,
  row,
  docked,
  children,
  onClose,
  onDuplicate,
  onDelete,
  deleteBlocked,
  onUpdate,
}: {
  tone: 'catalog' | 'collection'
  purpose: string
  title: string
  row: Pick<Catalog, 'publication' | 'subscription'>
  docked: 'results' | 'preview'
  children: (lead: ReactNode) => ReactNode
} & FromCommunityActions) {
  const stickers = rowStickers(row).filter((sticker) => sticker.tone !== 'update')
  const updating = row.subscription?.update_available === true
  const lead = (
    <>
      <div className="flex flex-wrap items-center gap-1.5 sm:hidden">
        <SharingStickers stickers={stickers} />
      </div>
      {updating && (
        <div className="tone-community">
          <button type="button" className="btn-primary" onClick={onUpdate}>
            <Icon icon={RefreshCw} size={16} />
            Update…
          </button>
        </div>
      )}
    </>
  )
  return (
    <EditorShell
      purpose={purpose}
      tone={tone}
      badges={<SharingStickers stickers={stickers} />}
      title={title}
      onRequestClose={onClose}
      onDuplicate={onDuplicate}
      onDelete={onDelete}
      deleteBlocked={deleteBlocked}
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
