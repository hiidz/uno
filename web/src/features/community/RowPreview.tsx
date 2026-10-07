import { useEffect, useRef, useState, type CSSProperties, type RefObject } from 'react'
import { useQuery } from '@tanstack/react-query'
import { fetchCatalogPreview, queryKeys, type CommunityFolder, type CommunityItem, type SnapshotCatalog } from '@/api'
import { TILE_ASPECT } from '@/features/preview/tiles'
import { PREVIEW_QUERY_OPTIONS } from '@/features/preview/useRecipesTiles'
import { firstPosters, shownFolders, STACK_SIZE } from './rowPreviewModel'

/** A row's posters, on a catalog's row only. */
export function RowPosters({ item }: { item: CommunityItem }) {
  if (!item.catalog) return null
  return <PosterStack catalog={item.catalog} />
}

interface RowFoldersProps {
  item: CommunityItem
  /** Opens the row's page, for a tap on the strip where it scrolls. */
  onOpen(): void
}

/** A row's folder tiles, on a collection's row only. */
export function RowFolders({ item, onOpen }: RowFoldersProps) {
  if (item.kind !== 'collection') return null
  return <FolderStrip folders={item.folders ?? []} onOpen={onOpen} />
}

/**
 * A catalog row's posters: the first five of what its recipe returns, fanned
 * front to back in result order. Fetched once the row first scrolls into
 * view, under the same key a publication page's results use, so opening the
 * page draws from the same answer. A face stays blank while its poster loads
 * and where TMDB has none. Decorative: the row's name button says what it is.
 */
function PosterStack({ catalog }: { catalog: SnapshotCatalog }) {
  const ref = useRef<HTMLDivElement>(null)
  const seen = useSeen(ref)
  const posters = usePosters(catalog, seen)
  return (
    <div ref={ref} aria-hidden="true" className="poster-stack">
      {posters.map((poster, i) => (
        <span key={i} className="row-face" style={{ left: `calc(${i} * var(--step))`, zIndex: STACK_SIZE - i }}>
          {poster && <img src={poster} alt="" loading="lazy" decoding="async" />}
        </span>
      ))}
    </div>
  )
}

function usePosters(catalog: SnapshotCatalog, enabled: boolean): string[] {
  const params = JSON.stringify(catalog.params)
  const query = useQuery({
    queryKey: queryKeys.catalogPreview(catalog.type, params),
    queryFn: () => fetchCatalogPreview({ type: catalog.type, params }),
    enabled,
    ...PREVIEW_QUERY_OPTIONS,
  })
  return firstPosters(query.data)
}

/** Whether `ref`'s element has entered the viewport yet. It stays true once
 *  it has, and stays false where nothing can observe it. */
function useSeen(ref: RefObject<HTMLElement | null>): boolean {
  const [seen, setSeen] = useState(false)
  useEffect(() => {
    const element = ref.current
    if (seen || !element || typeof IntersectionObserver === 'undefined') return
    const observer = new IntersectionObserver(function onIntersect(entries) {
      if (entries.some((entry) => entry.isIntersecting)) setSeen(true)
    })
    observer.observe(element)
    return () => observer.disconnect()
  }, [ref, seen])
  return seen
}

/**
 * A collection row's folders as Nuvio draws their tiles, in order and at
 * their own shapes: the cover image over the cover emoji over the folder's
 * name. Up to six, then how many more. Decorative, like the poster stack.
 * From `sm` a click passes through it to the row's name button; below `sm`,
 * where it scrolls and so sits above that button, a tap on it opens the page
 * itself.
 */
function FolderStrip({ folders, onOpen }: { folders: CommunityFolder[]; onOpen(): void }) {
  const { shown, more } = shownFolders(folders)
  return (
    <div aria-hidden="true" className="folder-strip" onClick={onOpen}>
      {shown.map((folder, i) => (
        <FolderFace key={i} folder={folder} />
      ))}
      {more > 0 && <span className="type-data text-dimmer shrink-0 self-center pl-1 text-[12.5px]">+{more} more</span>}
    </div>
  )
}

function FolderFace({ folder }: { folder: CommunityFolder }) {
  return (
    <span className="row-face" style={{ '--ratio': TILE_ASPECT[folder.tile_shape] } as CSSProperties}>
      <FolderLabel folder={folder} />
      {folder.cover_image_url.trim() && <img src={folder.cover_image_url} alt="" loading="lazy" decoding="async" />}
    </span>
  )
}

/** What a folder tile shows under its cover image, and alone without one:
 *  the emoji, else the title. */
function FolderLabel({ folder }: { folder: CommunityFolder }) {
  if (folder.cover_emoji.trim()) return <span className="text-[24px] leading-none">{folder.cover_emoji}</span>
  return (
    <span className="row-face-name text-dimmer line-clamp-2 px-0.5 text-center leading-tight font-semibold [overflow-wrap:anywhere]">
      {folder.title.trim() || 'Untitled folder'}
    </span>
  )
}
