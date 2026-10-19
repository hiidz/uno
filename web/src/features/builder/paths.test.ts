import { describe, expect, it } from 'vitest'
import { IN_APP, builderPath, builderView, leaveStep, pushedInApp } from './paths'

describe('builderView', () => {
  it('reads the profile, the section and what is open from the path', () => {
    expect(builderView(builderPath.workspace(2))).toEqual({
      profileIndex: 2,
      section: 'workspace',
      editor: null,
      publicationID: null,
    })
    expect(builderView(builderPath.editor(1, { kind: 'catalog', id: 'c1' }))).toMatchObject({
      section: 'workspace',
      editor: { kind: 'catalog', id: 'c1' },
    })
    expect(builderView(builderPath.editor(1, { kind: 'collection', id: 'k1' }))?.editor).toEqual({
      kind: 'collection',
      id: 'k1',
    })
    expect(builderView(builderPath.community(6))).toMatchObject({ profileIndex: 6, section: 'community', publicationID: null })
    expect(builderView(builderPath.publication(1, 'pub1'))).toMatchObject({ section: 'community', publicationID: 'pub1' })
  })

  it('is null outside the builder, for a slot outside 1–6 and for a path it has no screen at', () => {
    for (const path of [
      '/profiles',
      '/p/0',
      '/p/7',
      '/p/1.5',
      '/p/x',
      '/p/1/elsewhere',
      '/p/1/catalog',
      '/p/1/catalog/c1/more',
      '/p/1/community/pub1/more',
    ]) {
      expect(builderView(path)).toBeNull()
    }
  })
})

describe('leaveStep', () => {
  it('discards nothing on the same path, as a page’s own history entry moves between', () => {
    expect(leaveStep('/p/1', '/p/1')).toBe('none')
  })

  it('discards nothing within Community, or opening an editor from Home', () => {
    expect(leaveStep('/p/1/community', '/p/1/community/pub1')).toBe('none')
    expect(leaveStep('/p/1', '/p/1/catalog/c1')).toBe('none')
  })

  it('discards the open editor on a move to another editor, Home or the other section', () => {
    expect(leaveStep('/p/1/catalog/c1', '/p/1/collection/k1')).toBe('editor')
    expect(leaveStep('/p/1/catalog/c1', '/p/1')).toBe('editor')
    expect(leaveStep('/p/1/catalog/c1', '/p/1/community/pub1')).toBe('editor')
  })

  it('leaves the builder for a path outside it or another profile’s', () => {
    expect(leaveStep('/p/1', '/profiles')).toBe('builder')
    expect(leaveStep('/p/1/community', '/p/2')).toBe('builder')
  })

  it('asks nothing of a move that starts outside the builder', () => {
    expect(leaveStep('/profiles', '/p/1')).toBe('none')
  })
})

describe('pushedInApp', () => {
  it('reads the mark on a pushed entry, and nothing else', () => {
    expect(pushedInApp(IN_APP)).toBe(true)
    expect(pushedInApp({ kind: 'catalog' })).toBe(false)
    expect(pushedInApp(null)).toBe(false)
  })
})
