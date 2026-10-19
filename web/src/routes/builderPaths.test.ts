import { describe, expect, it } from 'vitest'
import { builderPath, builderView, leaveStep } from './builderPaths'

describe('builderView', () => {
  it('reads the profile and the section from the path', () => {
    expect(builderView(builderPath.workspace(2))).toEqual({ profileIndex: 2, section: 'workspace' })
    expect(builderView(builderPath.community(6))).toEqual({ profileIndex: 6, section: 'community' })
  })

  it('is null outside the builder and for a slot outside 1–6', () => {
    for (const path of ['/profiles', '/p/0', '/p/7', '/p/1.5', '/p/x', '/p/1/elsewhere']) {
      expect(builderView(path)).toBeNull()
    }
  })
})

describe('leaveStep', () => {
  it('discards nothing on the same path, as a page’s own history entry moves between', () => {
    expect(leaveStep('/p/1', '/p/1')).toBe('none')
  })

  it('discards the open editor on a move to the other section', () => {
    expect(leaveStep('/p/1', '/p/1/community')).toBe('editor')
    expect(leaveStep('/p/1/community', '/p/1')).toBe('editor')
  })

  it('leaves the builder for a path outside it or another profile’s', () => {
    expect(leaveStep('/p/1', '/profiles')).toBe('builder')
    expect(leaveStep('/p/1/community', '/p/2')).toBe('builder')
  })

  it('asks nothing of a move that starts outside the builder', () => {
    expect(leaveStep('/profiles', '/p/1')).toBe('none')
  })
})
