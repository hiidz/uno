import { useEffect, useMemo, useState } from 'react'
import type { CommunityCatalog, CommunityCollection } from '@/api'
import { Segmented } from '@/components/fields'
import { ListState } from '@/components/ListState'
import { useLibrary } from '@/features/library/useLibrary'
import {
  catalogSummary,
  CommunityCatalogPreview,
  CommunityCollectionPreview,
  CommunityRow,
  collectionSummary,
} from './CommunityRow'
import { useCommunityCatalogs, useCommunityCollections } from './useCommunity'
import { useCommunityMutations } from './useCommunityMutations'

type Kind = 'catalogs' | 'collections'
type Sort = 'name' | 'newest'

/**
 * The Community tab: everyone else's public catalogs and collections, browsed
 * and copied rather than referenced — the closed-graph model's only path
 * across an owner boundary. No author, no handle, no "copied from" line
 * anywhere here: that provenance is retired,
 * not merely hidden — `taken_from` exists only so this profile's own copies
 * can say "you already took this."
 *
 * `useLibrary` is called here only for its genre lookups, which
 * `describeRecipe`/`buildRefOptions` need to render a recipe or a folder's
 * sources in plain English. It's the same query key `Workspace` and
 * `HomeSelectionContext` already hold open, so this is a third subscriber to
 * cached data, not a third network round trip (see `docs/frontend.md`'s note
 * on `useLibrary`'s existing multiple call sites).
 */
export function CommunityView({ profileIndex }: { profileIndex: number }) {
  const { genres } = useLibrary(profileIndex)
  const catalogsQuery = useCommunityCatalogs(profileIndex)
  const collectionsQuery = useCommunityCollections(profileIndex)
  const mutations = useCommunityMutations(profileIndex)

  const [kind, setKind] = useState<Kind>('catalogs')
  const [search, setSearch] = useState('')
  const [sort, setSort] = useState<Sort>('name')
  const [previewID, setPreviewID] = useState<string | null>(null)
  const [toast, setToast] = useState<{ text: string; tone: 'success' | 'danger' } | null>(null)
  const [pendingCatalogIDs, setPendingCatalogIDs] = useState<ReadonlySet<string>>(new Set())
  const [pendingCollectionIDs, setPendingCollectionIDs] = useState<ReadonlySet<string>>(new Set())

  // Auto-dismissing, unlike the push outcome strip: nothing here needs a
  // decision, so there is nothing worth keeping on screen once it's been read.
  useEffect(() => {
    if (!toast) return
    const timer = window.setTimeout(() => setToast(null), 2500)
    return () => window.clearTimeout(timer)
  }, [toast])

  const query = search.trim().toLowerCase()

  const catalogs = useMemo(
    () => sortRows(filterByName(catalogsQuery.data ?? [], (c) => c.name, query), sort),
    [catalogsQuery.data, query, sort],
  )
  const collections = useMemo(
    () => sortRows(filterByName(collectionsQuery.data ?? [], (c) => c.title, query), sort),
    [collectionsQuery.data, query, sort],
  )

  function togglePreview(id: string) {
    setPreviewID((current) => (current === id ? null : id))
  }

  function take(catalog: CommunityCatalog) {
    setPendingCatalogIDs((ids) => new Set(ids).add(catalog.id))
    mutations.takeCatalog.mutate(catalog.id, {
      onSuccess: () =>
        setToast({
          text: catalog.taken ? 'Added another copy to your catalogs' : 'Added to your catalogs',
          tone: 'success',
        }),
      onError: (error) =>
        setToast({ text: `Couldn't take this catalog: ${error.message}`, tone: 'danger' }),
      onSettled: () =>
        setPendingCatalogIDs((ids) => {
          const next = new Set(ids)
          next.delete(catalog.id)
          return next
        }),
    })
  }

  function takeCollection(collection: CommunityCollection) {
    setPendingCollectionIDs((ids) => new Set(ids).add(collection.id))
    mutations.takeCollection.mutate(collection.id, {
      onSuccess: () =>
        setToast({
          text: collection.taken
            ? 'Added another copy to your collections'
            : 'Added to your collections',
          tone: 'success',
        }),
      onError: (error) =>
        setToast({ text: `Couldn't take this collection: ${error.message}`, tone: 'danger' }),
      onSettled: () =>
        setPendingCollectionIDs((ids) => {
          const next = new Set(ids)
          next.delete(collection.id)
          return next
        }),
    })
  }

  const activeQuery = kind === 'catalogs' ? catalogsQuery : collectionsQuery
  const activeCount = kind === 'catalogs' ? catalogs.length : collections.length

  return (
    <section className="mx-auto flex w-full max-w-[900px] flex-col gap-4 p-4 lg:p-6">
      <div className="flex flex-col gap-1">
        <h1 className="type-display m-0 text-[17px] lg:text-[21px]">Community</h1>
        <p className="type-data text-dimmer m-0 text-[11px]">
          Take a copy of anything shared here. It's yours from then on — nothing you do to it
          reaches the original, and nothing that happens to the original reaches your copy.
        </p>
      </div>

      {toast && (
        <div
          role="status"
          className={`bg-raised type-data border px-3 py-2 text-[11px] ${
            toast.tone === 'danger' ? 'border-danger text-danger' : 'border-line text-ink'
          }`}
        >
          {toast.text}
        </div>
      )}

      <div className="flex flex-wrap items-center gap-3">
        <Segmented
          ariaLabel="Community kind"
          value={kind}
          onChange={setKind}
          options={[
            { value: 'catalogs', label: 'Catalogs' },
            { value: 'collections', label: 'Collections' },
          ]}
        />
        <input
          type="search"
          value={search}
          onChange={(event) => setSearch(event.target.value)}
          placeholder="Search by name…"
          aria-label="Search the community"
          className="field type-data min-w-[200px] flex-1 text-[12px] pointer-coarse:text-[16px]"
        />
        <div className="flex items-center gap-2">
          <span className="type-eyebrow">Sort</span>
          <Segmented
            ariaLabel="Sort community results"
            value={sort}
            onChange={setSort}
            options={[
              { value: 'name', label: 'Name' },
              { value: 'newest', label: 'Newest' },
            ]}
          />
        </div>
      </div>

      {query && (activeQuery.data?.length ?? 0) > 0 && (
        <p className="type-data text-dimmer m-0 text-[11px]">
          {activeCount} of {activeQuery.data?.length} {kind}
        </p>
      )}

      <ListState
        isLoading={activeQuery.isPending}
        error={activeQuery.error as Error | null}
        isEmpty={activeCount === 0}
        loadingLabel={`Loading community ${kind}…`}
        errorLabel={`Couldn't load community ${kind}.`}
        onRetry={activeQuery.refetch}
        emptyLabel={
          query
            ? `No ${kind} match this search.`
            : kind === 'catalogs'
              ? 'Nobody has shared a catalog yet.'
              : 'Nobody has shared a collection yet.'
        }
      >
        {kind === 'catalogs'
          ? catalogs.map((catalog) => (
              <CommunityRow
                key={catalog.id}
                name={catalog.name}
                summary={catalogSummary(catalog)}
                taken={catalog.taken}
                taking={pendingCatalogIDs.has(catalog.id)}
                previewOpen={previewID === catalog.id}
                onTogglePreview={() => togglePreview(catalog.id)}
                onTake={() => take(catalog)}
                preview={<CommunityCatalogPreview catalog={catalog} />}
              />
            ))
          : collections.map((collection) => (
              <CommunityRow
                key={collection.id}
                name={collection.title}
                summary={collectionSummary(collection)}
                taken={collection.taken}
                taking={pendingCollectionIDs.has(collection.id)}
                previewOpen={previewID === collection.id}
                onTogglePreview={() => togglePreview(collection.id)}
                onTake={() => takeCollection(collection)}
                preview={<CommunityCollectionPreview collection={collection} genres={genres} />}
              />
            ))}
      </ListState>
    </section>
  )
}

function filterByName<T>(rows: T[], name: (row: T) => string, query: string): T[] {
  if (!query) return rows
  return rows.filter((row) => name(row).toLowerCase().includes(query))
}

function sortRows<T extends { name?: string; title?: string; created_at: string }>(
  rows: T[],
  sort: Sort,
): T[] {
  const sorted = [...rows]
  if (sort === 'newest') {
    sorted.sort((a, b) => b.created_at.localeCompare(a.created_at))
  } else {
    sorted.sort((a, b) => (a.name ?? a.title ?? '').localeCompare(b.name ?? b.title ?? ''))
  }
  return sorted
}
