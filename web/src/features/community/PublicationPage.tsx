import { useEffect, useMemo, useRef } from 'react'
import { ArrowLeft } from 'lucide-react'
import type { CommunityItem, PublicationDetail } from '@/api'
import { ListState } from '@/components/ListState'
import { Icon } from '@/components/Icon'
import type { GenreLookups } from '@/features/library/useLibrary'
import { CatalogBody, CollectionBody } from '@/features/sharing/PublicationBodies'
import { snapshotAsCollection, snapshotCatalog } from '@/features/sharing/snapshot'
import { ItemActions, type RowActions } from './CommunityRow'
import { usePublication } from './useCommunity'

/**
 * One publication's page, in place of the list: its name beside the way back
 * (DESIGN.md's One Way Back rule — the arrow and Escape both leave), how many
 * have added it, the same actions its row offers, then what it holds. While
 * an update waits for this profile's added row, what it holds is the new version,
 * and Update applies it. A catalog shows its spec tiles beside one page of
 * its results; a collection shows a card for each folder, its catalogs
 * opening in place, beside its Preview panel (`features/sharing`, the same
 * blocks a row added from Community opens as). The header draws from the
 * list's row at once; the snapshot comes from the detail call.
 */
export function PublicationPage({
  profileIndex,
  item,
  summary,
  meta,
  genres,
  actions,
  onBack,
}: {
  profileIndex: number
  item: CommunityItem
  summary: string
  meta: string
  genres: GenreLookups
  actions: RowActions
  onBack: () => void
}) {
  const detail = usePublication(profileIndex, item.id)
  const headingRef = useRef<HTMLHeadingElement>(null)

  useEffect(() => headingRef.current?.focus(), [])

  useEffect(() => {
    function onKeyDown(event: KeyboardEvent) {
      if (event.key !== 'Escape' || event.defaultPrevented || document.querySelector('[role="dialog"]')) return
      onBack()
    }
    document.addEventListener('keydown', onKeyDown)
    return () => document.removeEventListener('keydown', onKeyDown)
  }, [onBack])

  return (
    <div className="flex flex-col gap-5">
      <div className="flex flex-wrap items-center gap-3">
        <button type="button" onClick={onBack} aria-label="Back to Community" className="fp-back tap">
          <Icon icon={ArrowLeft} size={18} />
        </button>
        <div className="flex min-w-0 flex-1 flex-col gap-1">
          <h2 ref={headingRef} tabIndex={-1} className="m-0 truncate text-[20px] font-bold outline-none">
            {item.title}
          </h2>
          <span className="text-dim text-[13.5px]">{summary}</span>
          <span className="type-data text-dimmer text-[12.5px]">{meta}</span>
        </div>
        <ItemActions item={item} actions={actions} />
      </div>

      <ListState
        isLoading={detail.isPending}
        error={detail.error as Error | null}
        loadingLabel="Loading…"
        errorLabel="Couldn’t load this. Its publisher may have unpublished it."
        onRetry={detail.refetch}
      >
        {detail.data && <PublicationBody detail={detail.data} genres={genres} />}
      </ListState>
    </div>
  )
}

/** What a publication holds: a collection's folders beside its Preview
 *  panel, or a catalog's spec tiles beside one page of its results. */
function PublicationBody({ detail, genres }: { detail: PublicationDetail; genres: GenreLookups }) {
  const collection = useMemo(
    () => snapshotAsCollection(detail),
    [detail],
  )
  if (collection) return <CollectionBody collection={collection} genres={genres} />
  const catalog = snapshotCatalog(detail)
  return catalog ? <CatalogBody catalog={catalog} genres={genres} /> : null
}
