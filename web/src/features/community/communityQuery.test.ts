import { describe, expect, it } from 'vitest'
import { communityFolder, communityItem } from '@/test/fixtures'
import {
  DEFAULT_FILTERS,
  itemKind,
  itemMeta,
  itemSummary,
  openRow,
  relativeDay,
  rowsOf,
  searchProblem,
  startingView,
} from './communityQuery'

describe('where Community starts', () => {
  it('starts on the list, or on a publication’s page over a list of its kind', () => {
    expect(startingView(null)).toEqual({ filters: DEFAULT_FILTERS, openID: null })
    expect(startingView({ id: 'n', kind: 'collection' })).toEqual({
      filters: { ...DEFAULT_FILTERS, kind: 'collection' },
      openID: 'n',
    })
  })
  it('opens the listed row, else the detail of one not read yet, never a failed one', () => {
    const row = communityItem({ id: 'n' })
    const detail = communityItem({ id: 'far', title: 'Far down' })
    const loaded = { data: detail, isError: false }
    expect(openRow([row], 'n', loaded)).toBe(row)
    expect(openRow([row], 'far', loaded)).toBe(detail)
    expect(openRow([row], 'far', { data: detail, isError: true })).toBeNull()
    expect(openRow([row], 'far', { data: undefined, isError: false })).toBeNull()
    expect(openRow([row], null, loaded)).toBeNull()
  })
})

describe('searchProblem', () => {
  it('takes eight distinct words, counting a word once whatever its case', () => {
    expect(searchProblem('')).toBe('')
    expect(searchProblem('a b c d e f g h')).toBe('')
    expect(searchProblem('a A b B c C d D e E f F g G h H')).toBe('')
    expect(searchProblem('best horror movies of the 80s and 90s too')).toBe('Search with 8 words at most.')
  })
})

describe('rowsOf', () => {
  it('reads every page’s rows in order, none before the first arrives', () => {
    const [a, b, c] = ['a', 'b', 'c'].map((id) => communityItem({ id }))
    expect(rowsOf(undefined)).toEqual([])
    expect(rowsOf([{ items: [a, b], next_cursor: 'x' }, { items: [c], next_cursor: null }])).toEqual([a, b, c])
  })
})

describe('row words', () => {
  it('summarizes a catalog by its recipe, and a collection by its folders as Home words them', () => {
    expect(itemSummary(communityItem(), 'Most popular · Horror')).toBe('Most popular · Horror')
    expect(itemSummary(communityItem(), '')).toBe('No filters')
    const collection = (titles: string[] | null) =>
      itemSummary(communityItem({ kind: 'collection', catalog: null, folders: titles && titles.map((t) => communityFolder(t)) }), '')
    expect(collection(['Action', 'Drama', 'Comedy'])).toBe('3 folders · Action, Drama, Comedy')
    expect(collection(['Action'])).toBe('1 folder · Action')
    expect(collection([])).toBe('0 folders')
    expect(collection(null)).toBe('0 folders')
  })
  it('names a catalog’s kind by its type, and a collection as a collection', () => {
    expect(itemKind(communityItem())).toEqual({ label: 'Movies', tone: { hue: 'catalog', fill: false } })
    expect(itemKind(communityItem({ kind: 'collection', catalog: null }))).toEqual({ label: 'Collection', tone: { hue: 'collection', fill: false } })
  })
  it('says how many took it, when it was published and when it last changed', () => {
    const now = new Date(2026, 8, 29, 12)
    const meta = (subscriber_count: number, updated_at: string) =>
      itemMeta(communityItem({ subscriber_count, published_at: '2026-09-08T10:00:00Z', updated_at }), now).join(' · ')
    expect(meta(0, '2026-09-08T10:00:00Z')).toBe('Published 3 weeks ago')
    expect(meta(0, '2026-09-27T10:00:00Z')).toBe('Published 3 weeks ago · Updated 2 days ago')
    expect(meta(3, '2026-09-08T10:00:00Z')).toBe('Added by 3 · Published 3 weeks ago')
    expect(meta(3, '2026-09-27T10:00:00Z')).toBe('Added by 3 · Published 3 weeks ago · Updated 2 days ago')
  })
  it('counts days, weeks, months and years', () => {
    const now = new Date(2026, 8, 29, 12)
    const ago = (days: number) => relativeDay(new Date(2026, 8, 29 - days, 9).toISOString(), now)
    expect(ago(0)).toBe('today')
    expect(ago(-1)).toBe('today')
    expect(ago(6)).toBe('6 days ago')
    expect(ago(14)).toBe('2 weeks ago')
    expect(ago(65)).toBe('2 months ago')
    expect(ago(362)).toBe('11 months ago')
    expect(ago(800)).toBe('2 years ago')
  })
})
