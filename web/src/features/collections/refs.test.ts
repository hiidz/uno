import { describe, expect, it, vi } from 'vitest'
import { catalog } from '@/test/fixtures'
import { buildRefOptions, filterRefOptions } from './refs'

// `tmdbKind` is the one value this needs from the API barrel, which would
// otherwise load the auth session and its `window` listener.
vi.mock('@/api', async () => ({ tmdbKind: (await import('@/api/types')).tmdbKind }))

const genres = {
  movie: new Map([[27, 'Horror']]),
  tv: new Map([[18, 'Drama']]),
}

const options = buildRefOptions(
  [
    catalog({ id: 'c1', name: 'Late Night', params: '{"with_genres":"27"}' }),
    catalog({ id: 'c2', name: 'Prestige', type: 'series', params: '{"with_genres":"18"}' }),
    catalog({ id: 'c3', name: 'Everything', params: '{}' }),
  ],
  genres,
)

describe('buildRefOptions', () => {
  it('describes each recipe with the genre names of its own type', () => {
    expect(options.map((option) => option.recipe)).toEqual(['Horror', 'Drama', 'No filters'])
  })
})

describe('filterRefOptions', () => {
  it('finds a catalog by a genre in its recipe, not only by its name', () => {
    expect(filterRefOptions(options, '  HORROR ', new Set()).map((o) => o.id)).toEqual(['c1'])
  })

  it('offers everything for an empty query', () => {
    expect(filterRefOptions(options, '', new Set())).toHaveLength(3)
  })

  it('leaves out the catalogs the folder already holds unfiltered', () => {
    expect(filterRefOptions(options, '', new Set(['c1', 'c3'])).map((o) => o.id)).toEqual(['c2'])
  })
})
