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
}) {
  const collections = options.collections ?? []
  selection.current = {
    ready: true,
    isLoading: false,
    error: null,
    retry: () => {},
    catalogs: options.catalogs.map((c) => ({ id: c.catalog.id, showInHome: c.showInHome })),
    collections: collections.map((c) => ({ id: c.id, pinToTop: false })),
    catalogById: new Map(options.catalogs.map((c) => [c.catalog.id, c.catalog])),
    collectionById: new Map(collections.map((c) => [c.id, c])),
    waitingForPush: new Set(options.waiting),
    isDetached: () => false,
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
  it('carries every flag on a row: its kind, its Community sticker, then Push to Nuvio', () => {
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
    expect(flagsOf('Action')).toEqual(['Movies', 'Publish changes', 'Push to Nuvio'])
    expect(flagsOf('Noir')).toEqual(['Series', 'Update available', 'Push to Nuvio'])
    expect(flagsOf('Quiet')).toEqual(['Movies'])
  })

  it('flags a collection Push to Nuvio when something it holds waits for a push', () => {
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
    expect(flagsOf('Night shift')).toEqual(['Collection', 'Push to Nuvio'])
    expect(flagsOf('Day shift')).toEqual(['Collection', 'From Community', 'Unpublished'])
  })

  it('flags a row that is only on Discover too', () => {
    const tray = catalog({ id: 'c4', name: 'Discover only', publication: live })
    mount({ catalogs: [{ catalog: tray, showInHome: false }], waiting: ['c4'] })
    const section = screen.getByRole('heading', { name: 'Not on home' }).closest('section')!
    expect(within(section).getByText('Discover only')).toBeInTheDocument()
    expect(flagsOf('Discover only')).toEqual(['Published', 'Push to Nuvio'])
  })
})

