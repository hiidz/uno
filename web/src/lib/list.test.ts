import { expect, it } from 'vitest'
import { andList, orList } from './list'

it('joins items the way an English sentence does, with no comma before the last', () => {
  expect(andList([])).toBe('')
  expect(andList(['Action'])).toBe('Action')
  expect(andList(['Action', 'Comedy'])).toBe('Action and Comedy')
  expect(andList(['Action', 'Comedy', 'Drama'])).toBe('Action, Comedy and Drama')
  expect(orList(['Action', 'Comedy', 'Drama'])).toBe('Action, Comedy or Drama')
})
