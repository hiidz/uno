import { QueryClient } from '@tanstack/react-query'
import { ApiError } from '@/api'

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
      // trigger a refetch storm across four queries.
      refetchOnWindowFocus: false,
      staleTime: 30_000,
    },
  },
})
