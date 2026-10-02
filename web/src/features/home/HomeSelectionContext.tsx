import { createContext, useCallback, useMemo, useState } from 'react'
import type { ReactNode } from 'react'
import { useQueries, useQueryClient } from '@tanstack/react-query'
import { fetchCatalogSelection, fetchCollectionSelection, fetchPendingPush, queryKeys } from '@/api'
import type { Catalog, Collection } from '@/api'
import { useLibrary } from '@/features/library/useLibrary'
import { waitingIDs } from '@/features/sharing/sharingState'
import { computeHomeChanges, countUnsaved } from './changes'
import type { HomeChange } from './changes'
import {
  EMPTY_HOME,
  moveCollectionInBand,
  moveWithinBand,
  reorderCollectionBand,
  reorderWithinBand,
  togglePinToTop,
  existingRowIDs,
} from './pending'
import type { HomeCatalogEntry, HomeCollectionEntry, HomeState } from './pending'
import { usePrunedHome } from './usePrunedHome'

export interface HomeSelection extends HomeEdits {
  /** False until the server's current selection has loaded. Edits are blocked
   *  until then — hydrating over a user's changes would silently discard them. */
  ready: boolean
  isLoading: boolean
  /** What stops the Home pane rendering: a selection that never loaded. A
   *  selection refetch that fails after hydration doesn't count — pending edits
   *  never read it again, and the lookup maps keep the rows they already had —
   *  and neither does the library, whose failures the rail reports itself. */
  error: Error | null
  /** Re-runs whatever failed. */
  retry: () => void

  catalogs: HomeCatalogEntry[]
  collections: HomeCollectionEntry[]

  /** Every catalog/collection the Home pane might need to render, keyed by id.
   *  Assembled from the selection response *and* the library, because a
   *  selected row can be absent from the library — see `isDetached`. */
  catalogById: ReadonlyMap<string, Catalog>
  collectionById: ReadonlyMap<string, Collection>

  /** The ids of the rows a push would change in Nuvio, which the rows flag
   *  To push (`waitingIDs`). */
  waitingForPush: ReadonlySet<string>

  /** True when a selected item is not in the library, which the lists can show
   *  for the moment between a delete and their refetch: the provider then drops
   *  the row from the pending selection (`usePrunedHome`). */
  isDetached: (id: string) => boolean

  /** Genre lookups, so the Home pane can render recipes without calling
   *  `useLibrary` again and re-deriving the whole dataset. */
  genres: ReturnType<typeof useLibrary>['genres']

  /** Named, per-row edits waiting to be pushed — see `computeHomeChanges`.
   *  `pendingCount` is this list's length, so the header's count and the list
   *  of changes it opens can never disagree about how many there are. */
  changes: HomeChange[]
  pendingCount: number
  /** The changes that exist in this tab alone, the ones leaving the page
   *  loses. A collection saved but not yet pushed is not one of them. */
  unsavedCount: number
  /** `unsavedCount > 0`: what the leave dialog and the page-close guard ask
   *  about. */
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
}

/**
 * The edits to the pending selection, also on a context of their own: they
 * change only with the collections an added one reads its last pushed pin
 * from, never with the selection they edit, so a component that makes edits
 * without reading the selection uses `useHomeEdits` and doesn't re-render on
 * every change to it.
 */
export interface HomeEdits {
  addCatalog: (id: string) => void
  removeCatalog: (id: string) => void
  toggleShowInHome: (id: string) => void
  /** Reorders the shown (home-row) catalogs only — Discover-only catalogs
   *  keep their existing relative order, off to one side. */
  reorderCatalogs: (orderedShownIds: string[]) => void
  /** Moves a catalog one step within its own band: shown, or Discover-only. */
  moveCatalog: (id: string, direction: -1 | 1) => void
  /** Adds a collection, starting from the Show first it was last pushed with. */
  addCollection: (id: string) => void
  removeCollection: (id: string) => void
  /** Flips a collection's Show first — a pending edit, like Home or Discover
   *  for a catalog, that only Push writes. */
  togglePinToTop: (id: string) => void
  /** Reorders one collection band — pinned or not — leaving the other
   *  untouched. A row moves only within its own band, matching the running
   *  order's three groups. */
  reorderCollections: (band: 'pinned' | 'unpinned', orderedBandIds: string[]) => void
  /** Moves a collection one step within its own band (pinned or not). */
  moveCollection: (id: string, direction: -1 | 1) => void
}

export const HomeSelectionContext = createContext<HomeSelection | null>(null)
export const HomeEditsContext = createContext<HomeEdits | null>(null)

export function HomeSelectionProvider({
  profileIndex,
  children,
}: {
  profileIndex: number
  children: ReactNode
}) {
  const library = useLibrary(profileIndex)

  const [catalogSelection, collectionSelection, pendingPush] = useQueries({
    queries: [
      {
        queryKey: queryKeys.catalogSelection(profileIndex),
        queryFn: () => fetchCatalogSelection(profileIndex),
      },
      {
        queryKey: queryKeys.collectionSelection(profileIndex),
        queryFn: () => fetchCollectionSelection(profileIndex),
      },
      {
        queryKey: queryKeys.pendingPush(profileIndex),
        queryFn: () => fetchPendingPush(profileIndex),
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

  if (current === null && selectionLoaded) {
    const hydrated: HomeState = {
      catalogs: (catalogSelectionData ?? []).map((c) => ({
        id: c.id,
        showInHome: c.show_in_home,
      })),
      collections: (collectionSelectionData ?? []).map((c) => ({ id: c.id, pinToTop: c.pin_to_top })),
    }
    setBaseline(hydrated)
    setCurrent(hydrated)
  }

  const collectionById = useMemo(() => {
    const map = new Map<string, Collection>()
    for (const c of collectionSelectionData ?? []) map.set(c.id, c)
    for (const c of library.collections) map.set(c.id, c)
    return map
  }, [collectionSelectionData, library.collections])

  const catalogById = useMemo(() => {
    // Library first — it's the canonical row — then anything only the
    // selection knows about, so a detached selection still renders. Last,
    // every catalog a known collection's own folders reference: those are
    // always scoped (never listed, so never in `library.catalogs`) —
    // `Collection.catalogs` is the only place they're carried.
    const map = new Map<string, Catalog>()
    for (const c of catalogSelectionData ?? []) map.set(c.id, c)
    for (const c of library.catalogs) map.set(c.id, c)
    for (const collection of collectionById.values()) {
      for (const c of collection.catalogs ?? []) map.set(c.id, c)
    }
    return map
  }, [catalogSelectionData, library.catalogs, collectionById])

  const libraryLoaded = !library.isLoading && !library.failed.catalogs && !library.failed.collections

  const libraryIds = useMemo(
    () =>
      new Set([
        ...library.catalogs.map((c) => c.id),
        ...library.collections.map((c) => c.id),
      ]),
    [library.catalogs, library.collections],
  )

  // A row deleted since — here, or in another tab — leaves the pending
  // selection and the baseline both, once the lists have refetched without it:
  // Push would be refused for naming it. Nuvio still holds it until the next
  // push, which the list of changes says from `pendingPush`.
  usePrunedHome(
    current,
    baseline,
    existingRowIDs(selectionLoaded, libraryLoaded, libraryIds, catalogSelectionData, collectionSelectionData),
    setCurrent,
    setBaseline,
  )

  // The Show first a collection was last pushed with, which one added to the
  // home screen starts from.
  const storedPin = useCallback(
    (id: string) => collectionById.get(id)?.pin_to_top === true,
    [collectionById],
  )

  const state = current ?? EMPTY_HOME
  const changes = useMemo(
    () => computeHomeChanges({ baseline, current: state, catalogById, collectionById, waiting: pendingPush.data }),
    [baseline, state, catalogById, collectionById, pendingPush.data],
  )
  const pendingCount = changes.length
  const unsavedCount = countUnsaved(changes)

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
      waitingForPush: waitingIDs(pendingPush.data),

      // Only meaningful once the library has actually loaded; before that, or
      // when it failed to, everything would look detached.
      isDetached: (id: string) => libraryLoaded && !libraryIds.has(id),
      genres: library.genres,

      changes,
      pendingCount,
      unsavedCount,
      isDirty: unsavedCount > 0,
    }),
    [
      catalogById,
      collectionById,
      pendingPush.data,
      libraryLoaded,
      libraryIds,
      library.genres,
      changes,
      pendingCount,
      unsavedCount,
    ],
  )

  // Every one of these only closes over `edit` — stable for the life of the
  // provider — and, for `addCollection`, `storedPin`, so this whole cluster
  // needs recomputing only when the collections change, not on every render
  // that changes `state`. The band-aware edits read each row's pin from the
  // state they edit.
  const editFns = useMemo<HomeEdits>(
    function homeEdits() {
      return {
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
            previous.collections.some((c) => c.id === id)
              ? previous
              : { ...previous, collections: [...previous.collections, { id, pinToTop: storedPin(id) }] },
          ),
        removeCollection: (id: string) =>
          edit((previous) => ({
            ...previous,
            collections: previous.collections.filter((c) => c.id !== id),
          })),
        togglePinToTop: (id: string) =>
          edit((previous) => ({ ...previous, collections: togglePinToTop(previous.collections, id) })),
        reorderCollections: (band: 'pinned' | 'unpinned', orderedBandIds: string[]) =>
          edit((previous) => ({
            ...previous,
            collections: reorderCollectionBand(previous.collections, band, orderedBandIds),
          })),
        moveCollection: (id: string, direction: -1 | 1) =>
          edit((previous) => ({
            ...previous,
            collections: moveCollectionInBand(previous.collections, id, direction),
          })),
      }
    },
    [edit, storedPin],
  )

  // Every failed query in this profile's subtree — the two selections, and the
  // library's own lists if they failed too.
  const queryClient = useQueryClient()
  const retry = useCallback(() => {
    void queryClient.refetchQueries({
      queryKey: queryKeys.profile(profileIndex),
      predicate: (query) => query.state.status === 'error',
    })
  }, [queryClient, profileIndex])

  const value = useMemo<HomeSelection>(
    function selection() {
      return {
        ready: current !== null,
        isLoading: catalogSelection.isPending || collectionSelection.isPending || library.isLoading,
        error:
          current === null
            ? ((catalogSelection.error ?? collectionSelection.error) as Error | null)
            : null,
        retry,

        catalogs: state.catalogs,
        collections: state.collections,
        ...readData,

        snapshot: () => state,
        markPushed: (pushed) => setBaseline(pushed),

        hasCatalog: (id) => state.catalogs.some((c) => c.id === id),
        hasCollection: (id) => state.collections.some((c) => c.id === id),

        ...editFns,
      }
    },
    [
      current,
      state,
      library.isLoading,
      catalogSelection.isPending,
      catalogSelection.error,
      collectionSelection.isPending,
      collectionSelection.error,
      retry,
      readData,
      editFns,
    ],
  )

  return (
    <HomeEditsContext.Provider value={editFns}>
      <HomeSelectionContext.Provider value={value}>{children}</HomeSelectionContext.Provider>
    </HomeEditsContext.Provider>
  )
}
