import { QueryClientProvider } from '@tanstack/react-query'
import { RouterProvider } from 'react-router-dom'
import { AuthProvider } from '@/auth'
import { ToastProvider } from '@/components/Toasts'
import { queryClient } from '@/lib/query-client'
import { watchActiveProfile } from '@/routes/activeProfile'
import { StartupShell } from '@/routes/LoadingShells'
import { router } from '@/routes/router'

watchActiveProfile(queryClient)

export default function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <ToastProvider>
        <AuthProvider fallback={<StartupShell />}>
          <RouterProvider router={router} />
        </AuthProvider>
      </ToastProvider>
    </QueryClientProvider>
  )
}
