import { describe, expect, it } from 'vitest'
import { toastLift } from './toastLift'

describe('toastLift', () => {
  it('lifts the toasts by the tallest bar, and not at all with none', () => {
    expect(toastLift([])).toBe(0)
    expect(toastLift([56, 0, 88])).toBe(88)
  })
})
