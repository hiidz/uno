import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useIsFetching, useQueryClient } from '@tanstack/react-query'
import { RefreshCw } from 'lucide-react'
import { queryKeys } from '@/api'
import { Icon } from '@/components/Icon'
import { plural } from '@/lib/plural'
import { folderRecipes } from '@/features/preview/model'
import { noTiles } from '@/features/preview/tiles'
import { useRecipesTiles } from '@/features/preview/useRecipesTiles'
import { useHomeSelection } from './useHomeSelection'
import { useCatalogTiles } from './useCatalogTiles'
import { buildHomePreview, findFolderPage } from './preview'
import type { FolderPageTarget, HomeScreenPreview, PreviewRow } from './preview'
import { FOLDER_LAYOUT_LABEL, TVCatalogRow, TVCollectionRow, TVFolderPage } from './tv'

/**
 * The Home pane's second view: the same pending state, drawn as the framed
 * 16:9 picture DESIGN.md's "TV preview (signature)" specs — Roboto inside
 * the frame, Jost around it. Reorder in List, flip to here, see it move.
 *
 * **Two levels, matching the real screen.** Home is one page — pinned
 * collections, then catalog rows, then the rest of the collections. A
 * collection is a single row whose tiles are its *folders*. Clicking a folder
 * tile opens that folder's page, where the collection's `view_mode` decides
 * whether its catalogs are tabs over a grid or stacked rows.
 *
 * **Read-only by construction.** The one interaction — opening a folder — is
 * navigation within the mock, not an edit. Every edit lives in the List view.
 *
 * **Uno pins nothing onto the screen — the Clean Preview amendment.** Every
 * caveat this view has to state (the pending count, how to get back to home)
 * is Uno's own words *around* the frame, in Jost; the picture itself carries
 * only what the real TV would show. See `tv.tsx` for what that drops.
 */
export function HomePreview() {
  const home = useHomeSelection()

  const preview = useMemo(
    () =>
      buildHomePreview({
        catalogs: home.catalogs,
        collections: home.collections,
        catalogById: home.catalogById,
        collectionById: home.collectionById,
      }),
    [home.catalogs, home.collections, home.catalogById, home.collectionById],
  )

  const { page, openFolder, closeFolder } = useFolderPage(preview)

  if (preview.isEmpty) return <EmptyHomeScreen />

  return page ? (
    <FolderPageView collection={page.collection} folder={page.folder} onBack={closeFolder} />
  ) : (
    <HomeScreenView preview={preview} onOpenFolder={openFolder} pendingCount={home.pendingCount} />
  )
}

/**
 * Which folder page is open, and the browser history entry that makes the
 * back arrow, Escape and the browser's own Back all do the same thing — the
 * Back Like the Remote amendment. Held as ids, not indices, so removing the
 * collection or folder in the List view falls back to home rather than
 * pointing at whatever now sits in the same slot.
 */
function useFolderPage(preview: HomeScreenPreview) {
  const [target, setTarget] = useState<FolderPageTarget | null>(null)
  const page = target ? findFolderPage(preview, target) : null

  // Whether opening the current target pushed a history entry. A sandboxed
  // frame can throw on `pushState`; Escape and the in-screen arrow still work
  // without it, only the browser's own Back doesn't.
  const pushed = useRef(false)

  const openFolder = useCallback((next: FolderPageTarget) => {
    try {
      window.history.pushState({ unoFolder: true }, '')
      pushed.current = true
    } catch {
      pushed.current = false
    }
    setTarget(next)
  }, [])

  // The one way this component asks to leave: the in-screen back arrow or
  // Escape. If opening pushed an entry, consuming it via `history.back()` is
  // what makes the browser's own Back symmetric with these — the `popstate`
  // listener below does the actual close once that navigation lands.
  const closeFolder = useCallback(() => {
    if (pushed.current) {
      window.history.back()
    } else {
      setTarget(null)
    }
  }, [])

  useEffect(() => {
    function onPopState() {
      if (pushed.current) {
        pushed.current = false
        setTarget(null)
      }
    }
    window.addEventListener('popstate', onPopState)
    return () => window.removeEventListener('popstate', onPopState)
  }, [])

  useEffect(() => {
    if (target === null) return
    function onKeyDown(e: KeyboardEvent) {
      if (e.key === 'Escape') closeFolder()
    }
    document.addEventListener('keydown', onKeyDown)
    return () => document.removeEventListener('keydown', onKeyDown)
  }, [target, closeFolder])

  // Leaving any other way — the target stopped resolving (removed in List
  // view), or this view unmounts entirely (the List | Preview switch, or an
  // editor opens) — still has to consume a pushed entry, or a later physical
  // Back press does nothing: the history stack would hold a dead entry this
  // component is no longer listening for.
  useEffect(() => {
    if (target !== null && page === null) setTarget(null)
  }, [target, page])

  useEffect(() => {
    if (target === null && pushed.current) {
      pushed.current = false
      window.history.back()
    }
  }, [target])

  useEffect(() => {
    return () => {
      if (pushed.current) {
        pushed.current = false
        window.history.back()
      }
    }
  }, [])

  return { target, page, openFolder, closeFolder }
}

function FolderPageView({
  collection,
  folder,
  onBack,
}: {
  collection: NonNullable<ReturnType<typeof findFolderPage>>['collection']
  folder: NonNullable<ReturnType<typeof findFolderPage>>['folder']
  onBack: () => void
}) {
  // From the sources' own recipes rather than `useCatalogTiles`' id lookup: a
  // folder reference can be narrowed to a genre, which the catalog id alone
  // doesn't carry.
  const recipes = useMemo(() => folderRecipes(folder), [folder])
  const tiles = useRecipesTiles(recipes)

  return (
    <div className="flex flex-col gap-3">
      <div className="flex items-center justify-between gap-3">
        <div className="flex flex-wrap items-baseline gap-x-3 gap-y-1">
          <p className="text-dim m-0 text-[13px] leading-[18px]">
            {collection.title || 'Untitled collection'}
            <span aria-hidden="true" className="text-dimmer px-1.5">/</span>
            <b className="text-ink font-medium">{folder.title || 'Untitled folder'}</b>
          </p>
          <span className="text-dim text-[12px] leading-[18px]">{FOLDER_LAYOUT_LABEL[collection.viewMode]}</span>
        </div>
        <RefreshPreviewButton />
      </div>

      <div className="tv-bezel">
        <div
          className="tv-screen"
          tabIndex={0}
          role="region"
          aria-label={`${folder.title || 'Untitled folder'}, a folder in ${collection.title || 'Untitled collection'}, as your TV shows it`}
        >
          <TVFolderPage collection={collection} folder={folder} tiles={tiles} onBack={onBack} />
        </div>
      </div>

      <p className="text-dim m-0 max-w-[72ch] text-[12px] leading-[17px] pointer-coarse:hidden">
        The arrow beside the folder name goes back to the home screen, like Back on the remote. So
        do Esc and your browser's Back.
      </p>
      <p className="text-dim m-0 hidden max-w-[72ch] text-[12px] leading-[17px] pointer-coarse:block">
        The arrow beside the folder name goes back to the home screen, like Back on the remote. So
        does your phone's Back.
      </p>
    </div>
  )
}

function HomeScreenView({
  preview,
  onOpenFolder,
  pendingCount,
}: {
  preview: HomeScreenPreview
  onOpenFolder: (target: FolderPageTarget) => void
  pendingCount: number
}) {
  const nothingOnHome =
    preview.rows.length === 0 &&
    preview.pinnedCollections.length === 0 &&
    preview.unpinnedCollections.length === 0

  // Every row's tiles fetched together when Preview opens: a row is one TMDB
  // page and that page is the whole row, so there is never a second call to
  // defer.
  const tiles = useCatalogTiles(preview.rows.map((row) => row.id))

  return (
    <div className="flex flex-col gap-3">
      <div className="flex items-start justify-between gap-3">
        <div className="flex flex-col gap-1">
          <p className="text-dim m-0 max-w-[72ch] text-[13px] leading-[18px]">
            Your TV shows pinned collections first, then catalog rows, then your other collections.
            Nothing here can be changed; open a folder to look inside it.
          </p>
          {pendingCount > 0 && (
            <p className="text-pending m-0 flex items-center gap-2 text-[13px] leading-[18px]">
              <span aria-hidden="true" className="bg-pending size-1.5 shrink-0" />
              Includes the {pendingCount} {plural(pendingCount, 'change')} not on your TV yet.
            </p>
          )}
        </div>
        {preview.rows.length > 0 && <RefreshPreviewButton />}
      </div>

      {nothingOnHome ? (
        <>
          <p className="text-dim m-0 text-[13px] leading-[18px]">
            Nothing on home — every selected catalog is set to Discover only.
          </p>
          <div className="tv-bezel">
            <div
              className="tv-screen"
              role="img"
              aria-label="Preview of your TV home screen, with no rows yet"
            />
          </div>
        </>
      ) : (
        <div className="tv-bezel">
          <div
            className="tv-screen"
            tabIndex={0}
            role="region"
            aria-label="Preview of your TV home screen. Scroll inside it to see every row."
          >
            {preview.pinnedCollections.map((collection) => (
              <TVCollectionRow key={collection.id} collection={collection} onOpenFolder={onOpenFolder} />
            ))}
            {preview.rows.map((row) => (
              <TVCatalogRow key={row.id} row={row} tiles={tiles.get(row.id) ?? noTiles()} />
            ))}
            {preview.unpinnedCollections.map((collection) => (
              <TVCollectionRow key={collection.id} collection={collection} onOpenFolder={onOpenFolder} />
            ))}
          </div>
        </div>
      )}

      <p className="text-dim m-0 max-w-[72ch] text-[12px] leading-[17px]">
        The screen scrolls, like your TV — sideways within a row, up and down between them. A flat
        tone shows behind a tile until its poster loads, or in place of one it doesn't have.
      </p>

      {preview.discoverOnly.length > 0 && <DiscoverOnly rows={preview.discoverOnly} />}
    </div>
  )
}

/**
 * Fetches every row on screen again. Tiles are cached for five minutes, so
 * without this a shuffling catalog keeps showing the page it drew first.
 * Only active queries refetch — the rows of whichever view is mounted.
 */
function RefreshPreviewButton() {
  const queryClient = useQueryClient()
  const fetching = useIsFetching({ queryKey: queryKeys.catalogPreviews() }) > 0

  return (
    <button
      type="button"
      onClick={() => void queryClient.refetchQueries({ queryKey: queryKeys.catalogPreviews(), type: 'active' })}
      disabled={fetching}
      className="btn-secondary btn-sm shrink-0"
    >
      <Icon icon={RefreshCw} size={14} className={fetching ? 'motion-safe:animate-spin' : undefined} />
      {fetching ? 'Refreshing…' : 'Refresh preview'}
    </button>
  )
}

/**
 * `show_in_home = false` means "stay in Discover, don't take a row on home".
 * `buildManifest` (`internal/addon/addon.go`) enforces this by marking the
 * catalog's genre filter `isRequired`, so these rows are a genuine omission
 * from the home screen, not just from this preview. Listed beneath the
 * frame, never drawn as a TV row.
 */
function DiscoverOnly({ rows }: { rows: PreviewRow[] }) {
  const home = useHomeSelection()
  return (
    <section className="border-line flex flex-col gap-2 border-t pt-4">
      <div className="flex items-baseline gap-3">
        <span className="type-eyebrow">Not on home</span>
      </div>
      <div className="flex flex-col gap-1">
        {rows.map((row) => (
          <div key={row.id} className="flex flex-wrap items-baseline gap-x-2 gap-y-0.5">
            <span className="text-ink truncate text-[13px]">{row.name}</span>
            <span className="text-dim text-[12px]">
              · in Discover only, so not a row on the home screen
            </span>
            {home.isDetached(row.id) && (
              <span
                className="text-series text-[12px]"
                title="Deleted. It still works on your home screen, but removing it here can't be undone."
              >
                · not in library
              </span>
            )}
          </div>
        ))}
      </div>
    </section>
  )
}

/**
 * First run, nothing selected anywhere — not merely "no rows on home", which
 * `HomeScreenView`'s own empty case reports. The tab still works and shows
 * its own picture, per DESIGN.md's empty-state list; this one Uno moment
 * predates the redesign and stays: SMPTE colour bars, television's own
 * artifact for "nothing to show", now filling the real TV screen instead of
 * a plain panel. `docs/frontend.md`'s Visual direction section has the
 * rationale.
 */
function EmptyHomeScreen() {
  const bars = [
    'var(--smpte-white)',
    'var(--smpte-yellow)',
    'var(--smpte-cyan)',
    'var(--smpte-green)',
    'var(--smpte-magenta)',
    'var(--smpte-red)',
    'var(--smpte-blue)',
  ]

  return (
    <div className="flex flex-col gap-3">
      <p className="text-dim m-0 max-w-[72ch] text-[13px] leading-[18px]">
        Nothing is on your home screen yet, so there's nothing to show. Add rows and they appear
        here the way your TV shows them.
      </p>
      <div className="tv-bezel">
        <div
          className="tv-screen relative"
          role="img"
          aria-label="Preview of your TV home screen, with no rows yet"
        >
          <div aria-hidden="true" className="absolute inset-0 flex">
            {bars.map((color) => (
              <span key={color} className="flex-1" style={{ background: color }} />
            ))}
          </div>
          <p className="type-display bg-ground text-ink absolute inset-x-0 top-1/2 m-0 -translate-y-1/2 px-5 py-3 text-center text-[13px]">
            No signal — nothing on your home screen yet
          </p>
        </div>
      </div>
      <p className="text-dim m-0 text-[12px] leading-[17px]">
        Add catalogs and collections from the sidebar, then push.
      </p>
    </div>
  )
}
