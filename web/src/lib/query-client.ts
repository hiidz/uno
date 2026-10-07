import { MutationCache, QueryCache, QueryClient } from '@tanstack/react-query'
import { ApiError } from '@/api'
import { getAuthState, subscribeAuth } from '@/auth'
import { clearKeyProblem, noteKeyProblem } from '@/features/account/keyProblem'

export const queryClient = new QueryClient({
  // Any call can be the one that finds the account's TMDB key unusable, so
  // every failure is offered to the store the builder's banner reads.
  queryCache: new QueryCache({ onError: noteKeyProblem }),
  mutationCache: new MutationCache({ onError: noteKeyProblem }),
  defaultOptions: {
    queries: {
      // A 4xx is an answer, not a hiccup — retrying a 400 or a 404 just
      // delays the error the user needs to see. 401s never reach here:
      // apiFetch refreshes and retries them one layer down.
      retry: (failureCount, error) => {
        if (error instanceof ApiError && error.status < 500) return false
        return failureCount < 2
      },
      // Another tab or device can change what a list holds, so a refocus
      // refetches the stale queries; the ones kept with `staleTime: Infinity`
      // never go stale and stay put.
      refetchOnWindowFocus: true,
      staleTime: 30_000,
    },
  },
})

// No query key carries the account, so everything cached belongs to whoever
// was signed in. Dropping the cache whenever the signed-in user changes —
// sign-out, or another tab's session for a different user — keeps one
// account's profiles and rows from rendering for the next, and one account's
// key problem from showing for the next.
let cachedUserId = getAuthState().user?.id ?? null
subscribeAuth(() => {
  const userId = getAuthState().user?.id ?? null
  if (userId === cachedUserId) return
  cachedUserId = userId
  queryClient.clear()
  clearKeyProblem()
})
