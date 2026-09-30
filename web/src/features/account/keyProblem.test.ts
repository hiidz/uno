// @vitest-environment jsdom
import { renderHook } from '@testing-library/react'
import { beforeEach, describe, expect, it } from 'vitest'
import { ApiError, RateLimitedError } from '@/api/http'
import { clearKeyProblem, noteKeyProblem, useKeyProblem } from './keyProblem'

describe('keyProblem', () => {
  beforeEach(() => clearKeyProblem())

  it('is noted only for a 422, and cleared', () => {
    const { result, rerender } = renderHook(() => useKeyProblem())
    for (const error of [new ApiError(400, 'bad'), new ApiError(502, 'down'), new RateLimitedError('1'), new Error('x'), null]) {
      noteKeyProblem(error)
    }
    rerender()
    expect(result.current).toBe(false)

    noteKeyProblem(new ApiError(422, 'TMDB didn’t accept your key.'))
    rerender()
    expect(result.current).toBe(true)

    clearKeyProblem()
    rerender()
    expect(result.current).toBe(false)
  })
})
