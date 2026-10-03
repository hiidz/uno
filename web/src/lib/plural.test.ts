import { describe, expect, it } from 'vitest'
import { plural, pluralCount } from './plural'

describe('plural', () => {
  it('adds s, and turns a consonant’s trailing y into ies', () => {
    expect(plural(2, 'folder')).toBe('folders')
    expect(plural(2, 'production company')).toBe('production companies')
    expect(plural(2, 'day')).toBe('days')
    expect(plural(1, 'production company')).toBe('production company')
  })

  it('joins the count', () => {
    expect(pluralCount(0, 'year')).toBe('0 years')
    expect(pluralCount(1, 'catalog')).toBe('1 catalog')
  })
})
