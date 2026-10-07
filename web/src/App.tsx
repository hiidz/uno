import { QueryClientProvider } from '@tanstack/react-query'
import { RouterProvider } from 'react-router-dom'
import { AuthProvider } from '@/auth'
import { queryClient } from '@/lib/query-client'
import { watchActiveProfile } from '@/routes/activeProfile'
import { router } from '@/routes/router'

watchActiveProfile(router, queryClient)

export default function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <AuthProvider>
        <RouterProvider router={router} />
      </AuthProvider>
    </QueryClientProvider>
  )
}
