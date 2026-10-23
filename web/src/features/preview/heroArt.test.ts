import { describe, expect, it } from 'vitest'
import { heroArt, shapeClass, siteOf } from './heroArt'

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
