import { useCallback, useMemo, useState } from 'react'
import { Navigate } from 'react-router-dom'
import { Search } from 'lucide-react'
import { ApiError, ProfileNotSelectedError, type CommunityItem } from '@/api'
import { Segmented } from '@/components/fields'
import { Icon } from '@/components/Icon'
import { ListState } from '@/components/ListState'
import { Toast } from '@/components/Toast'
import { useToast, type ToastMessage } from '@/components/useToast'
import { useGenreLookups, type GenreLookups } from '@/features/library/useLibrary'
import { snapshotRecipeLine } from '@/features/sharing/snapshot'
import {
  itemMeta,
  itemSummary,
  ofKind,
  openItemIn,
  startingView,
  visibleItems,
  type CommunityFilters,
  type OpenPublication,
} from './communityQuery'
import { CommunityRow, type RowActions } from './CommunityRow'
import { PublicationPage } from './PublicationPage'
import { focusRow, rowButtonID, useScrollMemory } from './scroll'
import { useCommunityList } from './useCommunity'
import { useCommunityMutations, type CommunityAction } from './useCommunityMutations'

const NO_ITEMS: CommunityItem[] = []

/**
 * The Community tab: what other profiles publish, loaded in one call and
 * searched, filtered and sorted here. A row opens its publication's page in
 * place of the list, and the way back returns to the same scroll position.
 * Add puts it in the library, read-only, following its publisher's updates;
 * Update… opens the page, which shows the new version and applies it;
 * Duplicate adds a copy that is the profile's own. No publisher is named
 * anywhere here. `initialOpen` starts on a publication's page: an added row's
 * own Update… lands there.
 */
export function CommunityView({
  profileIndex,
  initialOpen,
}: {
  profileIndex: number
  initialOpen: OpenPublication | null
}) {
  const { genres } = useGenreLookups()
  const mutations = useCommunityMutations(profileIndex)
  const list = useCommunityList(profileIndex)

  const [start] = useState(() => startingView(initialOpen))
  const [filters, setFilters] = useState<CommunityFilters>(start.filters)
  const [openID, setOpenID] = useState<string | null>(start.openID)
  const [toast, setToast] = useToast()
  // Keyed by publication id. Actions on different rows can overlap.
  const [pending, setPending] = useState<ReadonlyMap<string, CommunityAction>>(new Map())

  const all = list.data ?? NO_ITEMS
  const items = useMemo(() => visibleItems(all, filters), [all, filters])
  const open = openItemIn(all, openID)
  const now = new Date()

  const { scrollRef, remember, restore } = useScrollMemory()

  const back = useCallback(
    (id: string) => {
      setOpenID(null)
      restore(() => focusRow(id))
    },
    [restore],
  )

  function openItem(id: string) {
    remember()
    setOpenID(id)
  }

  function run(item: CommunityItem, action: CommunityAction) {
    setPending((current) => new Map(current).set(item.id, action))
    mutations[action]
      .mutateAsync(item.id)
      .then(
        () => setToast({ text: doneText(action, item), tone: 'success' }),
        (error: Error) => setToast(failure(action, error)),
      )
      .finally(() =>
        setPending((current) => {
          const next = new Map(current)
          next.delete(item.id)
          return next
        }),
      )
  }

  function actionsFor(item: CommunityItem, onUpdate: () => void, updateLabel: string): RowActions {
    return {
      pending: pending.get(item.id),
      onSubscribe: () => run(item, 'subscribe'),
      onDuplicate: () => run(item, 'duplicate'),
      onUpdate,
      updateLabel,
    }
  }

  // A 404 on the list means this slot was never selected; there is nothing
  // to retry, so send the user back to pick one.
  if (list.error instanceof ProfileNotSelectedError) {
    return <Navigate to="/profiles" replace />
  }

  const describe = (item: CommunityItem) => describeItem(item, genres)

  return (
    <div ref={scrollRef} className="tone-community flex w-full flex-col lg:min-h-0 lg:flex-1 lg:overflow-y-auto">
      <div className="sign min-h-[64px] px-4 py-3 lg:min-h-[80px] lg:px-6">
        <h1 className="type-sign m-0 text-[18px] leading-tight lg:text-[25px]">Community</h1>
      </div>
      <section className="mx-auto flex w-full max-w-[1100px] flex-col gap-4 p-4 lg:p-6">
        <Toast toast={toast} />

        {open ? (
          <PublicationPage
            key={open.id}
            profileIndex={profileIndex}
            item={open}
            summary={describe(open)}
            meta={itemMeta(open, now)}
            genres={genres}
            actions={actionsFor(open, () => run(open, 'update'), 'Update')}
            onBack={() => back(open.id)}
          />
        ) : (
          <>
            <Controls filters={filters} onChange={setFilters} />
            <SearchCount filters={filters} shown={items.length} of={ofKind(all, filters.kind).length} />
            <ListState
              isLoading={list.isPending}
              error={list.error as Error | null}
              isEmpty={items.length === 0}
              loadingLabel="Loading Community…"
              errorLabel="Couldn’t load Community."
              onRetry={list.refetch}
              emptyLabel={emptyLabel(filters)}
            >
              {items.map((item) => (
                <CommunityRow
                  key={item.id}
                  item={item}
                  summary={describe(item)}
                  meta={itemMeta(item, now)}
                  buttonID={rowButtonID(item.id)}
                  onOpen={() => openItem(item.id)}
                  actions={actionsFor(item, () => openItem(item.id), 'Update…')}
                />
              ))}
            </ListState>
          </>
        )}
      </section>
    </div>
  )
}

function describeItem(item: CommunityItem, genres: GenreLookups): string {
  return itemSummary(item, item.catalog ? snapshotRecipeLine(item.catalog, genres) : '')
}

const KIND_WORD: Record<CommunityItem['kind'], string> = { catalog: 'catalogs', collection: 'collections' }

/** What an empty list says: that nothing matches the search, or that nobody
 *  has published anything of this kind. */
function emptyLabel(filters: CommunityFilters): string {
  if (filters.q.trim()) return `No ${KIND_WORD[filters.kind]} match this search.`
  return `Nobody has published any ${KIND_WORD[filters.kind]} yet. Publish one of your own from its editor.`
}

/** "3 of 12 catalogs" while a search narrows a kind that has any rows. */
function SearchCount({ filters, shown, of }: { filters: CommunityFilters; shown: number; of: number }) {
  if (!filters.q.trim() || of === 0) return null
  return (
    <p className="type-data text-dim m-0 text-[13.5px]">
      {shown} of {of} {KIND_WORD[filters.kind]}
    </p>
  )
}

/** The kind switch, the search box and the sort. */
function Controls({
  filters,
  onChange,
}: {
  filters: CommunityFilters
  onChange: (filters: CommunityFilters) => void
}) {
  const patch = (next: Partial<CommunityFilters>) => onChange({ ...filters, ...next })
  return (
    <div className="flex flex-wrap items-center gap-3">
      <Segmented<CommunityFilters['kind']>
        ariaLabel="Community kind"
        value={filters.kind}
        onChange={(kind) => patch({ kind })}
        options={[
          { value: 'catalog', label: 'Catalogs' },
          { value: 'collection', label: 'Collections' },
        ]}
      />
      <div className="relative min-w-[200px] flex-1">
        <Icon icon={Search} size={16} className="text-dimmer pointer-events-none absolute top-1/2 left-4 -translate-y-1/2" />
        <input
          type="search"
          value={filters.q}
          onChange={(event) => patch({ q: event.target.value })}
          placeholder="Search names and catalogs"
          aria-label="Search Community"
          className="field h-10 w-full rounded-full pl-10 text-[14px] pointer-coarse:text-[16px]"
        />
      </div>
      <div className="flex items-center gap-2">
        <span className="type-label">Sort</span>
        <Segmented<CommunityFilters['sort']>
          ariaLabel="Sort Community"
          value={filters.sort}
          onChange={(sort) => patch({ sort })}
          options={[
            { value: 'name', label: 'Name' },
            { value: 'newest', label: 'Newest' },
          ]}
        />
      </div>
    </div>
  )
}

/** An action's word in the UI: a subscribe is Add. */
const VERB: Record<CommunityAction, string> = { subscribe: 'add', update: 'update', duplicate: 'duplicate' }

/** The toast for an action that worked. */
function doneText(action: CommunityAction, item: CommunityItem): string {
  const kinds = KIND_WORD[item.kind]
  if (action === 'subscribe') return `Added to your ${kinds}`
  if (action === 'duplicate') return `Duplicated to your ${kinds}`
  return `Updated “${item.title}”`
}

/** The toast for a failed action. A stale failure has already refreshed the
 *  lists: a 409 from Add means this profile already added it, and a 404 means
 *  its publisher unpublished it. */
function failure(action: CommunityAction, error: Error): ToastMessage {
  const status = error instanceof ApiError ? error.status : undefined
  if (action === 'subscribe' && status === 409) return { text: 'Already added', tone: 'success' }
  if (status === 404) return { text: 'Its publisher unpublished it.', tone: 'danger' }
  return { text: `Couldn’t ${VERB[action]} it: ${error.message}`, tone: 'danger' }
}
