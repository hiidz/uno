import { describe, expect, it } from 'vitest'
import { communityItem } from '@/test/fixtures'
import {
  DEFAULT_FILTERS,
  itemKind,
  itemMeta,
  itemSummary,
  ofKind,
  openItemIn,
  relativeDay,
  startingView,
  visibleItems,
} from './communityQuery'

describe('where Community starts', () => {
  it('starts on the list, or on a publication’s page over a list of its kind', () => {
    expect(startingView(null)).toEqual({ filters: DEFAULT_FILTERS, openID: null })
    expect(startingView({ id: 'n', kind: 'collection' })).toEqual({
      filters: { ...DEFAULT_FILTERS, kind: 'collection' },
      openID: 'n',
    })
  })

  it('finds the open row while it is listed', () => {
    const row = communityItem({ id: 'n' })
    expect(openItemIn([row], 'n')).toBe(row)
    expect(openItemIn([row], 'gone')).toBeNull()
    expect(openItemIn([row], null)).toBeNull()
  })
})

describe('visibleItems', () => {
  const zebra = communityItem({ id: 'z', title: 'Zebra', published_at: '2026-09-01T10:00:00Z' })
  const apple = communityItem({ id: 'a', title: 'apple', published_at: '2026-09-20T10:00:00Z' })
  const night = communityItem({
    id: 'n',
    kind: 'collection',
    title: 'Movie Night',
    catalog: null,
    catalog_names: ['Ghost Stories', 'Slashers'],
    folder_titles: ['Ghosts', 'Slashers'],
  })
  const items = [zebra, night, apple]
  const ids = (list: ReturnType<typeof visibleItems>) => list.map((item) => item.id)

  it('shows one kind, by name', () => {
    expect(ids(visibleItems(items, DEFAULT_FILTERS))).toEqual(['a', 'z'])
    expect(ids(visibleItems(items, { ...DEFAULT_FILTERS, kind: 'collection' }))).toEqual(['n'])
    expect(ids(ofKind(items, 'catalog'))).toEqual(['z', 'a'])
  })

  it('sorts newest first', () => {
    expect(ids(visibleItems(items, { ...DEFAULT_FILTERS, sort: 'newest' }))).toEqual(['a', 'z'])
  })

  it('searches titles and catalog names, whatever the case', () => {
    expect(ids(visibleItems(items, { ...DEFAULT_FILTERS, q: ' ZEB ' }))).toEqual(['z'])
    expect(ids(visibleItems(items, { q: 'ghost', kind: 'collection', sort: 'name' }))).toEqual(['n'])
    expect(ids(visibleItems(items, { q: 'nothing', kind: 'collection', sort: 'name' }))).toEqual([])
  })

  it('finds each word of a search in any name, in any order', () => {
    const collections = (q: string) => ids(visibleItems(items, { q, kind: 'collection', sort: 'name' }))
    expect(collections('movie slashers')).toEqual(['n'])
    expect(collections('stories  NIGHT')).toEqual(['n'])
    expect(collections('movie zombies')).toEqual([])
  })

  it('reads a row without catalog names by its title alone', () => {
    const bare = communityItem({ id: 'b', title: 'Bare', catalog_names: null })
    expect(ids(visibleItems([bare], { ...DEFAULT_FILTERS, q: 'bare' }))).toEqual(['b'])
  })
})

describe('row words', () => {
  it('summarizes a catalog by its recipe, and a collection by its folders as Home words them', () => {
    expect(itemSummary(communityItem(), 'Most popular · Horror')).toBe('Most popular · Horror')
    expect(itemSummary(communityItem(), '')).toBe('No filters')
    const collection = (folder_titles: string[] | null) =>
      itemSummary(communityItem({ kind: 'collection', catalog: null, folder_titles }), '')
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
      itemMeta(communityItem({ subscriber_count, published_at: '2026-09-08T10:00:00Z', updated_at }), now)
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
