import { QueryClient } from '@tanstack/react-query'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { queryKeys } from '@/api/keys'
import { activeProfile, pickProfile, watchActiveProfile } from './activeProfile'

let queryClient: QueryClient
let unwatch: () => void

function seed() {
  queryClient.setQueryData(queryKeys.library(1), ['stale'])
  queryClient.setQueryData(queryKeys.genres('movie'), ['kept'])
}

/** Storage as this browser holds it, which `activeProfile` reads once per account. */
const stored = new Map<string, string>()

beforeEach(() => {
  queryClient = new QueryClient()
  unwatch = watchActiveProfile(queryClient)
  stored.clear()
  vi.stubGlobal('localStorage', {
    getItem: (key: string) => stored.get(key) ?? null,
    setItem: (key: string, value: string) => stored.set(key, value),
  })
})
afterEach(() => {
  unwatch()
  vi.unstubAllGlobals()
})

describe('activeProfile', () => {
  it('opens on the profile picked last, after a reload too, for that account only', () => {
    pickProfile('ana', 2)
    expect(activeProfile('ana')).toBe(2)
    expect(stored.get('uno:profile:ana')).toBe('2')
    expect(activeProfile('bo')).toBeNull()
  })

  it('keeps this tab’s profile when another tab picks one', () => {
    stored.set('uno:profile:cy', '1')
    expect(activeProfile('cy')).toBe(1)
    stored.set('uno:profile:cy', '4')
    expect(activeProfile('cy')).toBe(1)
  })

  it('reads a stored slot outside 1–6 as none', () => {
    stored.set('uno:profile:di', '9')
    expect(activeProfile('di')).toBeNull()
  })

  it('asks each time when storage is blocked', () => {
    vi.stubGlobal('localStorage', {
      getItem: () => {
        throw new Error('blocked')
      },
      setItem: () => {
        throw new Error('blocked')
      },
    })
    expect(activeProfile('ed')).toBeNull()
    pickProfile('ed', 3)
    expect(activeProfile('ed')).toBe(3)
  })
})

describe('watchActiveProfile', () => {
  it('drops every profile’s queries on each pick, the same profile’s included, and keeps the account-wide ones', () => {
    pickProfile('fay', 1)
    seed()
    pickProfile('fay', 1)
    expect(queryClient.getQueryData(queryKeys.library(1))).toBeUndefined()
    expect(queryClient.getQueryData(queryKeys.genres('movie'))).toEqual(['kept'])
  })

  it('leaves the cache alone between picks', () => {
    pickProfile('gus', 1)
    seed()
    expect(activeProfile('gus')).toBe(1)
    expect(queryClient.getQueryData(queryKeys.library(1))).toEqual(['stale'])
  })
})
