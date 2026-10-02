import { describe, expect, it } from 'vitest'
import { homeMounted, selectionAction } from './selection'
import type { EditorTarget } from './target'

const target = (kind: 'catalog' | 'collection', id: string) => ({ kind, id }) as EditorTarget
const a = target('catalog', 'a')

describe('selectionAction', () => {
  it('opens another row', () => {
    expect(selectionAction(null, a)).toBe('open')
    expect(selectionAction(a, target('catalog', 'b'))).toBe('open')
    expect(selectionAction(a, target('collection', 'a'))).toBe('open')
  })

  it('closes the open row', () => {
    expect(selectionAction(a, target('catalog', 'a'))).toBe('close')
  })
})

describe('homeMounted', () => {
  it('keeps Home mounted under an open editor only below lg', () => {
    expect(homeMounted(null, false)).toBe(true)
    expect(homeMounted(null, true)).toBe(true)
    expect(homeMounted(a, true)).toBe(true)
    expect(homeMounted(a, false)).toBe(false)
  })
})
