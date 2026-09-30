import type { CommunityItem } from '@/api'
import { typeLabel } from '@/features/library/recipe'
import { capitalize } from '@/lib/capitalize'
import { pluralCount } from '@/lib/plural'

/** What Community shows: the search words, which kind, and the order. */
export interface CommunityFilters {
  q: string
  kind: CommunityItem['kind']
  sort: 'name' | 'newest'
}

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

/** The row of `items` whose page is open, while it is listed. */
export function openItemIn(items: CommunityItem[], openID: string | null): CommunityItem | null {
  return items.find((item) => item.id === openID) ?? null
}

/** The rows of one kind; that kind's rows where every word of the search is
 *  in the title or a catalog's name, whatever the case; in order by name or
 *  newest first. */
export function visibleItems(items: CommunityItem[], filters: CommunityFilters): CommunityItem[] {
  const q = filters.q.trim().toLowerCase()
  return ofKind(items, filters.kind)
    .filter((item) => matches(item, q))
    .sort(filters.sort === 'name' ? byName : byNewest)
}

/** The rows of `kind`: what the search and its "N of M" count run over. */
export function ofKind(items: CommunityItem[], kind: CommunityItem['kind']): CommunityItem[] {
  return items.filter((item) => item.kind === kind)
}

/** Whether each word of `q`, which is trimmed and lower case, is in `item`'s
 *  title or one of its catalogs' names — not necessarily the same one, so
 *  "horror slashers" finds "Horror Night" holding "Slashers". An empty `q`
 *  matches every row. */
function matches(item: CommunityItem, q: string): boolean {
  const names = [item.title, ...(item.catalog_names ?? [])].map((name) => name.toLowerCase())
  return q.split(/\s+/).every((word) => names.some((name) => name.includes(word)))
}

function byName(a: CommunityItem, b: CommunityItem): number {
  return a.title.localeCompare(b.title)
}

function byNewest(a: CommunityItem, b: CommunityItem): number {
  return b.published_at.localeCompare(a.published_at)
}

/** A row's second line: a catalog's type and recipe, or a collection's size.
 *  `recipe` is the catalog's recipe line, when it has one. */
export function itemSummary(item: CommunityItem, recipe: string): string {
  if (item.kind === 'catalog' && item.catalog) {
    return [typeLabel(item.catalog.type), recipe].filter(Boolean).join(' · ')
  }
  return [pluralCount(item.folder_count, 'folder'), pluralCount(item.catalog_count, 'catalog')].join(' · ')
}

/** A row's third line: how many have taken it, then when it was shared or
 *  last updated — "Taken by 3 · updated 2 days ago". */
export function itemMeta(item: CommunityItem, now: Date): string {
  const updated = item.updated_at !== item.published_at
  const when = `${updated ? 'updated' : 'shared'} ${relativeDay(updated ? item.updated_at : item.published_at, now)}`
  return item.subscriber_count > 0 ? `Taken by ${item.subscriber_count} · ${when}` : capitalize(when)
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
