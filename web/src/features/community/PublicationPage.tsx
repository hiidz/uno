import { useMemo, type ReactNode } from 'react'
import type { Catalog, Collection, CommunityItem, PublicationDetail, SnapshotFolder } from '@/api'
import { ListState } from '@/components/ListState'
import type { GenreLookups } from '@/features/library/useLibrary'
import { UpdateChanges } from '@/features/sharing/Changes'
import { CatalogBody, CollectionBody } from '@/features/sharing/PublicationBodies'
import { stickerClass, UPDATE_AVAILABLE } from '@/features/sharing/sharingState'
import { snapshotAsCollection, snapshotCatalog } from '@/features/sharing/snapshot'
import { itemKind } from './communityQuery'
import { PageActions, type RowActions } from './CommunityRow'
import { usePublication } from './useCommunity'

/**
 * One publication's page, in place of the list, under a sign that carries its
 * name beside the way back (`CommunitySign`). The body leads with how many
 * have added it and when it changed, then the one primary — Add, Update while
 * one waits, or ✓ Added — beside Duplicate. While an update waits for this
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
}: {
  profileIndex: number
  item: CommunityItem
  meta: string
  genres: GenreLookups
  actions: RowActions
}) {
  const detail = usePublication(profileIndex, item.id)
  const lead = (
    <>
      <div className="flex flex-wrap items-center gap-1.5 sm:hidden">
        <PageStickers item={item} />
      </div>
      <p className="type-data text-dim m-0 text-[13px]">{meta}</p>
      <PageActions item={item} actions={actions} />
      <UpdateChanges profileIndex={profileIndex} item={item} folders={foldersOf(detail.data)} genres={genres} />
      <ListState
        isLoading={detail.isPending}
        error={detail.error as Error | null}
        loadingLabel="Loading…"
        errorLabel="Couldn’t load this. Its publisher may have unpublished it."
        onRetry={detail.refetch}
      >
        {null}
      </ListState>
    </>
  )
  return <PublicationBody kind={item.kind} detail={detail.data} genres={genres} lead={lead} />
}

/** What a publication holds: a collection's folders beside its Preview panel,
 *  or a catalog's spec tiles beside one page of its results. Only the lead
 *  until the detail has loaded. */
function PublicationBody({
  kind,
  detail,
  genres,
  lead,
}: {
  kind: CommunityItem['kind']
  detail: PublicationDetail | undefined
  genres: GenreLookups
  lead: ReactNode
}) {
  const collection = useMemo(() => collectionOf(detail), [detail])
  if (kind === 'collection') return <CollectionBody collection={collection} genres={genres} lead={lead} />
  return <CatalogBody catalog={catalogOf(detail)} genres={genres} lead={lead} />
}

/** The new version's folders, which order the In this update shelf. */
function foldersOf(detail: PublicationDetail | undefined): SnapshotFolder[] {
  return detail?.snapshot.collection?.folders ?? []
}

function collectionOf(detail: PublicationDetail | undefined): Collection | undefined {
  return detail ? (snapshotAsCollection(detail) ?? undefined) : undefined
}

function catalogOf(detail: PublicationDetail | undefined): Catalog | undefined {
  return detail && snapshotCatalog(detail)
}

/** The page's stickers: its kind, and Update available while an update waits.
 *  The sign prints them from `sm` up and the body below. */
export function PageStickers({ item }: { item: CommunityItem }) {
  return (
    <>
      <span className="stk stk-neutral">{itemKind(item)}</span>
      {item.update_available && <span className={stickerClass(UPDATE_AVAILABLE.tone)}>{UPDATE_AVAILABLE.label}</span>}
    </>
  )
}
