import { useEffect, useMemo, useRef } from 'react'
import { ArrowLeft, Tag } from 'lucide-react'
import type { Catalog, Collection, CommunityItem, PublicationDetail } from '@/api'
import { ListState } from '@/components/ListState'
import { Icon } from '@/components/Icon'
import { recipeSentence } from '@/features/library/recipe'
import type { GenreLookups } from '@/features/library/useLibrary'
import { CatalogList } from '@/features/sharing/CatalogList'
import { folderItems } from '@/features/sharing/listing'
import { SavedCatalogPreview, SavedCollectionPreview } from '@/features/sharing/SavedPreviews'
import { snapshotAsCollection, snapshotCatalog } from '@/features/sharing/snapshot'
import { ItemActions, type RowActions } from './CommunityRow'
import { usePublication } from './useCommunity'

/**
 * One publication's page, in place of the list: its name beside the way back
 * (DESIGN.md's One Way Back rule — the arrow and Escape both leave), how many
 * have taken it, the same actions its row offers, then what it holds. While
 * an update waits for this profile's copy, what it holds is the new version,
 * and Update applies it. A
 * catalog shows its recipe as a sentence beside one page of its results; a
 * collection shows each folder's catalogs in plain words beside its Preview
 * panel. The header draws from the list's row at once; the snapshot comes
 * from the detail call.
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
        errorLabel="Couldn’t load this. Its owner may have stopped sharing it."
        onRetry={detail.refetch}
      >
        {detail.data && <PublicationBody detail={detail.data} genres={genres} />}
      </ListState>
    </div>
  )
}

/** What a publication holds: a collection's folders beside its Preview
 *  panel, or a catalog's recipe beside one page of its results. */
function PublicationBody({ detail, genres }: { detail: PublicationDetail; genres: GenreLookups }) {
  const collection = useMemo(() => snapshotAsCollection(detail), [detail])
  if (collection) return <CollectionBody collection={collection} genres={genres} />
  const catalog = snapshotCatalog(detail)
  return catalog ? <CatalogBody catalog={catalog} genres={genres} /> : null
}

function CollectionBody({ collection, genres }: { collection: Collection; genres: GenreLookups }) {
  const byID = new Map((collection.catalogs ?? []).map((c) => [c.id, c]))
  return (
    <div className="ed-container">
      <div className="ed ed-preview">
        <div className="ed-form">
          <div className="flex flex-col gap-3">
            {(collection.folders ?? []).map((folder) => (
              <div key={folder.id} className="fold-detail">
                <span className="text-[15px] font-bold">{folder.title}</span>
                <CatalogList items={folderItems(folder, byID, genres)} />
              </div>
            ))}
          </div>
        </div>
        <SavedCollectionPreview collection={collection} />
      </div>
    </div>
  )
}

function CatalogBody({ catalog, genres }: { catalog: Catalog; genres: GenreLookups }) {
  return (
    <div className="ed-container">
      <div className="ed ed-results">
        <div className="ed-form">
          <p className="talker">
            <Icon icon={Tag} size={20} />
            <span>{recipeSentence(catalog, catalog.type === 'movie' ? genres.movie : genres.tv)}</span>
          </p>
        </div>
        <SavedCatalogPreview key={catalog.params} type={catalog.type} params={catalog.params} />
      </div>
    </div>
  )
}
