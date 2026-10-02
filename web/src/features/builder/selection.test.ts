import { describe, expect, it } from 'vitest'
import { selectionAction } from './selection'
import type { EditorTarget } from './target'

const target = (kind: 'catalog' | 'collection', id: string) => ({ kind, id }) as EditorTarget
const a = target('catalog', 'a')

describe('selectionAction', () => {
  it('opens another row', () => {
    expect(selectionAction(null, a, false)).toBe('open')
    expect(selectionAction(a, target('catalog', 'b'), false)).toBe('open')
    expect(selectionAction(a, target('collection', 'a'), true)).toBe('open')
  })

  it('closes the open row from lg, and scrolls back to it stacked', () => {
    expect(selectionAction(a, target('catalog', 'a'), false)).toBe('close')
    expect(selectionAction(a, target('catalog', 'a'), true)).toBe('scroll')
  })
})
