import { describe, expect, it } from 'vitest'
import { IN_APP, builderPath, builderView, leaveStep, pushedInApp } from './paths'

describe('builderView', () => {
  it('reads the section and what is open from the path', () => {
    expect(builderView(builderPath.workspace())).toEqual({ section: 'workspace', editor: null, publication: null })
    expect(builderView(builderPath.editor({ kind: 'catalog', id: 'c1' }))).toEqual({
      section: 'workspace',
      editor: { kind: 'catalog', id: 'c1' },
      publication: null,
    })
    expect(builderView(builderPath.editor({ kind: 'collection', id: 'k 1' }))?.editor).toEqual({
      kind: 'collection',
      id: 'k 1',
    })
    expect(builderView(builderPath.community())).toEqual({ section: 'community', editor: null, publication: null })
    expect(builderView(builderPath.publication({ kind: 'collection', id: 'pub1' }))).toEqual({
      section: 'community',
      editor: null,
      publication: { kind: 'collection', id: 'pub1' },
    })
    expect(builderView('/configure/')).toMatchObject({ section: 'workspace', editor: null })
  })

  it('is null outside the builder and for a path it has no screen at', () => {
    for (const path of [
      '/profiles',
      '/p/1',
      '/p/1/community',
      '/configure/elsewhere/c1',
      '/configure/catalog',
      '/configure/catalog/c1/more',
      '/community/pub1',
      '/community/catalog/pub1/more',
      '/configure/catalog/%E0',
    ]) {
      expect(builderView(path)).toBeNull()
    }
  })
})

describe('leaveStep', () => {
  it('asks nothing of a move that discards nothing', () => {
    expect(leaveStep('/configure', '/configure')).toBe('none')
    expect(leaveStep('/community', '/community/catalog/pub1')).toBe('none')
    expect(leaveStep('/configure', '/configure/catalog/c1')).toBe('none')
  })

  it('asks about the open editor on a move away from it', () => {
    expect(leaveStep('/configure/catalog/c1', '/configure/collection/k1')).toBe('editor')
    expect(leaveStep('/configure/catalog/c1', '/configure')).toBe('editor')
    expect(leaveStep('/configure/catalog/c1', '/community/catalog/pub1')).toBe('editor')
  })

  it('asks about the builder on a move out of it', () => {
    expect(leaveStep('/configure', '/profiles')).toBe('builder')
    expect(leaveStep('/community', '/login')).toBe('builder')
  })

  it('asks nothing of a move that starts outside the builder', () => {
    expect(leaveStep('/profiles', '/configure')).toBe('none')
  })
})

describe('pushedInApp', () => {
  it('reads the mark on a pushed entry, and nothing else', () => {
    expect(pushedInApp(IN_APP)).toBe(true)
    expect(pushedInApp({ kind: 'catalog' })).toBe(false)
    expect(pushedInApp(null)).toBe(false)
  })
})
