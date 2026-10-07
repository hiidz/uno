// @vitest-environment jsdom
import { render, screen, within } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import type { Catalog, Collection, SubscriptionState } from '@/api'
import { catalog, collection, folder } from '@/test/fixtures'
import { HomePane } from './HomePane'

const selection = vi.hoisted(() => ({ current: null as unknown }))

// The pane reads the pending home screen from the context; these tests hand
// it one directly and let the real preview builder order the rows.
vi.mock('./useHomeSelection', async () => {
  const { buildHomePreview } = await import('./preview')
  return {
    useHomeSelection: () => selection.current,
    useHomeEdits: () => selection.current,
    useHomePreview: () => buildHomePreview(selection.current as Parameters<typeof buildHomePreview>[0]),
  }
})
vi.mock('./useCatalogTiles', () => ({ useCatalogTiles: () => new Map() }))

const live = { id: 'p', changed_since_publish: false }
const added: SubscriptionState = { publication_id: 'pub', update_available: false }

function mount(options: {
  catalogs: { catalog: Catalog; showInHome: boolean }[]
  collections?: Collection[]
  waiting?: string[]
  pinned?: string[]
}) {
  const collections = options.collections ?? []
  selection.current = {
    ready: true,
    isLoading: false,
    error: null,
    retry: () => {},
    rows: [
      ...options.catalogs.map((c) => ({ kind: 'catalog' as const, id: c.catalog.id, showInHome: c.showInHome })),
      ...collections.map((c) => ({ kind: 'collection' as const, id: c.id, pinToTop: options.pinned?.includes(c.id) ?? false })),
    ],
    catalogById: new Map(options.catalogs.map((c) => [c.catalog.id, c.catalog])),
    collectionById: new Map(collections.map((c) => [c.id, c])),
    waitingForPush: new Set(options.waiting),
    genres: { movie: new Map(), tv: new Map() },
  } satisfies Record<string, unknown>
  render(<HomePane view="list" onViewChange={() => {}} onShowLibrary={() => {}} />)
}

/** The stickers a Home row carries after its name, in order. */
function flagsOf(name: string): string[] {
  const row = screen.getByText(name).closest('li')!
  return [...row.querySelectorAll('.stk')].map((sticker) => sticker.textContent ?? '')
}

describe('HomePane', () => {
  it('carries its kind, then To push, and nothing about Community', () => {
    const edited = catalog({ id: 'c1', name: 'Action', publication: { ...live, changed_since_publish: true } })
    const following = catalog({ id: 'c2', name: 'Noir', type: 'series', subscription: { ...added, update_available: true } })
    const plain = catalog({ id: 'c3', name: 'Quiet' })
    mount({
      catalogs: [
        { catalog: edited, showInHome: true },
        { catalog: following, showInHome: true },
        { catalog: plain, showInHome: true },
      ],
      waiting: ['c1', 'c2'],
    })
    expect(flagsOf('Action')).toEqual(['Movies', 'To push'])
    expect(flagsOf('Noir')).toEqual(['Series', 'To push'])
    expect(flagsOf('Quiet')).toEqual(['Movies'])
  })

  it('flags a collection To push when something it holds waits for a push', () => {
    const night = collection({
      id: 'k1',
      title: 'Night shift',
      folders: [
        folder({ id: 'f1', title: 'Slashers', cover_emoji: '🔪' }),
        folder({ id: 'f2', title: '', cover_emoji: '', cover_image_url: 'https://example.test/cover.png' }),
        folder({ id: 'f3', title: 'Plain', cover_emoji: '' }),
      ],
    })
    const day = collection({ id: 'k2', title: 'Day shift' })
    mount({ catalogs: [], collections: [night, day], waiting: ['k1'] })
    expect(flagsOf('Night shift')).toEqual(['Collection', 'To push'])
    expect(flagsOf('Day shift')).toEqual(['Collection'])
  })

  it('flags a row that is only on Discover too, beside its kind', () => {
    const tray = catalog({ id: 'c4', name: 'Discover only', publication: live })
    mount({ catalogs: [{ catalog: tray, showInHome: false }], waiting: ['c4'] })
    const section = screen.getByRole('heading', { name: 'Not on home' }).closest('section')!
    expect(within(section).getByText('Discover only')).toBeInTheDocument()
    expect(flagsOf('Discover only')).toEqual(['Movies', 'To push'])
  })

  it('heads the groups Pinned and Rows, the rows mixing catalogs and collections', () => {
    const pinned = collection({ id: 'k1', title: 'Up top' })
    const rest = collection({ id: 'k2', title: 'Down below' })
    mount({ catalogs: [{ catalog: catalog({ id: 'c1', name: 'Action' }), showInHome: true }], collections: [pinned, rest], pinned: ['k1'] })
    const headings = screen.getAllByRole('heading', { level: 2 }).map((h) => h.textContent)
    expect(headings).toEqual(['Pinned', 'Rows'])
    expect(screen.getByRole('button', { name: 'Move Action up, already first of the rows' })).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Move Down below down, already last of the rows' })).toBeDisabled()
  })

  it('shows a collection row’s folders as tiles, the first six and then how many more', () => {
    const night = collection({
      id: 'k1',
      title: 'Night shift',
      folders: [
        folder({ id: 'f1', title: 'Slashers', cover_emoji: '🔪' }),
        folder({ id: 'f2', title: '', cover_image_url: 'https://example.test/cover.png' }),
      ],
    })
    const titles = ['One', 'Two', 'Three', 'Four', 'Five', 'Six', 'Seven', 'Eight']
    const big = collection({ id: 'k2', title: 'Everything', folders: titles.map((title) => folder({ id: title, title })) })
    const empty = collection({ id: 'k3', title: 'Empty', folders: [] })
    mount({ catalogs: [], collections: [night, big, empty] })
    const rowOf = (name: string) => within(screen.getByText(name).closest('li')!)
    const tiles = (name: string) =>
      within(rowOf(name).getByRole('list', { name: 'Folders' }))
        .getAllByRole('listitem')
        .map((tile) => tile.textContent)
    expect(tiles('Night shift')).toEqual(['🔪Slashers', 'Untitled folder'])
    expect(tiles('Everything')).toEqual(['One', 'Two', 'Three', 'Four', 'Five', 'Six', '+2 more'])
    // A folder with no cover image or emoji has no face square, just its title.
    const plain = within(rowOf('Everything').getByRole('list', { name: 'Folders' })).getAllByRole('listitem')[0]
    expect(plain.querySelector('[aria-hidden="true"]')).toBeNull()
    const faced = within(rowOf('Night shift').getByRole('list', { name: 'Folders' })).getAllByRole('listitem')
    expect(faced.map((tile) => tile.querySelector('[aria-hidden="true"]') !== null)).toEqual([true, true])
    expect(rowOf('Empty').queryByRole('list', { name: 'Folders' })).toBeNull()
    expect(rowOf('Empty').getByText('0 folders')).toBeInTheDocument()
  })

  it('says where rows come from while the home screen is empty', () => {
    mount({ catalogs: [] })
    expect(screen.getByText('Nothing on your home screen yet. Add rows from the Library with +.')).toBeInTheDocument()
  })
})

