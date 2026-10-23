// @vitest-environment jsdom
import { act, fireEvent, render, screen, within } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { WithheldAddress } from '@/api'
import { collection, folder } from '@/test/fixtures'
import { folderPicture, PublicationLook, withheldLookup } from './PublicationLook'

const GIF = 'https://i.postimg.cc/Xq4mP2Lw/marvel-studios-focus.gif'
const COVER = 'https://images.weserv.nl/?url=image.tmdb.org/t/p/w500/cover.png'
const HERO = 'https://image.tmdb.org/t/p/w1280/hero.jpg'
const BACKDROP = 'https://i.postimg.cc/backdrop.jpg'

afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

describe('withheldLookup', () => {
  it('finds an address by its folder’s key and field, the background image under the empty key', () => {
    const leftOut = withheldLookup([
      { key: '', field: 'backdrop_image_url', url: BACKDROP },
      { key: 'f1', field: 'focus_gif_url', url: GIF },
    ])
    expect(leftOut('', 'backdrop_image_url')).toBe(BACKDROP)
    expect(leftOut('f1', 'focus_gif_url')).toBe(GIF)
    expect(leftOut('f2', 'focus_gif_url')).toBe('')
  })
})

describe('folderPicture', () => {
  it('reads a focus GIF turned off as Off, even when it was left out', () => {
    expect(folderPicture(folder({ focus_gif_enabled: false }), 'focus_gif_url', GIF)).toEqual({ state: 'off' })
  })

  it('reads a left-out picture as Left out, else a set one as itself, else None', () => {
    expect(folderPicture(folder(), 'hero_backdrop_url', HERO)).toEqual({ state: 'out', url: HERO })
    expect(folderPicture(folder({ hero_backdrop_url: HERO }), 'hero_backdrop_url', '')).toEqual({ state: 'set', url: HERO })
    expect(folderPicture(folder(), 'title_logo_url', '')).toEqual({ state: 'none' })
  })
})

describe('PublicationLook', () => {
  // Marvel's cover and focus GIF and the collection's background image are
  // left out; its hero backdrop is shown; it has no title logo.
  const marvel = folder({ id: 'f1', title: 'Marvel Studios', tile_shape: 'LANDSCAPE', cover_emoji: '🦸', hero_backdrop_url: HERO })
  const ghibli = folder({ id: 'f2', title: 'Studio Ghibli', cover_emoji: '🍃', focus_gif_enabled: false })
  const withheld: WithheldAddress[] = [
    { key: '', field: 'backdrop_image_url', url: BACKDROP },
    { key: 'f1', field: 'cover_image_url', url: COVER },
    { key: 'f1', field: 'focus_gif_url', url: GIF },
  ]

  function renderLook() {
    return render(<PublicationLook collection={collection({ folders: [marvel, ghibli] })} withheld={withheld} marks={null} />)
  }

  it('draws a set picture, None for one unset, Off for a GIF turned off, and a box for each left out, loading none of those', () => {
    const { container } = renderLook()
    const images = [...container.querySelectorAll('img')].map((img) => img.getAttribute('src'))
    expect(images).toEqual([HERO])

    const marvelCard = document.getElementById('update-folder-f1')!
    expect(within(marvelCard).getAllByText('Left out')).toHaveLength(2)
    expect(within(marvelCard).getByText('Cover')).toBeInTheDocument()
    expect(within(marvelCard).getByText('Title', { selector: 'dt' }).nextSibling).toHaveTextContent('Shown')
    expect(within(marvelCard).getByText('Hero backdrop')).toBeInTheDocument()
    expect(within(marvelCard).getByText('Title logo', { selector: 'dt' }).nextSibling).toHaveTextContent('None')
    expect(within(marvelCard).getByTitle(GIF)).toHaveTextContent('i.postimg.cc/…-focus.gif')

    const ghibliCard = document.getElementById('update-folder-f2')!
    expect(within(ghibliCard).getByText('Tile')).toBeInTheDocument()
    expect(within(ghibliCard).getByText('Focus GIF', { selector: 'dt' }).nextSibling).toHaveTextContent('Off')

    expect(screen.getByText('Background image')).toBeInTheDocument()
    expect(screen.getAllByTitle(BACKDROP)).toHaveLength(1)
  })

  it('says Tabbed Grids with All tab where the All tab is on', () => {
    render(<PublicationLook collection={collection({ view_mode: 'TABBED_GRID', show_all_tab: true })} withheld={[]} marks={null} />)
    expect(screen.getByText('Folder pages', { selector: 'dt' }).nextSibling).toHaveTextContent('Tabbed Grids with All tab')
  })

  it('draws no background image where none is set', () => {
    render(<PublicationLook collection={collection({ folders: [ghibli] })} withheld={[]} marks={null} />)
    expect(screen.queryByText('Background image')).toBeNull()
  })

  it('copies the whole left-out address', async () => {
    const writeText = vi.fn(() => Promise.resolve())
    vi.stubGlobal('navigator', { clipboard: { writeText } })
    renderLook()
    const card = document.getElementById('update-folder-f1')!
    const [coverCopy] = within(card).getAllByRole('button', { name: 'Copy address' })
    await act(async () => {
      fireEvent.click(coverCopy)
    })
    expect(writeText).toHaveBeenCalledWith(COVER)
  })
})
