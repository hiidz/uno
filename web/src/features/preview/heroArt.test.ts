import { describe, expect, it } from 'vitest'
import { cutAddress, heroArt, shapeClass, siteOf } from './heroArt'

const folder = {
  title: 'Marvel',
  hideTitle: false,
  coverEmoji: '',
  coverImageURL: '',
  heroBackdropURL: '',
  titleLogoURL: '',
  backdropImageURL: '',
}

describe('heroArt', () => {
  it('takes the hero backdrop, then the cover, then the collection background', () => {
    const all = { heroBackdropURL: 'https://a/hero', coverImageURL: 'https://a/cover', backdropImageURL: 'https://a/bg' }
    expect(heroArt({ ...folder, ...all }).backdrop).toBe('https://a/hero')
    expect(heroArt({ ...folder, ...all, heroBackdropURL: '  ' }).backdrop).toBe('https://a/cover')
    expect(heroArt({ ...folder, ...all, heroBackdropURL: '', coverImageURL: '' }).backdrop).toBe('https://a/bg')
    expect(heroArt(folder).backdrop).toBe('')
  })

  it('writes the title after the cover emoji and two spaces, as Nuvio TV does', () => {
    expect(heroArt(folder).text).toBe('Marvel')
    expect(heroArt({ ...folder, coverEmoji: '🎬' }).text).toBe('🎬  Marvel')
  })

  it('has no text when the folder hides its title', () => {
    expect(heroArt({ ...folder, coverEmoji: '🎬', hideTitle: true }).text).toBe('')
  })

  it('carries the logo trimmed', () => {
    expect(heroArt({ ...folder, titleLogoURL: ' https://a/logo.png ' }).logo).toBe('https://a/logo.png')
  })
})

describe('siteOf', () => {
  it('names the lower-cased host, or nothing for an address without one', () => {
    expect(siteOf('https://Media.Giphy.com/x.gif')).toBe('media.giphy.com')
    expect(siteOf('not an address')).toBe('')
  })
})

describe('shapeClass', () => {
  it('maps each tile shape', () => {
    expect([shapeClass('POSTER'), shapeClass('LANDSCAPE'), shapeClass('SQUARE')]).toEqual(['poster', 'landscape', 'square'])
  })
})

describe('cutAddress', () => {
  it('leaves an address that fits whole, without its scheme', () => {
    expect(cutAddress('https://i.postimg.cc/a.gif', 26)).toBe('i.postimg.cc/a.gif')
  })

  it('keeps the site and the end of a long one', () => {
    expect(cutAddress('https://i.postimg.cc/Xq4mP2Lw/marvel-studios-focus.gif', 26)).toBe('i.postimg.cc/…os-focus.gif')
  })

  it('keeps at least 8 characters of the end after a long site', () => {
    expect(cutAddress('https://images.weserv.nl/?url=image.tmdb.org/t/p/w500/a.png&w=640', 20)).toBe('images.weserv.nl/…ng&w=640')
  })
})
