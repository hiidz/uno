import { describe, expect, it } from 'vitest'
import { thumbnail } from './tmdbImage'

describe('thumbnail', () => {
  it('offers a TMDB w500 poster at the smaller TMDB sizes', () => {
    expect(thumbnail('https://image.tmdb.org/t/p/w500/abc.jpg', '3rem')).toEqual({
      src: 'https://image.tmdb.org/t/p/w185/abc.jpg',
      srcSet:
        'https://image.tmdb.org/t/p/w92/abc.jpg 92w, https://image.tmdb.org/t/p/w154/abc.jpg 154w, https://image.tmdb.org/t/p/w185/abc.jpg 185w, https://image.tmdb.org/t/p/w342/abc.jpg 342w',
      sizes: '3rem',
    })
  })

  it('leaves any other image as it is', () => {
    expect(thumbnail('https://image.tmdb.org/t/p/original/abc.jpg', '3rem')).toEqual({
      src: 'https://image.tmdb.org/t/p/original/abc.jpg',
    })
    expect(thumbnail('https://images.weserv.nl/?url=x', '3rem')).toEqual({ src: 'https://images.weserv.nl/?url=x' })
  })
})
