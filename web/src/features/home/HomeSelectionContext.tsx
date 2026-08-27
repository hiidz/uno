import { createContext, useCallback, useEffect, useMemo, useState } from 'react'
import type { ReactNode } from 'react'
import { useQueries } from '@tanstack/react-query'
import { fetchCatalogSelection, fetchCollectionSelection, queryKeys } from '@/api'
import type { Catalog, Collection } from '@/api'
import { useLibrary } from '@/features/library/useLibrary'
import { applyOrder, countPendingChanges, EMPTY_HOME } from './pending'
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

  /** True when a selected item is no longer in the library — typically a
   *  community item whose owner made it private after it was selected. It
   *  still works, but removing it is one-way, so the UI says so. */
  isDetached: (id: string) => boolean

  /** Ownership for the type bar. Derived from library membership, the same
   *  rule the rail uses — a selected row absent from the library isn't yours. */
  isOwned: (id: string) => boolean

  /** Genre lookups, so the Home pane can render recipes without calling
   *  `useLibrary` again and re-deriving the whole dataset. */
  genres: ReturnType<typeof useLibrary>['genres']

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
  reorderCatalogs: (orderedIds: string[]) => void
  addCollection: (id: string) => void
  removeCollection: (id: string) => void
  reorderCollections: (orderedIds: string[]) => void
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

  const ownedIds = useMemo(
    () =>
      new Set([
        ...library.catalogs.filter((c) => c.owned).map((c) => c.id),
        ...library.collections.filter((c) => c.owned).map((c) => c.id),
      ]),
    [library.catalogs, library.collections],
  )

  const state = current ?? EMPTY_HOME
  const pendingCount = useMemo(() => countPendingChanges(baseline, state), [baseline, state])

  const edit = useCallback((update: (previous: HomeState) => HomeState) => {
    setCurrent((previous) => (previous === null ? previous : update(previous)))
  }, [])

  const value = useMemo<HomeSelection>(
    () => ({
      ready: current !== null,
      isLoading: catalogSelection.isPending || collectionSelection.isPending || library.isLoading,
      error:
        ((catalogSelection.error ?? collectionSelection.error) as Error | null) ?? library.error,

      catalogs: state.catalogs,
      collections: state.collections,
      catalogById,
      collectionById,

      // Only meaningful once the library has actually loaded; before that
      // everything would look detached.
      isDetached: (id) => !library.isLoading && !libraryIds.has(id),
      isOwned: (id) => ownedIds.has(id),
      genres: library.genres,

      pendingCount,
      isDirty: pendingCount > 0,

      snapshot: () => state,
      markPushed: (pushed) => setBaseline(pushed),

      hasCatalog: (id) => state.catalogs.some((c) => c.id === id),
      hasCollection: (id) => state.collections.includes(id),

      addCatalog: (id) =>
        edit((previous) =>
          previous.catalogs.some((c) => c.id === id)
            ? previous
            : // New rows default to showing on home: adding a catalog you
              // can't see would be a confusing default.
              { ...previous, catalogs: [...previous.catalogs, { id, showInHome: true }] },
        ),
      removeCatalog: (id) =>
        edit((previous) => ({
          ...previous,
          catalogs: previous.catalogs.filter((c) => c.id !== id),
        })),
      toggleShowInHome: (id) =>
        edit((previous) => ({
          ...previous,
          catalogs: previous.catalogs.map((c) =>
            c.id === id ? { ...c, showInHome: !c.showInHome } : c,
          ),
        })),
      reorderCatalogs: (orderedIds) =>
        edit((previous) => ({ ...previous, catalogs: applyOrder(previous.catalogs, orderedIds) })),

      addCollection: (id) =>
        edit((previous) =>
          previous.collections.includes(id)
            ? previous
            : { ...previous, collections: [...previous.collections, id] },
        ),
      removeCollection: (id) =>
        edit((previous) => ({
          ...previous,
          collections: previous.collections.filter((c) => c !== id),
        })),
      reorderCollections: (orderedIds) =>
        edit((previous) => ({
          ...previous,
          collections: applyOrder(
            previous.collections.map((id) => ({ id })),
            orderedIds,
          ).map((entry) => entry.id),
        })),
    }),
    [
      current,
      state,
      baseline,
      pendingCount,
      catalogById,
      collectionById,
      libraryIds,
      ownedIds,
      library.genres,
      library.isLoading,
      library.error,
      catalogSelection.isPending,
      catalogSelection.error,
      collectionSelection.isPending,
      collectionSelection.error,
      edit,
    ],
  )

  return <HomeSelectionContext.Provider value={value}>{children}</HomeSelectionContext.Provider>
}
