import { describe, expect, it } from 'vitest'
import { MAX_TOASTS, withToast, type QueuedToast, type ToastTone } from './toastQueue'

function toast(id: number, tone: ToastTone = 'success', open = true): QueuedToast {
  return { id, tone, text: `toast ${id}`, open }
}

const ids = (list: QueuedToast[]) => list.map((t) => t.id)

describe('withToast', () => {
  it('adds the newest last while there is room', () => {
    expect(ids(withToast([toast(1), toast(2)], toast(3)))).toEqual([1, 2, 3])
    expect(MAX_TOASTS).toBe(3)
  })

  it('pushes out the oldest success once three are showing', () => {
    expect(ids(withToast([toast(1), toast(2), toast(3)], toast(4)))).toEqual([2, 3, 4])
  })

  it('keeps an unread failure, pushing out the oldest success after it', () => {
    const list = [toast(1, 'danger'), toast(2), toast(3, 'notice')]
    expect(ids(withToast(list, toast(4)))).toEqual([1, 3, 4])
  })

  it('pushes out the oldest failure when every toast is one', () => {
    const list = [toast(1, 'danger'), toast(2, 'danger'), toast(3, 'danger')]
    expect(ids(withToast(list, toast(4, 'danger')))).toEqual([2, 3, 4])
  })

  it('stacks the same words twice rather than merging them', () => {
    const list = withToast([toast(1)], { ...toast(2), text: 'toast 1' })
    expect(list.map((t) => t.text)).toEqual(['toast 1', 'toast 1'])
  })

  it('doesn’t count a toast already closing', () => {
    const list = [toast(1, 'success', false), toast(2), toast(3)]
    expect(ids(withToast(list, toast(4)))).toEqual([1, 2, 3, 4])
  })
})
