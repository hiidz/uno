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

const live = { id: 'p', status: 'live' as const, changed_since_publish: false }
const added: SubscriptionState = { publication_id: 'pub', update_available: false, unpublished: false }

function mount(options: {
  catalogs: { catalog: Catalog; showInHome: boolean }[]
  collections?: Collection[]
  waiting?: string[]
  pinned?: string[]
  detached?: string[]
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
    isDetached: (id: string) => options.detached?.includes(id) ?? false,
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
  it('carries every flag on a row: its kind, its Community sticker, then To push', () => {
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
    expect(flagsOf('Action')).toEqual(['Movies', 'To publish', 'To push'])
    expect(flagsOf('Noir')).toEqual(['Series', 'Update available', 'To push'])
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
    const day = collection({ id: 'k2', title: 'Day shift', subscription: { ...added, unpublished: true } })
    mount({ catalogs: [], collections: [night, day], waiting: ['k1'] })
    expect(flagsOf('Night shift')).toEqual(['Collection', 'To push'])
    expect(flagsOf('Day shift')).toEqual(['Collection', 'From Community', 'Unpublished'])
  })

  it('flags a row that is only on Discover too, beside its kind', () => {
    const tray = catalog({ id: 'c4', name: 'Discover only', publication: live })
    mount({ catalogs: [{ catalog: tray, showInHome: false }], waiting: ['c4'] })
    const section = screen.getByRole('heading', { name: 'Not on home' }).closest('section')!
    expect(within(section).getByText('Discover only')).toBeInTheDocument()
    expect(flagsOf('Discover only')).toEqual(['Movies', 'Published', 'To push'])
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

  it('flags a row whose catalog was deleted, on the list and in the tray', () => {
    const gone = catalog({ id: 'c5', name: 'Gone' })
    const hidden = catalog({ id: 'c6', name: 'Hidden gone' })
    mount({
      catalogs: [
        { catalog: gone, showInHome: true },
        { catalog: hidden, showInHome: false },
      ],
      detached: ['c5', 'c6'],
    })
    expect(flagsOf('Gone')).toEqual(['Movies', 'Deleted'])
    expect(flagsOf('Hidden gone')).toEqual(['Movies', 'Deleted'])
  })

  it('says where rows come from while the home screen is empty', () => {
    mount({ catalogs: [] })
    expect(screen.getByText('Nothing on your home screen yet. Add rows from the Library with +.')).toBeInTheDocument()
  })
})

