import { useMemo, type ReactNode } from 'react'
import type { Catalog, Collection, CommunityItem, PublicationDetail } from '@/api'
import { StateView } from '@/components/StateView'
import type { GenreLookups } from '@/features/library/useLibrary'
import { UpdateChanges, useUpdateMarks } from '@/features/sharing/Changes'
import { RenamedFrom } from '@/features/sharing/MarkViews'
import { CatalogBody, CollectionBody } from '@/features/sharing/PublicationBodies'
import { SharingStickers } from '@/features/sharing/SharingStickers'
import type { SharingSticker } from '@/features/sharing/sharingState'
import { snapshotAsCollection, snapshotCatalog } from '@/features/sharing/snapshot'
import { renamedFrom, type UpdateMarks } from '@/features/sharing/updateMarks'
import { itemKind } from './communityQuery'
import { PageActions, type OwnPageActions, type RowActions } from './CommunityRow'
import { MetaParts } from './MetaParts'
import { usePublication } from './useCommunity'

/**
 * One publication's page, in place of the list, under a sign that carries its
 * name beside the way back (`CommunitySign`). The body leads with how many
 * have added it and when it changed, then the one primary — Add, Update while
 * one waits, or ✓ Added — beside Duplicate, or on the profile's own
 * publication a line saying it published this. While an update waits for this
 * profile's added row, a shelf of what the update changes comes next, then
 * what the page holds, which is the new version, and Update applies it. A
 * catalog shows its spec tiles beside one page of its results; a collection
 * shows a card for each folder, its catalogs opening in place, beside its
 * Preview panel (`features/sharing`, the same blocks a row added from
 * Community opens as). The lead draws from the list's row at once; the
 * snapshot comes from the detail call, and the lead stays put while it loads.
 */
export function PublicationPage({
  profileIndex,
  item,
  meta,
  genres,
  actions,
  own,
  ownStickers = [],
}: {
  profileIndex: number
  item: CommunityItem
  meta: string[]
  genres: GenreLookups
  actions: RowActions
  /** The profile's own publication's actions; null while the library
   *  hasn't listed its row. */
  own?: OwnPageActions | null
  /** The own row's Community sticker, after the kind. */
  ownStickers?: SharingSticker[]
}) {
  const detail = usePublication(profileIndex, item.id)
  const marks = useUpdateMarks(profileIndex, item)
  const lead = (
    <>
      <div className="flex flex-wrap items-center gap-1.5 sm:hidden">
        <PageStickers item={item} extra={ownStickers} />
      </div>
      <p className="type-data text-dim text-meta m-0">
        <MetaParts parts={meta} />
      </p>
      <RenamedFrom name={renamedName(marks, item.kind)} />
      <PageActions item={item} actions={actions} own={own} />
      <UpdateChanges profileIndex={profileIndex} item={item} genres={genres} />
      <StateView
        isLoading={detail.isPending}
        error={detail.error as Error | null}
        loadingLabel="Loading…"
        errorLabel="Couldn’t load this. Its publisher may have unpublished it."
        onRetry={detail.refetch}
      >
        {null}
      </StateView>
    </>
  )
  return <PublicationBody kind={item.kind} detail={detail.data} genres={genres} lead={lead} marks={marks} />
}

/** What the update renames the publication from; '' while none waits. */
function renamedName(marks: UpdateMarks | null, kind: CommunityItem['kind']): string {
  return marks ? renamedFrom(marks, kind) : ''
}

/** What a publication holds: a collection's folders beside its Preview panel,
 *  or a catalog's spec tiles beside one page of its results. Only the lead
 *  until the detail has loaded. */
function PublicationBody({
  kind,
  detail,
  genres,
  lead,
  marks,
}: {
  kind: CommunityItem['kind']
  detail: PublicationDetail | undefined
  genres: GenreLookups
  lead: ReactNode
  marks: UpdateMarks | null
}) {
  const collection = useMemo(() => collectionOf(detail), [detail])
  if (kind === 'collection') return <CollectionBody collection={collection} genres={genres} lead={lead} marks={marks} />
  return <CatalogBody catalog={catalogOf(detail)} genres={genres} lead={lead} marks={marks} />
}

function collectionOf(detail: PublicationDetail | undefined): Collection | undefined {
  return detail ? (snapshotAsCollection(detail) ?? undefined) : undefined
}

function catalogOf(detail: PublicationDetail | undefined): Catalog | undefined {
  return detail && snapshotCatalog(detail)
}

/** The page's stickers: its kind, then `extra` (the own publication's
 *  Published or To publish). The Update button and the In this update shelf
 *  say an update waits, so no sticker does. The sign prints them from `sm`
 *  up and the body below. */
export function PageStickers({ item, extra = [] }: { item: CommunityItem; extra?: SharingSticker[] }) {
  return <SharingStickers stickers={[itemKind(item), ...extra]} />
}
