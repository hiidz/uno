import { QueryClient } from '@tanstack/react-query'
import { createMemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { queryKeys } from '@/api/keys'
import { watchActiveProfile } from './activeProfile'

let queryClient: QueryClient
let unwatch: () => void

function setup(initial: string, state?: unknown) {
  const router = createMemoryRouter(
    [
      { path: '/profiles', element: null },
      { path: '/configure', element: null },
    ],
    { initialEntries: [{ pathname: initial, state }] },
  )
  unwatch = watchActiveProfile(router, queryClient)
  return router
}

function seed() {
  queryClient.setQueryData(queryKeys.ownedCatalogs(1), ['stale'])
  queryClient.setQueryData(queryKeys.genres('movie'), ['kept'])
}

beforeEach(() => {
  queryClient = new QueryClient()
})
afterEach(() => unwatch())

describe('watchActiveProfile', () => {
  it('drops every profile’s queries when the builder opens, and keeps the account-wide ones', async () => {
    const router = setup('/profiles')
    seed()
    await router.navigate('/configure', { state: { profileIndex: 1 } })
    expect(queryClient.getQueryData(queryKeys.ownedCatalogs(1))).toBeUndefined()
    expect(queryClient.getQueryData(queryKeys.genres('movie'))).toEqual(['kept'])
  })

  it('drops them when the builder reopens on the same profile', async () => {
    const router = setup('/configure', { profileIndex: 1 })
    await router.navigate('/profiles')
    seed()
    await router.navigate('/configure', { state: { profileIndex: 1 } })
    expect(queryClient.getQueryData(queryKeys.ownedCatalogs(1))).toBeUndefined()
  })

  it('drops them when one profile’s entry gives way to another’s, as history back does', async () => {
    const router = setup('/configure', { profileIndex: 1 })
    seed()
    await router.navigate('/configure', { state: { profileIndex: 2 } })
    expect(queryClient.getQueryData(queryKeys.ownedCatalogs(1))).toBeUndefined()
  })

  it('leaves the cache alone while the active profile stays the same or the builder closes', async () => {
    const router = setup('/configure', { profileIndex: 1 })
    seed()
    await router.navigate('/configure', { state: { profileIndex: 1 } })
    await router.navigate('/profiles')
    expect(queryClient.getQueryData(queryKeys.ownedCatalogs(1))).toEqual(['stale'])
  })
})
