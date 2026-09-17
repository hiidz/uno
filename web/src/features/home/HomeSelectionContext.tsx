import { createContext, useCallback, useEffect, useMemo, useState } from 'react'
import type { ReactNode } from 'react'
import { useQueries } from '@tanstack/react-query'
import { fetchCatalogSelection, fetchCollectionSelection, queryKeys } from '@/api'
import type { Catalog, Collection } from '@/api'
import { useLibrary } from '@/features/library/useLibrary'
import { computeHomeChanges } from './changes'
import type { HomeChange } from './changes'
import { EMPTY_HOME, moveWithinBand, reorderWithinBand } from './pending'
import type { HomeCatalogEntry, HomeState } from './pending'

export interface HomeSelection {
  /** False until the server's current selection has loaded. Edits are blocked
   *  until then — hydrating over a user's changes would silently discard them. */
  ready: boolean
  isLoading: boolean
  error: Error | null

  catalogs: HomeCatalogEntry[]
  collections: string[]

  /** Every catalog/collection the Home pane might need to render, keyed by id.
   *  Assembled from the selection response *and* the library, because a
   *  selected row can be absent from the library — see `isDetached`. */
  catalogById: ReadonlyMap<string, Catalog>
  collectionById: ReadonlyMap<string, Collection>

  /** True when a selected item is no longer in the library — the row was
   *  deleted after being selected. It still works, but removing it is
   *  one-way, so the UI says so. */
  isDetached: (id: string) => boolean

  /** `pin_to_top`, read through `collectionById` — a property of the
   *  collection itself, set in its own editor, never edited from this pane.
   *  Drives which of the two collection bands a row renders in. */
  isPinned: (id: string) => boolean

  /** Genre lookups, so the Home pane can render recipes without calling
   *  `useLibrary` again and re-deriving the whole dataset. */
  genres: ReturnType<typeof useLibrary>['genres']

  /** Named, per-row edits waiting to be pushed — see `computeHomeChanges`.
   *  `pendingCount` is this list's length, so the header's count and the list
   *  of changes it opens can never disagree about how many there are. */
  changes: HomeChange[]
  pendingCount: number
  isDirty: boolean

  /** The exact state to send to Push, snapshotted by the caller so a later
   *  `markPushed` can name what it is acknowledging. */
  snapshot: () => HomeState

  /**
   * Advance the baseline to the state a push just persisted. Takes the pushed
   * state rather than reading `current`, because the user can keep editing
   * while a push is in flight — acknowledging "whatever is current now" would
   * silently swallow those edits and report them as already live.
   */
  markPushed: (pushed: HomeState) => void

  hasCatalog: (id: string) => boolean
  hasCollection: (id: string) => boolean
  addCatalog: (id: string) => void
  removeCatalog: (id: string) => void
  toggleShowInHome: (id: string) => void
  /** Reorders the shown (home-row) catalogs only — Discover-only catalogs
   *  keep their existing relative order, off to one side. */
  reorderCatalogs: (orderedShownIds: string[]) => void
  /** Moves a catalog one step within its own band: shown, or Discover-only. */
  moveCatalog: (id: string, direction: -1 | 1) => void
  addCollection: (id: string) => void
  removeCollection: (id: string) => void
  /** Reorders one collection band — pinned or not — leaving the other
   *  untouched. A row moves only within its own band, matching the running
   *  order's three groups. */
  reorderCollections: (band: 'pinned' | 'unpinned', orderedBandIds: string[]) => void
  /** Moves a collection one step within its own band (pinned or not). */
  moveCollection: (id: string, direction: -1 | 1) => void
}

export const HomeSelectionContext = createContext<HomeSelection | null>(null)

export function HomeSelectionProvider({
  profileIndex,
  children,
}: {
  profileIndex: number
  children: ReactNode
}) {
  const library = useLibrary(profileIndex)

  const [catalogSelection, collectionSelection] = useQueries({
    queries: [
      {
        queryKey: queryKeys.catalogSelection(profileIndex),
        queryFn: () => fetchCatalogSelection(profileIndex),
      },
      {
        queryKey: queryKeys.collectionSelection(profileIndex),
        queryFn: () => fetchCollectionSelection(profileIndex),
      },
    ],
  })

  // `baseline` is what the server says is live; `current` is what the user has
  // pending. Both are snapshotted once, at hydration — a later refetch must
  // not move the baseline under the user and silently change the diff.
  const [current, setCurrent] = useState<HomeState | null>(null)
  const [baseline, setBaseline] = useState<HomeState>(EMPTY_HOME)

  const selectionLoaded = catalogSelection.isSuccess && collectionSelection.isSuccess
  const catalogSelectionData = catalogSelection.data
  const collectionSelectionData = collectionSelection.data

  useEffect(() => {
    if (current !== null || !selectionLoaded) return
    const hydrated: HomeState = {
      catalogs: (catalogSelectionData ?? []).map((c) => ({
        id: c.id,
        showInHome: c.show_in_home,
      })),
      collections: (collectionSelectionData ?? []).map((c) => c.id),
    }
    setBaseline(hydrated)
    setCurrent(hydrated)
  }, [current, selectionLoaded, catalogSelectionData, collectionSelectionData])

  const catalogById = useMemo(() => {
    // Library first — it's the canonical row — then anything only the
    // selection knows about, so a detached selection still renders.
    const map = new Map<string, Catalog>()
    for (const c of catalogSelectionData ?? []) map.set(c.id, c)
    for (const c of library.catalogs) map.set(c.id, c)
    return map
  }, [catalogSelectionData, library.catalogs])

  const collectionById = useMemo(() => {
    const map = new Map<string, Collection>()
    for (const c of collectionSelectionData ?? []) map.set(c.id, c)
    for (const c of library.collections) map.set(c.id, c)
    return map
  }, [collectionSelectionData, library.collections])

  const libraryIds = useMemo(
    () =>
      new Set([
        ...library.catalogs.map((c) => c.id),
        ...library.collections.map((c) => c.id),
      ]),
    [library.catalogs, library.collections],
  )

  const isPinned = useCallback(
    (id: string) => collectionById.get(id)?.pin_to_top ?? false,
    [collectionById],
  )

  const state = current ?? EMPTY_HOME
  const changes = useMemo(
    () => computeHomeChanges({ baseline, current: state, catalogById, collectionById, isPinned }),
    [baseline, state, catalogById, collectionById, isPinned],
  )
  const pendingCount = changes.length

  const edit = useCallback((update: (previous: HomeState) => HomeState) => {
    setCurrent((previous) => (previous === null ? previous : update(previous)))
  }, [])

  // Data derived from the library and the selection responses — changes only
  // when one of those actually changes, not on every edit to `current`. Split
  // out so an edit that doesn't touch any of this (most of them) doesn't force
  // a new `isDetached` closure on every keystroke.
  const readData = useMemo(
    () => ({
      catalogById,
      collectionById,

      // Only meaningful once the library has actually loaded; before that
      // everything would look detached.
      isDetached: (id: string) => !library.isLoading && !libraryIds.has(id),
      isPinned,
      genres: library.genres,

      changes,
      pendingCount,
    }),
    [catalogById, collectionById, library.isLoading, libraryIds, isPinned, library.genres, changes, pendingCount],
  )

  // Every one of these only closes over `edit` (plus, for the band-aware ones,
  // `isPinned`) — stable for the life of the provider — so this whole cluster
  // needs recomputing only when `isPinned` itself changes, not on every render
  // that changes `state`.
  const editFns = useMemo(
    () => ({
      addCatalog: (id: string) =>
        edit((previous) =>
          previous.catalogs.some((c) => c.id === id)
            ? previous
            : // New rows default to showing on home: adding a catalog you
              // can't see would be a confusing default.
              { ...previous, catalogs: [...previous.catalogs, { id, showInHome: true }] },
        ),
      removeCatalog: (id: string) =>
        edit((previous) => ({
          ...previous,
          catalogs: previous.catalogs.filter((c) => c.id !== id),
        })),
      toggleShowInHome: (id: string) =>
        edit((previous) => ({
          ...previous,
          catalogs: previous.catalogs.map((c) =>
            c.id === id ? { ...c, showInHome: !c.showInHome } : c,
          ),
        })),
      reorderCatalogs: (orderedShownIds: string[]) =>
        edit((previous) => ({
          ...previous,
          catalogs: reorderWithinBand(previous.catalogs, (c) => c.showInHome, orderedShownIds),
        })),
      moveCatalog: (id: string, direction: -1 | 1) =>
        edit((previous) => ({
          ...previous,
          catalogs: moveWithinBand(previous.catalogs, (c) => c.showInHome, id, direction),
        })),

      addCollection: (id: string) =>
        edit((previous) =>
          previous.collections.includes(id)
            ? previous
            : { ...previous, collections: [...previous.collections, id] },
        ),
      removeCollection: (id: string) =>
        edit((previous) => ({
          ...previous,
          collections: previous.collections.filter((c) => c !== id),
        })),
      reorderCollections: (band: 'pinned' | 'unpinned', orderedBandIds: string[]) =>
        edit((previous) => {
          const inBand = (id: string) => (band === 'pinned' ? isPinned(id) : !isPinned(id))
          const asEntries = previous.collections.map((id) => ({ id }))
          return {
            ...previous,
            collections: reorderWithinBand(
              asEntries,
              (entry) => inBand(entry.id),
              orderedBandIds,
            ).map((entry) => entry.id),
          }
        }),
      moveCollection: (id: string, direction: -1 | 1) =>
        edit((previous) => {
          const pinned = isPinned(id)
          const asEntries = previous.collections.map((cid) => ({ id: cid }))
          return {
            ...previous,
            collections: moveWithinBand(
              asEntries,
              (entry) => isPinned(entry.id) === pinned,
              id,
              direction,
            ).map((entry) => entry.id),
          }
        }),
    }),
    [edit, isPinned],
  )

  const value = useMemo<HomeSelection>(
    () => ({
      ready: current !== null,
      isLoading: catalogSelection.isPending || collectionSelection.isPending || library.isLoading,
      error:
        ((catalogSelection.error ?? collectionSelection.error) as Error | null) ?? library.error,

      catalogs: state.catalogs,
      collections: state.collections,
      ...readData,

      isDirty: pendingCount > 0,

      snapshot: () => state,
      markPushed: (pushed) => setBaseline(pushed),

      hasCatalog: (id) => state.catalogs.some((c) => c.id === id),
      hasCollection: (id) => state.collections.includes(id),

      ...editFns,
    }),
    [
      current,
      state,
      pendingCount,
      library.isLoading,
      library.error,
      catalogSelection.isPending,
      catalogSelection.error,
      collectionSelection.isPending,
      collectionSelection.error,
      readData,
      editFns,
    ],
  )

  return <HomeSelectionContext.Provider value={value}>{children}</HomeSelectionContext.Provider>
}
