import { QueryClient } from '@tanstack/react-query'
import { ApiError } from '@/api'
import { getAuthState, subscribeAuth } from '@/auth'

export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      // A 4xx is an answer, not a hiccup — retrying a 400 or a 404 just
      // delays the error the user needs to see. 401s never reach here:
      // apiFetch refreshes and retries them one layer down.
      retry: (failureCount, error) => {
        if (error instanceof ApiError && error.status < 500) return false
        return failureCount < 2
      },
      // Every list endpoint is unpaginated and small, and nothing else
      // mutates them behind our back — a window refocus doesn't need to
      // refetch every query on the page.
      refetchOnWindowFocus: false,
      staleTime: 30_000,
    },
  },
})

// No query key carries the account, so everything cached belongs to whoever
// was signed in. Dropping the cache whenever the signed-in user changes —
// sign-out, or another tab's session for a different user — keeps one
// account's profiles and rows from rendering for the next.
let cachedUserId = getAuthState().user?.id ?? null
subscribeAuth(() => {
  const userId = getAuthState().user?.id ?? null
  if (userId === cachedUserId) return
  cachedUserId = userId
  queryClient.clear()
})
