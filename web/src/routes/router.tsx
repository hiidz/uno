import { Navigate, createBrowserRouter } from 'react-router-dom'
import { RequireAuth } from '@/auth'
import { ErrorPage } from './ErrorPage'
import { Login } from './Login'
import { ProfilePicker } from './ProfilePicker'

export const router = createBrowserRouter([
  {
    // Pathless, so every page below shares one error page for whatever throws
    // while it renders or loads.
    errorElement: <ErrorPage kind="crashed" />,
    children: [
      { path: '/login', element: <Login /> },
      {
        path: '/',
        element: (
          <RequireAuth>
            <Navigate to="/profiles" replace />
          </RequireAuth>
        ),
      },
      {
        path: '/profiles',
        element: (
          <RequireAuth>
            <ProfilePicker />
          </RequireAuth>
        ),
      },
      {
        // Each profile's builder, its sections as children (`builderPaths.ts`)
        // that `Builder` reads from the path itself, so moving between them
        // keeps it mounted. The builder carries dnd-kit, Radix and every
        // feature, so it is its own chunk, fetched on the first visit. A page
        // load that lands here renders nothing until it arrives.
        path: '/p/:profile',
        HydrateFallback: () => null,
        lazy: async () => {
          const { Builder } = await import('./Builder')
          return {
            element: (
              <RequireAuth>
                <Builder />
              </RequireAuth>
            ),
          }
        },
        children: [{ index: true }, { path: 'community' }],
      },
      { path: '*', element: <ErrorPage kind="missing" /> },
    ],
  },
])
