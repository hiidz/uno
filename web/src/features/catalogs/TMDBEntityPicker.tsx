import { useMemo, useRef, useState, type KeyboardEvent } from 'react'
import { useQueries, useQuery, useQueryClient } from '@tanstack/react-query'
import { X } from 'lucide-react'
import {
  fetchCollection,
  fetchCompany,
  fetchKeyword,
  fetchNetwork,
  queryKeys,
  searchCollections,
  searchCompanies,
  searchKeywords,
  searchNetworks,
} from '@/api'
import type { CatalogType } from '@/api'
import { FieldNote, Segmented } from '@/components/fields'
import { Icon } from '@/components/Icon'
import { useDebounce } from '@/lib/useDebounce'
import { MAX_ENTITY_IDS } from './catalogForm'
import { parseIdList, serializeIdList, type IdJoin } from './params'
import { companyDetail } from './summary'

/**
 * TMDB entities picked by name from a server search, kept as chips — the
 * production companies, keywords, movie collection and series networks of a
 * recipe.
 *
 * TMDB's API has no "list them all" endpoint for any of them, so unlike every
 * other picker in the editor this one searches the server as you type: the
 * query settles for `SEARCH_DELAY_MS`, then
 * `GET /api/{companies,keywords,collections,networks}/search` runs once it is
 * at least `MIN_QUERY` characters. Company search is scoped to the catalog's
 * type, network search to series, and each of their results names its country
 * and title count.
 *
 * The stored value is one id-list string, comma-joined for "all of them" and
 * pipe-joined for "any of them". An `exclude` list is always comma-joined:
 * TMDB drops a title carrying any of its ids either way, so it has no join to
 * pick. A single-pick kind stores one bare id, and a new pick replaces it. A
 * multi-pick kind holds at most `MAX_ENTITY_IDS`. Chips name their ids through the by-id route,
 * cached for the session; an id TMDB no longer knows still shows as a chip,
 * marked, so it can be removed.
 */

export type EntityKind = 'company' | 'keyword' | 'collection' | 'network'

interface Entity {
  id: number
  name: string
}

/** A search row: an entity plus whatever the kind's search reports beside it. */
interface SearchResult extends Entity {
  origin_country?: string
  title_count?: number
}

interface EntitySource {
  /** Lower case, for labels: "Search production companies". */
  plural: string
  /** Names a chip whose id has no name to show: "Company 420". */
  singular: string
  /** The join a second pick gets before anyone has chosen one. */
  defaultJoin: IdJoin
  /** At most one pick: TMDB takes a single id for this field. */
  single?: true
  search: (query: string, type: CatalogType) => Promise<SearchResult[]>
  searchKey: (query: string, type: CatalogType) => readonly unknown[]
  /** Shown after a search row's name, when the kind reports more than one. */
  rowDetail?: (result: SearchResult, type: CatalogType) => string
  fetchOne: (id: number) => Promise<Entity>
  oneKey: (id: number) => readonly unknown[]
}

const SOURCES: Record<EntityKind, EntitySource> = {
  company: {
    plural: 'production companies',
    singular: 'Company',
    // A title rarely has two named studios, so "all of them" is almost always
    // an empty row.
    defaultJoin: 'or',
    search: searchCompanies,
    searchKey: (query, type) => queryKeys.companySearch(type, query),
    rowDetail: (result, type) =>
      companyDetail(
        { origin_country: result.origin_country ?? '', title_count: result.title_count ?? 0 },
        type,
      ),
    fetchOne: fetchCompany,
    oneKey: queryKeys.company,
  },
  keyword: {
    plural: 'keywords',
    singular: 'Keyword',
    defaultJoin: 'and',
    search: searchKeywords,
    searchKey: (query) => queryKeys.keywordSearch(query),
    fetchOne: fetchKeyword,
    oneKey: queryKeys.keyword,
  },
  collection: {
    plural: 'collections',
    singular: 'Collection',
    defaultJoin: 'and',
    single: true,
    search: searchCollections,
    searchKey: (query) => queryKeys.collectionSearch(query),
    fetchOne: fetchCollection,
    oneKey: queryKeys.collection,
  },
  network: {
    plural: 'networks',
    singular: 'Network',
    // A show rarely airs on two networks, so "all of them" is almost always
    // an empty row.
    defaultJoin: 'or',
    search: searchNetworks,
    searchKey: (query) => queryKeys.networkSearch(query),
    rowDetail: (result) =>
      companyDetail(
        { origin_country: result.origin_country ?? '', title_count: result.title_count ?? 0 },
        'series',
      ),
    fetchOne: fetchNetwork,
    oneKey: queryKeys.network,
  },
}

const SEARCH_DELAY_MS = 300
const MIN_QUERY = 2

export function TMDBEntityPicker({
  kind,
  type,
  value,
  onChange,
  inputId,
  exclude = false,
  hiddenIds,
}: {
  kind: EntityKind
  /** The catalog's type, which company search counts titles for. Network
   *  search always counts series. */
  type: CatalogType
  value: string | undefined
  onChange: (value: string | undefined) => void
  /** For focusing the search box from outside, e.g. on an invalid Save. */
  inputId?: string
  /** The list of ids to leave out rather than to match. */
  exclude?: boolean
  /** Ids search never offers: the section's other list, so one id can't be
   *  both matched and left out. */
  hiddenIds?: number[]
}) {
  const source = SOURCES[kind]
  const plural = exclude ? `${source.plural} to leave out` : source.plural
  const queryClient = useQueryClient()
  const inputRef = useRef<HTMLInputElement>(null)
  const resultsRef = useRef<HTMLUListElement>(null)

  const ids = useMemo(() => [...new Set(parseIdList(value).ids)], [value])

  // Outside an exclude list, one id carries no separator, so it takes the
  // kind's default join.
  const join: IdJoin = exclude
    ? 'and'
    : value?.includes('|')
      ? 'or'
      : value?.includes(',')
        ? 'and'
        : source.defaultJoin

  const full = !source.single && ids.length >= MAX_ENTITY_IDS

  const [query, setQuery] = useState('')
  const settled = useDebounce(query.trim(), SEARCH_DELAY_MS)
  const searchable = settled.length >= MIN_QUERY && !full

  const results = useQuery({
    queryKey: source.searchKey(settled, type),
    queryFn: () => source.search(settled, type),
    enabled: searchable,
  })

  const names = useQueries({
    queries: ids.map((id) => ({
      queryKey: source.oneKey(id),
      queryFn: () => source.fetchOne(id),
      staleTime: Infinity,
    })),
  })

  const matches = useMemo(
    () =>
      (results.data ?? []).filter(
        (entity) => !ids.includes(entity.id) && !hiddenIds?.includes(entity.id),
      ),
    [results.data, ids, hiddenIds],
  )

  function write(nextIDs: number[], nextJoin: IdJoin) {
    onChange(nextIDs.length ? serializeIdList(nextIDs, nextJoin) : undefined)
  }

  function add(entity: SearchResult) {
    if (full) return
    queryClient.setQueryData(source.oneKey(entity.id), { id: entity.id, name: entity.name })
    write(source.single ? [entity.id] : [...ids, entity.id], join)
    setQuery('')
    inputRef.current?.focus()
  }

  function remove(id: number) {
    write(
      ids.filter((existing) => existing !== id),
      join,
    )
  }

  function resultButtons(): HTMLButtonElement[] {
    return Array.from(resultsRef.current?.querySelectorAll('button') ?? [])
  }

  function onInputKeyDown(event: KeyboardEvent<HTMLInputElement>) {
    if (event.key === 'ArrowDown') {
      const first = resultButtons()[0]
      if (first) {
        event.preventDefault()
        first.focus()
      }
    } else if (event.key === 'Escape' && query) {
      // Clears the search rather than reaching the editor's own
      // Escape-to-close.
      event.preventDefault()
      event.stopPropagation()
      setQuery('')
    }
  }

  function onResultsKeyDown(event: KeyboardEvent<HTMLUListElement>) {
    const buttons = resultButtons()
    const index = buttons.indexOf(document.activeElement as HTMLButtonElement)
    if (index === -1) return
    if (event.key === 'ArrowDown') {
      event.preventDefault()
      buttons[Math.min(index + 1, buttons.length - 1)]?.focus()
    } else if (event.key === 'ArrowUp') {
      event.preventDefault()
      if (index === 0) inputRef.current?.focus()
      else buttons[index - 1]?.focus()
    } else if (event.key === 'Escape') {
      event.preventDefault()
      event.stopPropagation()
      inputRef.current?.focus()
    }
  }

  const trimmed = query.trim()
  const searching = trimmed !== settled || results.isFetching
  const failed = !searching && results.isError
  const status = full
    ? `A row can ${exclude ? 'leave out' : 'use'} up to ${MAX_ENTITY_IDS} ${source.plural}. Remove one to add another.`
    : trimmed.length === 0
      ? ''
      : trimmed.length < MIN_QUERY
        ? `Type at least ${MIN_QUERY} letters to search.`
        : searching
          ? 'Searching…'
          : failed
            ? `Couldn't search ${source.plural}. Try again.`
            : matches.length === 0
              ? `No ${source.plural} match that name.`
              : `${matches.length} ${matches.length === 1 ? 'match' : 'matches'}. Press Down to pick one.`

  const showResults = !full && trimmed.length >= MIN_QUERY && trimmed === settled && matches.length > 0

  return (
    // No label of its own: the section head above already names the field.
    <div className="flex w-full flex-col gap-2">
      {ids.length > 0 && (
        <ul aria-label={`Chosen ${plural}`} className="m-0 flex list-none flex-wrap gap-1.5 p-0">
          {ids.map((id, index) => {
            const lookup = names[index]
            const missing = lookup?.isError ?? false
            const label = lookup?.data?.name ?? `${source.singular} ${id}`
            return (
              <li
                key={id}
                className={`inline-flex items-center rounded-full border pl-2 text-[12.5px] ${
                  missing ? 'border-danger text-danger' : 'bg-raised-hi border-dim text-ink'
                }`}
              >
                <span className="py-1 pointer-coarse:py-3">
                  {label}
                  {missing && <span className="text-dimmer"> · not found</span>}
                </span>
                <button
                  type="button"
                  onClick={() => remove(id)}
                  aria-label={`Remove ${label}`}
                  className="text-dimmer hover:text-ink grid h-6 w-6 place-items-center transition-colors pointer-coarse:h-11 pointer-coarse:w-11"
                >
                  <Icon icon={X} size={12} />
                </button>
              </li>
            )
          })}
        </ul>
      )}

      {!source.single && !exclude && ids.length >= 2 && (
        <Segmented
          ariaLabel={`How to combine ${source.plural}`}
          value={join}
          onChange={(next) => write(ids, next)}
          options={[
            { value: 'and', label: 'All of them' },
            { value: 'or', label: 'Any of them' },
          ]}
        />
      )}

      <input
        ref={inputRef}
        id={inputId}
        type="search"
        value={query}
        onChange={(event) => setQuery(event.target.value)}
        onKeyDown={onInputKeyDown}
        placeholder={source.single && ids.length > 0 ? 'Search to replace it…' : 'Search by name…'}
        aria-label={`Search ${plural}`}
        autoComplete="off"
        className="field type-data w-full max-w-[var(--w-entry)] text-[12.5px] pointer-coarse:text-[16px]"
      />

      <div role="status">{status && <FieldNote tone={failed ? 'danger' : 'dim'}>{status}</FieldNote>}</div>

      {showResults && (
        <ul
          ref={resultsRef}
          aria-label={`Matching ${plural}`}
          onKeyDown={onResultsKeyDown}
          className="border-line m-0 flex max-h-[50svh] w-full max-w-[var(--w-entry)] list-none flex-col overflow-y-auto overscroll-contain rounded-xl border p-1 lg:max-h-[13rem]"
        >
          {matches.map((entity) => (
            <li key={entity.id}>
              <button
                type="button"
                onClick={() => add(entity)}
                aria-label={`Add ${entity.name}`}
                className="text-dim hover:bg-raised-hi hover:text-ink focus-visible:bg-raised-hi focus-visible:text-ink w-full rounded-lg px-2 py-1.5 text-left text-[12.5px] transition-colors pointer-coarse:py-3"
              >
                {entity.name}
                {source.rowDetail && (
                  <span className="text-dimmer"> · {source.rowDetail(entity, type)}</span>
                )}
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
