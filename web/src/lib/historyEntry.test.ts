// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { consumeEntry, hasFlag, leftEntry, pushEntry } from './historyEntry'

afterEach(() => {
  vi.restoreAllMocks()
  window.history.replaceState(null, '')
})

describe('leftEntry', () => {
  it('is false when nothing was pushed', () => {
    expect(leftEntry(false, null, 'unoFolder')).toBe(false)
  })

  it('is true once the current entry no longer carries the flag', () => {
    expect(leftEntry(true, null, 'unoFolder')).toBe(true)
    expect(leftEntry(true, { usr: {} }, 'unoFolder')).toBe(true)
  })

  it('keeps a folder page open when Back steps off an entry pushed over it', () => {
    // Back off the editor layer's entry lands on the folder page's own.
    expect(leftEntry(true, { unoFolder: true }, 'unoFolder')).toBe(false)
  })
})

describe('pushEntry', () => {
  it('keeps the router state, adds the flag and drops the cleared ones', () => {
    window.history.replaceState({ usr: { profileIndex: 1 }, idx: 1, unoFolder: true }, '')
    expect(pushEntry('unoEditor', ['unoFolder'])).toBe(true)
    expect(window.history.state).toEqual({ usr: { profileIndex: 1 }, idx: 1, unoEditor: true })
  })

  it('reports a push that throws', () => {
    vi.spyOn(window.history, 'pushState').mockImplementation(() => {
      throw new Error('sandboxed')
    })
    expect(pushEntry('unoFolder')).toBe(false)
  })
})

describe('consumeEntry', () => {
  it('steps back only while the flagged entry is current', () => {
    const back = vi.spyOn(window.history, 'back').mockImplementation(() => {})
    window.history.replaceState({ idx: 1 }, '')
    consumeEntry('unoEditor')
    expect(back).not.toHaveBeenCalled()

    window.history.replaceState({ idx: 1, unoEditor: true }, '')
    consumeEntry('unoEditor')
    expect(back).toHaveBeenCalledTimes(1)
  })

  it('reads only a true flag', () => {
    expect(hasFlag({ unoEditor: 'yes' }, 'unoEditor')).toBe(false)
    expect(hasFlag('state', 'unoEditor')).toBe(false)
  })
})
