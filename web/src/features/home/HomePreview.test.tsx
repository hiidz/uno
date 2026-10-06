// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { catalog, collection, folder } from '@/test/fixtures'
import { HomePreview } from './HomePreview'

const selection = vi.hoisted(() => ({ current: null as unknown }))

vi.mock('@/api/client', () => ({ apiFetch: vi.fn(() => new Promise(() => {})) }))
vi.mock('./useHomeSelection', async () => {
  const { buildHomePreview } = await import('./preview')
  return {
    useHomeSelection: () => selection.current,
    useHomePreview: () => buildHomePreview(selection.current as Parameters<typeof buildHomePreview>[0]),
  }
})
vi.mock('./useCatalogTiles', () => ({ useCatalogTiles: () => new Map() }))
vi.mock('@/features/preview/useRecipesTiles', () => ({ useRecipesTiles: () => new Map() }))

let back: ReturnType<typeof vi.spyOn>

beforeEach(() => {
  window.history.replaceState({ idx: 1 }, '')
  // What the browser does on Back: the entry under the current one comes back.
  back = vi.spyOn(window.history, 'back').mockImplementation(() => popTo({ idx: 1 }))
  const night = collection({
    id: 'k1',
    title: 'Night shift',
    folders: [folder({ id: 'f1', title: 'Slashers', refs: [{ catalog_id: 'c1' } as never] })],
  })
  const noir = catalog({ id: 'c1', name: 'Noir' })
  selection.current = {
    ready: true,
    isLoading: false,
    error: null,
    retry: () => {},
    pendingCount: 0,
    rows: [
      { kind: 'catalog', id: noir.id, showInHome: false },
      { kind: 'collection', id: night.id, pinToTop: false },
    ],
    catalogById: new Map([[noir.id, noir]]),
    collectionById: new Map([[night.id, night]]),
    waitingForPush: new Set(),
    genres: { movie: new Map(), tv: new Map() },
  }
})

afterEach(() => {
  vi.restoreAllMocks()
})

function popTo(state: unknown) {
  act(() => {
    window.history.replaceState(state, '')
    window.dispatchEvent(new PopStateEvent('popstate', { state }))
  })
}

function openFolder() {
  render(
    <QueryClientProvider client={new QueryClient()}>
      <HomePreview />
    </QueryClientProvider>,
  )
  fireEvent.click(screen.getByRole('button', { name: 'Open Slashers' }))
  expect(window.history.state).toEqual({ idx: 1, unoFolder: true })
}

const folderOpen = () => screen.queryByRole('button', { name: 'Open Slashers' }) === null

describe('the folder page', () => {
  it('stays open when Back steps off an entry pushed over its own', () => {
    openFolder()
    popTo({ idx: 1, unoFolder: true })
    expect(folderOpen()).toBe(true)

    popTo({ idx: 1 })
    expect(folderOpen()).toBe(false)
  })

  it('closes on Escape, by stepping back off its entry', () => {
    openFolder()
    fireEvent.keyDown(window, { key: 'Escape' })
    expect(back).toHaveBeenCalledTimes(1)
    expect(folderOpen()).toBe(false)
  })

  it('leaves an Escape something else already handled', () => {
    openFolder()
    const handled = new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true })
    handled.preventDefault()
    window.dispatchEvent(handled)
    fireEvent.keyDown(window, { key: 'Enter' })
    expect(folderOpen()).toBe(true)
  })
})
