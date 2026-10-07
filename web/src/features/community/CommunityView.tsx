import { useCallback, useState } from 'react'
import { Navigate } from 'react-router-dom'
import { Search } from 'lucide-react'
import { ApiError, ProfileNotSelectedError, type CommunityItem } from '@/api'
import { Segmented } from '@/components/fields'
import { Icon } from '@/components/Icon'
import { ListState } from '@/components/ListState'
import { Toast } from '@/components/Toast'
import { useToast, type ToastMessage } from '@/components/useToast'
import { useGenreLookups, type GenreLookups } from '@/features/library/useLibrary'
import { useDebounce } from '@/lib/useDebounce'
import { snapshotRecipeLine } from '@/features/sharing/snapshot'
import {
  itemMeta,
  itemSummary,
  listError,
  openRow,
  rowsOf,
  searchProblem,
  startingView,
  type CommunityFilters,
  type OpenPublication,
} from './communityQuery'
import { CommunityRow, type RowActions } from './CommunityRow'
import { CommunitySign } from './CommunitySign'
import { PublicationPage } from './PublicationPage'
import { focusRow, rowButtonID, useScrollMemory } from './scroll'
import { SearchNote } from './SearchNote'
import { ShowMore } from './ShowMore'
import { useCommunityList, useOpenPublication } from './useCommunity'
import { useCommunityMutations, type CommunityAction } from './useCommunityMutations'

/** How long the search box holds still before its words reach the server. */
const SEARCH_SETTLE_MS = 300

/** The longest search the server takes, its `maxSearchLen`. */
const SEARCH_MAX_LENGTH = 200

/**
 * The Community tab: what other profiles publish, a page at a time, searched,
 * filtered and sorted by the server; Show more reads the next page. A row opens its publication's page in
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
  const [start] = useState(() => startingView(initialOpen))
  const [filters, setFilters] = useState<CommunityFilters>(start.filters)
  const [openID, setOpenID] = useState<string | null>(start.openID)
  const q = useDebounce(filters.q.trim(), SEARCH_SETTLE_MS)
  const problem = searchProblem(q)
  const list = useCommunityList(profileIndex, { ...filters, q }, problem === '')
  const openDetail = useOpenPublication(profileIndex, openID)

  const [toast, setToast] = useToast()
  // Keyed by publication id. Actions on different rows can overlap.
  const [pending, setPending] = useState<ReadonlyMap<string, CommunityAction>>(new Map())

  const items = rowsOf(list.data?.pages)
  const open = openRow(items, openID, openDetail)
  const now = new Date()

  const { scrollRef, remember, restore } = useScrollMemory()

  const back = useCallback(
    (id: string) => {
      setOpenID(null)
      restore(() => focusRow(id))
    },
    [restore],
  )

  function closePage() {
    if (open) back(open.id)
  }

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

  return (
    <div ref={scrollRef} className="tone-community flex w-full flex-col lg:min-h-0 lg:flex-1 lg:overflow-y-auto">
      <CommunitySign open={open} onBack={closePage} />
      <section className="community-body mx-auto flex w-full flex-col gap-4 p-4 lg:p-6 [&_.ed]:pt-0">
        <Toast toast={toast} />

        {open ? (
          <PublicationPage
            key={open.id}
            profileIndex={profileIndex}
            item={open}
            meta={itemMeta(open, now)}
            genres={genres}
            actions={actionsFor(open, () => run(open, 'update'), 'Update')}
          />
        ) : (
          <>
            <Controls filters={filters} onChange={setFilters} />
            <SearchNote problem={problem} />
            <ListState
              isLoading={list.isPending}
              error={listError(list)}
              isEmpty={items.length === 0}
              loadingLabel="Loading Community…"
              errorLabel="Couldn’t load Community."
              onRetry={list.refetch}
              emptyLabel={emptyLabel(filters)}
            >
              <div className="flex flex-col gap-1">
                {items.map((item) => (
                  <CommunityRow
                    key={item.id}
                    item={item}
                    summary={describeItem(item, genres)}
                    meta={itemMeta(item, now)}
                    showKind={filters.kind !== 'collection'}
                    buttonID={rowButtonID(item.id)}
                    onOpen={() => openItem(item.id)}
                    actions={actionsFor(item, () => openItem(item.id), 'Update…')}
                  />
                ))}
              </div>
              <ShowMore pages={list} />
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
          placeholder="Search"
          aria-label="Search Community"
          maxLength={SEARCH_MAX_LENGTH}
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
