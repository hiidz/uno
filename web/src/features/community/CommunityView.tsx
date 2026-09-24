import { useEffect, useMemo, useState } from 'react'
import { Navigate } from 'react-router-dom'
import { ApiError, ProfileNotSelectedError } from '@/api'
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
import { useCommunityMutations, type CommunityAction } from './useCommunityMutations'

type Kind = 'catalogs' | 'collections'
type Sort = 'name' | 'newest'

/**
 * The Community tab: everyone else's public catalogs and collections, browsed
 * and copied rather than referenced — the closed-graph model's only path
 * across an owner boundary. No author, no handle, no "copied from" line
 * anywhere here: that provenance is retired,
 * not merely hidden — `taken_from` exists only to link this profile's own
 * copy to its original, so a row can say "✓ Taken" or offer Update.
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
  const [toast, setToast] = useState<Toast | null>(null)
  // Keyed by the original's id; catalog and collection ids never collide.
  const [pending, setPending] = useState<ReadonlyMap<string, CommunityAction>>(new Map())

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

  // Actions on different rows can overlap, so each settles through its own
  // mutateAsync promise: callbacks passed to mutate() run only for the latest
  // call, which would leave an earlier row stuck on "Taking…". The promise
  // settles once the lists have refetched (`useCommunityMutations`), so the
  // toast arrives with the row already showing the outcome.
  function run(rowKind: Kind, id: string, action: CommunityAction) {
    setPending((current) => new Map(current).set(id, action))
    mutations[rowKind][action]
      .mutateAsync(id)
      .then(
        () => setToast({ text: DONE[action][rowKind], tone: 'success' }),
        (error: Error) => setToast(failure(action, rowKind, error)),
      )
      .finally(() =>
        setPending((current) => {
          const next = new Map(current)
          next.delete(id)
          return next
        }),
      )
  }

  const activeQuery = kind === 'catalogs' ? catalogsQuery : collectionsQuery
  const activeCount = kind === 'catalogs' ? catalogs.length : collections.length

  // A 404 on a profile-scoped route means this slot was never selected —
  // there's nothing to retry, so send the user back to pick one.
  if (activeQuery.error instanceof ProfileNotSelectedError) {
    return <Navigate to="/profiles" replace />
  }

  return (
    <section className="mx-auto flex w-full max-w-[900px] flex-col gap-4 p-4 lg:p-6">
      <div className="flex flex-col gap-1">
        <h1 className="type-display m-0 text-[17px] lg:text-[21px]">Community</h1>
        <p className="type-data text-dimmer m-0 text-[11px]">
          Take a copy of anything shared here. Nothing you do to it reaches the original. When
          the owner changes theirs, Update brings your copy in line, as long as you haven't
          edited it.
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
                updateAvailable={catalog.update_available}
                pending={pending.get(catalog.id)}
                previewOpen={previewID === catalog.id}
                onTogglePreview={() => togglePreview(catalog.id)}
                onTake={() => run('catalogs', catalog.id, 'take')}
                onUpdate={() => run('catalogs', catalog.id, 'update')}
                onDuplicate={() => run('catalogs', catalog.id, 'duplicate')}
                preview={<CommunityCatalogPreview catalog={catalog} />}
              />
            ))
          : collections.map((collection) => (
              <CommunityRow
                key={collection.id}
                name={collection.title}
                summary={collectionSummary(collection)}
                taken={collection.taken}
                updateAvailable={collection.update_available}
                pending={pending.get(collection.id)}
                previewOpen={previewID === collection.id}
                onTogglePreview={() => togglePreview(collection.id)}
                onTake={() => run('collections', collection.id, 'take')}
                onUpdate={() => run('collections', collection.id, 'update')}
                onDuplicate={() => run('collections', collection.id, 'duplicate')}
                preview={<CommunityCollectionPreview collection={collection} genres={genres} />}
              />
            ))}
      </ListState>
    </section>
  )
}

interface Toast {
  text: string
  tone: 'success' | 'danger'
}

const DONE: Record<CommunityAction, Record<Kind, string>> = {
  take: { catalogs: 'Added to your catalogs', collections: 'Added to your collections' },
  update: { catalogs: 'Updated your copy', collections: 'Updated your copy' },
  duplicate: { catalogs: 'Duplicated to your catalogs', collections: 'Duplicated to your collections' },
}

/** The toast for a failed action. A stale failure (`isStale`) has already
 *  refreshed the lists, so the answers that mean "the row was behind" say what
 *  happened rather than repeating the server's message: a 409 from Take means
 *  the copy exists; a 409 from Update means Update found the copy edited and
 *  unlinked it; a 404 from Update means the original went private or was
 *  deleted, or the copy was already unlinked, and the response can't say
 *  which. */
function failure(action: CommunityAction, rowKind: Kind, error: Error): Toast {
  const noun = rowKind === 'catalogs' ? 'catalog' : 'collection'
  const status = error instanceof ApiError ? error.status : undefined
  if (action === 'take' && status === 409) {
    return { text: 'Already taken', tone: 'success' }
  }
  if (action === 'update' && status === 409) {
    return {
      text: "Your copy was edited, so it's no longer linked. Take it again to get the latest",
      tone: 'danger',
    }
  }
  if (action === 'update' && status === 404) {
    return {
      text: "Couldn't update: the original is no longer available, or your copy is no longer linked.",
      tone: 'danger',
    }
  }
  const what = action === 'update' ? 'your copy' : `this ${noun}`
  return { text: `Couldn't ${action} ${what}: ${error.message}`, tone: 'danger' }
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
