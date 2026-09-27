import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useIsFetching, useQueryClient } from '@tanstack/react-query'
import { RefreshCw } from 'lucide-react'
import { queryKeys } from '@/api'
import { Icon } from '@/components/Icon'
import { plural } from '@/lib/plural'
import { folderRecipes } from '@/features/preview/model'
import { noTiles } from '@/features/preview/tiles'
import { useRecipesTiles } from '@/features/preview/useRecipesTiles'
import { useHomePreview, useHomeSelection } from './useHomeSelection'
import { useCatalogTiles } from './useCatalogTiles'
import { findFolderPage } from './preview'
import type { FolderPageTarget, HomeScreenPreview, PreviewRow } from './preview'
import { FOLDER_LAYOUT_LABEL, TVCatalogRow, TVCollectionRow, TVFolderPage } from './tv'

/**
 * The Home pane's second view: the same pending state, drawn as the TV lays
 * it out — DESIGN.md's "Preview rows", in one raised panel. Reorder in List,
 * flip to here, see it move.
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
 * **Uno puts nothing between the rows — the Clean Preview rule.** Every
 * caveat this view has to state (the pending count, how to get back to home)
 * is Uno's own words above and below the panel; the panel itself carries only
 * what the real TV would show. See `tv.tsx` for what that drops.
 */
export function HomePreview() {
  const home = useHomeSelection()
  const preview = useHomePreview()
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
 * One Way Back rule in DESIGN.md. Held as ids, not indices, so removing the
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
      // Carries React Router's own state along, so a browser Forward onto this
      // entry still hands Builder its profile.
      window.history.pushState({ ...window.history.state, unoFolder: true }, '')
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
      // A Radix layer over the folder page (a dialog, a menu) handles Escape
      // first, in the capture phase, and marks it handled. It has usually
      // already unmounted by now, so it can't be looked up in the DOM.
      if (e.key !== 'Escape' || e.defaultPrevented) return
      closeFolder()
    }
    document.addEventListener('keydown', onKeyDown)
    return () => document.removeEventListener('keydown', onKeyDown)
  }, [target, closeFolder])

  // Leaving any other way — the target stopped resolving (removed in List
  // view), or this view unmounts entirely (the List | Preview switch, or an
  // editor opens) — still has to consume a pushed entry, or a later physical
  // Back press does nothing: the history stack would hold a dead entry this
  // component is no longer listening for. Only while that entry is still the
  // current one: an in-app navigation away (Switch profile) has already pushed
  // past it, and going back from there would land on it again and re-render
  // the builder the user just left.
  if (target !== null && page === null) setTarget(null)

  useEffect(() => {
    if (target === null && pushed.current) {
      pushed.current = false
      consumeFolderEntry()
    }
  }, [target])

  useEffect(() => {
    return () => {
      if (pushed.current) {
        pushed.current = false
        consumeFolderEntry()
      }
    }
  }, [])

  return { page, openFolder, closeFolder }
}

/** Steps back off the folder's history entry, if it is the current one. */
function consumeFolderEntry() {
  if ((window.history.state as { unoFolder?: boolean } | null)?.unoFolder) window.history.back()
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

      <div
        className="pv-panel"
        role="region"
        aria-label={`${folder.title || 'Untitled folder'}, a folder in ${collection.title || 'Untitled collection'}`}
      >
        <TVFolderPage collection={collection} folder={folder} tiles={tiles} onBack={onBack} />
      </div>

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
          {pendingCount > 0 && (
            <p className="text-pending m-0 flex items-center gap-2 text-[13px] leading-[18px]">
              <span aria-hidden="true" className="bg-pending size-1.5 shrink-0" />
              Includes {pendingCount} unpushed {plural(pendingCount, 'change')}.
            </p>
          )}
        </div>
        {preview.rows.length > 0 && <RefreshPreviewButton />}
      </div>

      {nothingOnHome ? (
        <p className="text-dim m-0 text-[13px] leading-[18px]">
          Nothing on home — every selected catalog is set to Discover only.
        </p>
      ) : (
        <>
          <div className="pv-panel" role="region" aria-label="Home screen preview">
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
        </>
      )}

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
 * panel, never drawn as a row.
 */
function DiscoverOnly({ rows }: { rows: PreviewRow[] }) {
  const home = useHomeSelection()
  return (
    <section className="border-line flex flex-col gap-2 border-t pt-4">
      <div className="flex items-baseline gap-3">
        <span className="type-label">Not on home</span>
      </div>
      <div className="flex flex-col gap-1">
        {rows.map((row) => (
          <div key={row.id} className="flex flex-wrap items-baseline gap-x-2 gap-y-0.5">
            <span className="text-ink truncate text-[13px]">{row.name}</span>
            <span className="text-dim text-[12px]">
              · Discover only
            </span>
            {home.isDetached(row.id) && (
              <span
                className="text-danger text-[12.5px]"
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
 * its own picture, per DESIGN.md's empty-state list: SMPTE colour bars,
 * television's own artifact for "nothing to show", on a rounded card.
 * `docs/frontend.md`'s Visual direction section has the rationale.
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
      <div className="pv-nosignal" role="img" aria-label="Home screen preview, with no rows yet">
        <div aria-hidden="true" className="absolute inset-0 flex">
          {bars.map((color) => (
            <span key={color} className="flex-1" style={{ background: color }} />
          ))}
        </div>
        <p className="type-display bg-ground text-ink absolute inset-x-0 top-1/2 m-0 -translate-y-1/2 px-5 py-3 text-center text-[13px]">
          No signal — nothing on your home screen yet
        </p>
      </div>
      <p className="text-dim m-0 text-[12px] leading-[17px]">
        Add catalogs and collections from the Library, then push.
      </p>
    </div>
  )
}
