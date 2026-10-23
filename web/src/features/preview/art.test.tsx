// @vitest-environment jsdom
import { fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { FocusGIFTile, HeroPreview, LeftOutPicture, PictureTile } from './art'

function prefersLessMotion(reduce: boolean) {
  window.matchMedia = ((query: string) => ({
    matches: reduce && query.includes('reduce'),
    media: query,
  })) as unknown as typeof window.matchMedia
}

afterEach(() => {
  vi.restoreAllMocks()
  // jsdom has no matchMedia of its own.
  delete (window as { matchMedia?: unknown }).matchMedia
})

describe('PictureTile', () => {
  it('says the picture failed in its place, and forgets that for a new address', () => {
    const { container, rerender } = render(<PictureTile src="https://a/broken.png" shape="POSTER" />)
    fireEvent.error(container.querySelector('img')!)
    expect(screen.getByText('Couldn’t load this picture')).toBeTruthy()

    rerender(<PictureTile src="https://a/fixed.png" shape="POSTER" />)
    expect(screen.queryByText('Couldn’t load this picture')).toBeNull()
    expect(container.querySelector('img')?.getAttribute('src')).toBe('https://a/fixed.png')
  })
})

describe('FocusGIFTile', () => {
  it('draws the GIF where motion is fine', () => {
    prefersLessMotion(false)
    const { container } = render(<FocusGIFTile src="https://media.giphy.com/x.gif" shape="LANDSCAPE" />)
    expect(container.querySelector('img')?.getAttribute('src')).toBe('https://media.giphy.com/x.gif')
  })

  it('leaves the GIF unloaded where the device asks for less motion, naming its site', () => {
    prefersLessMotion(true)
    const { container } = render(<FocusGIFTile src="https://media.giphy.com/x.gif" shape="LANDSCAPE" />)
    expect(container.querySelector('img')).toBeNull()
    expect(screen.getByText('Set')).toBeTruthy()
    expect(screen.getByText('media.giphy.com')).toBeTruthy()
  })
})

describe('HeroPreview', () => {
  it('shows the title text when the logo fails', () => {
    const { container } = render(
      <HeroPreview art={{ backdrop: 'https://a/bg.jpg', logo: 'https://a/logo.png', text: 'Marvel' }} />,
    )
    expect(screen.queryByText('Marvel')).toBeNull()
    const logo = [...container.querySelectorAll('img')].find((img) => img.getAttribute('src') === 'https://a/logo.png')!
    fireEvent.error(logo)
    expect(screen.getByText('Marvel')).toBeTruthy()
  })

  it('shows nothing in the logo place when there is no logo and the title is hidden', () => {
    const { container } = render(<HeroPreview art={{ backdrop: 'https://a/bg.jpg', logo: '', text: '' }} />)
    expect(container.querySelector('.art-hero-title')?.textContent).toBe('')
  })
})

describe('LeftOutPicture', () => {
  const url = 'https://i.postimg.cc/Xq4mP2Lw/marvel-studios-focus.gif'

  it('cuts the address to the box, opens it whole on a tap and cuts it again on another, loading nothing', () => {
    const { container } = render(<LeftOutPicture url={url} shape="LANDSCAPE" />)
    const address = screen.getByTitle(url)
    expect(address).toHaveTextContent('i.postimg.cc/…-focus.gif')
    expect(address).toHaveAttribute('aria-expanded', 'false')

    fireEvent.click(address)
    expect(address).toHaveTextContent(url)
    expect(address).toHaveAttribute('aria-expanded', 'true')

    fireEvent.click(address)
    expect(address).toHaveTextContent('i.postimg.cc/…-focus.gif')
    expect(container.querySelector('img')).toBeNull()
  })

  it('names the site alone in a narrow box until opened', () => {
    render(<LeftOutPicture url={url} shape="POSTER" />)
    const address = screen.getByTitle(url)
    expect(address).toHaveTextContent(/^i\.postimg\.cc$/)
    fireEvent.click(address)
    expect(address).toHaveTextContent(url)
  })
})
