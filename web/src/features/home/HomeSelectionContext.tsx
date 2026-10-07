import { createContext, useCallback, useMemo, useState } from 'react'
import type { ReactNode } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { queryKeys } from '@/api'
import type { Catalog, Collection } from '@/api'
import { useLibrary } from '@/features/library/useLibrary'
import { waitingIDs } from '@/features/sharing/sharingState'
import { computeHomeChanges, countUnsaved } from './changes'
import type { HomeChange } from './changes'
import {
  EMPTY_HOME,
  catalogEntries,
  collectionEntries,
  hydrateHome,
  moveInBand,
  reorderBand,
  toggleShowInHome,
  togglePinToTop,
  withRow,
  withoutRow,
} from './pending'
import type { HomeCollectionEntry, HomeEntry, HomeRowsEdit, HomeState } from './pending'
import { usePrunedHome } from './usePrunedHome'

export interface HomeSelection extends HomeEdits {
  /** False until both owned lists have loaded, which the Home is read from.
   *  Edits are blocked until then — hydrating over a user's changes would
   *  silently discard them. */
  ready: boolean
  isLoading: boolean
  /** What stops the Home pane rendering: an owned list that never loaded. A
   *  refetch that fails after hydration doesn't count — pending edits never
   *  read the lists again, and the lookup maps keep the rows they already
   *  had. */
  error: Error | null
  /** Re-runs whatever failed. */
  retry: () => void

  /** Every row on Home, in Home order. */
  rows: HomeEntry[]
  /** The collections among `rows`, in Home order. */
  collections: HomeCollectionEntry[]

  /** Every catalog/collection the Home pane might need to render, keyed by id:
   *  the library's rows, and the scoped catalogs its collections' folders use. */
  catalogById: ReadonlyMap<string, Catalog>
  collectionById: ReadonlyMap<string, Collection>

  /** The ids of the rows a push would change in Nuvio, which the rows flag
   *  To push (`waitingIDs`). */
  waitingForPush: ReadonlySet<string>

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
  /** Reorders one band — the pinned collections, or the home rows, catalogs
   *  and collections mixed — leaving every other row as it was. */
  reorderBand: (band: 'pinned' | 'home', orderedIds: string[]) => void
  /** Moves a row one step within its own band. */
  moveRow: (id: string, direction: -1 | 1) => void
  /** Adds a collection, starting from the Show first it was last pushed with. */
  addCollection: (id: string) => void
  removeCollection: (id: string) => void
  /** Flips a collection's Show first — a pending edit, like Home or Discover
   *  for a catalog, that only Push writes. */
  togglePinToTop: (id: string) => void
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

  const pending = library.pending

  // `baseline` is what the server says is live; `current` is what the user has
  // pending. Both are snapshotted once, at hydration — a later refetch must
  // not move the baseline under the user and silently change the diff.
  const [current, setCurrent] = useState<HomeState | null>(null)
  const [baseline, setBaseline] = useState<HomeState>(EMPTY_HOME)

  if (current === null && library.loaded) {
    const hydrated = hydrateHome(library.catalogs, library.collections)
    setBaseline(hydrated)
    setCurrent(hydrated)
  }

  const collectionById = useMemo(() => byId(library.collections), [library.collections])

  const catalogById = useMemo(
    () => byId([...library.catalogs, ...folderCatalogs(library.collections)]),
    [library.catalogs, library.collections],
  )

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
  // push, which the list of changes says from `pending`.
  usePrunedHome(current, baseline, library.loaded ? libraryIds : null, setCurrent, setBaseline)

  // The Show first a collection was last pushed with, which one added to the
  // home screen starts from.
  const storedPin = useCallback(
    function storedPin(id: string) {
      return collectionById.get(id)?.pin_to_top === true
    },
    [collectionById],
  )

  const state = current ?? EMPTY_HOME
  const changes = useMemo(
    () => computeHomeChanges({ baseline, current: state, catalogById, collectionById, waiting: pending }),
    [baseline, state, catalogById, collectionById, pending],
  )
  const pendingCount = changes.length
  const unsavedCount = countUnsaved(changes)

  const edit = useCallback((update: (previous: HomeState) => HomeState) => {
    setCurrent(function edited(previous) {
      return previous === null ? previous : update(previous)
    })
  }, [])

  const waitingForPush = useMemo(() => waitingIDs(pending), [pending])

  // Every one of these only closes over `edit` — stable for the life of the
  // provider — and, for `addCollection`, `storedPin`, so this whole cluster
  // needs recomputing only when the collections change, not on every render
  // that changes `state`. The band-aware edits read each row's band from the
  // state they edit.
  const editFns = useMemo<HomeEdits>(
    function homeEdits() {
      function editRows(update: HomeRowsEdit) {
        edit((previous) => ({ rows: update(previous.rows) }))
      }
      return {
        // New rows default to showing on home: adding a catalog you can't see
        // would be a confusing default.
        addCatalog: (id: string) => editRows((rows) => withRow(rows, { kind: 'catalog', id, showInHome: true })),
        removeCatalog: (id: string) => editRows((rows) => withoutRow(rows, id)),
        toggleShowInHome: (id: string) => editRows((rows) => toggleShowInHome(rows, id)),
        reorderBand: (band: 'pinned' | 'home', orderedIds: string[]) =>
          editRows((rows) => reorderBand(rows, band, orderedIds)),
        moveRow: (id: string, direction: -1 | 1) => editRows((rows) => moveInBand(rows, id, direction)),
        addCollection: (id: string) =>
          editRows((rows) => withRow(rows, { kind: 'collection', id, pinToTop: storedPin(id) })),
        removeCollection: (id: string) => editRows((rows) => withoutRow(rows, id)),
        togglePinToTop: (id: string) => editRows((rows) => togglePinToTop(rows, id)),
      }
    },
    [edit, storedPin],
  )

  // Every failed query in this profile's subtree: the library and the rest.
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
        isLoading: library.isLoading,
        error: current === null ? library.error : null,
        retry,

        rows: state.rows,
        collections: collectionEntries(state),
        catalogById,
        collectionById,
        waitingForPush,
        genres: library.genres,

        changes,
        pendingCount,
        unsavedCount,
        isDirty: unsavedCount > 0,

        snapshot: () => state,
        markPushed: (pushed) => setBaseline(pushed),

        hasCatalog: (id) => catalogEntries(state).some((c) => c.id === id),
        hasCollection: (id) => collectionEntries(state).some((c) => c.id === id),

        ...editFns,
      }
    },
    [
      current,
      state,
      library.isLoading,
      library.error,
      library.genres,
      retry,
      catalogById,
      collectionById,
      waitingForPush,
      changes,
      pendingCount,
      unsavedCount,
      editFns,
    ],
  )

  return (
    <HomeEditsContext.Provider value={editFns}>
      <HomeSelectionContext.Provider value={value}>{children}</HomeSelectionContext.Provider>
    </HomeEditsContext.Provider>
  )
}

/** Every catalog the folders of `collections` reference, in order. */
function folderCatalogs(collections: Iterable<Collection>): Catalog[] {
  const catalogs: Catalog[] = []
  for (const collection of collections) catalogs.push(...(collection.catalogs ?? []))
  return catalogs
}

/** `rows` by id, a later row over an earlier one with the same id. */
function byId<T extends { id: string }>(rows: readonly T[]): Map<string, T> {
  return new Map(rows.map((row) => [row.id, row]))
}
