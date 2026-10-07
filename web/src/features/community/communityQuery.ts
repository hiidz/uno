import type { CommunityItem, CommunityPage, CommunityQuery } from '@/api'
import { COLLECTION_KIND, kindSticker, type SharingSticker } from '@/features/sharing/sharingState'
import { describeFolders } from '@/features/library/collection'

/** What Community shows: the search words, which kind, and the order. The
 *  server searches, filters and sorts, a page at a time. */
export type CommunityFilters = CommunityQuery

export const DEFAULT_FILTERS: CommunityFilters = { q: '', kind: 'catalog', sort: 'name' }

/** A publication whose page Community opens on, and its kind, which the list
 *  shows once the page closes. */
export interface OpenPublication {
  id: string
  kind: CommunityItem['kind']
}

/** Where Community starts: on `open`'s page, over a list of its kind, or on
 *  the list as it defaults. */
export function startingView(open: OpenPublication | null): { filters: CommunityFilters; openID: string | null } {
  if (!open) return { filters: DEFAULT_FILTERS, openID: null }
  return { filters: { ...DEFAULT_FILTERS, kind: open.kind }, openID: open.id }
}

/** Every row of the pages read so far, in order. */
export function rowsOf(pages: CommunityPage[] | undefined): CommunityItem[] {
  const rows: CommunityItem[] = []
  for (const page of pages ?? []) rows.push(...page.items)
  return rows
}

/** What the open publication's detail query holds. */
export interface OpenDetail {
  data: CommunityItem | undefined
  isError: boolean
}

/** The publication whose page is open: its row while one is listed, else its
 *  detail, for a page opened on a publication not among the rows read. A
 *  detail that failed to refresh counts for nothing, so a page whose
 *  publication has gone closes as it does when the list stops holding it. */
export function openRow(rows: CommunityItem[], openID: string | null, detail: OpenDetail): CommunityItem | null {
  if (openID === null) return null
  return listedRow(rows, openID) ?? detailRow(detail)
}

function listedRow(rows: CommunityItem[], id: string): CommunityItem | null {
  for (const row of rows) if (row.id === id) return row
  return null
}

function detailRow(detail: OpenDetail): CommunityItem | null {
  if (detail.isError) return null
  return detail.data ?? null
}

/** A row's kind sticker: a catalog's type (Movies, Series), or Collection. */
export function itemKind(item: CommunityItem): SharingSticker {
  if (item.catalog) return kindSticker(item.catalog.type)
  return COLLECTION_KIND
}

/** A row's second line: a catalog's recipe, or a collection's folders worded as
 *  the Library rail and Home word them. The kind sticker says a catalog's
 *  type. `recipe` is the catalog's recipe line, when it has one. */
export function itemSummary(item: CommunityItem, recipe: string): string {
  if (item.kind === 'catalog') return recipe || 'No filters'
  return describeFolders(folderTitles(item))
}

/** A row's third line, fact by fact so a narrow column breaks between them:
 *  how many have added it, when it was published, and when it was last
 *  updated, if it was — "Added by 3", "Published 3 weeks ago", "Updated 2
 *  days ago". */
export function itemMeta(item: CommunityItem, now: Date): string[] {
  return [addedBy(item), `Published ${relativeDay(item.published_at, now)}`, updatedAgo(item, now)].filter(Boolean)
}

function addedBy(item: CommunityItem): string {
  return item.subscriber_count > 0 ? `Added by ${item.subscriber_count}` : ''
}

function updatedAgo(item: CommunityItem, now: Date): string {
  return item.updated_at !== item.published_at ? `Updated ${relativeDay(item.updated_at, now)}` : ''
}

const DAY_MS = 24 * 60 * 60 * 1000
const RELATIVE = new Intl.RelativeTimeFormat('en-GB', { numeric: 'auto' })

/** How long ago `iso` was, in whole days, weeks, months or years: "today",
 *  "yesterday", "3 days ago", "2 weeks ago". */
export function relativeDay(iso: string, now: Date): string {
  const days = Math.max(0, Math.floor((startOfDay(now) - startOfDay(new Date(iso))) / DAY_MS))
  if (days < 7) return RELATIVE.format(-days, 'day')
  if (days < 30) return RELATIVE.format(-Math.floor(days / 7), 'week')
  if (days < 365) return RELATIVE.format(-Math.min(11, Math.floor(days / 30)), 'month')
  return RELATIVE.format(-Math.floor(days / 365), 'year')
}

function startOfDay(date: Date): number {
  return new Date(date.getFullYear(), date.getMonth(), date.getDate()).getTime()
}

/** A listed collection's folder titles in order; none for a catalog. */
function folderTitles(item: CommunityItem): string[] {
  const titles: string[] = []
  for (const folder of item.folders ?? []) titles.push(folder.title)
  return titles
}

/** What the list's own error shows: a failed first page. A failed next page
 *  leaves the rows read so far on show, under Show more's own line. */
export function listError(pages: { error: Error | null; isFetchNextPageError: boolean }): Error | null {
  if (pages.isFetchNextPageError) return null
  return pages.error
}

/** The most distinct words a search may hold, the server's `maxSearchWords`. */
export const MAX_SEARCH_WORDS = 8

/** Why the server would refuse the search `q`, or `''` when it takes it: the
 *  server counts each word once, whatever its case. */
export function searchProblem(q: string): string {
  const words = new Set(q.toLowerCase().split(/\s+/).filter(Boolean))
  if (words.size <= MAX_SEARCH_WORDS) return ''
  return `Search with ${MAX_SEARCH_WORDS} words at most.`
}
