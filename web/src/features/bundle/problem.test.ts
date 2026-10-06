import { describe, expect, it, vi } from 'vitest'
import { ApiError } from '@/api/http'
import { checkProblem, writeProblem } from './problem'

vi.mock('@/api/client', () => ({ apiFetch: vi.fn() }))

function failed(status: number, message: string): Error {
  return new ApiError(status, message)
}

describe('checkProblem', () => {
  it('words a refused bundle and a TMDB outage by status, keeping the server’s words as the detail', () => {
    expect(checkProblem(failed(400, 'unknown field "pin_to_top"'))).toEqual({
      headline: "This JSON isn't a bundle Uno can import.",
      detail: 'unknown field "pin_to_top"',
    })
    expect(checkProblem(failed(502, 'tmdb unreachable')).headline).toBe(
      "TMDB couldn't be reached to check the filters. Try again in a moment.",
    )
  })

  it('falls back to a plain failure for any other status or error', () => {
    expect(checkProblem(failed(500, 'boom')).headline).toBe("Couldn't check this JSON. Try again.")
    expect(checkProblem(new Error('offline'))).toEqual({
      headline: "Couldn't check this JSON. Try again.",
      detail: 'offline',
    })
  })
})

describe('writeProblem', () => {
  it('says nothing was added, with the server’s words as the detail', () => {
    expect(writeProblem(failed(500, 'disk full'))).toEqual({
      headline: "Couldn't import. Nothing was added.",
      detail: 'disk full',
    })
  })
})
